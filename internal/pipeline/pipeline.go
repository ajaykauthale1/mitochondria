package pipeline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"mitochondria/internal/core"
)

var (
	// ErrPipelineClosed is returned when Publish is called after Close or after Run exits.
	ErrPipelineClosed = errors.New("event pipeline is closed")

	// ErrAlreadyRunning is returned when Run is invoked more than once on the same EventPipeline.
	ErrAlreadyRunning = errors.New("event pipeline consumer is already running")
)

// EventHandler processes a single normalized TelemetryEvent drained from the bounded queue.
type EventHandler func(ctx context.Context, event core.TelemetryEvent) error

// EventPipeline is a bounded, non-blocking telemetry queue that enforces backpressure
// and prevents unbounded memory growth during event bursts.
type EventPipeline struct {
	capacity int
	handler  EventHandler
	queue    chan core.TelemetryEvent
	// dropped is a cumulative, monotonically increasing count of events discarded
	// due to a full queue (never reset); feeds ResourceSnapshot.DroppedEvents.
	dropped atomic.Uint64
	started atomic.Bool
	mu      sync.RWMutex
	closed  bool
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

	// Hold RLock across the channel send so Close (which acquires Lock) cannot
	// close p.queue concurrently and trigger a send-on-closed-channel panic.
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

// Run starts the single consumer loop that drains queued events and invokes the handler.
//
// Shutdown semantics:
//   - Calling Close() drains all remaining queued events to the handler before Run returns nil.
//   - Canceling ctx aborts immediately (discarding any remaining queued events) and returns ctx.Err().
//   - If handler returns a non-nil error, Run closes the pipeline and returns that error so
//     subsequent Publish calls fail with ErrPipelineClosed instead of silently dropping events.
//
// Run may only be called once per EventPipeline instance; subsequent calls return ErrAlreadyRunning.
func (p *EventPipeline) Run(ctx context.Context) error {
	if !p.started.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	defer func() {
		_ = p.Close()
	}()

	for {
		// Prioritize context cancellation so an aborted context deterministically stops
		// even when p.queue is non-empty.
		if err := ctx.Err(); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-p.queue:
			if !ok {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := p.handler(ctx, event); err != nil {
				return err
			}
		}
	}
}

// Stats returns current queue depth, maximum capacity, and the cumulative monotonic dropped-event count.
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
