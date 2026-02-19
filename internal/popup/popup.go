package popup

import (
	"context"
	"image"
	"image/color"
	"image/gif"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"stream-guy/internal/assets"
	"stream-guy/internal/render"
	"stream-guy/internal/window"

	"gioui.org/app"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/sys/windows"
)

const (
	DefaultWindowWidth  = 400
	DefaultWindowHeight = 100
	MinWindowHeight     = 60
	MaxWindowHeight     = 600
	FrameUpdateInterval = 500 * time.Millisecond
	PhotoPopupWidth     = 500
	PhotoPopupHeight    = 450
	PhotoButtonHeight   = 40
)

type Window struct {
	GioWindow       *app.Window
	Title           string
	Username        string
	UserColor       color.NRGBA
	Badges          []render.Badge
	Message         string
	MessageSegments []assets.EmoteSegment
	Compositor      *assets.GIFCompositor
	CurrentFrame    int
	FramesMu        sync.RWMutex
	StartTime       time.Time

	ContextMenu    bool
	ContextMenuPos image.Point
	CopyButton     widget.Clickable
	EventTag       event.Tag

	AnimCtx    context.Context
	AnimCancel context.CancelFunc

	// Command popup fields
	CommandImage image.Image

	// Photo popup fields
	PhotoImage image.Image
	PhotoURL   string
	PhotoMime  string
	AcceptBtn  widget.Clickable
	RejectBtn  widget.Clickable
	OnAccept   func(url, mimeType string)

	// Window positioning
	InitialX int
	InitialY int
	HWND     windows.HWND
}

func ClampHeight(height int) int {
	if height < MinWindowHeight {
		return MinWindowHeight
	}
	if height > MaxWindowHeight {
		return MaxWindowHeight
	}
	return height
}

func HandleContextMenuEvents(gtx layout.Context, popup *Window) {
	for {
		ev, ok := gtx.Source.Event(pointer.Filter{
			Target: &popup.EventTag,
			Kinds:  pointer.Press | pointer.Release,
		})
		if !ok {
			break
		}

		if pe, ok := ev.(pointer.Event); ok {
			if pe.Buttons == pointer.ButtonSecondary && pe.Kind == pointer.Press {
				popup.ContextMenu = true
				popup.ContextMenuPos = image.Point{
					X: int(pe.Position.X),
					Y: int(pe.Position.Y),
				}
			} else if pe.Buttons == pointer.ButtonPrimary && pe.Kind == pointer.Press {
				popup.ContextMenu = false
			}
		}
	}
}

func RenderContextMenu(gtx layout.Context, popup *Window, th *material.Theme) {
	menuWidth := 120
	menuHeight := 40

	x := popup.ContextMenuPos.X
	y := popup.ContextMenuPos.Y

	if x+menuWidth > gtx.Constraints.Max.X {
		x = gtx.Constraints.Max.X - menuWidth
	}
	if y+menuHeight > gtx.Constraints.Max.Y {
		y = gtx.Constraints.Max.Y - menuHeight
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	offset := op.Offset(image.Point{X: x, Y: y}).Push(gtx.Ops)
	defer offset.Pop()

	gtx.Constraints = layout.Exact(image.Point{X: menuWidth, Y: menuHeight})

	menuRect := clip.Rect{Max: image.Point{X: menuWidth, Y: menuHeight}}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, color.NRGBA{R: 40, G: 40, B: 40, A: 255})
	menuRect.Pop()

	borderRect := clip.Stroke{
		Path:  clip.Rect{Max: image.Point{X: menuWidth, Y: menuHeight}}.Path(),
		Width: 1,
	}.Op().Push(gtx.Ops)
	paint.Fill(gtx.Ops, color.NRGBA{R: 100, G: 100, B: 100, A: 255})
	borderRect.Pop()

	layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, &popup.CopyButton, "Copy Message")
		btn.Background = color.NRGBA{R: 60, G: 60, B: 60, A: 255}
		btn.Color = render.BlackText
		btn.TextSize = unit.Sp(12)
		return btn.Layout(gtx)
	})
}

