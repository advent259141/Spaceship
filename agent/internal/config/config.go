package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultHeartbeatInterval = 20 * time.Second
const defaultReconnectMinDelay = 1 * time.Second
const defaultReconnectMaxDelay = 30 * time.Second

type Config struct {
	ServerURL         string
	NodeID            string
	Token             string
	Alias             string
	LogLevel          string
	HeartbeatInterval time.Duration
	ReconnectMinDelay time.Duration
	ReconnectMaxDelay time.Duration
	Platform          string
	Arch              string
	PythonPath        string // Override auto-detected Python binary path
	SkipPythonVenv    bool   // If true, use system Python directly without creating a venv
}

func LoadFromEnv() (Config, error) {
	loadDotEnv()

	cfg := Config{
		ServerURL:         strings.TrimSpace(os.Getenv("SPACESHIP_SERVER_URL")),
		NodeID:            strings.TrimSpace(os.Getenv("SPACESHIP_NODE_ID")),
		Token:             strings.TrimSpace(os.Getenv("SPACESHIP_NODE_TOKEN")),
		Alias:             strings.TrimSpace(os.Getenv("SPACESHIP_NODE_ALIAS")),
		LogLevel:          defaultString(os.Getenv("SPACESHIP_LOG_LEVEL"), "info"),
		HeartbeatInterval: defaultHeartbeatInterval,
		ReconnectMinDelay: defaultReconnectMinDelay,
		ReconnectMaxDelay: defaultReconnectMaxDelay,
		Platform:          runtime.GOOS,
		Arch:              runtime.GOARCH,
		PythonPath:        strings.TrimSpace(os.Getenv("SPACESHIP_PYTHON_PATH")),
		SkipPythonVenv:    strings.EqualFold(strings.TrimSpace(os.Getenv("SPACESHIP_SKIP_PYTHON_VENV")), "true"),
	}

	if value := strings.TrimSpace(os.Getenv("SPACESHIP_HEARTBEAT_INTERVAL")); value != "" {
		interval, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, err
		}
		cfg.HeartbeatInterval = interval
	}

	if value := strings.TrimSpace(os.Getenv("SPACESHIP_RECONNECT_MIN_DELAY")); value != "" {
		interval, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, err
		}
		cfg.ReconnectMinDelay = interval
	}

	if value := strings.TrimSpace(os.Getenv("SPACESHIP_RECONNECT_MAX_DELAY")); value != "" {
		interval, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, err
		}
		cfg.ReconnectMaxDelay = interval
	}

	if cfg.ReconnectMinDelay <= 0 {
		cfg.ReconnectMinDelay = defaultReconnectMinDelay
	}
	if cfg.ReconnectMaxDelay < cfg.ReconnectMinDelay {
		cfg.ReconnectMaxDelay = cfg.ReconnectMinDelay
	}

	if cfg.ServerURL == "" {
		return Config{}, errors.New("SPACESHIP_SERVER_URL is required")
	}
	if cfg.NodeID == "" {
		return Config{}, errors.New("SPACESHIP_NODE_ID is required")
	}
	if cfg.Token == "" {
		return Config{}, errors.New("SPACESHIP_NODE_TOKEN is required")
	}
	if cfg.Alias == "" {
		cfg.Alias = cfg.NodeID
	}

	return cfg, nil
}

func defaultString(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func loadDotEnv() {
	for _, candidate := range dotenvCandidates() {
		if err := loadDotEnvFile(candidate); err == nil {
			return
		}
	}
}

func dotenvCandidates() []string {
	candidates := []string{}

	if envFile := strings.TrimSpace(os.Getenv("SPACESHIP_ENV_FILE")); envFile != "" {
		candidates = append(candidates, envFile)
	}

	candidates = append(candidates, ".env")

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates, filepath.Join(exeDir, ".env"))
	}

	return candidates
}

func loadDotEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}

	return scanner.Err()
}
