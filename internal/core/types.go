package core

import (
	"errors"
	"time"
)

// ErrNotImplemented is returned by Step-1 TDD stubs before implementation.
var ErrNotImplemented = errors.New("not implemented")

// Budget constants enforcing non-negotiable resource requirements.
const (
	MaxIdleMemoryMB       = 100.0
	MaxAverageCPUPercent  = 2.0
	MaxIdleAIPercent      = 0.0
	DefaultEventQueueCap  = 1024
)

// EventKind identifies the telemetry domain of a normalized event.
type EventKind string

const (
	EventProcessSpawn EventKind = "process_spawn"
	EventProcessExit  EventKind = "process_exit"
	EventFileActivity EventKind = "file_activity"
	EventNetworkConn  EventKind = "network_conn"
	EventStartupMod   EventKind = "startup_mod"
)

// Severity represents deterministic alert or incident risk level.
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// BaselineState tracks the per-device behavioral learning lifecycle.
type BaselineState string

const (
	BaselineNewDevice  BaselineState = "new_device"
	BaselineLearning   BaselineState = "learning"
	BaselineEstablished BaselineState = "established"
	BaselineDeviation  BaselineState = "suspicious_deviation"
	BaselineException  BaselineState = "user_approved_exception"
)

// ProcessInfo captures OS-neutral process lineage and binary metadata.
type ProcessInfo struct {
	PID        uint32 `json:"pid"`
	PPID       uint32 `json:"ppid"`
	Name       string `json:"name"`
	ExePath    string `json:"exePath"`
	CmdLine    string `json:"cmdLine,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	User       string `json:"user,omitempty"`
	Signed     bool   `json:"signed"`
}

// FileEvent captures metadata-only file or sensitive-path operations (never file contents).
type FileEvent struct {
	Path        string `json:"path"`
	Operation   string `json:"operation"` // create, write, remove, rename, chmod
	IsSensitive bool   `json:"isSensitive"`
}

// NetworkEvent captures outbound or inbound socket metadata.
type NetworkEvent struct {
	Protocol   string `json:"protocol"` // tcp, udp
	LocalAddr  string `json:"localAddr"`
	RemoteAddr string `json:"remoteAddr"`
	RemotePort uint16 `json:"remotePort"`
	Outbound   bool   `json:"outbound"`
}

// TelemetryEvent is the single normalized envelope emitted by all OS collectors.
type TelemetryEvent struct {
	ID        string        `json:"id"`
	Timestamp time.Time     `json:"timestamp"`
	Kind      EventKind     `json:"kind"`
	Process   *ProcessInfo  `json:"process,omitempty"`
	File      *FileEvent    `json:"file,omitempty"`
	Network   *NetworkEvent `json:"network,omitempty"`
}

// Alert is a single deterministic rule trigger.
type Alert struct {
	ID          string    `json:"id"`
	RuleID      string    `json:"ruleId"`
	Title       string    `json:"title"`
	Severity    Severity  `json:"severity"`
	Score       int       `json:"score"` // 1..100
	EventID     string    `json:"eventId"`
	ObservedAt  time.Time `json:"observedAt"`
	Explanation string    `json:"explanation"`
}

// Incident correlates multiple related events and alerts into one security story.
type Incident struct {
	ID               string    `json:"id"`
	Title            string    `json:"title"`
	Severity         Severity  `json:"severity"`
	Score            int       `json:"score"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	EventIDs         []string  `json:"eventIds"`
	AlertIDs         []string  `json:"alertIds"`
	ObservedFacts    []string  `json:"observedFacts"`
	Inferences       []string  `json:"inferences,omitempty"`
	Uncertainties    []string  `json:"uncertainties,omitempty"`
	RecommendedFixes []string  `json:"recommendedFixes,omitempty"`
}

// ResourceSnapshot captures Mitochondria's own resource consumption.
type ResourceSnapshot struct {
	Timestamp      time.Time `json:"timestamp"`
	MemoryMB       float64   `json:"memoryMb"`
	CPUPercent     float64   `json:"cpuPercent"`
	IdleAIPercent  float64   `json:"idleAiPercent"`
	QueueDepth     int       `json:"queueDepth"`
	QueueCapacity  int       `json:"queueCapacity"`
	DroppedEvents  uint64    `json:"droppedEvents"`
}

// Validate checks whether a TelemetryEvent has valid required fields and payload.
func (e TelemetryEvent) Validate() error {
	if e.ID == "" {
		return errors.New("event ID is required")
	}
	if e.Timestamp.IsZero() {
		return errors.New("event timestamp is required")
	}
	switch e.Kind {
	case EventProcessSpawn, EventProcessExit:
		if e.Process == nil || e.Process.PID == 0 || e.Process.Name == "" {
			return errors.New("process event requires non-nil ProcessInfo with PID and Name")
		}
	case EventFileActivity, EventStartupMod:
		if e.File == nil || e.File.Path == "" || e.File.Operation == "" {
			return errors.New("file/startup event requires non-nil FileEvent with Path and Operation")
		}
	case EventNetworkConn:
		if e.Network == nil || e.Network.Protocol == "" || e.Network.RemoteAddr == "" {
			return errors.New("network event requires non-nil NetworkEvent with Protocol and RemoteAddr")
		}
	default:
		return errors.New("unknown event kind: " + string(e.Kind))
	}
	return nil
}

// SeverityFromScore maps a numeric risk score (1..100) to a deterministic Severity level.
func SeverityFromScore(score int) (Severity, error) {
	if score < 1 || score > 100 {
		return "", errors.New("score must be between 1 and 100")
	}
	switch {
	case score >= 90:
		return SeverityCritical, nil
	case score >= 70:
		return SeverityHigh, nil
	case score >= 40:
		return SeverityMedium, nil
	default:
		return SeverityLow, nil
	}
}

// WithinBudget checks if the resource snapshot satisfies all non-negotiable performance targets.
func (r ResourceSnapshot) WithinBudget() (bool, []string) {
	var violations []string
	if r.MemoryMB > MaxIdleMemoryMB {
		violations = append(violations, "idle memory exceeds 100 MB budget")
	}
	if r.CPUPercent > MaxAverageCPUPercent {
		violations = append(violations, "average CPU exceeds 2% budget")
	}
	if r.IdleAIPercent > MaxIdleAIPercent {
		violations = append(violations, "idle AI inference exceeds 0% budget")
	}
	return len(violations) == 0, violations
}
