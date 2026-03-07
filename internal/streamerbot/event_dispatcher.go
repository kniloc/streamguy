package streamerbot

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Ordering guarantees:
//   - FIFO within chat messages.
//   - FIFO within reward redemptions.
//   - No ordering guarantee between chat and rewards.
//
// Concurrency guarantee:
//   - EventHandler methods may be invoked concurrently across event types.

const (
	chatQueueSize   = 256
	rewardQueueSize = 32

	depthLogThreshold = 0.80
)

type chatEvent struct {
	data      json.RawMessage
	timestamp string
	enqueued  time.Time
}

type rewardEvent struct {
	data     json.RawMessage
	enqueued time.Time
}

type dispatcherStats struct {
	chatEnqueued    atomic.Int64
	chatProcessed   atomic.Int64
	chatDropped     atomic.Int64
	rewardEnqueued  atomic.Int64
	rewardProcessed atomic.Int64
	rewardDropped   atomic.Int64
}

type eventDispatcher struct {
	handler EventHandler

	chatCh   chan chatEvent
	rewardCh chan rewardEvent

	done chan struct{}
	wg   sync.WaitGroup

	stats dispatcherStats

	closeOnce sync.Once
}

func newEventDispatcher(handler EventHandler) *eventDispatcher {
	d := &eventDispatcher{
		handler:  handler,
		chatCh:   make(chan chatEvent, chatQueueSize),
		rewardCh: make(chan rewardEvent, rewardQueueSize),
		done:     make(chan struct{}),
	}

	d.wg.Add(2)
	go d.chatWorker()
	go d.rewardWorker()

	return d
}

func (d *eventDispatcher) chatWorker() {
	defer d.wg.Done()
	for {
		select {
		case <-d.done:
			d.drainChat()
			return
		case ev := <-d.chatCh:
			d.handleChat(ev)
		}
	}
}

func (d *eventDispatcher) drainChat() {
	for {
		select {
		case ev := <-d.chatCh:
			d.handleChat(ev)
		default:
			return
		}
	}
}

func (d *eventDispatcher) handleChat(ev chatEvent) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("streamerbot dispatcher: recovered panic in chat worker: %v", r)
		}
	}()

	wait := time.Since(ev.enqueued)
	if wait > 500*time.Millisecond {
		log.Printf("Chat event queued for %v before processing", wait)
	}

	d.handler.HandleChatMessage(ev.data, ev.timestamp)
	d.stats.chatProcessed.Add(1)
}

func (d *eventDispatcher) rewardWorker() {
	defer d.wg.Done()
	for {
		select {
		case <-d.done:
			d.drainRewards()
			return
		case ev := <-d.rewardCh:
			d.handleReward(ev)
		}
	}
}

func (d *eventDispatcher) drainRewards() {
	for {
		select {
		case ev := <-d.rewardCh:
			d.handleReward(ev)
		default:
			return
		}
	}
}

func (d *eventDispatcher) handleReward(ev rewardEvent) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("streamerbot dispatcher: recovered panic in reward worker: %v", r)
		}
	}()

	wait := time.Since(ev.enqueued)
	if wait > 500*time.Millisecond {
		log.Printf("Reward event queued for %v before processing", wait)
	}

	d.handler.HandleRewardRedemption(ev.data)
	d.stats.rewardProcessed.Add(1)
}

func (d *eventDispatcher) enqueueChat(data json.RawMessage, timestamp string) {
	ev := chatEvent{
		data:      data,
		timestamp: timestamp,
		enqueued:  time.Now(),
	}

	depth := len(d.chatCh)
	if float64(depth) >= float64(chatQueueSize)*depthLogThreshold {
		log.Printf("Chat queue depth high: %d/%d", depth, chatQueueSize)
	}

	select {
	case d.chatCh <- ev:
		d.stats.chatEnqueued.Add(1)
	default:
		d.stats.chatDropped.Add(1)
		dropped := d.stats.chatDropped.Load()
		if dropped == 1 || dropped%10 == 0 {
			log.Printf("Warning: Chat queue full (%d/%d), dropped %d total", chatQueueSize, chatQueueSize, dropped)
		}
	}
}

func (d *eventDispatcher) enqueueReward(data json.RawMessage) {
	ev := rewardEvent{
		data:     data,
		enqueued: time.Now(),
	}

	// Rewards are high-value, low-volume: allow a brief wait before dropping.
	select {
	case d.rewardCh <- ev:
		d.stats.rewardEnqueued.Add(1)
		return
	default:
	}

	// Short bounded wait before giving up.
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case d.rewardCh <- ev:
		d.stats.rewardEnqueued.Add(1)
	case <-timer.C:
		d.stats.rewardDropped.Add(1)
		log.Printf("Error: Reward queue full (%d/%d), dropped reward event (total dropped: %d)",
			rewardQueueSize, rewardQueueSize, d.stats.rewardDropped.Load())
	case <-d.done:
	}
}

// shutdown stops the dispatcher, drains remaining events, and waits for
// workers to finish up to the given context deadline.
func (d *eventDispatcher) shutdown(ctx context.Context) {
	d.closeOnce.Do(func() {
		close(d.done)
	})

	ch := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(ch)
	}()

	select {
	case <-ch:
	case <-ctx.Done():
		log.Printf("Dispatcher shutdown timed out; chat processed=%d dropped=%d, rewards processed=%d dropped=%d",
			d.stats.chatProcessed.Load(), d.stats.chatDropped.Load(),
			d.stats.rewardProcessed.Load(), d.stats.rewardDropped.Load())
	}
}
