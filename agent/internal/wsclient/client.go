package wsclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"spaceship/agent/internal/config"
	"spaceship/agent/internal/executor"
	"spaceship/agent/internal/heartbeat"
	"spaceship/agent/internal/metadata"
	"spaceship/agent/internal/protocol"
	"spaceship/agent/internal/registrar"
	"spaceship/agent/internal/shell"

	"github.com/gorilla/websocket"
)

type Client struct {
	serverURL string
	logger    *slog.Logger

	mu      sync.Mutex
	writeMu sync.Mutex
	seq     uint64
}

func New(serverURL string, logger *slog.Logger) *Client {
	return &Client{serverURL: serverURL, logger: logger}
}

func (c *Client) Run(ctx context.Context, cfg config.Config) error {
	attempt := 0

	for {
		if ctx.Err() != nil {
			return nil
		}

		attempt++
		c.logger.Info("connecting to spaceship gateway",
			"server_url", c.serverURL,
			"node_id", cfg.NodeID,
			"attempt", attempt,
		)

		err := c.runSession(ctx, cfg)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return nil
		}

		delay := backoffDelay(attempt, cfg.ReconnectMinDelay, cfg.ReconnectMaxDelay)
		c.logger.Warn("spaceship connection lost; scheduling reconnect",
			"attempt", attempt,
			"retry_in", delay.String(),
			"error", err,
		)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(delay):
		}
	}
}

func (c *Client) runSession(ctx context.Context, cfg config.Config) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.serverURL, nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	c.logger.Info("connected to spaceship gateway",
		"server_url", c.serverURL,
		"node_id", cfg.NodeID,
	)

	if err := c.sendHello(conn, cfg); err != nil {
		return err
	}
	c.logger.Info("node.hello sent",
		"node_id", cfg.NodeID,
		"alias", cfg.Alias,
	)

	welcome, err := c.readWelcome(conn)
	if err != nil {
		return err
	}

	heartbeatInterval := cfg.HeartbeatInterval
	if welcome.HeartbeatIntervalSec > 0 {
		heartbeatInterval = time.Duration(welcome.HeartbeatIntervalSec) * time.Second
	}
	c.logger.Info("node.welcome received",
		"node_id", cfg.NodeID,
		"heartbeat_interval", heartbeatInterval.String(),
		"granted_scopes", welcome.GrantedScopes,
		"resume_support", welcome.ResumeSupport,
	)

	execDispatcher := executor.NewDispatcher(c.logger, shell.NewRunner(c.logger))
	errCh := make(chan error, 1)
	go func() {
		errCh <- c.readLoop(ctx, conn, cfg, execDispatcher)
	}()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("shutdown signal received; closing websocket",
				"node_id", cfg.NodeID,
			)
			return c.writeControl(conn, websocket.CloseMessage)
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) {
				c.logger.Warn("websocket read loop exited",
					"node_id", cfg.NodeID,
					"error", err,
				)
			}
			return err
		case <-ticker.C:
			if err := c.sendHeartbeat(conn, cfg.NodeID); err != nil {
				c.logger.Warn("heartbeat send failed",
					"node_id", cfg.NodeID,
					"error", err,
				)
				return err
			}
			c.logger.Debug("node.heartbeat sent", "node_id", cfg.NodeID)
		}
	}
}

func (c *Client) NextSeq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	return c.seq
}

func (c *Client) NewHelloEnvelope(nodeID string, payload protocol.HelloPayload, ts string) protocol.Envelope[protocol.HelloPayload] {
	return protocol.Envelope[protocol.HelloPayload]{
		Type:      protocol.EventNodeHello,
		RequestID: "req_boot_001",
		SessionID: "",
		NodeID:    nodeID,
		Seq:       c.NextSeq(),
		TS:        ts,
		Payload:   payload,
	}
}

func (c *Client) sendHello(conn *websocket.Conn, cfg config.Config) error {
	meta := metadata.Collect()
	payload := registrar.Service{}.BuildHelloPayload(cfg.Token, meta.Hostname, cfg.Alias, cfg.Platform, cfg.Arch)
	c.logger.Debug("building node.hello payload",
		"node_id", cfg.NodeID,
		"hostname", meta.Hostname,
		"platform", payload.Platform,
		"arch", payload.Arch,
		"capabilities", payload.DeclaredCapabilities,
	)
	return c.writeJSON(conn, c.NewHelloEnvelope(cfg.NodeID, payload, nowRFC3339()))
}

