package window

import (
	"log"
	"math/rand"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	MinWindowSpacing        = 10
	PositionFindMaxAttempts = 75
	MaxWindowsOnScreen      = 20
)

var (
	User32                   = windows.NewLazySystemDLL("user32.dll")
	ProcSetWindowPos         = User32.NewProc("SetWindowPos")
	ProcGetWindowRect        = User32.NewProc("GetWindowRect")
	ProcEnumWindows          = User32.NewProc("EnumWindows")
	ProcGetWindowText        = User32.NewProc("GetWindowTextW")
	ProcSystemParametersInfo = User32.NewProc("SystemParametersInfoW")
	ProcGetWindowLongPtr     = User32.NewProc("GetWindowLongPtrW")
	ProcSetWindowLongPtr     = User32.NewProc("SetWindowLongPtrW")
	ProcGetDesktopWindow     = User32.NewProc("GetDesktopWindow")
	ProcMonitorFromWindow    = User32.NewProc("MonitorFromWindow")
	ProcGetMonitorInfo       = User32.NewProc("GetMonitorInfoW")
	ProcIsWindow             = User32.NewProc("IsWindow")
	ProcCreateWindowEx       = User32.NewProc("CreateWindowExW")
	ProcRegisterClass        = User32.NewProc("RegisterClassW")
	ProcDefWindowProc        = User32.NewProc("DefWindowProcW")
	ProcShowWindow           = User32.NewProc("ShowWindow")

	Kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	ProcGetModuleHandle = Kernel32.NewProc("GetModuleHandleW")
)

var (
	popupOwnerHWND     windows.HWND
	popupOwnerInitOnce sync.Once
)

const (
	SwpNoSize       = 0x0001
	SwpNoMove       = 0x0002
	SwpNoZOrder     = 0x0004
	SwpNoActivate   = 0x0010
	SwpFrameChanged = 0x0020
	SpiGetWorkArea  = 0x0030
	SwpShowWindow   = 0x0040

	MonitorDefaultToPrimary = 0x00000001

	SwShowNoActivate = 4
)

const (
	GwlpHwndParent = ^uintptr(7)
	GwlExstyle     = ^uintptr(19)

	WsExToolWindow = 0x00000080
	WsExAppWindow  = 0x00040000

	HwndMessageOnly = ^uintptr(2) // HWND_MESSAGE (-3)
	WsPopup         = 0x80000000
)

type WNDCLASS struct {
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
}

func getPopupOwnerHWND() windows.HWND {
	popupOwnerInitOnce.Do(func() {
		className, _ := windows.UTF16PtrFromString("StreamGuyPopupOwner")

		hInstance, _, _ := ProcGetModuleHandle.Call(0)

		wc := WNDCLASS{
			LpfnWndProc:   ProcDefWindowProc.Addr(),
			HInstance:     hInstance,
			LpszClassName: className,
		}

		ProcRegisterClass.Call(uintptr(unsafe.Pointer(&wc)))

		hwnd, _, _ := ProcCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			0,
			uintptr(WsPopup),
			0, 0, 0, 0,
			HwndMessageOnly,
			0,
			hInstance,
			0,
		)
		popupOwnerHWND = windows.HWND(hwnd)
	})
	return popupOwnerHWND
}

type RECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type MonitorInfo struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type Bounds struct {
	X      int
	Y      int
	Width  int
	Height int
}

type GridCell struct {
	Occupied bool
	WindowID int
}

type PlacementManager struct {
	grid         [][]GridCell
	gridWidth    int
	gridHeight   int
	cellSize     int
	workAreaX    int
	workAreaY    int
	screenWidth  int
	screenHeight int
	nextWindowID int
	windowRects  map[int]Bounds
	Rng          *rand.Rand
	mu           sync.Mutex
	initialized  bool
}

type HandleCache struct {
	handles map[string]windows.HWND
	mu      sync.RWMutex
}

var GlobalHandleCache = &HandleCache{
	handles: make(map[string]windows.HWND),
}

func GetRect(hwnd windows.HWND) (RECT, bool) {
	var rect RECT
	ret, _, _ := ProcGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	return rect, ret != 0
}

