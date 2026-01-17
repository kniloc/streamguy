package overlay

import "image/color"

func ToolbarRect() (x, y, w, h int) {
	return ToolbarX, ToolbarY, ToolbarW, ToolbarH
}

func ToolbarButtonRect(i int) (x, y, w, h int) {
	x = ToolbarX + ToolbarPadding + i*(ToolbarButton+ToolbarGap)
	y = ToolbarY + ToolbarPadding
	return x, y, ToolbarButton, ToolbarButton
}

func (o *Window) IsToolbarPixel(x, y int) bool {
	if !o.drawMode.Load() {
		return false
	}
	return x >= ToolbarX && x < ToolbarX+ToolbarW && y >= ToolbarY && y < ToolbarY+ToolbarH
}

func (o *Window) ToolbarButtonAt(p Point) (int, bool) {
	if !o.drawMode.Load() {
		return 0, false
	}

	px := int(p.X)
	py := int(p.Y)
	tx, ty, tw, th := ToolbarRect()
	if px < tx || px >= tx+tw || py < ty || py >= ty+th {
		return 0, false
	}

	rX := px - (tx + ToolbarPadding)
	rY := py - (ty + ToolbarPadding)
	if rX < 0 || rY < 0 || rY >= ToolbarButton {
		return 0, false
	}

	cell := ToolbarButton + ToolbarGap
	idx := rX / cell
	if idx < 0 || idx >= ToolbarBtnCount {
		return 0, false
	}
	if rX%cell >= ToolbarButton {
		return 0, false
	}

	return idx, true
}

func (o *Window) HandleToolbarClick(p Point) bool {
	idx, ok := o.ToolbarButtonAt(p)
	if !ok {
		return false
	}

	switch idx {
	case 0:
		o.drawMode.Store(false)
		o.ResetCurrent()
		o.Drawing = false
		o.ApplyClickThrough(true)
		o.Redraw()
		return true
	case 1:
		o.ClearAndRedraw()
		return true
	default:
		paletteIdx := idx - 2
		if paletteIdx >= 0 && paletteIdx < len(Palette) {
			o.SelectedColor = paletteIdx
			o.CurrentColor = Palette[paletteIdx]
			o.Redraw()
			return true
		}
	}

	return true
}

func (o *Window) DrawToolbar(selectedColor int) {
	tx, ty, tw, th := ToolbarRect()
	o.FillRect(tx, ty, tw, th, color.NRGBA{A: 0xB0})
	o.DrawRectBorder(tx, ty, tw, th, 1, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xC0})

	offX, offY, bw, bh := ToolbarButtonRect(0)
	o.FillRect(offX, offY, bw, bh, color.NRGBA{R: 0xD0, G: 0x30, B: 0x30, A: 0xFF})
	o.DrawX(offX+6, offY+6, bw-12, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})

	clrX, clrY, _, _ := ToolbarButtonRect(1)
	o.FillRect(clrX, clrY, bw, bh, color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xFF})
	o.DrawRectBorder(clrX+7, clrY+8, bw-14, bh-14, 1, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
	o.FillRect(clrX+10, clrY+18, bw-20, 3, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})

	for i, c := range Palette {
		sx, sy, _, _ := ToolbarButtonRect(i + 2)
		o.FillRect(sx, sy, bw, bh, c)
		border := color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
		th := 1

		if i == selectedColor {
			border = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
			th = 2

			if c == DefaultStrokeColor {
				border = color.NRGBA{R: 0x80, G: 0x00, B: 0x80, A: 0xFF}
			}
		}

		o.DrawRectBorder(sx, sy, bw, bh, th, border)
	}
}
