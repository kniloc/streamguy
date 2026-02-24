package overlay

import (
	"context"
	"time"
)

func IsAltPressed() bool {
	ret, _, _ := procGetAsyncKeyState.Call(VkAlt)
	return ret&0x8000 != 0
}

func (o *Window) MonitorDrawModeHotkey(ctx context.Context) {
	maxInterval := 200 * time.Millisecond
	var lastKeyRelease time.Time
	wasPressed := false

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pressed := IsAltPressed()
			if wasPressed && !pressed {
				now := time.Now()
				if now.Sub(lastKeyRelease) < maxInterval {
					o.EnableDrawMode()
					lastKeyRelease = time.Time{}
				} else {
					lastKeyRelease = now
				}
			}
			wasPressed = pressed
		}
	}
}
