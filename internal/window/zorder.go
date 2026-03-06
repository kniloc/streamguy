package window

import (
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

const (
	zorderDebounce     = 15 * time.Millisecond
	zorderReassertReps = 5
	zorderReassertWait = 30 * time.Millisecond
)

// ZOrderManager serialises all HWND_TOPMOST calls through a single
// goroutine so that concurrent popup creation never races on z-order.
type ZOrderManager struct {
	ch   chan windows.HWND
	once sync.Once
}

// NewZOrderManager creates a manager whose background goroutine runs until
// the channel is closed (see Stop).
func NewZOrderManager() *ZOrderManager {
	return &ZOrderManager{
		ch: make(chan windows.HWND, 32),
	}
}

// RequestTopmost enqueues hwnd to be set as the topmost window.
// The manager debounces rapid-fire requests so only the most recent
// HWND actually gets the topmost call, then re-asserts it a few times
// to survive Win32 activation changes.
func (m *ZOrderManager) RequestTopmost(hwnd windows.HWND) {
	if hwnd == 0 {
		return
	}
	m.once.Do(func() { go m.run() })
	select {
	case m.ch <- hwnd:
	default:
		// Channel full — drop oldest, push new.
		select {
		case <-m.ch:
		default:
		}
		m.ch <- hwnd
	}
}

// Stop shuts down the background goroutine.
func (m *ZOrderManager) Stop() {
	close(m.ch)
}

func (m *ZOrderManager) run() {
	for hwnd := range m.ch {
		// Drain any queued requests that arrived in the meantime so we
		// only act on the very latest HWND.
		latest := hwnd
		time.Sleep(zorderDebounce)
	drain:
		for {
			select {
			case h, ok := <-m.ch:
				if !ok {
					return
				}
				latest = h
			default:
				break drain
			}
		}

		// Set topmost, then re-assert a few times to survive focus
		// changes from the window manager.
		for i := range zorderReassertReps {
			if !IsWindowValid(latest) {
				break
			}
			SetWindowTopmost(latest)
			if i < zorderReassertReps-1 {
				time.Sleep(zorderReassertWait)
			}
		}
	}
}
