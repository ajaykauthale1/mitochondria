package pipeline

import (
	"context"
	"errors"
	"mitochondria/internal/core"
	"sync"
	"sync/atomic"
)

// ErrPipelineClosed is returned when Publish is called after Close.
var ErrPipelineClosed = errors.New("event pipeline is closed")

// EventHandler processes a single normalized TelemetryEvent drained from the bounded queue.
type EventHandler func(ctx context.Context, event core.TelemetryEvent) error

// EventPipeline is a bounded, non-blocking telemetry queue that enforces backpressure
// and prevents unbounded memory growth during event bursts.
type EventPipeline struct {
	capacity int
	handler  EventHandler
	queue    chan core.TelemetryEvent
	dropped  atomic.Uint64
	mu       sync.RWMutex
	closed   bool
}

// New constructs a bounded EventPipeline with the given capacity (defaults to DefaultEventQueueCap if <= 0).
func New(capacity int, handler EventHandler) (*EventPipeline, error) {
	if handler == nil {
		return nil, errors.New("event handler is required")
	}
	if capacity <= 0 {
		capacity = core.DefaultEventQueueCap
	}
	return &EventPipeline{
		capacity: capacity,
		handler:  handler,
		queue:    make(chan core.TelemetryEvent, capacity),
	}, nil
}

// Publish validates and enqueues an event without blocking.
// Returns (true, nil) if enqueued, (false, nil) if dropped due to full queue, or (false, err) if invalid/closed.
func (p *EventPipeline) Publish(event core.TelemetryEvent) (bool, error) {
	if err := event.Validate(); err != nil {
		return false, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return false, ErrPipelineClosed
	}

	select {
	case p.queue <- event:
		return true, nil
	default:
		p.dropped.Add(1)
		return false, nil
	}
}

// Run drains queued events and invokes the handler until ctx is canceled or Close is called.
func (p *EventPipeline) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-p.queue:
			if !ok {
				return nil
			}
			if err := p.handler(ctx, event); err != nil {
				return err
			}
		}
	}
}

// Stats returns current queue depth, maximum capacity, and total dropped events.
func (p *EventPipeline) Stats() (depth int, capacity int, dropped uint64) {
	return len(p.queue), p.capacity, p.dropped.Load()
}

// Close stops accepting new events and closes the underlying channel idempotently.
func (p *EventPipeline) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		close(p.queue)
	}
	return nil
}
