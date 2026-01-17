package overlay

import (
	"errors"
	"runtime"
	"unsafe"

	"stream-guy/internal/window"

	"golang.org/x/sys/windows"
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	gdi32                = windows.NewLazySystemDLL("gdi32.dll")
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procLoadCursorW      = user32.NewProc("LoadCursorW")
	procSetCursor        = user32.NewProc("SetCursor")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procPostMessageW     = user32.NewProc("PostMessageW")

	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
)

func New() *Window {
	return &Window{
		CurrentColor: DefaultStrokeColor,
		Strokes:      make([]Stroke, MaxStrokes),
	}
}

func (o *Window) DrawMode() bool { return o.drawMode.Load() }

func (o *Window) RequestRedraw() {
	if o.Hwnd == 0 {
		return
	}
	if o.pendingRedraw.CompareAndSwap(false, true) {
		procPostMessageW.Call(uintptr(o.Hwnd), WmOverlayRedraw, 0, 0)
	}
}

func (o *Window) ToggleDrawMode() {
	next := !o.drawMode.Load()
	o.drawMode.Store(next)
	if o.Hwnd == 0 {
		return
	}
	if o.pendingModeChange.CompareAndSwap(false, true) {
		procPostMessageW.Call(uintptr(o.Hwnd), WmOverlayModeChange, 0, 0)
	}
}

func (o *Window) Clear() {
	o.pendingClear.Store(true)
	o.RequestRedraw()
}

func (o *Window) Close() {
	if o.Hwnd != 0 {
		procPostMessageW.Call(uintptr(o.Hwnd), WmClose, 0, 0)
	}
}

func (o *Window) Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	x, y, w, h := window.GetPrimaryMonitorArea()
	o.W = w
	o.H = h

	className, _ := windows.UTF16PtrFromString("PopupGuyOverlayClass")
	title, _ := windows.UTF16PtrFromString(WindowTitle)

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	if hInstance == 0 {
		return windows.ERROR_INVALID_HANDLE
	}

	cursor, _, _ := procLoadCursorW.Call(0, uintptr(idcCross))
	Cursor = windows.Handle(cursor)

	wc := WndClassEx{
		CbSize:        uint32(unsafe.Sizeof(WndClassEx{})),
		LpfnWndProc:   WndProc,
		HInstance:     windows.Handle(hInstance),
		HCursor:       Cursor,
		LpszClassName: className,
	}

	ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if ret == 0 && !errors.Is(err, windows.ERROR_CLASS_ALREADY_EXISTS) {
		return err
	}

	exStyle := uint32(wsExLayered | wsExToolWindow | wsExNoActivate | wsExTopmost)
	if !o.drawMode.Load() {
		exStyle |= wsExTransparent
	}
	style := uint32(wsPopup)

	hwndRaw, _, err := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		uintptr(style),
		uintptr(int32(x)), uintptr(int32(y)),
		uintptr(int32(w)), uintptr(int32(h)),
		0, 0, hInstance, 0,
	)

	if hwndRaw == 0 {
		return err
	}
	o.Hwnd = windows.HWND(hwndRaw)

	GlobalMu.Lock()
	ByHwnd[o.Hwnd] = o
	GlobalMu.Unlock()

	window.ProcSetWindowPos.Call(
		uintptr(o.Hwnd),
		^uintptr(0),
		uintptr(int32(x)), uintptr(int32(y)),
		uintptr(int32(w)), uintptr(int32(h)),
		uintptr(swpNoActivate|swpShowWindow),
	)

	o.ApplyClickThrough(!o.drawMode.Load())

	if err := o.initDIB(); err != nil {
		return err
	}

	o.Redraw()

	var m Msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	o.cleanup()

	GlobalMu.Lock()
	delete(ByHwnd, o.Hwnd)
	GlobalMu.Unlock()

	return nil
}

func (o *Window) ApplyClickThrough(enabled bool) {
	if o.Hwnd == 0 {
		return
	}

	ex, _, _ := window.ProcGetWindowLongPtr.Call(uintptr(o.Hwnd), ^uintptr(19))
	if enabled {
		ex |= wsExTransparent
	} else {
		ex &^= wsExTransparent
	}
	window.ProcSetWindowLongPtr.Call(uintptr(o.Hwnd), ^uintptr(19), ex)
	window.ProcSetWindowPos.Call(
		uintptr(o.Hwnd),
		0,
		0, 0, 0, 0,
		uintptr(0x0002|0x0001|0x0004|0x0020|swpNoActivate),
	)
}

func (o *Window) initDIB() error {
	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return windows.ERROR_INVALID_HANDLE
	}
	defer procReleaseDC.Call(0, hdcScreen)

	hdcMem, _, _ := procCreateCompatibleDC.Call(hdcScreen)
	if hdcMem == 0 {
		return windows.ERROR_INVALID_HANDLE
	}
	o.MemDC = windows.Handle(hdcMem)

	bi := BitmapInfo{}
	bi.Header.Size = uint32(unsafe.Sizeof(BitmapInfoHeader{}))
	bi.Header.Width = int32(o.W)
	bi.Header.Height = -int32(o.H)
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = 0

	var bits *byte
	hbmp, _, err := procCreateDIBSection.Call(
		uintptr(o.MemDC),
		uintptr(unsafe.Pointer(&bi)),
		0,
		uintptr(unsafe.Pointer(&bits)),
		0,
		0,
	)
	if hbmp == 0 {
		return err
	}
	o.Bitmap = windows.Handle(hbmp)
	o.Bits = bits

	procSelectObject.Call(uintptr(o.MemDC), uintptr(o.Bitmap))

	o.Buf = unsafe.Slice(o.Bits, o.W*o.H*4)

	return nil
}

func (o *Window) cleanup() {
	if o.MemDC != 0 {
		procDeleteDC.Call(uintptr(o.MemDC))
		o.MemDC = 0
	}
	if o.Bitmap != 0 {
		procDeleteObject.Call(uintptr(o.Bitmap))
		o.Bitmap = 0
	}
}

func (o *Window) Present() {
	if o.Hwnd == 0 || o.MemDC == 0 || o.Bits == nil {
		return
	}

	hdcScreen, _, _ := procGetDC.Call(0)
	if hdcScreen == 0 {
		return
	}
	defer procReleaseDC.Call(0, hdcScreen)

	sz := Size{int32(o.W), int32(o.H)}
	src := Point{0, 0}
	blend := BlendFunction{BlendOp: acSrcOver, BlendFlags: 0, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}

	procUpdateLayeredWindow.Call(
		uintptr(o.Hwnd),
		hdcScreen,
		0,
		uintptr(unsafe.Pointer(&sz)),
		uintptr(o.MemDC),
		uintptr(unsafe.Pointer(&src)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		ulwAlpha,
	)
}
