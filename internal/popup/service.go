package popup

import (
	"context"
	"fmt"
	"image/color"
	"image/gif"
	"log"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"stream-guy/internal/assets"
	"stream-guy/internal/render"
	"stream-guy/internal/window"
)

type Service struct {
	ImageCache       *assets.ImageCache
	EmoteManager     *assets.EmoteManager
	BadgeManager     *assets.BadgeManager
	TextParser       *render.TextParser
	WindowRegistry   *window.Registry
	PlacementManager *window.PlacementManager

	Theme          *material.Theme
	LoadedFontFace font.FontFace

	// Optional hook; when provided, Create*Popup will be a no-op while paused.
	IsPaused func() bool

	// Keyword mapping for resolving GIF filenames.
	Keywords map[string]string
}

func (s *Service) themeWithFont() *material.Theme {
	th := s.Theme
	if th == nil {
		th = material.NewTheme()
	}
	if s.LoadedFontFace.Face != nil {
		th.Shaper = text.NewShaper(text.WithCollection([]font.FontFace{s.LoadedFontFace}))
	}
	return th
}

func (s *Service) paused() bool {
	if s == nil || s.IsPaused == nil {
		return false
	}
	return s.IsPaused()
}

func (s *Service) CreateChatPopup(username string, userColor color.NRGBA, badges []render.Badge, message string, emotesTag []render.Emote) error {
	if s.paused() {
		return nil
	}
	if s.TextParser == nil || s.WindowRegistry == nil || s.PlacementManager == nil {
		return fmt.Errorf("popup service not initialized")
	}

	segments := s.TextParser.Parse(message, emotesTag)
	s.prefetchAssets(segments, badges)

	pw := &Window{
		GioWindow:       new(app.Window),
		Title:           time.Now().Format("2006-01-02 15:04:05.00"),
		Username:        username,
		UserColor:       userColor,
		Badges:          badges,
		Message:         message,
		MessageSegments: segments,
		StartTime:       time.Now(),
	}

	if err := Initialize(pw, pw.Title, DefaultWindowWidth, DefaultWindowHeight, s.PlacementManager); err != nil {
		return fmt.Errorf("failed to initialize popup window: %w", err)
	}

	s.WindowRegistry.Register(pw.GioWindow)

	go func() {
		th := s.themeWithFont()
		if err := s.runChatPopup(pw, th); err != nil {
			log.Printf("Chat popup error: %v", err)
		}
	}()

	return nil
}

func (s *Service) CreateGifPopup(keyword string, message string) error {
	if s.paused() {
		return nil
	}
	if keyword == "" {
		return fmt.Errorf("empty keyword")
	}
	if s.ImageCache == nil || s.WindowRegistry == nil || s.PlacementManager == nil {
		return fmt.Errorf("popup service not initialized")
	}

	gifName := assets.ResolveKeyword(keyword, s.Keywords)
	gifData, _ := s.ImageCache.Gifs[gifName]

	pw := &Window{
		GioWindow: new(app.Window),
		Title:     time.Now().Format("2006-01-02 15:04:05.00"),
		Message:   message,
		StartTime: time.Now(),
	}
	if gifData != nil {
		pw.Compositor = assets.NewGIFCompositor(gifData)
	}

	width := DefaultWindowWidth
	height := DefaultWindowHeight
	if gifData != nil {
		width = gifData.Config.Width
		height = gifData.Config.Height
	}

	if err := Initialize(pw, pw.Title, width, height, s.PlacementManager); err != nil {
		return fmt.Errorf("failed to initialize popup window: %w", err)
	}

	s.WindowRegistry.Register(pw.GioWindow)

	go func() {
		if err := s.runGifPopup(pw, gifData); err != nil {
			log.Printf("GIF popup error: %v", err)
		}
	}()

	return nil
}

func (s *Service) prefetchAssets(segments []assets.EmoteSegment, badges []render.Badge) {
	if s == nil {
		return
	}

	if s.EmoteManager != nil {
		for _, seg := range segments {
			if seg.IsEmote && IsValidEmoteURL(seg.ImageURL) {
				s.EmoteManager.PrefetchEmote(seg.ImageURL)
			}
		}
	}

	if s.BadgeManager != nil {
		for _, b := range badges {
			s.BadgeManager.Prefetch(b.ImageURL)
		}
	}
}

