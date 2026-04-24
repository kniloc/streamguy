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

	CommandImage image.Image

	PhotoImage image.Image
	PhotoURL   string
	PhotoMime  string
	AcceptBtn  widget.Clickable
	RejectBtn  widget.Clickable
	OnAccept   func(url, mimeType string)

	InitialX int
	InitialY int
	HWND     windows.HWND

	hoveredEmoteName   string
	hoveredEmoteSize   image.Point
	hoveredEmoteWinPos image.Point
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

func ConfigureFromViewEvent(popup *Window, hwnd windows.HWND, zorder *window.ZOrderManager) {
	if popup.HWND != 0 {
		return
	}
	popup.HWND = hwnd

	go func() {
		time.Sleep(10 * time.Millisecond)
		window.ConfigurePopWindow(hwnd)
		window.SetPopupWindowPositionByHandle(hwnd, popup.InitialX, popup.InitialY)
		window.ClampWindowToWorkArea(hwnd)
		zorder.RequestTopmost(hwnd)
	}()
}

func HandleEmoteHoverEvents(gtx layout.Context, pw *Window, em *assets.EmoteManager) {
	if em == nil {
		return
	}
	for _, seg := range pw.MessageSegments {
		if !seg.IsEmote {
			continue
		}
		if strings.Contains(seg.ImageURL, assets.TwemojiCDNPath) {
			continue
		}
		tag := em.GetOrCreateHoverTag(seg.ImageURL, seg.Text, pw.GioWindow)
		for {
			ev, ok := gtx.Source.Event(pointer.Filter{
				Target: tag,
				Kinds:  pointer.Enter | pointer.Leave,
			})
			if !ok {
				break
			}
			if pe, ok := ev.(pointer.Event); ok {
				switch pe.Kind {
				case pointer.Enter:
					pw.hoveredEmoteName = seg.Text
					pw.hoveredEmoteSize = tag.Size
					pw.hoveredEmoteWinPos = tag.WindowPos
					pw.GioWindow.Invalidate()
				case pointer.Leave:
					pw.hoveredEmoteName = ""
					pw.hoveredEmoteSize = image.Point{}
					pw.hoveredEmoteWinPos = image.Point{}
					pw.GioWindow.Invalidate()
				}
			}
		}
	}
}

func RenderEmoteTooltip(gtx layout.Context, name string, emoteWinPos image.Point, emoteSize image.Point, th *material.Theme) {
	const (
		tooltipPaddingX = 8
		tooltipHeight   = 24
		tooltipGap      = 2
	)

	label := material.Body1(th, name)
	label.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	label.TextSize = unit.Sp(12)

	macro := op.Record(gtx.Ops)
	measureGtx := gtx
	measureGtx.Constraints = layout.Constraints{
		Max: image.Point{X: 1 << 20, Y: tooltipHeight},
	}
	dims := label.Layout(measureGtx)
	macro.Stop()

	tooltipWidth := dims.Size.X + tooltipPaddingX*2

	emoteCenterX := emoteWinPos.X + emoteSize.X/2
	x := emoteCenterX - tooltipWidth/2
	y := emoteWinPos.Y - tooltipHeight - tooltipGap

	if x+tooltipWidth > gtx.Constraints.Max.X {
		x = gtx.Constraints.Max.X - tooltipWidth
	}
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	offset := op.Offset(image.Point{X: x, Y: y}).Push(gtx.Ops)
	defer offset.Pop()

	bgRect := clip.RRect{
		Rect: image.Rectangle{Max: image.Point{X: tooltipWidth, Y: tooltipHeight}},
		NE:   3, NW: 3, SE: 3, SW: 3,
	}.Push(gtx.Ops)
	paint.Fill(gtx.Ops, color.NRGBA{R: 30, G: 30, B: 30, A: 220})
	bgRect.Pop()

	innerGtx := gtx
	innerGtx.Constraints = layout.Exact(image.Point{X: tooltipWidth, Y: tooltipHeight})
	layout.Inset{Left: unit.Dp(tooltipPaddingX), Right: unit.Dp(tooltipPaddingX)}.Layout(innerGtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Center.Layout(gtx, label.Layout)
	})
}
