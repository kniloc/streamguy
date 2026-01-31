package main

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"math/rand"
	"os"
	"runtime"
	"stream-guy/internal/tts"
	"time"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/jackc/pgx/v5/pgxpool"

	"stream-guy/internal/assets"
	"stream-guy/internal/config"
	"stream-guy/internal/download"
	"stream-guy/internal/overlay"
	"stream-guy/internal/pi"
	"stream-guy/internal/popup"
	"stream-guy/internal/render"
	"stream-guy/internal/streamerbot"
	"stream-guy/internal/window"
)

const (
	ControlPanelWidth  = 450
	ControlPanelHeight = 300
	ControlPanelTitle  = "Stream Guy"
	SpacerHeight       = 10
)

func NewApp() *App {
	if ttsErr := tts.InitSpeech(); ttsErr != nil {
		log.Printf("Warning: TTS initialization failed: %v", ttsErr)
	}
	application := &App{}
	application.ctx, application.cancel = context.WithCancel(context.Background())

	loadedFontFace, err := assets.LoadFont(assets.FontName)
	if err != nil {
		log.Printf("Warning: Failed to load font: %v", err)
	}
	application.loadedFontFace = loadedFontFace

	application.theme = material.NewTheme()
	if err == nil {
		application.theme.Shaper = text.NewShaper(text.WithCollection([]font.FontFace{application.loadedFontFace}))
	}

	application.config = config.Load()

	application.downloadPool = download.NewPool(defaultDownloadWorkers())

	application.imageCache = assets.NewImageCache()
	application.imageCache.LoadImages()
	application.emoteManager = assets.NewEmoteManager(application.downloadPool)
	application.badgeManager = assets.NewBadgeManager(application.downloadPool)
	application.windowRegistry = window.NewRegistry()
	application.textParser = render.NewTextParser()

	application.placementManager = window.NewPlacementManager()
	application.placementManager.Rng = rand.New(rand.NewSource(time.Now().UnixNano()))

	application.popupService = &popup.Service{
		ImageCache:       application.imageCache,
		EmoteManager:     application.emoteManager,
		BadgeManager:     application.badgeManager,
		TextParser:       application.textParser,
		WindowRegistry:   application.windowRegistry,
		PlacementManager: application.placementManager,
		DownloadPool:     application.downloadPool,
		Theme:            application.theme,
		LoadedFontFace:   application.loadedFontFace,
		IsPaused:         application.isPaused,
		Keywords:         application.config.Keywords,
	}

	application.streamerBotClient = streamerbot.NewClient(application, application.config.StreamerbotHost, application.config.StreamerbotPort)

	if application.config.PiURL != "" {
		application.piClient = pi.NewClient(application.config.PiURL)
	}

	if application.config.PostgresURL != "" {
		pool, dbErr := pgxpool.New(application.ctx, application.config.PostgresURL)
		if dbErr != nil {
			log.Printf("Warning: Failed to connect to database: %v", dbErr)
		} else {
			application.dbPool = pool
			fmt.Println("Database pool configured")
		}
	}

	return application
}

func main() {
	application := NewApp()
	go application.streamerBotClient.Start(application.ctx)

	application.overlay = overlay.New()
	go func() {
		if err := application.overlay.Run(); err != nil {
			log.Printf("Overlay error: %v", err)
		}
	}()
	go application.overlay.MonitorDoubleShift(application.ctx)

	go func() {
		w := new(app.Window)
		w.Option(app.Title(ControlPanelTitle))
		w.Option(app.Size(unit.Dp(ControlPanelWidth), unit.Dp(ControlPanelHeight)))

		go func() {
			for range 80 {
				time.Sleep(250 * time.Millisecond)
				hwnd := window.FindWindowByTitleCached(ControlPanelTitle)
				if hwnd != 0 {
					overlay.SetControlPanelHwnd(hwnd)
					window.SetWindowTopmost(hwnd)
					overlayHwnd := window.FindWindowByTitleCached(overlay.WindowTitle)
					if overlayHwnd != 0 {
						x, y, w, h := window.GetPrimaryMonitorArea()
						window.SetOverlayBoundsBelow(overlayHwnd, hwnd, x, y, w, h)
						return
					}
				}
			}
			log.Printf("control panel/overlay HWND not found")
		}()

		if err := application.runControlPanel(w); err != nil {
			log.Fatal(err)
		}
	}()

	app.Main()
}

func getWebSocketStatus(client *streamerbot.Client) string {
	if client == nil {
		return "disconnected"
	}
	if client.Connected.Load() {
		return "connected"
	}
	if client.Reconnecting.Load() {
		return "reconnecting..."
	}
	return "disconnected"
}

func (a *App) buildControlPanelLayout(gtx layout.Context, th *material.Theme, wsStatus string) layout.Dimensions {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memoryMB := m.Alloc / 1024 / 1024
	numGoroutines := runtime.NumGoroutine()

	leftPadding := layout.Inset{Left: unit.Dp(10)}

	if a.clearAllBtn.Clicked(gtx) {
		a.closeAllWindows()
	}

	if a.pauseResumeBtn.Clicked(gtx) {
		a.setPaused(!a.isPaused())
	}

	if a.clearImagesBtn.Clicked(gtx) {
		a.piClient.ClearImages()
	}

	pauseText := "Pause Popups"
	if a.isPaused() {
		pauseText = "Resume Popups"
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			wsText := fmt.Sprintf("Streamer.bot: %s", wsStatus)
			return leftPadding.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return assets.WithFont(material.H6(th, wsText), assets.FontName).Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			winText := fmt.Sprintf("Active windows: %d", a.windowRegistry.Count())
			return leftPadding.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return assets.WithFont(material.H6(th, winText), assets.FontName).Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			perfText := fmt.Sprintf("Performance: %d MB | %d goroutines", memoryMB, numGoroutines)
			return leftPadding.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return assets.WithFont(material.H6(th, perfText), assets.FontName).Layout(gtx)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Spacer{Height: unit.Dp(SpacerHeight)}.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{
				Axis:    layout.Horizontal,
				Spacing: layout.SpaceEvenly,
			}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &a.clearAllBtn, "Clear All")
					return btn.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &a.pauseResumeBtn, pauseText)
					return btn.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &a.clearImagesBtn, "Clear Images")
					return btn.Layout(gtx)
				}),
			)
		}),
	)
}

func (a *App) runControlPanel(w *app.Window) error {
	var ops op.Ops

	ticker := time.NewTicker(popup.FrameUpdateInterval)
	defer ticker.Stop()
	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				w.Invalidate()
			}
		}
	}()

	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			a.Shutdown()
			os.Exit(0)
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			wsStatus := getWebSocketStatus(a.streamerBotClient)

			paint.Fill(gtx.Ops, color.NRGBA{R: 0xbd, G: 0xbd, B: 0xbd, A: 255})
			a.buildControlPanelLayout(gtx, a.theme, wsStatus)
			e.Frame(gtx.Ops)
		}
	}
}

func defaultDownloadWorkers() int {
	workers := min(max(runtime.NumCPU(), 2), 8)
	return workers
}