func (s *Service) runChatPopup(pw *Window, th *material.Theme) error {
	var ops op.Ops
	resized := false

	defer func() {
		window.CleanupWindowHandle(pw.Title)
		if s.EmoteManager != nil {
			s.EmoteManager.UnregisterWindow(pw.GioWindow)
		}
		if s.BadgeManager != nil {
			s.BadgeManager.UnregisterWindow(pw.GioWindow)
		}
		if s.WindowRegistry != nil {
			s.WindowRegistry.Unregister(pw.GioWindow)
		}
	}()

	for {
		e := pw.GioWindow.Event()
		switch ev := e.(type) {
		case app.DestroyEvent:
			return ev.Err

		case app.FrameEvent:
			gtx := app.NewContext(&ops, ev)

			HandleContextMenuEvents(gtx, pw)
			HandleCopyButton(gtx, pw)

			if !resized && time.Since(pw.StartTime) > 100*time.Millisecond {
				resized = s.resizeWindowToContent(gtx, pw, th)
			}

			paint.Fill(gtx.Ops, render.SilverBackground)
			s.renderChatContent(gtx, pw, th)

			if pw.ContextMenu {
				RenderContextMenu(gtx, pw, th)
			}

			ev.Frame(gtx.Ops)
		}
	}
}

func (s *Service) runGifPopup(pw *Window, gifData *gif.GIF) error {
	var ops op.Ops

	defer func() {
		window.CleanupWindowHandle(pw.Title)
		if s.WindowRegistry != nil {
			s.WindowRegistry.Unregister(pw.GioWindow)
		}
		if pw.AnimCancel != nil {
			pw.AnimCancel()
		}
	}()

	hasGif := gifData != nil && len(gifData.Image) > 0
	if hasGif {
		pw.AnimCtx, pw.AnimCancel = context.WithCancel(context.Background())
		go AnimateGif(pw, gifData)
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
					return RenderGifFrame(gtx, pw, gifData)
				}
				return layout.Dimensions{}
			})
			ev.Frame(gtx.Ops)
		}
	}
}

func (s *Service) resizeWindowToContent(gtx layout.Context, pw *Window, th *material.Theme) bool {
	macro := op.Record(gtx.Ops)
	measureGtx := gtx
	measureGtx.Constraints.Min.X = DefaultWindowWidth
	measureGtx.Constraints.Max.X = DefaultWindowWidth
	measureGtx.Constraints.Max.Y = 10000

	dims := s.renderChatContent(measureGtx, pw, th)
	macro.Stop()

	desiredHeight := ClampHeight(dims.Size.Y + 10)
	pw.GioWindow.Option(app.Size(unit.Dp(DefaultWindowWidth), unit.Dp(desiredHeight)))

	go func(title string) {
		time.Sleep(50 * time.Millisecond)
		wnd := window.FindWindowByTitleCached(title)
		if wnd != 0 {
			window.ClampWindowToWorkArea(wnd)
		}
	}(pw.Title)

	return true
}

func (s *Service) renderChatContent(gtx layout.Context, pw *Window, th *material.Theme) layout.Dimensions {
	return layout.UniformInset(unit.Dp(render.MessagePadding)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{
			Axis:      layout.Vertical,
			Alignment: layout.Start,
		}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{
					Axis:      layout.Horizontal,
					Alignment: layout.Middle,
				}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if s.BadgeManager == nil {
							return layout.Dimensions{}
						}
						return render.LayoutBadges(gtx, pw.Badges, pw.GioWindow, s.BadgeManager)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return render.LayoutUsername(gtx, th, pw.Username, pw.UserColor)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Spacer{Height: unit.Dp(render.MessageSpacing)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.EmoteManager == nil {
					return layout.Dimensions{}
				}
				return s.EmoteManager.LayoutMessageWithEmotes(gtx, th, pw.MessageSegments, pw.GioWindow)
			}),
		)
	})
}
