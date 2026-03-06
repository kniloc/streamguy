package assets

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"log"
	"strings"
	"sync"
	"time"

	"stream-guy/internal/download"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const (
	TwemojiCDNPath       = "cdn.jsdelivr.net/gh/jdecked/twemoji@17.0.2"
	DefaultGIFFrameDelay = 10
	MSPerFrameUnit       = 10 * time.Millisecond
	EmoteScaledSize      = 28
)

var BlackText = color.NRGBA{R: 0, G: 0, B: 0, A: 255}

type EmoteManager struct {
	*BaseManager
	gifCache         map[string]*gif.GIF
	gifCacheMu       sync.RWMutex
	currentFrames    map[string]int
	framesMu         sync.RWMutex
	animationTickers map[string]*animationTicker
	animatingMu      sync.RWMutex
	compositors      map[string]*GIFCompositor
	compositorsMu    sync.RWMutex
}

type animationTicker struct {
	ctx    context.Context
	cancel context.CancelFunc
}

type EmoteData struct {
	GIF    *gif.GIF
	Static image.Image
	IsGIF  bool
}

func NewEmoteManager(downloadPool *download.Pool) *EmoteManager {
	return &EmoteManager{
		BaseManager:      NewBaseManager(downloadPool),
		gifCache:         make(map[string]*gif.GIF),
		currentFrames:    make(map[string]int),
		animationTickers: make(map[string]*animationTicker),
		compositors:      make(map[string]*GIFCompositor),
	}
}

func (em *EmoteManager) GetEmote(url string) (*EmoteData, error) {
	em.gifCacheMu.RLock()
	if gifImg, ok := em.gifCache[url]; ok {
		em.gifCacheMu.RUnlock()
		return &EmoteData{GIF: gifImg, IsGIF: true}, nil
	}
	em.gifCacheMu.RUnlock()

	if staticImg, ok := em.GetCached(url); ok {
		return &EmoteData{Static: staticImg, IsGIF: false}, nil
	}

	if em.IsDownloading(url) {
		return nil, ErrAssetDownloading
	}

	return nil, em.downloadEmote(url)
}

func (em *EmoteManager) PrefetchEmote(url string) {
	em.gifCacheMu.RLock()
	_, inGifCache := em.gifCache[url]
	em.gifCacheMu.RUnlock()

	if inGifCache {
		return
	}

	if _, ok := em.GetCached(url); ok {
		return
	}

	if em.IsDownloading(url) {
		return
	}

	em.downloadEmote(url)
}

func (em *EmoteManager) downloadEmote(url string) error {
	return em.DownloadAsset(url, "emote", func(result *download.Result) error {
		if result.IsGIF {
			if result.GIF == nil {
				return fmt.Errorf("no GIF data")
			}
			em.gifCacheMu.Lock()
			em.gifCache[url] = result.GIF
			em.gifCacheMu.Unlock()

			em.framesMu.Lock()
			em.currentFrames[url] = 0
			em.framesMu.Unlock()

			em.compositorsMu.Lock()
			em.compositors[url] = NewGIFCompositor(result.GIF)
			em.compositorsMu.Unlock()
		} else {
			if result.StaticImage == nil {
				return fmt.Errorf("no static image data")
			}
			em.SetCached(url, result.StaticImage)
		}

		return nil
	})
}

func (em *EmoteManager) GetCurrentFrame(url string) int {
	em.framesMu.RLock()
	defer em.framesMu.RUnlock()
	return em.currentFrames[url]
}

func (em *EmoteManager) GetCompositedFrame(url string, gifImg *gif.GIF, frameIndex int) image.Image {
	em.compositorsMu.RLock()
	compositor := em.compositors[url]
	em.compositorsMu.RUnlock()

	if compositor == nil {
		compositor = NewGIFCompositor(gifImg)
		em.compositorsMu.Lock()
		em.compositors[url] = compositor
		em.compositorsMu.Unlock()
	}

	return compositor.CompositeFrame(gifImg, frameIndex)
}

func (em *EmoteManager) StartAnimationTicker(url string, gifImg *gif.GIF, window *app.Window) {
	if len(gifImg.Image) <= 1 {
		return
	}

	em.RegisterWindow(url, window)

	em.animatingMu.Lock()
	if _, exists := em.animationTickers[url]; exists {
		em.animatingMu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	em.animationTickers[url] = &animationTicker{
		ctx:    ctx,
		cancel: cancel,
	}
	em.animatingMu.Unlock()

	go em.animateEmote(ctx, url, gifImg)
}

func (em *EmoteManager) animateEmote(ctx context.Context, url string, gifImg *gif.GIF) {
	defer func() {
		em.animatingMu.Lock()
		delete(em.animationTickers, url)
		em.animatingMu.Unlock()
	}()

	frameIndex := 0

	delay := gifImg.Delay[frameIndex]
	if delay == 0 {
		delay = DefaultGIFFrameDelay
	}
	timer := time.NewTimer(time.Duration(delay) * MSPerFrameUnit)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		numFrames := len(gifImg.Image)
		if numFrames == 0 {
			return
		}

		frameIndex = (frameIndex + 1) % numFrames
		em.framesMu.Lock()
		em.currentFrames[url] = frameIndex
		em.framesMu.Unlock()

		em.InvalidateWindows(url)

		delay = gifImg.Delay[frameIndex]
		if delay == 0 {
			delay = DefaultGIFFrameDelay
		}
		timer.Reset(time.Duration(delay) * MSPerFrameUnit)
	}
}

