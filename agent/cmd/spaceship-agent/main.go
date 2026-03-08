package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"spaceship/agent/internal/config"
	agentlogger "spaceship/agent/internal/logger"
	"spaceship/agent/internal/wsclient"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	logger := agentlogger.New(cfg.LogLevel)
	client := wsclient.New(cfg.ServerURL, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("spaceship agent initialized",
		"node_id", cfg.NodeID,
		"alias", cfg.Alias,
		"server_url", cfg.ServerURL,
		"heartbeat_interval", cfg.HeartbeatInterval.String(),
		"reconnect_min_delay", cfg.ReconnectMinDelay.String(),
		"reconnect_max_delay", cfg.ReconnectMaxDelay.String(),
	)

	if err := client.Run(ctx, cfg); err != nil {
		logger.Error("agent stopped with error", "error", err)
		os.Exit(1)
	}

	logger.Info("spaceship agent stopped")
}
