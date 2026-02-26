package assets

import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"strconv"
	"strings"
)

const (
	DefaultHexColorLength = 6
	DefaultColorAlpha     = 255
)

func ScaleImage(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := src.Bounds()
	xRatio := float64(bounds.Dx()) / float64(width)
	yRatio := float64(bounds.Dy()) / float64(height)

	for y := range height {
		for x := range width {
			srcX := int(float64(x) * xRatio)
			srcY := int(float64(y) * yRatio)
			dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return dst
}

type GIFCompositor struct {
	Buffer *image.RGBA
}

func NewGIFCompositor(gifImg *gif.GIF) *GIFCompositor {
	if gifImg == nil || len(gifImg.Image) == 0 {
		return nil
	}

	bounds := image.Rect(0, 0, gifImg.Config.Width, gifImg.Config.Height)
	buffer := image.NewRGBA(bounds)

	if int(gifImg.BackgroundIndex) < len(gifImg.Image[0].Palette) {
		bgColor := gifImg.Image[0].Palette[gifImg.BackgroundIndex]
		draw.Draw(buffer, bounds, &image.Uniform{C: bgColor}, image.Point{}, draw.Src)
	} else {
		draw.Draw(buffer, bounds, &image.Uniform{C: color.Transparent}, image.Point{}, draw.Src)
	}

	return &GIFCompositor{Buffer: buffer}
}

func (gc *GIFCompositor) CompositeFrame(gifImg *gif.GIF, frameIndex int) image.Image {
	if gc == nil || gc.Buffer == nil || gifImg == nil || len(gifImg.Image) == 0 {
		return nil
	}

	if frameIndex < 0 || frameIndex >= len(gifImg.Image) {
		return gc.Buffer
	}

	if frameIndex > 0 {
		disposal := gifImg.Disposal[frameIndex-1]
		prevBounds := gifImg.Image[frameIndex-1].Bounds()

		switch disposal {
		case gif.DisposalBackground, gif.DisposalPrevious:
			if int(gifImg.BackgroundIndex) < len(gifImg.Image[0].Palette) {
				bgColor := gifImg.Image[0].Palette[gifImg.BackgroundIndex]
				draw.Draw(gc.Buffer, prevBounds, &image.Uniform{C: bgColor}, image.Point{}, draw.Src)
			} else {
				draw.Draw(gc.Buffer, prevBounds, &image.Uniform{C: color.Transparent}, image.Point{}, draw.Src)
			}
		}
	}

	frame := gifImg.Image[frameIndex]
	bounds := frame.Bounds()
	draw.Draw(gc.Buffer, bounds, frame, bounds.Min, draw.Over)

	return gc.Buffer
}

func ParseHexColor(hexColor string) color.NRGBA {
	defaultColor := color.NRGBA{R: 255, G: 255, B: 255, A: DefaultColorAlpha}
	if hexColor == "" {
		return defaultColor
	}
	hexColor = strings.TrimPrefix(hexColor, "#")
	if len(hexColor) == 3 {
		hexColor = string([]byte{hexColor[0], hexColor[0], hexColor[1], hexColor[1], hexColor[2], hexColor[2]})
	}
	if len(hexColor) != DefaultHexColorLength {
		return defaultColor
	}
	r, err1 := strconv.ParseUint(hexColor[0:2], 16, 8)
	g, err2 := strconv.ParseUint(hexColor[2:4], 16, 8)
	b, err3 := strconv.ParseUint(hexColor[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return defaultColor
	}
	return color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: DefaultColorAlpha}
}