func ClampWindowToWorkArea(hwnd windows.HWND) {
	if hwnd == 0 {
		return
	}

	rect, ok := GetRect(hwnd)
	if !ok {
		return
	}

	workX, workY, workW, workH := GetDesktopArea()
	workLeft := int32(workX)
	workTop := int32(workY)
	workRight := int32(workX + workW)
	workBottom := int32(workY + workH)

	width := rect.Right - rect.Left
	height := rect.Bottom - rect.Top

	newLeft := rect.Left
	newTop := rect.Top

	if rect.Right > workRight {
		newLeft = workRight - width
	}
	if rect.Left < workLeft {
		newLeft = workLeft
	}
	if rect.Bottom > workBottom {
		newTop = workBottom - height
	}
	if rect.Top < workTop {
		newTop = workTop
	}

	if newLeft == rect.Left && newTop == rect.Top {
		return
	}

	ProcSetWindowPos.Call(
		uintptr(hwnd),
		0,
		uintptr(newLeft), uintptr(newTop), 0, 0,
		uintptr(SwpNoSize|SwpNoZOrder|SwpNoActivate|SwpShowWindow),
	)
}

func (wc *HandleCache) Get(title string) (windows.HWND, bool) {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	hwnd, exists := wc.handles[title]
	return hwnd, exists
}

func (wc *HandleCache) Set(title string, hwnd windows.HWND) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	wc.handles[title] = hwnd
}

func (wc *HandleCache) Remove(title string) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	delete(wc.handles, title)
}

func GetDesktopArea() (x, y, width, height int) {
	var rect RECT
	ret, _, _ := ProcSystemParametersInfo.Call(SpiGetWorkArea, 0, uintptr(unsafe.Pointer(&rect)), 0)
	if ret == 0 {
		return 0, 0, 1920, 1080
	}
	return int(rect.Left), int(rect.Top), int(rect.Right - rect.Left), int(rect.Bottom - rect.Top)
}

func GetPrimaryMonitorArea() (x, y, width, height int) {
	desktop, _, _ := ProcGetDesktopWindow.Call()
	if desktop == 0 {
		return GetDesktopArea()
	}

	mon, _, _ := ProcMonitorFromWindow.Call(desktop, MonitorDefaultToPrimary)
	if mon == 0 {
		return GetDesktopArea()
	}

	mi := MonitorInfo{CbSize: uint32(unsafe.Sizeof(MonitorInfo{}))}
	ret, _, _ := ProcGetMonitorInfo.Call(mon, uintptr(unsafe.Pointer(&mi)))
	if ret == 0 {
		return GetDesktopArea()
	}

	r := mi.RcMonitor
	return int(r.Left), int(r.Top), int(r.Right - r.Left), int(r.Bottom - r.Top)
}

func NewPlacementManager() *PlacementManager {
	return &PlacementManager{
		cellSize:    50,
		windowRects: make(map[int]Bounds),
		Rng:         rand.New(rand.NewSource(1)),
	}
}

func (pm *PlacementManager) initializeGrid() {
	if pm.initialized {
		return
	}

	pm.workAreaX, pm.workAreaY, pm.screenWidth, pm.screenHeight = GetDesktopArea()
	pm.gridWidth = (pm.screenWidth + pm.cellSize - 1) / pm.cellSize
	pm.gridHeight = (pm.screenHeight + pm.cellSize - 1) / pm.cellSize

	pm.grid = make([][]GridCell, pm.gridHeight)
	for i := range pm.grid {
		pm.grid[i] = make([]GridCell, pm.gridWidth)
	}

	pm.initialized = true
}

func (pm *PlacementManager) markCellsOccupied(rect Bounds, windowID int) {
	startCol := (rect.X - pm.workAreaX) / pm.cellSize
	endCol := (rect.X - pm.workAreaX + rect.Width + pm.cellSize - 1) / pm.cellSize
	startRow := (rect.Y - pm.workAreaY) / pm.cellSize
	endRow := (rect.Y - pm.workAreaY + rect.Height + pm.cellSize - 1) / pm.cellSize

	if startCol < 0 {
		startCol = 0
	}
	if endCol > pm.gridWidth {
		endCol = pm.gridWidth
	}
	if startRow < 0 {
		startRow = 0
	}
	if endRow > pm.gridHeight {
		endRow = pm.gridHeight
	}

	for row := startRow; row < endRow; row++ {
		for col := startCol; col < endCol; col++ {
			pm.grid[row][col].Occupied = true
			pm.grid[row][col].WindowID = windowID
		}
	}
}

