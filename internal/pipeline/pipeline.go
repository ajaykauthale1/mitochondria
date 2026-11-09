package pipeline

import (
	"context"
	"mitochondria/internal/core"
)

// EventHandler processes a single normalized TelemetryEvent drained from the bounded queue.
type EventHandler func(ctx context.Context, event core.TelemetryEvent) error

// EventPipeline is a bounded, non-blocking telemetry queue that enforces backpressure
// and prevents unbounded memory growth during event bursts.
type EventPipeline struct {
	capacity int
	handler  EventHandler
}

// New constructs a bounded EventPipeline with the given capacity (defaults to DefaultEventQueueCap if <= 0).
func New(capacity int, handler EventHandler) (*EventPipeline, error) {
	return nil, core.ErrNotImplemented
}

// Publish validates and enqueues an event without blocking.
// Returns (true, nil) if enqueued, (false, nil) if dropped due to full queue, or (false, err) if invalid/closed.
func (p *EventPipeline) Publish(event core.TelemetryEvent) (bool, error) {
	return false, core.ErrNotImplemented
}

// Run drains queued events and invokes the handler until ctx is canceled or Close is called.
func (p *EventPipeline) Run(ctx context.Context) error {
	return core.ErrNotImplemented
}

// Stats returns current queue depth, maximum capacity, and total dropped events.
func (p *EventPipeline) Stats() (depth int, capacity int, dropped uint64) {
	return 0, 0, 0
}

// Close stops accepting new events and closes the underlying channel idempotently.
func (p *EventPipeline) Close() error {
	return core.ErrNotImplemented
}
