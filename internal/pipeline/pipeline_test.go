package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mitochondria/internal/core"
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

func TestRunBeforePublishLiveStream(t *testing.T) {
	var processed atomic.Int32
	p, err := New(32, func(_ context.Context, _ core.TelemetryEvent) error {
		processed.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- p.Run(ctx)
	}()

	for i := 0; i < 25; i++ {
		ok, pubErr := p.Publish(validProcessEvent(fmt.Sprintf("live-%d", i), uint32(i+1)))
		if !ok || pubErr != nil {
			t.Fatalf("Publish(%d) failed while Run active: ok=%v err=%v", i, ok, pubErr)
		}
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if err := <-runErrCh; err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if got := processed.Load(); got != 25 {
		t.Fatalf("expected 25 processed events, got %d", got)
	}
}

func TestRunHandlerErrorStopsAndClosesPipeline(t *testing.T) {
	wantErr := errors.New("storage write failed")
	var calls atomic.Int32

	p, err := New(16, func(_ context.Context, _ core.TelemetryEvent) error {
		if calls.Add(1) == 2 {
			return wantErr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, pubErr := p.Publish(validProcessEvent(fmt.Sprintf("evt-%d", i), uint32(i+1))); pubErr != nil {
			t.Fatalf("Publish(%d) failed: %v", i, pubErr)
		}
	}

	err = p.Run(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected Run error %v, got %v", wantErr, err)
	}

	// Once Run exits due to handler failure, Publish must fail with ErrPipelineClosed
	// rather than silently queueing and dropping events.
	ok, pubErr := p.Publish(validProcessEvent("evt-after-handler-err", 999))
	if ok || !errors.Is(pubErr, ErrPipelineClosed) {
		t.Fatalf("expected ErrPipelineClosed after handler error exit, got ok=%v err=%v", ok, pubErr)
	}
}

func TestRunContextCancellationAbortsAndCloses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var calls atomic.Int32
	p, err := New(16, func(_ context.Context, _ core.TelemetryEvent) error {
		calls.Add(1)
		cancel()
		return nil
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	for i := 0; i < 8; i++ {
		if _, pubErr := p.Publish(validProcessEvent(fmt.Sprintf("evt-%d", i), uint32(i+1))); pubErr != nil {
			t.Fatalf("Publish(%d) failed: %v", i, pubErr)
		}
	}

	err = p.Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled from Run, got %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("expected Run to abort immediately after first event canceled ctx, got %d handler calls", got)
	}

	ok, pubErr := p.Publish(validProcessEvent("evt-after-cancel", 888))
	if ok || !errors.Is(pubErr, ErrPipelineClosed) {
		t.Fatalf("expected ErrPipelineClosed after context cancellation, got ok=%v err=%v", ok, pubErr)
	}
}

func TestRunRejectsSecondInvocation(t *testing.T) {
	p, err := New(8, func(_ context.Context, _ core.TelemetryEvent) error { return nil })
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	_ = p.Close()

	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("first Run failed: %v", err)
	}
	if err := p.Run(context.Background()); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("expected ErrAlreadyRunning on second Run, got %v", err)
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
	if !errors.Is(err, ErrPipelineClosed) || ok {
		t.Fatalf("expected ErrPipelineClosed publishing after Close, got ok=%v err=%v", ok, err)
	}
}

func TestConcurrentPublishAndCloseSafe(t *testing.T) {
	p, err := New(64, func(_ context.Context, _ core.TelemetryEvent) error {
		return nil
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = p.Run(ctx)
	}()

	const workers = 8
	const perWorker = 200
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				_, pubErr := p.Publish(validProcessEvent(fmt.Sprintf("w%d-%d", workerID, i), uint32(workerID*1000+i+1)))
				if pubErr != nil && !errors.Is(pubErr, ErrPipelineClosed) {
					t.Errorf("unexpected Publish error: %v", pubErr)
				}
			}
		}(w)
	}

	time.Sleep(5 * time.Millisecond)
	if err := p.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	wg.Wait()
}
