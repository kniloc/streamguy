package main

import (
	"context"
	"encoding/json"
	"log"
	"stream-guy/internal/render"
	"strings"
	"sync"

	"gioui.org/font"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"stream-guy/internal/assets"
	"stream-guy/internal/config"
	"stream-guy/internal/download"
	"stream-guy/internal/overlay"
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
	downloadPool *download.Pool
	popupService *popup.Service

	// Lifetime
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownOnce sync.Once

	// Control state
	paused         bool
	pausedMu       sync.RWMutex
	clearAllBtn    widget.Clickable
	pauseResumeBtn widget.Clickable

	// Drawing Overlay
	overlay        *overlay.Window
	overlayDrawBtn widget.Clickable
}

func (a *App) HandleChatMessage(data json.RawMessage, timestamp string) {
	var msgData streamerbot.ChatMessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		log.Printf("Error parsing chat message: %v", err)
		return
	}

	username := msgData.User.DisplayName
	message := strings.TrimSpace(msgData.Message.Message)
	if strings.HasPrefix(message, "!") {
		return
	}
	emotesTag := msgData.Message.Emotes
	log.Printf("Chat: %s: %s", username, message)

	userColor := streamerbot.ParseHexColor(msgData.User.Color)

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
	log.Printf("Reward: %s: %s", username, redemption)
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
		if a.streamerBotClient != nil {
			a.streamerBotClient.Close()
		}

		if a.downloadPool != nil {
			a.downloadPool.Close()
		}
	})
}