func (c *Client) readWelcome(conn *websocket.Conn) (protocol.WelcomePayload, error) {
	var envelope protocol.RawEnvelope
	if err := conn.ReadJSON(&envelope); err != nil {
		return protocol.WelcomePayload{}, err
	}
	c.logger.Debug("websocket message received during bootstrap",
		"type", envelope.Type,
		"request_id", envelope.RequestID,
		"session_id", envelope.SessionID,
	)
	if envelope.Type != protocol.EventNodeWelcome {
		return protocol.WelcomePayload{}, fmt.Errorf("unexpected event: %s", envelope.Type)
	}
	var payload protocol.WelcomePayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return protocol.WelcomePayload{}, err
	}
	return payload, nil
}

func (c *Client) sendHeartbeat(conn *websocket.Conn, nodeID string) error {
	payload := heartbeat.BuildPayload(0, 0, 0, "")
	envelope := protocol.Envelope[protocol.HeartbeatPayload]{
		Type:      protocol.EventNodeHeartbeat,
		RequestID: fmt.Sprintf("hb_%d", time.Now().Unix()),
		SessionID: "",
		NodeID:    nodeID,
		Seq:       c.NextSeq(),
		TS:        nowRFC3339(),
		Payload:   payload,
	}
	return c.writeJSON(conn, envelope)
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn, cfg config.Config, dispatcher executor.Dispatcher) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var raw protocol.RawEnvelope
		if err := conn.ReadJSON(&raw); err != nil {
			return err
		}

		c.logger.Debug("websocket event received",
			"type", raw.Type,
			"request_id", raw.RequestID,
			"session_id", raw.SessionID,
			"seq", raw.Seq,
		)

		switch raw.Type {
		case protocol.EventTaskDispatch:
			var task protocol.TaskSpec
			if err := json.Unmarshal(raw.Payload, &task); err != nil {
				return err
			}
			c.logger.Info("task.dispatch received",
				"task_id", task.TaskID,
				"task_type", task.TaskType,
				"requested_by", task.RequestedBy,
			)
			go c.handleTask(conn, cfg.NodeID, raw, task, dispatcher)
		default:
			c.logger.Debug("ignoring unsupported websocket event",
				"type", raw.Type,
				"node_id", cfg.NodeID,
			)
		}
	}
}

