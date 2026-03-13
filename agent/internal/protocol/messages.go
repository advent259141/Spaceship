package protocol

import "encoding/json"

type EventType string

const (
	EventNodeHello     EventType = "node.hello"
	EventNodeWelcome   EventType = "node.welcome"
	EventNodeHeartbeat EventType = "node.heartbeat"
	EventTaskDispatch  EventType = "task.dispatch"
	EventTaskAccepted  EventType = "task.accepted"
	EventTaskStarted   EventType = "task.started"
	EventTaskOutput    EventType = "task.output"
	EventTaskResult    EventType = "task.result"
	EventTaskError     EventType = "task.error"
	EventTaskCancel    EventType = "task.cancel"
	EventNodeInflight  EventType = "node.inflight"
)

type Envelope[T any] struct {
	Type      EventType `json:"type"`
	RequestID string    `json:"request_id"`
	SessionID string    `json:"session_id"`
	NodeID    string    `json:"node_id"`
	Seq       uint64    `json:"seq"`
	TS        string    `json:"ts"`
	Payload   T         `json:"payload"`
}

type RawEnvelope struct {
	Type      EventType       `json:"type"`
	RequestID string          `json:"request_id"`
	SessionID string          `json:"session_id"`
	NodeID    string          `json:"node_id"`
	Seq       uint64          `json:"seq"`
	TS        string          `json:"ts"`
	Payload   json.RawMessage `json:"payload"`
}

type HelloPayload struct {
	Token                string   `json:"token"`
	Hostname             string   `json:"hostname"`
	Platform             string   `json:"platform"`
	Arch                 string   `json:"arch"`
	AgentVersion         string   `json:"agent_version"`
	DeclaredCapabilities []string `json:"declared_capabilities"`
}

type WelcomePayload struct {
	HeartbeatIntervalSec int      `json:"heartbeat_interval_sec"`
	ResumeSupport        bool     `json:"resume_support"`
	ServerTime           string   `json:"server_time"`
	GrantedScopes        []string `json:"granted_scopes"`
	PolicyVersion        string   `json:"policy_version"`
}

type HeartbeatPayload struct {
	ActiveTasks  int     `json:"active_tasks"`
	Load         float64 `json:"load"`
	MemoryUsedMB int64   `json:"memory_used_mb"`
	LastError    string  `json:"last_error"`
}

type TaskSpec struct {
	TaskID         string         `json:"task_id"`
	TaskType       string         `json:"task_type"`
	NodeID         string         `json:"node_id"`
	RequestedBy    string         `json:"requested_by"`
	RequestedVia   string         `json:"requested_via"`
	ToolCallID     string         `json:"tool_call_id"`
	TimeoutSec     int            `json:"timeout_sec"`
	MaxOutputBytes int            `json:"max_output_bytes"`
	RiskLevel      string         `json:"risk_level"`
	Args           map[string]any `json:"args"`
}

type TaskAcceptedPayload struct {
	TaskID   string `json:"task_id"`
	Executor string `json:"executor"`
	Started  bool   `json:"started"`
	Queued   bool   `json:"queued"`
}

type TaskStartedPayload struct {
	TaskID    string `json:"task_id"`
	StartedAt string `json:"started_at"`
	PID       int    `json:"pid,omitempty"`
}

type TaskOutputPayload struct {
	TaskID string `json:"task_id"`
	Stream string `json:"stream"`
	Offset int64  `json:"offset"`
	Chunk  string `json:"chunk"`
}

type TaskResultPayload struct {
	TaskID      string `json:"task_id"`
	ExitCode    int    `json:"exit_code"`
	DurationMS  int64  `json:"duration_ms"`
	TimedOut    bool   `json:"timed_out"`
	Truncated   bool   `json:"truncated"`
	FinalState  string `json:"final_state"`
	StdoutBytes int64  `json:"stdout_bytes"`
	StderrBytes int64  `json:"stderr_bytes"`
}

type TaskErrorPayload struct {
	TaskID  string `json:"task_id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type TaskCancelPayload struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

type NodeInflightPayload struct {
	TaskIDs []string `json:"task_ids"`
}
