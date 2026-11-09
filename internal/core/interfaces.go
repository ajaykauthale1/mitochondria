package core

import "context"

// ProcessCollector abstracts OS-specific process snapshot and lineage inspection.
type ProcessCollector interface {
	CollectDiff(ctx context.Context) ([]TelemetryEvent, error)
}

// FileCollector abstracts OS-specific filesystem and sensitive-path watchers.
type FileCollector interface {
	Start(ctx context.Context, out chan<- TelemetryEvent) error
	Close() error
}

// Detector evaluates a single normalized TelemetryEvent and returns zero or more deterministic Alerts.
type Detector interface {
	ID() string
	Evaluate(event TelemetryEvent) ([]Alert, error)
}

// EventStore abstracts local SQLite persistence for events, alerts, and correlated incidents.
type EventStore interface {
	SaveEvent(ctx context.Context, event TelemetryEvent) error
	SaveAlert(ctx context.Context, alert Alert) error
	ListRecentAlerts(ctx context.Context, limit int) ([]Alert, error)
	PruneOldEvents(ctx context.Context, maxRows int) (int64, error)
}

// ResourceSampler abstracts self-monitoring of memory, CPU, and queue depth.
type ResourceSampler interface {
	Sample() (ResourceSnapshot, error)
}
