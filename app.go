package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"image/gif"
	"log"
	"stream-guy/internal/popup"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"stream-guy/internal/assets"
	"stream-guy/internal/config"
	"stream-guy/internal/download"
	"stream-guy/internal/overlay"
	"stream-guy/internal/render"
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

type TwitchChatMessageData struct {
	Message struct {
		Message string         `json:"message"`
		Role    int            `json:"role"`
		Emotes  []render.Emote `json:"emotes"`
	} `json:"message"`

	User struct {
		Badges      []BadgeData `json:"badges"`
		ID          string      `json:"id"`
		Login       string      `json:"login"`
		DisplayName string      `json:"name"`
		Color       string      `json:"color"`
	} `json:"user"`
}

type BadgeData struct {
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}

type TwitchRewardRedemptionData struct {
	Username string `json:"user_name"`
	Reward   struct {
		Title     string `json:"title"`
		UserInput string `json:"user_input"`
	} `json:"reward"`
}

func (a *App) HandleChatMessage(data json.RawMessage, timestamp string) {
	var msgData TwitchChatMessageData
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
		if err := a.createGifPopup(foundKeyword, message); err != nil {
			log.Printf("[%s] Failed to create GIF popup: %v", streamerbot.FormatTimeStamp(timestamp), err)
		}
	} else {
		badges := convertBadges(msgData.User.Badges)
		if err := a.createChatPopup(username, userColor, badges, message, emotesTag); err != nil {
			log.Printf("[%s] Failed to create chat popup: %v", streamerbot.FormatTimeStamp(timestamp), err)
		}
	}
}

func (a *App) HandleRewardRedemption(data json.RawMessage) {
	var rData TwitchRewardRedemptionData
	if err := json.Unmarshal(data, &rData); err != nil {
		log.Printf("Error parsing redemption data: %v", err)
		return
	}

	username := rData.Username
	redemption := rData.Reward.Title
	log.Printf("Reward: %s: %s", username, redemption)
}

func convertBadges(badges []BadgeData) []render.Badge {
	result := make([]render.Badge, len(badges))
	for i, b := range badges {
		result[i] = render.Badge{Name: b.Name, ImageURL: b.ImageURL}
	}
	return result
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

func (a *App) createThemeWithFont() *material.Theme {
	th := material.NewTheme()
	if a.loadedFontFace.Face != nil {
		th.Shaper = text.NewShaper(text.WithCollection([]font.FontFace{a.loadedFontFace}))
	}
	return th
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

func (a *App) createChatPopup(username string, userColor color.NRGBA, badges []render.Badge, message string, emotesTag []render.Emote) error {
	if a.isPaused() {
		return nil
	}

	segments := a.textParser.Parse(message, emotesTag)
	a.prefetchAssets(segments, badges)

	uniqueTitle := time.Now().Format("2006-01-02 15:04:05.00")

	pw := &popup.Window{
		GioWindow:       new(app.Window),
		Title:           uniqueTitle,
		Username:        username,
		UserColor:       userColor,
		Badges:          badges,
		Message:         message,
		MessageSegments: segments,
		StartTime:       time.Now(),
	}

	if err := popup.Initialize(pw, pw.Title, popup.DefaultWindowWidth, popup.DefaultWindowHeight, a.placementManager); err != nil {
		return fmt.Errorf("failed to initialize popup window: %w", err)
	}

	a.windowRegistry.Register(pw.GioWindow)

	go func() {
		th := a.createThemeWithFont()
		if err := a.runChatPopup(pw, th); err != nil {
			log.Printf("Chat popup error: %v", err)
		}
	}()

	return nil
}

func (a *App) createGifPopup(keyword string, message string) error {
	if a.isPaused() {
		return nil
	}

	if keyword == "" {
		return fmt.Errorf("empty keyword")
	}

	gifName := assets.ResolveKeyword(keyword, a.config.Keywords)
	gifData, hasGif := a.imageCache.Gifs[gifName]

	if !hasGif {
		log.Printf("No GIF found for keyword '%s' (resolved to '%s')", keyword, gifName)
	}

	uniqueTitle := time.Now().Format("2006-01-02 15:04:05.00")

	pw := &popup.Window{
		GioWindow: new(app.Window),
		Title:     uniqueTitle,
		Message:   message,
		StartTime: time.Now(),
	}

	if gifData != nil {
		pw.Compositor = assets.NewGIFCompositor(gifData)
	}

	width := popup.DefaultWindowWidth
	height := popup.DefaultWindowHeight
	if gifData != nil {
		width = gifData.Config.Width
		height = gifData.Config.Height
	}

	if err := popup.Initialize(pw, pw.Title, width, height, a.placementManager); err != nil {
		return fmt.Errorf("failed to initialize popup window: %w", err)
	}

	a.windowRegistry.Register(pw.GioWindow)

	go func() {
		if err := a.runGifPopup(pw, gifData); err != nil {
			log.Printf("GIF popup error: %v", err)
		}
	}()

	return nil
}

func (a *App) prefetchAssets(segments []assets.EmoteSegment, badges []render.Badge) {
	for _, seg := range segments {
		if seg.IsEmote && popup.IsValidEmoteURL(seg.ImageURL) {
			a.emoteManager.PrefetchEmote(seg.ImageURL)
		}
	}

	for _, badge := range badges {
		a.badgeManager.Prefetch(badge.ImageURL)
	}
}

func (a *App) runChatPopup(pw *popup.Window, th *material.Theme) error {
	var ops op.Ops
	resized := false

	defer func() {
		window.CleanupWindowHandle(pw.Title)
		a.emoteManager.UnregisterWindow(pw.GioWindow)
		a.badgeManager.UnregisterWindow(pw.GioWindow)
		a.windowRegistry.Unregister(pw.GioWindow)
	}()

	for {
		e := pw.GioWindow.Event()
		switch ev := e.(type) {
		case app.DestroyEvent:
			return ev.Err

		case app.FrameEvent:
			gtx := app.NewContext(&ops, ev)

			popup.HandleContextMenuEvents(gtx, pw)
			popup.HandleCopyButton(gtx, pw)

			if !resized && time.Since(pw.StartTime) > 100*time.Millisecond {
				resized = a.resizeWindowToContent(gtx, pw, th)
			}

			paint.Fill(gtx.Ops, render.SilverBackground)
			a.renderChatContent(gtx, pw, th)

			if pw.ContextMenu {
				popup.RenderContextMenu(gtx, pw, th)
			}

			ev.Frame(gtx.Ops)
		}
	}
}

func (a *App) runGifPopup(pw *popup.Window, gifData *gif.GIF) error {
	var ops op.Ops

	defer func() {
		window.CleanupWindowHandle(pw.Title)
		a.windowRegistry.Unregister(pw.GioWindow)
		if pw.AnimCancel != nil {
			pw.AnimCancel()
		}
	}()

	hasGif := gifData != nil && len(gifData.Image) > 0

	if hasGif {
		pw.AnimCtx, pw.AnimCancel = context.WithCancel(context.Background())
		go popup.AnimateGif(pw, gifData)
	}

	for {
		e := pw.GioWindow.Event()
		switch ev := e.(type) {
		case app.DestroyEvent:
			return ev.Err

		case app.FrameEvent:
			gtx := app.NewContext(&ops, ev)
			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if hasGif {
					return popup.RenderGifFrame(gtx, pw, gifData)
				}
				return layout.Dimensions{}
			})
			ev.Frame(gtx.Ops)
		}
	}
}

