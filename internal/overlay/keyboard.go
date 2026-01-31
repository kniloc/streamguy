package overlay

import (
	"context"
	"time"
)

func IsShiftPressed() bool {
	ret, _, _ := procGetAsyncKeyState.Call(VkShift)
	return ret&0x8000 != 0
}

func (o *Window) MonitorDoubleShift(ctx context.Context) {
	const maxInterval = 400 * time.Millisecond
	var lastShiftRelease time.Time
	wasPressed := false

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pressed := IsShiftPressed()
			if wasPressed && !pressed {
				now := time.Now()
				if now.Sub(lastShiftRelease) < maxInterval {
					o.EnableDrawMode()
					lastShiftRelease = time.Time{}
				} else {
					lastShiftRelease = now
				}
			}
			wasPressed = pressed
		}
	}
}