func (c *Client) handleTask(conn *websocket.Conn, nodeID string, raw protocol.RawEnvelope, task protocol.TaskSpec, dispatcher executor.Dispatcher) {
	c.logger.Info("task execution started",
		"task_id", task.TaskID,
		"task_type", task.TaskType,
		"node_id", nodeID,
	)

	accepted := protocol.Envelope[protocol.TaskAcceptedPayload]{
		Type:      protocol.EventTaskAccepted,
		RequestID: raw.RequestID,
		SessionID: raw.SessionID,
		NodeID:    nodeID,
		Seq:       c.NextSeq(),
		TS:        nowRFC3339(),
		Payload: protocol.TaskAcceptedPayload{
			TaskID:   task.TaskID,
			Executor: task.TaskType,
			Started:  true,
			Queued:   false,
		},
	}
	if err := c.writeJSON(conn, accepted); err != nil {
		c.logger.Error("send task.accepted failed", "task_id", task.TaskID, "error", err)
		return
	}

	started := protocol.Envelope[protocol.TaskStartedPayload]{
		Type:      protocol.EventTaskStarted,
		RequestID: raw.RequestID,
		SessionID: raw.SessionID,
		NodeID:    nodeID,
		Seq:       c.NextSeq(),
		TS:        nowRFC3339(),
		Payload: protocol.TaskStartedPayload{
			TaskID:    task.TaskID,
			StartedAt: nowRFC3339(),
		},
	}
	if err := c.writeJSON(conn, started); err != nil {
		c.logger.Error("send task.started failed", "task_id", task.TaskID, "error", err)
		return
	}

	result, err := dispatcher.Dispatch(task)
	if err != nil {
		c.logger.Error("task execution failed",
			"task_id", task.TaskID,
			"task_type", task.TaskType,
			"node_id", nodeID,
			"error", err,
		)
		_ = c.writeJSON(conn, protocol.Envelope[protocol.TaskErrorPayload]{
			Type:      protocol.EventTaskError,
			RequestID: raw.RequestID,
			SessionID: raw.SessionID,
			NodeID:    nodeID,
			Seq:       c.NextSeq(),
			TS:        nowRFC3339(),
			Payload: protocol.TaskErrorPayload{
				TaskID:  task.TaskID,
				Code:    "exec_failed",
				Message: err.Error(),
			},
		})
		return
	}

	if result.Stdout != "" {
		c.logger.Debug("sending task stdout chunk",
			"task_id", task.TaskID,
			"bytes", len(result.Stdout),
		)
		_ = c.writeJSON(conn, protocol.Envelope[protocol.TaskOutputPayload]{
			Type:      protocol.EventTaskOutput,
			RequestID: raw.RequestID,
			SessionID: raw.SessionID,
			NodeID:    nodeID,
			Seq:       c.NextSeq(),
			TS:        nowRFC3339(),
			Payload: protocol.TaskOutputPayload{
				TaskID: task.TaskID,
				Stream: "stdout",
				Offset: 0,
				Chunk:  result.Stdout,
			},
		})
	}
	if result.Stderr != "" {
		c.logger.Debug("sending task stderr chunk",
			"task_id", task.TaskID,
			"bytes", len(result.Stderr),
		)
		_ = c.writeJSON(conn, protocol.Envelope[protocol.TaskOutputPayload]{
			Type:      protocol.EventTaskOutput,
			RequestID: raw.RequestID,
			SessionID: raw.SessionID,
			NodeID:    nodeID,
			Seq:       c.NextSeq(),
			TS:        nowRFC3339(),
			Payload: protocol.TaskOutputPayload{
				TaskID: task.TaskID,
				Stream: "stderr",
				Offset: 0,
				Chunk:  result.Stderr,
			},
		})
	}

	finalState := "success"
	if result.ExitCode != 0 || result.TimedOut {
		finalState = "failed"
	}

	c.logger.Info("task execution finished",
		"task_id", task.TaskID,
		"task_type", task.TaskType,
		"node_id", nodeID,
		"final_state", finalState,
		"exit_code", result.ExitCode,
		"timed_out", result.TimedOut,
		"stdout_bytes", len(result.Stdout),
		"stderr_bytes", len(result.Stderr),
		"duration_ms", result.DurationMS,
	)

	_ = c.writeJSON(conn, protocol.Envelope[protocol.TaskResultPayload]{
		Type:      protocol.EventTaskResult,
		RequestID: raw.RequestID,
		SessionID: raw.SessionID,
		NodeID:    nodeID,
		Seq:       c.NextSeq(),
		TS:        nowRFC3339(),
		Payload: protocol.TaskResultPayload{
			TaskID:      task.TaskID,
			ExitCode:    result.ExitCode,
			DurationMS:  result.DurationMS,
			TimedOut:    result.TimedOut,
			Truncated:   result.Truncated,
			FinalState:  finalState,
			StdoutBytes: int64(len(result.Stdout)),
			StderrBytes: int64(len(result.Stderr)),
		},
	})
}

func (c *Client) writeJSON(conn *websocket.Conn, payload any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if envelope := describeEnvelope(payload); envelope != nil {
		c.logger.Debug("sending websocket event",
			"type", envelope.Type,
			"request_id", envelope.RequestID,
			"session_id", envelope.SessionID,
			"node_id", envelope.NodeID,
			"seq", envelope.Seq,
		)
	}
	return conn.WriteJSON(payload)
}

func (c *Client) writeControl(conn *websocket.Conn, messageType int) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.WriteControl(messageType, []byte{}, time.Now().Add(2*time.Second))
}

func nowRFC3339() string {
	return time.Now().Format(time.RFC3339)
}

type envelopeSummary struct {
	Type      string
	RequestID string
	SessionID string
	NodeID    string
	Seq       uint64
}

func describeEnvelope(payload any) *envelopeSummary {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}

	var raw protocol.RawEnvelope
	if err := json.Unmarshal(encoded, &raw); err != nil {
		return nil
	}

	return &envelopeSummary{
		Type:      string(raw.Type),
		RequestID: raw.RequestID,
		SessionID: raw.SessionID,
		NodeID:    raw.NodeID,
		Seq:       raw.Seq,
	}
}

func backoffDelay(attempt int, minDelay time.Duration, maxDelay time.Duration) time.Duration {
	if attempt <= 1 {
		return minDelay
	}

	delay := minDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= maxDelay {
			return maxDelay
		}
	}
	if delay < minDelay {
		return minDelay
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}
