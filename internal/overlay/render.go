package overlay

import (
	"image/color"
	"math"
)

func (o *Window) ResetCurrent() {
	if o.Current != nil {
		o.Current = o.Current[:0]
	}
}

func (o *Window) ClearStrokes() {
	if len(o.Strokes) > 0 {
		for i := 0; i < o.StrokeCount; i++ {
			idx := (o.StrokeStart + i) % len(o.Strokes)
			o.Strokes[idx] = Stroke{}
		}
	}
	o.StrokeStart = 0
	o.StrokeCount = 0
	o.ResetCurrent()
	o.Drawing = false
	o.pendingClear.Store(false)
}

func (o *Window) UndoLastStroke() {
	if o.StrokeCount == 0 {
		return
	}
	o.StrokeCount--
	idx := (o.StrokeStart + o.StrokeCount) % len(o.Strokes)
	o.Strokes[idx] = Stroke{}
}

func (o *Window) AppendStroke(s Stroke) {
	if len(s.Points) == 0 || len(o.Strokes) == 0 {
		return
	}

	idx := 0
	if o.StrokeCount < len(o.Strokes) {
		idx = (o.StrokeStart + o.StrokeCount) % len(o.Strokes)
		o.StrokeCount++
	} else {
		idx = o.StrokeStart
		o.StrokeStart = (o.StrokeStart + 1) % len(o.Strokes)
	}
	o.Strokes[idx] = s
}

func (o *Window) CurrentLast() (Point, bool) {
	if len(o.Current) == 0 {
		return Point{}, false
	}
	return o.Current[len(o.Current)-1], true
}

func (o *Window) StoreCurrentPoint(p Point) {
	if len(o.Current) == 0 {
		o.Current = append(o.Current, p)
		return
	}

	last := o.Current[len(o.Current)-1]
	dx := int64(p.X - last.X)
	dy := int64(p.Y - last.Y)
	minSq := int64(MinPointDist * MinPointDist)
	if dx*dx+dy*dy < minSq {
		if len(o.Current) >= MaxPoints {
			o.Current[len(o.Current)-1] = p
		}
		return
	}

	if len(o.Current) < MaxPoints {
		o.Current = append(o.Current, p)
		return
	}
	o.Current[len(o.Current)-1] = p
}

func (o *Window) ClearAndRedraw() {
	o.ClearStrokes()
	o.Redraw()
}

func (o *Window) Redraw() {
	if o.Hwnd == 0 || o.MemDC == 0 || o.Bits == nil {
		return
	}

	clear(o.Buf)
	drawMode := o.drawMode.Load()
	bgA := byte(0)

	if drawMode {
		bgA = 0x10
	}

	if bgA != 0 {
		for i := 3; i < len(o.Buf); i += 4 {
			o.Buf[i] = bgA
		}
	}

	if drawMode {
		o.DrawToolbar(o.SelectedColor)
	}

	r := StrokeRadius
	for i := 0; i < o.StrokeCount; i++ {
		idx := (o.StrokeStart + i) % len(o.Strokes)
		s := o.Strokes[idx]
		o.drawStrokeOrDot(s.Points, r, s.Color)
	}
	o.drawStrokeOrDot(o.Current, r, o.CurrentColor)

	o.Present()
}

func (o *Window) RedrawToolbarOnly() {
	if o.Hwnd == 0 || o.MemDC == 0 || o.Bits == nil {
		return
	}
	if !o.drawMode.Load() {
		return
	}
	for y := ToolbarY; y < ToolbarY+ToolbarH && y < o.H; y++ {
		for x := ToolbarX; x < ToolbarX+ToolbarW && x < o.W; x++ {
			off := (y*o.W + x) * 4
			o.Buf[off+0] = 0
			o.Buf[off+1] = 0
			o.Buf[off+2] = 0
			o.Buf[off+3] = 0x10
		}
	}
	o.DrawToolbar(o.SelectedColor)
	o.Present()
}

func (o *Window) SetPixelRaw(x, y int, c color.NRGBA) {
	if x < 0 || y < 0 || x >= o.W || y >= o.H {
		return
	}
	off := (y*o.W + x) * 4
	o.Buf[off+0] = c.B
	o.Buf[off+1] = c.G
	o.Buf[off+2] = c.R
	o.Buf[off+3] = c.A
}

func (o *Window) setPixelStroke(x, y int, c color.NRGBA) {
	if o.drawMode.Load() && x >= ToolbarX && x < ToolbarX+ToolbarW && y >= ToolbarY && y < ToolbarY+ToolbarH {
		return
	}
	o.SetPixelRaw(x, y, c)
}

func (o *Window) FillRect(x, y, w, h int, c color.NRGBA) {
	for yy := range h {
		for xx := range w {
			o.SetPixelRaw(x+xx, y+yy, c)
		}
	}
}

func (o *Window) DrawRectBorder(x, y, w, h, thickness int, c color.NRGBA) {
	for t := range thickness {
		for xx := range w {
			o.SetPixelRaw(x+xx, y+t, c)
			o.SetPixelRaw(x+xx, y+h-1-t, c)
		}

		for yy := range h {
			o.SetPixelRaw(x+t, y+yy, c)
			o.SetPixelRaw(x+w-1-t, y+yy, c)
		}
	}
}

func (o *Window) DrawX(x, y, size int, c color.NRGBA) {
	for i := range size {
		o.SetPixelRaw(x+i, y+i, c)
		o.SetPixelRaw(x+(size-1-i), y+i, c)
	}
}

func (o *Window) drawStrokeOrDot(points []Point, radius int, c color.NRGBA) {
	if len(points) == 0 {
		return
	}

	if len(points) == 1 {
		o.DrawCircle(int(points[0].X), int(points[0].Y), radius, c)
		return
	}

	o.drawStroke(points, radius, c)
}

func (o *Window) drawStroke(points []Point, radius int, c color.NRGBA) {
	if len(points) < 2 {
		return
	}
	for i := 1; i < len(points); i++ {
		o.DrawLine(points[i-1], points[i], radius, c)
	}
}

func (o *Window) DrawLine(a, b Point, radius int, c color.NRGBA) {
	dx := float64(b.X - a.X)
	dy := float64(b.Y - a.Y)
	steps := int(math.Max(math.Abs(dx), math.Abs(dy)))

	if steps < 1 {
		steps = 1
	}

	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		x := int(math.Round(float64(a.X) + dx*t))
		y := int(math.Round(float64(a.Y) + dy*t))
		o.DrawCircle(x, y, radius, c)
	}
}

func (o *Window) DrawCircle(cx, cy, r int, c color.NRGBA) {
	_ = r
	for _, off := range CircleOffsets {
		o.setPixelStroke(cx+int(off.X), cy+int(off.Y), c)
	}
}
