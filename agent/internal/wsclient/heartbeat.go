package wsclient

import (
	"fmt"
	"time"

	"spaceship/agent/internal/protocol"

	"github.com/gorilla/websocket"
)

// sendHeartbeat builds and sends a heartbeat envelope to the server.
func (c *Client) sendHeartbeat(conn *websocket.Conn, nodeID string) error {
	payload := protocol.HeartbeatPayload{
		ActiveTasks:  0,
		Load:         0,
		MemoryUsedMB: 0,
		LastError:    "",
	}
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
