package window

import (
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
)

type Registry struct {
	windows []*app.Window
	mu      sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		windows: make([]*app.Window, 0),
	}
}

func (wr *Registry) Register(w *app.Window) {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	wr.windows = append(wr.windows, w)
}

func (wr *Registry) Unregister(w *app.Window) {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	filtered := make([]*app.Window, 0, len(wr.windows))
	for _, win := range wr.windows {
		if win != w {
			filtered = append(filtered, win)
		}
	}
	wr.windows = filtered
}

func (wr *Registry) Count() int {
	wr.mu.RLock()
	defer wr.mu.RUnlock()
	return len(wr.windows)
}

func (wr *Registry) CloseAll() {
	wr.mu.Lock()
	windows := make([]*app.Window, len(wr.windows))
	copy(windows, wr.windows)
	wr.mu.Unlock()

	for _, w := range windows {
		if w != nil {
			w.Perform(system.ActionClose)
		}
	}

	time.Sleep(100 * time.Millisecond)
}

func (wr *Registry) WithEachWindow(fn func(*app.Window)) {
	wr.mu.RLock()
	windows := make([]*app.Window, len(wr.windows))
	copy(windows, wr.windows)
	wr.mu.RUnlock()

	for _, w := range windows {
		if w != nil {
			fn(w)
		}
	}
}