func (pm *PlacementManager) markCellsFree(windowID int) {
	for row := range pm.grid {
		for col := range pm.grid[row] {
			if pm.grid[row][col].WindowID == windowID {
				pm.grid[row][col].Occupied = false
				pm.grid[row][col].WindowID = 0
			}
		}
	}
}

func (pm *PlacementManager) canPlaceWindowAtPosition(x, y, width, height int) bool {
	startCol := (x - pm.workAreaX) / pm.cellSize
	endCol := (x - pm.workAreaX + width + pm.cellSize - 1) / pm.cellSize
	startRow := (y - pm.workAreaY) / pm.cellSize
	endRow := (y - pm.workAreaY + height + pm.cellSize - 1) / pm.cellSize

	spacingCells := (MinWindowSpacing + pm.cellSize - 1) / pm.cellSize
	startCol -= spacingCells
	endCol += spacingCells
	startRow -= spacingCells
	endRow += spacingCells

	if startCol < 0 {
		startCol = 0
	}
	if endCol > pm.gridWidth {
		endCol = pm.gridWidth
	}
	if startRow < 0 {
		startRow = 0
	}
	if endRow > pm.gridHeight {
		endRow = pm.gridHeight
	}

	for row := startRow; row < endRow; row++ {
		for col := startCol; col < endCol; col++ {
			if pm.grid[row][col].Occupied {
				return false
			}
		}
	}

	return true
}

func (pm *PlacementManager) FindNonOverlappingPosition(windowWidth, windowHeight int) (x, y int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.initializeGrid()

	maxX := pm.screenWidth - windowWidth
	maxY := pm.screenHeight - windowHeight

	if maxX < 0 || maxY < 0 {
		x = pm.workAreaX
		y = pm.workAreaY
		goto placeWindow
	}

	if pm.Rng == nil {
		pm.Rng = rand.New(rand.NewSource(1))
	}

	for range PositionFindMaxAttempts {
		x = pm.workAreaX + pm.Rng.Intn(maxX+1)
		y = pm.workAreaY + pm.Rng.Intn(maxY+1)

		if pm.canPlaceWindowAtPosition(x, y, windowWidth, windowHeight) {
			goto placeWindow
		}
	}

	x = pm.workAreaX + pm.Rng.Intn(maxX+1)
	y = pm.workAreaY + pm.Rng.Intn(maxY+1)

placeWindow:
	pm.nextWindowID++
	windowID := pm.nextWindowID

	rect := Bounds{
		X:      x,
		Y:      y,
		Width:  windowWidth,
		Height: windowHeight,
	}

	pm.windowRects[windowID] = rect
	pm.markCellsOccupied(rect, windowID)

	return x, y
}

func (pm *PlacementManager) RemoveWindow(x, y int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for windowID, rect := range pm.windowRects {
		if rect.X == x && rect.Y == y {
			pm.markCellsFree(windowID)
			delete(pm.windowRects, windowID)
			return
		}
	}
}

func (pm *PlacementManager) ClearOldWindows(maxWindows int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.windowRects) <= maxWindows {
		return
	}

	toRemove := len(pm.windowRects) - maxWindows

	windowIDs := make([]int, 0, len(pm.windowRects))
	for windowID := range pm.windowRects {
		windowIDs = append(windowIDs, windowID)
	}

	for i := 0; i < toRemove && i < len(windowIDs); i++ {
		minIdx := i
		for j := i + 1; j < len(windowIDs); j++ {
			if windowIDs[j] < windowIDs[minIdx] {
				minIdx = j
			}
		}
		windowIDs[i], windowIDs[minIdx] = windowIDs[minIdx], windowIDs[i]
	}

	for i := 0; i < toRemove && i < len(windowIDs); i++ {
		windowID := windowIDs[i]
		pm.markCellsFree(windowID)
		delete(pm.windowRects, windowID)
	}
}

