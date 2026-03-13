package wsclient

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"spaceship/agent/internal/config"
	"spaceship/agent/internal/protocol"

	"github.com/gorilla/websocket"
)

// sendHello builds and sends the node.hello envelope to the server.
func (c *Client) sendHello(conn *websocket.Conn, cfg config.Config) error {
	payload := buildHelloPayload(cfg, c.pythonPath)
	c.logger.Debug("building node.hello payload",
		"node_id", cfg.NodeID,
		"hostname", payload.Hostname,
		"platform", payload.Platform,
		"arch", payload.Arch,
		"capabilities", payload.DeclaredCapabilities,
	)
	envelope := protocol.Envelope[protocol.HelloPayload]{
		Type:      protocol.EventNodeHello,
		RequestID: "req_boot_001",
		SessionID: "",
		NodeID:    cfg.NodeID,
		Seq:       c.NextSeq(),
		TS:        nowRFC3339(),
		Payload:   payload,
	}
	return c.writeJSON(conn, envelope)
}

// readWelcome reads the initial node.welcome envelope from the server.
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

// buildHelloPayload constructs the hello payload from config and system metadata.
func buildHelloPayload(cfg config.Config, pythonPath string) protocol.HelloPayload {
	hostname, _ := os.Hostname()

	capabilities := []string{
		"exec", "list_dir", "read_file", "write_file",
		"edit_file", "grep", "delete_file", "move_file", "copy_file",
		"upload_file", "download_file", "sysinfo",
	}
	if pythonPath != "" {
		capabilities = append(capabilities, "exec_python")
	}

	return protocol.HelloPayload{
		Token:                cfg.Token,
		Hostname:             hostname,
		Alias:                cfg.Alias,
		Platform:             cfg.Platform,
		Arch:                 cfg.Arch,
		AgentVersion:         "0.2.0",
		DeclaredCapabilities: capabilities,
	}
}

// collectPlatformInfo returns the current OS and architecture.
// Kept as a standalone function in case metadata collection grows.
func collectPlatformInfo() (platform string, arch string) {
	return runtime.GOOS, runtime.GOARCH
}
