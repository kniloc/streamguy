package overlay

import (
	"sync"
	"unsafe"

	"stream-guy/internal/window"

	"golang.org/x/sys/windows"
)

var WndProc = windows.NewCallback(wndProcFn)

var GlobalMu sync.RWMutex
var ByHwnd = make(map[windows.HWND]*Window)
var Cursor windows.Handle

const ControlPanelTitle = "Control Panel"

var (
	controlPanelHwnd   windows.HWND
	controlPanelHwndMu sync.RWMutex
)

func SetControlPanelHwnd(hwnd windows.HWND) {
	controlPanelHwndMu.Lock()
	controlPanelHwnd = hwnd
	controlPanelHwndMu.Unlock()
}

func getControlPanelHwnd() windows.HWND {
	controlPanelHwndMu.RLock()
	defer controlPanelHwndMu.RUnlock()
	return controlPanelHwnd
}

func wndProcFn(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	GlobalMu.RLock()
	ow := ByHwnd[windows.HWND(hwnd)]
	GlobalMu.RUnlock()

	switch msg {
	case WmOverlayModeChange:
		if ow != nil {
			ow.pendingModeChange.Store(false)
			drawMode := ow.drawMode.Load()
			if !drawMode {
				ow.ResetCurrent()
				ow.Drawing = false
			}
			ow.ApplyClickThrough(!drawMode)
			ow.Redraw()
		}
		return 0
	case WmOverlayRedraw:
		if ow != nil {
			ow.pendingRedraw.Store(false)
			if ow.pendingClear.Swap(false) {
				ow.ClearStrokes()
			}
			ow.Redraw()
		}
		return 0
	case WmSetCursor:
		if Cursor != 0 {
			procSetCursor.Call(uintptr(Cursor))
			return 1
		}
		return 0
	case WmNCHitTest:
		if ow != nil && ow.drawMode.Load() {
			sx := int32(int16(lParam & 0xFFFF))
			sy := int32(int16((lParam >> 16) & 0xFFFF))

			var wr window.RECT
			ret, _, _ := window.ProcGetWindowRect.Call(uintptr(ow.Hwnd), uintptr(unsafe.Pointer(&wr)))
			if ret != 0 {
				cx := int(sx - wr.Left)
				cy := int(sy - wr.Top)
				if ow.IsToolbarPixel(cx, cy) {
					return 1
				}
			}

			cp := getControlPanelHwnd()
			if cp != 0 {
				var r window.RECT
				ret, _, _ := window.ProcGetWindowRect.Call(uintptr(cp), uintptr(unsafe.Pointer(&r)))
				if ret != 0 && sx >= r.Left && sx < r.Right && sy >= r.Top && sy < r.Bottom {
					return HtTransparent
				}
			}
		}
	case WmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	case WmClose:
		procPostQuitMessage.Call(0)
		return 0
	case WmLButtonDown:
		if ow != nil && ow.drawMode.Load() {
			p := Point{int32(int16(lParam & 0xFFFF)), int32(int16((lParam >> 16) & 0xFFFF))}
			if ow.HandleToolbarClick(p) {
				return 0
			}

			ow.Drawing = true
			if cap(ow.Current) > 0 {
				ow.Current = ow.Current[:0]
				ow.Current = append(ow.Current, p)
			} else {
				ow.Current = []Point{p}
			}
			ow.LastPoint = p
			c := ow.CurrentColor

			r := StrokeRadius
			ow.DrawCircle(int(p.X), int(p.Y), r, c)
			ow.Present()
		}
		return 0
	case WmMouseMove:
		if ow != nil && ow.drawMode.Load() {
			if (wParam & 0x0001) != 0 {
				p := Point{int32(int16(lParam & 0xFFFF)), int32(int16((lParam >> 16) & 0xFFFF))}

				if !ow.Drawing {
					return 0
				}
				last := ow.LastPoint
				ow.LastPoint = p
				ow.StoreCurrentPoint(p)
				c := ow.CurrentColor

				r := StrokeRadius
				ow.DrawLine(last, p, r, c)
				ow.Present()
			}
		}
		return 0
	case WmRButtonDown:
		if ow != nil && ow.drawMode.Load() {
			ow.ClearAndRedraw()
		}
		return 0
	case WmLButtonUp:
		if ow != nil && ow.drawMode.Load() {
			p := Point{int32(int16(lParam & 0xFFFF)), int32(int16((lParam >> 16) & 0xFFFF))}

			if !ow.Drawing {
				return 0
			}
			last := ow.LastPoint
			ow.LastPoint = p
			ow.StoreCurrentPoint(p)
			pts := make([]Point, len(ow.Current))
			copy(pts, ow.Current)
			c := ow.CurrentColor
			ow.AppendStroke(Stroke{Points: pts, Color: c})
			ow.ResetCurrent()
			ow.Drawing = false

			r := StrokeRadius
			ow.DrawLine(last, p, r, c)
			ow.Present()
		}
		return 0
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return ret
}
