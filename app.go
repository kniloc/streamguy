package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"stream-guy/internal/command"
	"stream-guy/internal/render"
	"stream-guy/internal/tts"
	"strings"
	"sync"

	"gioui.org/font"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/jackc/pgx/v5/pgxpool"

	"stream-guy/internal/assets"
	"stream-guy/internal/config"
	"stream-guy/internal/db"
	"stream-guy/internal/download"
	"stream-guy/internal/overlay"
	"stream-guy/internal/pi"
	"stream-guy/internal/popup"
	"stream-guy/internal/streamerbot"
	"stream-guy/internal/window"
)

type App struct {
	// Core managers
	imageCache        *assets.ImageCache
	emoteManager      *assets.EmoteManager
	badgeManager      *assets.BadgeManager
	windowRegistry    *window.Registry
	textParser        *render.TextParser
	config            *config.Config
	placementManager  *window.PlacementManager
	theme             *material.Theme
	streamerBotClient *streamerbot.Client
	loadedFontFace    font.FontFace

	// Shared services
	downloadPool    *download.Pool
	popupService    *popup.Service
	zorderManager   *window.ZOrderManager
	piClient        *pi.Client
	dbPool          *pgxpool.Pool
	commandRegistry *command.Registry

	// Lifetime
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownOnce sync.Once

	// Control state
	paused         bool
	pausedMu       sync.RWMutex
	clearAllBtn    widget.Clickable
	pauseResumeBtn widget.Clickable
	clearImagesBtn widget.Clickable

	// Drawing Overlay
	overlay *overlay.Window
}

func (a *App) HandleChatMessage(data json.RawMessage, timestamp string) {
	var msgData streamerbot.ChatMessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		log.Printf("Error parsing chat message: %v", err)
		return
	}

	userID := msgData.User.ID
	username := msgData.User.DisplayName
	message := strings.TrimSpace(msgData.Message.Message)
	if strings.HasPrefix(message, "!") {
		if a.commandRegistry != nil {
			a.commandRegistry.Dispatch(message, userID, username)
		}
		return
	}
	emotesTag := msgData.Message.Emotes
	log.Printf("Chat: %s: %s", username, message)

	userColor := assets.ParseHexColor(msgData.User.Color)

	foundKeyword := a.findMatchingKeyword(message)
	if foundKeyword != "" {
		log.Printf("[%s] Creating '%s' GIF popup", streamerbot.FormatTimeStamp(timestamp), foundKeyword)
		if a.popupService != nil {
			if err := a.popupService.CreateGifPopup(foundKeyword, message); err != nil {
				log.Printf("[%s] Failed to create GIF popup: %v", streamerbot.FormatTimeStamp(timestamp), err)
			}
		}
		return
	}

	badges := streamerbot.ConvertBadges(msgData.User.Badges)
	if a.popupService != nil {
		if err := a.popupService.CreateChatPopup(username, userColor, badges, message, emotesTag); err != nil {
			log.Printf("[%s] Failed to create chat popup: %v", streamerbot.FormatTimeStamp(timestamp), err)
		}
	}
}

func (a *App) HandleRewardRedemption(data json.RawMessage) {
	var rData streamerbot.RewardRedemptionData
	if err := json.Unmarshal(data, &rData); err != nil {
		log.Printf("Error parsing redemption data: %v", err)
		return
	}

	username := rData.Username
	redemption := rData.Reward.Title
	log.Printf("Redemption: %s", redemption)

	if strings.EqualFold(redemption, "TTS") {
		userInput := strings.TrimSpace(rData.UserInput)
		hasBlockedPrefix := strings.HasPrefix(userInput, "!") || strings.HasPrefix(userInput, ":")
		if userInput != "" && !hasBlockedPrefix {
			speech := tts.GetSpeech()
			if speech != nil {
				log.Printf("TTS: %s says %s", username, userInput)
				if err := speech.SpeakQueued(userInput); err != nil {
					log.Printf("TTS error: %v", err)
				}
			}
		}
	}

	if strings.Contains(redemption, "Photograph!") {
		userInput := strings.TrimSpace(rData.UserInput)
		if userInput != "" && a.popupService != nil {
			if err := a.popupService.CreatePhotoPopup(userInput, func(url, mimeType string) {
				log.Printf("Photo accepted: url=%s, mimeType=%s", url, mimeType)
				if a.piClient != nil {
					go func() {
						resp, err := a.piClient.SendImage(url, mimeType)
						if err != nil {
							log.Printf("Failed to send image to Pi: %v", err)
						} else {
							log.Printf("Pi response: status=%s, message=%s", resp.Status, resp.Message)
						}
					}()
				}
			}); err != nil {
				log.Printf("Failed to create photo popup: %v", err)
			}
		}
	}

	if strings.Contains(redemption, "TreeShaker") {
		re := regexp.MustCompile(`\d+`)
		turnsStr := re.FindString(redemption)
		if turnsStr != "" && a.dbPool != nil {
			turns := 0
			fmt.Sscanf(turnsStr, "%d", &turns)
			formattedUsername := strings.ToLower(username)
			if err := db.UpdateNumberOfTurns(a.ctx, a.dbPool, turns, formattedUsername); err != nil {
				log.Printf("Failed to update turns for %s: %v", formattedUsername, err)
			}
		}
	}
}

func (a *App) findMatchingKeyword(message string) string {
	if a == nil || a.config == nil {
		return ""
	}

	messageLower := strings.ToLower(message)
	for keyword := range a.config.Keywords {
		if messageLower == strings.ToLower(keyword) {
			return keyword
		}
	}
	return ""
}

func (a *App) getTheme() *material.Theme {
	return a.theme
}

func (a *App) isPaused() bool {
	a.pausedMu.RLock()
	defer a.pausedMu.RUnlock()
	return a.paused
}

func (a *App) setPaused(p bool) {
	a.pausedMu.Lock()
	defer a.pausedMu.Unlock()
	a.paused = p
}

func (a *App) closeAllWindows() {
	if a.windowRegistry != nil {
		a.windowRegistry.CloseAll()
	}
	if a.emoteManager != nil {
		a.emoteManager.StopAllAnimations()
	}
}

func (a *App) Shutdown() {
	a.shutdownOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}

		if a.overlay != nil {
			a.overlay.Close()
		}

		a.closeAllWindows()
		if a.zorderManager != nil {
			a.zorderManager.Stop()
		}
		if a.streamerBotClient != nil {
			a.streamerBotClient.Close()
		}

		if a.downloadPool != nil {
			a.downloadPool.Close()
		}

		if a.dbPool != nil {
			a.dbPool.Close()
		}
	})
}
