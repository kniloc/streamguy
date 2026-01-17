package render

import (
	"image"
	"image/color"

	"stream-guy/internal/assets"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const (
	BadgeSize      = 18
	BadgeSpacing   = 4
	MessagePadding = 16
	MessageSpacing = 8
)

var (
	SilverBackground = color.NRGBA{R: 0xbd, G: 0xbd, B: 0xbd, A: 255}
	BlackText        = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
)

func GetLuminance(c color.NRGBA) float64 {
	r := float64(c.R) / 255.0
	g := float64(c.G) / 255.0
	b := float64(c.B) / 255.0

	if r <= 0.03928 {
		r = r / 12.92
	} else {
		r = (r + 0.055) / 1.055
		r = r * r * r
	}
	if g <= 0.03928 {
		g = g / 12.92
	} else {
		g = (g + 0.055) / 1.055
		g = g * g * g
	}
	if b <= 0.03928 {
		b = b / 12.92
	} else {
		b = (b + 0.055) / 1.055
		b = b * b * b
	}

	return 0.2126*r + 0.7152*g + 0.0722*b
}

func GetContrastRatio(c1, c2 color.NRGBA) float64 {
	l1 := GetLuminance(c1)
	l2 := GetLuminance(c2)

	lighter := l1
	darker := l2
	if l2 > l1 {
		lighter = l2
		darker = l1
	}

	return (lighter + 0.05) / (darker + 0.05)
}

func EnsureReadableColor(foreground, background color.NRGBA) color.NRGBA {
	const minContrastRatio = 4.5

	contrast := GetContrastRatio(foreground, background)

	if contrast >= minContrastRatio {
		return foreground
	}

	adjusted := foreground
	for range 50 {
		adjusted.R = uint8(float64(adjusted.R) * 0.85)
		adjusted.G = uint8(float64(adjusted.G) * 0.85)
		adjusted.B = uint8(float64(adjusted.B) * 0.85)

		contrast = GetContrastRatio(adjusted, background)
		if contrast >= minContrastRatio {
			return adjusted
		}

		if adjusted.R < 20 && adjusted.G < 20 && adjusted.B < 20 {
			break
		}
	}

	return color.NRGBA{R: 0, G: 0, B: 0, A: 255}
}

type Badge struct {
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
}

func LayoutBadges(gtx layout.Context, badges []Badge, window *app.Window, badgeManager *assets.BadgeManager) layout.Dimensions {
	if len(badges) == 0 {
		return layout.Dimensions{}
	}

	var children []layout.FlexChild

	for _, badge := range badges {
		b := badge
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			img, err := badgeManager.GetBadge(b.ImageURL, window)
			if err != nil {
				return layout.Dimensions{
					Size: image.Point{X: BadgeSize, Y: BadgeSize},
				}
			}

			imgOp := paint.NewImageOp(img)
			imgOp.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)

			return layout.Dimensions{
				Size: image.Point{X: BadgeSize, Y: BadgeSize},
			}
		}))

		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Spacer{Width: unit.Dp(BadgeSpacing)}.Layout(gtx)
		}))
	}

	return layout.Flex{
		Axis: layout.Horizontal,
	}.Layout(gtx, children...)
}

func LayoutUsername(gtx layout.Context, th *material.Theme, username string, userColor color.NRGBA) layout.Dimensions {
	readableColor := EnsureReadableColor(userColor, SilverBackground)

	label := material.Body1(th, username+":")
	label.Color = readableColor
	label.Font.Typeface = assets.FontName
	label.TextSize = unit.Sp(18)
	return label.Layout(gtx)
}
