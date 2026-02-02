package overlay

import (
	"context"
	"time"
)

func IsShiftPressed() bool {
	ret, _, _ := procGetAsyncKeyState.Call(VkShift)
	return ret&0x8000 != 0
}

func IsControlPressed() bool {
	ret, _, _ := procGetAsyncKeyState.Call(VkControl)
	return ret&0x8000 != 0
}

func (o *Window) MonitorDrawModeHotkey(ctx context.Context) {
	wasPressed := false

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pressed := IsControlPressed() && IsShiftPressed()
			if pressed && !wasPressed {
				o.EnableDrawMode()
			}
			wasPressed = pressed
		}
	}
}
