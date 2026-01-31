package overlay

import (
	"image/color"
	"sync/atomic"

	"golang.org/x/sys/windows"
)

const WindowTitle = "Stream Guy Overlay"

var DefaultStrokeColor = color.NRGBA{R: 0xAC, G: 0x47, B: 0x38, A: 0xFF}

var Palette = []color.NRGBA{
	DefaultStrokeColor,                   // rust red
	{R: 0xCC, G: 0x84, B: 0x00, A: 0xFF}, // amber orange
	{R: 0x4A, G: 0x5D, B: 0x23, A: 0xFF}, // dark olive
	{R: 0x2A, G: 0x9D, B: 0x8F, A: 0xFF}, // teal
	{R: 0x00, G: 0x78, B: 0x99, A: 0xFF}, // deep cyan
	{R: 0x94, G: 0x21, B: 0x6A, A: 0xFF}, // magenta
	{R: 0x96, G: 0x2C, B: 0xEA, A: 0xFF}, // purple
}

const (
	StrokeRadius = 3
	MaxStrokes   = 200
	MaxPoints    = 4000
	MinPointDist = 2

	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000
	wsExTopmost     = 0x00000008

	wsPopup = 0x80000000

	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	ulwAlpha = 0x00000002

	acSrcOver  = 0x00
	acSrcAlpha = 0x01

	WmDestroy = 0x0002
	WmClose   = 0x0010

	WmApp               = 0x8000
	WmOverlayModeChange = WmApp + 1
	WmOverlayRedraw     = WmApp + 2

	WmSetCursor = 0x0020
	WmNCHitTest = 0x0084

	WmMouseMove   = 0x0200
	WmLButtonDown = 0x0201
	WmLButtonUp   = 0x0202
	WmRButtonDown = 0x0204
	WmMButtonDown = 0x0207
	WmMouseWheel  = 0x020A
	HtTransparent = ^uintptr(0)

	VkShift  = 0x10
	idcCross = 32515

	ToolbarX       = 10
	ToolbarY       = 10
	ToolbarPadding = 6
	ToolbarButton  = 28
	ToolbarGap     = 6
)

var (
	ToolbarBtnCount = 1 + len(Palette)
	ToolbarW        = ToolbarPadding*2 + ToolbarBtnCount*ToolbarButton + (ToolbarBtnCount-1)*ToolbarGap
	ToolbarH        = ToolbarPadding*2 + ToolbarButton
)

type Point struct {
	X int32
	Y int32
}

var CircleOffsets = func() []Point {
	offsets := make([]Point, 0, (StrokeRadius*2+1)*(StrokeRadius*2+1))
	for y := -StrokeRadius; y <= StrokeRadius; y++ {
		for x := -StrokeRadius; x <= StrokeRadius; x++ {
			if x*x+y*y > StrokeRadius*StrokeRadius {
				continue
			}
			offsets = append(offsets, Point{X: int32(x), Y: int32(y)})
		}
	}
	return offsets
}()

type Stroke struct {
	Points []Point
	Color  color.NRGBA
}

type Size struct {
	Cx int32
	Cy int32
}

type Msg struct {
	Hwnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      Point
}

type WndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type BlendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type BitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type BitmapInfo struct {
	Header BitmapInfoHeader
	Colors [1]uint32
}

type Window struct {
	Hwnd windows.HWND

	drawMode atomic.Bool

	pendingModeChange atomic.Bool
	pendingRedraw     atomic.Bool
	pendingClear      atomic.Bool

	Strokes      []Stroke
	StrokeStart  int
	StrokeCount  int
	Current      []Point
	CurrentColor color.NRGBA

	SelectedColor int
	Drawing       bool
	LastPoint     Point

	W int
	H int

	MemDC  windows.Handle
	Bitmap windows.Handle
	Bits   *byte
	Buf    []byte
}