func IsWindowValid(hwnd windows.HWND) bool {
	ret, _, _ := ProcIsWindow.Call(uintptr(hwnd))
	return ret != 0
}

type enumWindowsState struct {
	targetTitle string
	foundHwnd   windows.HWND
}

var (
	enumWindowsCallback uintptr
	enumWindowsMu       sync.Mutex
)

func init() {
	enumWindowsCallback = windows.NewCallback(func(hwnd windows.HWND, lParam uintptr) uintptr {
		state := (*enumWindowsState)(unsafe.Pointer(lParam))
		textLen := 256
		buf := make([]uint16, textLen)
		ProcGetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(textLen))
		windowTitle := windows.UTF16ToString(buf)

		if windowTitle == state.targetTitle && state.foundHwnd == 0 {
			state.foundHwnd = hwnd
			return 0
		}
		return 1
	})
}

func FindWindowByTitle(title string) windows.HWND {
	enumWindowsMu.Lock()
	defer enumWindowsMu.Unlock()

	state := &enumWindowsState{targetTitle: title}
	ProcEnumWindows.Call(enumWindowsCallback, uintptr(unsafe.Pointer(state)))
	return state.foundHwnd
}

func FindWindowByTitleCached(title string) windows.HWND {
	if hwnd, exists := GlobalHandleCache.Get(title); exists {
		if IsWindowValid(hwnd) {
			return hwnd
		}
		GlobalHandleCache.Remove(title)
	}

	hwnd := FindWindowByTitle(title)
	if hwnd != 0 {
		GlobalHandleCache.Set(title, hwnd)
	}
	return hwnd
}

func CleanupWindowHandle(title string) {
	GlobalHandleCache.Remove(title)
}

func SetPopupWindowPositionByHandle(wnd windows.HWND, x, y int) {
	if wnd == 0 {
		log.Printf("Invalid window handle\n")
		return
	}

	ret, _, err := ProcSetWindowPos.Call(
		uintptr(wnd),
		^uintptr(0),
		uintptr(x), uintptr(y), 0, 0,
		uintptr(SwpNoSize|SwpNoZOrder|SwpNoActivate|SwpShowWindow),
	)

	if ret == 0 {
		log.Printf("SetWindowPos failed: %v\n", err)
	}
}

func ConfigurePopWindow(hwnd windows.HWND) {
	if hwnd == 0 {
		return
	}

	owner := getPopupOwnerHWND()
	ProcSetWindowLongPtr.Call(uintptr(hwnd), GwlpHwndParent, uintptr(owner))

	ex, _, _ := ProcGetWindowLongPtr.Call(uintptr(hwnd), GwlExstyle)
	ex |= WsExToolWindow
	ex &^= WsExAppWindow
	ProcSetWindowLongPtr.Call(uintptr(hwnd), GwlExstyle, ex)

	ProcSetWindowPos.Call(
		uintptr(hwnd),
		^uintptr(0),
		0, 0, 0, 0,
		uintptr(SwpNoMove|SwpNoSize|SwpNoActivate|SwpFrameChanged|SwpShowWindow),
	)

	ProcShowWindow.Call(uintptr(hwnd), SwShowNoActivate)
}

func SetOverlayBoundsBelow(hwnd windows.HWND, below windows.HWND, x, y, width, height int) {
	if hwnd == 0 {
		return
	}

	ret, _, err := ProcSetWindowPos.Call(
		uintptr(hwnd),
		uintptr(below),
		uintptr(x), uintptr(y),
		uintptr(width), uintptr(height),
		uintptr(SwpNoActivate|SwpShowWindow),
	)
	if ret == 0 {
		log.Printf("SetWindowPos (overlay below) failed: %v", err)
	}
}

func SetWindowTopmost(hwnd windows.HWND) {
	if hwnd == 0 {
		return
	}
	ret, _, err := ProcSetWindowPos.Call(
		uintptr(hwnd),
		^uintptr(0),
		0, 0, 0, 0,
		uintptr(SwpNoMove|SwpNoSize|SwpNoActivate|SwpShowWindow),
	)
	if ret == 0 {
		log.Printf("SetWindowPos (topmost) failed: %v", err)
	}
}
