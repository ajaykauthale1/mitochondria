package core

import (
	"context"
	"testing"
	"time"
)

type mockProcessCollector struct {
	events []TelemetryEvent
	err    error
}

func (m *mockProcessCollector) CollectDiff(_ context.Context) ([]TelemetryEvent, error) {
	return m.events, m.err
}

func TestTelemetryEventValidate(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name    string
		event   TelemetryEvent
		wantErr bool
	}{
		{
			name: "valid process spawn event",
			event: TelemetryEvent{
				ID:        "evt-1",
				Timestamp: now,
				Kind:      EventProcessSpawn,
				Process: &ProcessInfo{
					PID:     4120,
					PPID:    1024,
					Name:    "powershell.exe",
					ExePath: `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`,
				},
			},
			wantErr: false,
		},
		{
			name: "valid sensitive file event",
			event: TelemetryEvent{
				ID:        "evt-2",
				Timestamp: now,
				Kind:      EventFileActivity,
				File: &FileEvent{
					Path:        `C:\Users\alice\.ssh\id_ed25519`,
					Operation:   "write",
					IsSensitive: true,
				},
			},
			wantErr: false,
		},
		{
			name: "valid network connection event",
			event: TelemetryEvent{
				ID:        "evt-3",
				Timestamp: now,
				Kind:      EventNetworkConn,
				Network: &NetworkEvent{
					Protocol:   "tcp",
					LocalAddr:  "192.168.1.10",
					RemoteAddr: "198.51.100.42",
					RemotePort: 443,
					Outbound:   true,
				},
			},
			wantErr: false,
		},
		{
			name: "missing event ID fails validation",
			event: TelemetryEvent{
				Timestamp: now,
				Kind:      EventProcessSpawn,
				Process:   &ProcessInfo{PID: 100, Name: "cmd.exe"},
			},
			wantErr: true,
		},
		{
			name: "zero timestamp fails validation",
			event: TelemetryEvent{
				ID:      "evt-zero-time",
				Kind:    EventProcessSpawn,
				Process: &ProcessInfo{PID: 100, Name: "cmd.exe"},
			},
			wantErr: true,
		},
		{
			name: "process event without process payload fails validation",
			event: TelemetryEvent{
				ID:        "evt-no-proc",
				Timestamp: now,
				Kind:      EventProcessSpawn,
			},
			wantErr: true,
		},
		{
			name: "file event with empty path fails validation",
			event: TelemetryEvent{
				ID:        "evt-empty-file",
				Timestamp: now,
				Kind:      EventFileActivity,
				File:      &FileEvent{Path: "", Operation: "write"},
			},
			wantErr: true,
		},
		{
			name: "unknown event kind fails validation",
			event: TelemetryEvent{
				ID:        "evt-unknown",
				Timestamp: now,
				Kind:      EventKind("kernel_patch"),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.event.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestSeverityFromScore(t *testing.T) {
	tests := []struct {
		score   int
		want    Severity
		wantErr bool
	}{
		{score: 1, want: SeverityLow},
		{score: 39, want: SeverityLow},
		{score: 40, want: SeverityMedium},
		{score: 69, want: SeverityMedium},
		{score: 70, want: SeverityHigh},
		{score: 89, want: SeverityHigh},
		{score: 90, want: SeverityCritical},
		{score: 100, want: SeverityCritical},
		{score: 0, wantErr: true},
		{score: 101, wantErr: true},
		{score: -5, wantErr: true},
	}

	for _, tc := range tests {
		got, err := SeverityFromScore(tc.score)
		if (err != nil) != tc.wantErr {
			t.Fatalf("SeverityFromScore(%d) error = %v, wantErr %v", tc.score, err, tc.wantErr)
		}
		if !tc.wantErr && got != tc.want {
			t.Fatalf("SeverityFromScore(%d) = %q, want %q", tc.score, got, tc.want)
		}
	}
}

func TestResourceSnapshotWithinBudget(t *testing.T) {
	healthy := ResourceSnapshot{
		Timestamp:     time.Now().UTC(),
		MemoryMB:      28.4,
		CPUPercent:    0.4,
		IdleAIPercent: 0.0,
		QueueDepth:    12,
		QueueCapacity: DefaultEventQueueCap,
	}
	ok, violations := healthy.WithinBudget()
	if !ok || len(violations) != 0 {
		t.Fatalf("expected healthy snapshot to be within budget, got ok=%v violations=%v", ok, violations)
	}

	overBudget := ResourceSnapshot{
		Timestamp:     time.Now().UTC(),
		MemoryMB:      115.2,
		CPUPercent:    3.5,
		IdleAIPercent: 1.2,
		QueueDepth:    1024,
		QueueCapacity: DefaultEventQueueCap,
	}
	ok, violations = overBudget.WithinBudget()
	if ok {
		t.Fatal("expected overBudget snapshot to fail WithinBudget()")
	}
	if len(violations) != 3 {
		t.Fatalf("expected 3 budget violations (RAM, CPU, IdleAI), got %d: %v", len(violations), violations)
	}
}

func TestMockCollectorInterfaceCompliance(t *testing.T) {
	now := time.Now().UTC()
	var collector ProcessCollector = &mockProcessCollector{
		events: []TelemetryEvent{
			{
				ID:        "evt-mock-1",
				Timestamp: now,
				Kind:      EventProcessSpawn,
				Process:   &ProcessInfo{PID: 88, PPID: 4, Name: "mitod.exe", ExePath: `C:\Program Files\Mitochondria\mitod.exe`},
			},
		},
	}

	events, err := collector.CollectDiff(context.Background())
	if err != nil {
		t.Fatalf("unexpected error from mock collector: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if err := events[0].Validate(); err != nil {
		t.Fatalf("expected valid event from mock collector, got error: %v", err)
	}
}