func (a *App) resizeWindowToContent(gtx layout.Context, pw *popup.Window, th *material.Theme) bool {
	macro := op.Record(gtx.Ops)
	measureGtx := gtx
	measureGtx.Constraints.Min.X = popup.DefaultWindowWidth
	measureGtx.Constraints.Max.X = popup.DefaultWindowWidth
	measureGtx.Constraints.Max.Y = 10000

	dims := a.renderChatContent(measureGtx, pw, th)
	macro.Stop()

	desiredHeight := popup.ClampHeight(dims.Size.Y + 10)
	pw.GioWindow.Option(app.Size(unit.Dp(popup.DefaultWindowWidth), unit.Dp(desiredHeight)))

	go func(title string) {
		time.Sleep(50 * time.Millisecond)
		wnd := window.FindWindowByTitleCached(title)
		if wnd != 0 {
			window.ClampWindowToWorkArea(wnd)
		}
	}(pw.Title)

	return true
}

func (a *App) renderChatContent(gtx layout.Context, pw *popup.Window, th *material.Theme) layout.Dimensions {
	return layout.UniformInset(unit.Dp(render.MessagePadding)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return a.buildChatLayout(gtx, pw, th)
	})
}

func (a *App) buildChatLayout(gtx layout.Context, pw *popup.Window, th *material.Theme) layout.Dimensions {
	return layout.Flex{
		Axis:      layout.Vertical,
		Alignment: layout.Start,
	}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.buildHeaderLayout(gtx, pw, th)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Spacer{Height: unit.Dp(render.MessageSpacing)}.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.emoteManager.LayoutMessageWithEmotes(gtx, th, pw.MessageSegments, pw.GioWindow)
		}),
	)
}

func (a *App) buildHeaderLayout(gtx layout.Context, pw *popup.Window, th *material.Theme) layout.Dimensions {
	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return render.LayoutBadges(gtx, pw.Badges, pw.GioWindow, a.badgeManager)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return render.LayoutUsername(gtx, th, pw.Username, pw.UserColor)
		}),
	)
}