func RenderGifFrame(gtx layout.Context, popup *Window, gifImg *gif.GIF) layout.Dimensions {
	if gifImg == nil || len(gifImg.Image) == 0 {
		return layout.Dimensions{}
	}

	popup.FramesMu.RLock()
	frameIndex := popup.CurrentFrame
	popup.FramesMu.RUnlock()

	if frameIndex >= len(gifImg.Image) {
		return layout.Dimensions{}
	}

	if popup.Compositor == nil {
		return layout.Dimensions{}
	}

	composited := popup.Compositor.CompositeFrame(gifImg, frameIndex)
	if composited == nil {
		return layout.Dimensions{}
	}

	imgOp := paint.NewImageOp(composited)
	imgOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)

	return layout.Dimensions{
		Size: image.Point{X: gifImg.Config.Width, Y: gifImg.Config.Height},
	}
}

func AnimateGif(popup *Window, gifImg *gif.GIF) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		popup.FramesMu.RLock()
		frameIndex := popup.CurrentFrame
		popup.FramesMu.RUnlock()

		delay := gifImg.Delay[frameIndex]
		if delay == 0 {
			delay = assets.DefaultGIFFrameDelay
		}
		duration := time.Duration(delay) * assets.MSPerFrameUnit

		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(duration)

		select {
		case <-popup.AnimCtx.Done():
			return
		case <-timer.C:
		}

		if gifImg == nil || len(gifImg.Image) == 0 {
			return
		}

		popup.FramesMu.Lock()
		popup.CurrentFrame = (popup.CurrentFrame + 1) % len(gifImg.Image)
		popup.FramesMu.Unlock()

		popup.GioWindow.Invalidate()
	}
}

func IsValidEmoteURL(url string) bool {
	if url == "" {
		return false
	}

	if strings.Contains(url, assets.TwemojiCDNPath) {
		return true
	}

	if strings.HasSuffix(url, "/.png") ||
		strings.HasSuffix(url, "/.jpg") ||
		strings.HasSuffix(url, "/.gif") ||
		strings.HasSuffix(url, "/.webp") {
		log.Printf("Skipping malformed emote URL: %s", url)
		return false
	}

	return true
}

func ValidatePhotoURL(url string) (mimeType string, valid bool) {
	lowerURL := strings.ToLower(url)

	switch {
	case strings.HasSuffix(lowerURL, ".png"):
		return "image/png", true
	case strings.HasSuffix(lowerURL, ".jpg"), strings.HasSuffix(lowerURL, ".jpeg"):
		return "image/jpeg", true
	case strings.HasSuffix(lowerURL, ".webp"):
		return "image/webp", true
	}

	switch {
	case strings.HasSuffix(lowerURL, "@png"):
		return "image/png", true
	case strings.HasSuffix(lowerURL, "@jpg"), strings.HasSuffix(lowerURL, "@jpeg"):
		return "image/jpeg", true
	case strings.HasSuffix(lowerURL, "@webp"):
		return "image/webp", true
	}

	return "", false
}

func MimeTypeFromContentType(contentType string) (mimeType string, valid bool) {
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "image/png"):
		return "image/png", true
	case strings.Contains(ct, "image/jpeg"):
		return "image/jpeg", true
	case strings.Contains(ct, "image/webp"):
		return "image/webp", true
	default:
		return "", false
	}
}

func HandleCopyButton(gtx layout.Context, popup *Window) {
	if popup.CopyButton.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "text/plain",
			Data: io.NopCloser(strings.NewReader(popup.Message)),
		})
		popup.ContextMenu = false
	}
}

func Initialize(popup *Window, title string, width, height int, placementManager *window.PlacementManager) error {
	x, y := placementManager.FindNonOverlappingPosition(width, height)

	popup.GioWindow.Option(app.Title(title))
	popup.GioWindow.Option(app.Size(unit.Dp(width), unit.Dp(height)))
	popup.GioWindow.Option(app.Decorated(true))

	popup.InitialX = x
	popup.InitialY = y

	placementManager.ClearOldWindows(window.MaxWindowsOnScreen)

	return nil
}

func ConfigureFromViewEvent(popup *Window, hwnd windows.HWND) {
	if popup.HWND != 0 {
		return
	}
	popup.HWND = hwnd

	go func() {
		time.Sleep(10 * time.Millisecond)
		window.ConfigurePopWindow(hwnd)
		window.SetPopupWindowPositionByHandle(hwnd, popup.InitialX, popup.InitialY)
		window.ClampWindowToWorkArea(hwnd)
	}()
}