func (em *EmoteManager) StopAnimation(url string) {
	em.animatingMu.Lock()
	defer em.animatingMu.Unlock()

	if ticker, exists := em.animationTickers[url]; exists {
		ticker.cancel()
		delete(em.animationTickers, url)
	}
}

func (em *EmoteManager) StopAllAnimations() {
	em.animatingMu.Lock()
	defer em.animatingMu.Unlock()

	for url, ticker := range em.animationTickers {
		ticker.cancel()
		delete(em.animationTickers, url)
	}
}

func (em *EmoteManager) UnregisterWindow(window *app.Window) {
	em.windowsMu.Lock()

	for url, windows := range em.Windows {
		filtered := make([]*app.Window, 0, len(windows))
		for _, w := range windows {
			if w != window {
				filtered = append(filtered, w)
			}
		}
		em.Windows[url] = filtered

		if len(filtered) == 0 {
			em.windowsMu.Unlock()
			em.StopAnimation(url)
			em.windowsMu.Lock()
		}
	}

	em.windowsMu.Unlock()
}

func CreateTextLabel(th *material.Theme, text string) material.LabelStyle {
	label := material.Body1(th, text)
	label.Color = BlackText
	label.Font.Typeface = FontName
	label.TextSize = unit.Sp(18)
	return label
}

func IsValidEmoteURL(url string) bool {
	if url == "" {
		return false
	}

	if strings.Contains(url, TwemojiCDNPath) {
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

type EmoteSegment struct {
	IsEmote  bool
	Text     string
	ImageURL string
}

func (em *EmoteManager) LayoutMessageWithEmotes(gtx layout.Context, th *material.Theme, segments []EmoteSegment, window *app.Window) layout.Dimensions {
	if len(segments) == 0 {
		return layout.Dimensions{}
	}

	hasText := false
	for _, seg := range segments {
		if !seg.IsEmote {
			hasText = true
			break
		}
	}

	var children []layout.FlexChild
	for _, segment := range segments {
		seg := segment
		if seg.IsEmote {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return em.renderEmoteSegment(gtx, th, seg, window, hasText)
			}))
		} else {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return CreateTextLabel(th, seg.Text).Layout(gtx)
			}))
		}
	}

	return layout.Flex{
		Axis:      layout.Horizontal,
		Alignment: layout.Middle,
	}.Layout(gtx, children...)
}

func (em *EmoteManager) renderEmoteSegment(gtx layout.Context, th *material.Theme, seg EmoteSegment, window *app.Window, hasText bool) layout.Dimensions {
	isTwemojiURL := strings.Contains(seg.ImageURL, TwemojiCDNPath)
	if seg.ImageURL == "" || (!isTwemojiURL && !IsValidEmoteURL(seg.ImageURL)) {
		return CreateTextLabel(th, seg.Text).Layout(gtx)
	}

	emoteData, err := em.GetEmote(seg.ImageURL)
	if err != nil {
		return CreateTextLabel(th, seg.Text).Layout(gtx)
	}

	var img image.Image
	var bounds image.Rectangle

	if emoteData.IsGIF {
		if len(emoteData.GIF.Image) > 1 {
			em.StartAnimationTicker(seg.ImageURL, emoteData.GIF, window)
		}

		frameIndex := em.GetCurrentFrame(seg.ImageURL)
		if frameIndex < 0 || frameIndex >= len(emoteData.GIF.Image) {
			frameIndex = 0
		}

		if len(emoteData.GIF.Image) == 0 {
			return CreateTextLabel(th, seg.Text).Layout(gtx)
		}

		img = em.GetCompositedFrame(seg.ImageURL, emoteData.GIF, frameIndex)
		bounds = image.Rect(0, 0, emoteData.GIF.Config.Width, emoteData.GIF.Config.Height)
	} else {
		img = emoteData.Static
		bounds = img.Bounds()
	}

	isUnicodeEmoji := strings.Contains(seg.ImageURL, TwemojiCDNPath)

	if isUnicodeEmoji || hasText {
		scaleX := float32(EmoteScaledSize) / float32(bounds.Dx())
		scaleY := float32(EmoteScaledSize) / float32(bounds.Dy())
		scale := scaleX
		if scaleY < scaleX {
			scale = scaleY
		}
		scaledWidth := int(float32(bounds.Dx()) * scale)
		scaledHeight := int(float32(bounds.Dy()) * scale)

		scaledImg := ScaleImage(img, scaledWidth, scaledHeight)

		imgOp := paint.NewImageOp(scaledImg)
		imgOp.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)

		return layout.Dimensions{
			Size: image.Point{X: scaledWidth, Y: scaledHeight},
		}
	}

	imgOp := paint.NewImageOp(img)
	imgOp.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{
		Size: bounds.Size(),
	}
}
