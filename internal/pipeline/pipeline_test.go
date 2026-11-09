package pipeline

import (
	"context"
	"fmt"
	"mitochondria/internal/core"
	"sync/atomic"
	"testing"
	"time"
)

func validProcessEvent(id string, pid uint32) core.TelemetryEvent {
	return core.TelemetryEvent{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Kind:      core.EventProcessSpawn,
		Process: &core.ProcessInfo{
			PID:     pid,
			PPID:    1,
			Name:    "test.exe",
			ExePath: `C:\Temp\test.exe`,
		},
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(16, nil); err == nil {
		t.Fatal("expected error when handler is nil")
	}

	p, err := New(0, func(_ context.Context, _ core.TelemetryEvent) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error from New(0, handler): %v", err)
	}
	_, capGot, _ := p.Stats()
	if capGot != core.DefaultEventQueueCap {
		t.Fatalf("expected default capacity %d, got %d", core.DefaultEventQueueCap, capGot)
	}
}

func TestPublishRejectsInvalidEvent(t *testing.T) {
	p, err := New(8, func(_ context.Context, _ core.TelemetryEvent) error { return nil })
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ok, err := p.Publish(core.TelemetryEvent{})
	if err == nil || ok {
		t.Fatalf("expected validation error for empty event, got ok=%v err=%v", ok, err)
	}
}

func TestBoundedBurstDropsWithoutBlocking(t *testing.T) {
	const capLimit = 64
	const burstSize = 10000

	p, err := New(capLimit, func(_ context.Context, _ core.TelemetryEvent) error { return nil })
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	enqueued := 0
	for i := 0; i < burstSize; i++ {
		ok, pubErr := p.Publish(validProcessEvent(fmt.Sprintf("evt-%d", i), uint32(i+100)))
		if pubErr != nil {
			t.Fatalf("unexpected Publish error at %d: %v", i, pubErr)
		}
		if ok {
			enqueued++
		}
	}

	depth, capacity, dropped := p.Stats()
	if enqueued != capLimit {
		t.Fatalf("expected exactly %d enqueued events, got %d", capLimit, enqueued)
	}
	if depth != capLimit {
		t.Fatalf("expected queue depth %d, got %d", capLimit, depth)
	}
	if capacity != capLimit {
		t.Fatalf("expected capacity %d, got %d", capLimit, capacity)
	}
	wantDropped := uint64(burstSize - capLimit)
	if dropped != wantDropped {
		t.Fatalf("expected %d dropped events, got %d", wantDropped, dropped)
	}
}

func TestRunDrainsEventsToMockHandler(t *testing.T) {
	var processed atomic.Int32
	p, err := New(32, func(_ context.Context, _ core.TelemetryEvent) error {
		processed.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	for i := 0; i < 10; i++ {
		ok, pubErr := p.Publish(validProcessEvent(fmt.Sprintf("evt-%d", i), uint32(i+1)))
		if !ok || pubErr != nil {
			t.Fatalf("Publish(%d) failed: ok=%v err=%v", i, ok, pubErr)
		}
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := p.Run(ctx); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}

	if got := processed.Load(); got != 10 {
		t.Fatalf("expected 10 processed events, got %d", got)
	}
}

func TestPublishAfterCloseFails(t *testing.T) {
	p, err := New(8, func(_ context.Context, _ core.TelemetryEvent) error { return nil })
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	// Second Close must be idempotent
	if err := p.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}

	ok, err := p.Publish(validProcessEvent("evt-after-close", 99))
	if err == nil || ok {
		t.Fatalf("expected error publishing after Close, got ok=%v err=%v", ok, err)
	}
}
