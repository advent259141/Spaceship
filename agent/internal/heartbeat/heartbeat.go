package heartbeat

import "spaceship/agent/internal/protocol"

func BuildPayload(activeTasks int, load float64, memoryUsedMB int64, lastError string) protocol.HeartbeatPayload {
	return protocol.HeartbeatPayload{
		ActiveTasks:  activeTasks,
		Load:         load,
		MemoryUsedMB: memoryUsedMB,
		LastError:    lastError,
	}
}
