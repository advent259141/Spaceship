package config

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"spaceship/agent/internal/machineid"

	"gopkg.in/yaml.v3"
)

const defaultHeartbeatInterval = 20 * time.Second
const defaultReconnectMinDelay = 1 * time.Second
const defaultReconnectMaxDelay = 30 * time.Second

// Config is the final, validated configuration used by the agent.
type Config struct {
	ServerURL         string
	NodeID            string // auto-generated from machine-id, not user-configurable
	Token             string
	LogLevel          string
	HeartbeatInterval time.Duration
	ReconnectMinDelay time.Duration
	ReconnectMaxDelay time.Duration
	Platform          string
	Arch              string
	PythonPath        string // Override auto-detected Python binary path
	SkipPythonVenv    bool   // If true, use system Python directly without creating a venv

	// ConfigSource records where the configuration was loaded from (for logging).
	ConfigSource string
}

// FileConfig mirrors the YAML file structure. Pointer fields allow
// distinguishing "not set" from zero values during merge.
type FileConfig struct {
	ServerURL      *string         `yaml:"server_url"`
	Token          *string         `yaml:"token"`
	LogLevel       *string         `yaml:"log_level"`
	Heartbeat      *string         `yaml:"heartbeat_interval"`
	Reconnect      *ReconnectYAML  `yaml:"reconnect"`
	Python         *PythonYAML     `yaml:"python"`
}

type ReconnectYAML struct {
	MinDelay *string `yaml:"min_delay"`
	MaxDelay *string `yaml:"max_delay"`
}

type PythonYAML struct {
	Path     *string `yaml:"path"`
	SkipVenv *bool   `yaml:"skip_venv"`
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Load is the recommended entry point. It merges configuration from
// CLI flags, environment variables, and a YAML file (in that priority order),
// then applies defaults and validates required fields.
func Load() (Config, error) {
	flagCfg := parseFlags()

	// Determine config file path: --config flag > SPACESHIP_CONFIG_FILE env > default candidates.
	configPath := resolveConfigPath(flagCfg.configFile)

	// Load .env before reading env vars (backward compat).
	loadDotEnv()

	var yamlCfg FileConfig
	var yamlSource string
	if configPath != "" {
		var err error
		yamlCfg, err = loadYAML(configPath)
		if err != nil {
			return Config{}, fmt.Errorf("failed to load config file %s: %w", configPath, err)
		}
		yamlSource = configPath
	}

	envCfg := loadEnvConfig()

	cfg := merge(flagCfg.FileConfig, envCfg, yamlCfg)

	return buildConfig(cfg, yamlSource)
}

// LoadFromEnv is kept for backward compatibility. It loads configuration
// from .env / environment variables only (no YAML, no CLI flags).
func LoadFromEnv() (Config, error) {
	loadDotEnv()
	envCfg := loadEnvConfig()
	return buildConfig(envCfg, "")
}

// ---------------------------------------------------------------------------
// CLI flags
// ---------------------------------------------------------------------------

type flagResult struct {
	FileConfig
	configFile string
}

func parseFlags() flagResult {
	var r flagResult

	var server, token, logLevel, configFile string

	fs := flag.NewFlagSet("spaceship-agent", flag.ContinueOnError)
	fs.StringVar(&configFile, "config", "", "Path to YAML config file (default: spaceship.yaml)")
	fs.StringVar(&server, "server", "", "AstrBot gateway WebSocket URL")
	fs.StringVar(&token, "token", "", "Bootstrap token")
	fs.StringVar(&logLevel, "log-level", "", "Log level (debug, info, warn, error)")

	// Silently ignore parse errors so unknown flags from test frameworks don't crash the agent.
	_ = fs.Parse(os.Args[1:])

	r.configFile = configFile
	if server != "" {
		r.ServerURL = &server
	}
	if token != "" {
		r.Token = &token
	}
	if logLevel != "" {
		r.LogLevel = &logLevel
	}

	return r
}

// ---------------------------------------------------------------------------
// YAML file loading
// ---------------------------------------------------------------------------

func loadYAML(path string) (FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileConfig{}, err
	}

	var fc FileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return FileConfig{}, err
	}
	return fc, nil
}

// resolveConfigPath determines which config file to use.
// Priority: explicit flag value > SPACESHIP_CONFIG_FILE env > default candidates.
func resolveConfigPath(flagValue string) string {
	if flagValue != "" {
		// User explicitly specified a config file; it must exist.
		return flagValue
	}

	if envPath := strings.TrimSpace(os.Getenv("SPACESHIP_CONFIG_FILE")); envPath != "" {
		if fileExists(envPath) {
			return envPath
		}
	}

	for _, candidate := range yamlCandidates() {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func yamlCandidates() []string {
	candidates := []string{"spaceship.yaml", "spaceship.yml"}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "spaceship.yaml"),
			filepath.Join(exeDir, "spaceship.yml"),
		)
	}

	return candidates
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ---------------------------------------------------------------------------
// Environment variables
// ---------------------------------------------------------------------------

func loadEnvConfig() FileConfig {
	var fc FileConfig

	if v := envStr("SPACESHIP_SERVER_URL"); v != "" {
		fc.ServerURL = &v
	}
	if v := envStr("SPACESHIP_NODE_TOKEN"); v != "" {
		fc.Token = &v
	}
	if v := envStr("SPACESHIP_LOG_LEVEL"); v != "" {
		fc.LogLevel = &v
	}
	if v := envStr("SPACESHIP_HEARTBEAT_INTERVAL"); v != "" {
		fc.Heartbeat = &v
	}

	minDelay := envStr("SPACESHIP_RECONNECT_MIN_DELAY")
	maxDelay := envStr("SPACESHIP_RECONNECT_MAX_DELAY")
	if minDelay != "" || maxDelay != "" {
		fc.Reconnect = &ReconnectYAML{}
		if minDelay != "" {
			fc.Reconnect.MinDelay = &minDelay
		}
		if maxDelay != "" {
			fc.Reconnect.MaxDelay = &maxDelay
		}
	}

	pythonPath := envStr("SPACESHIP_PYTHON_PATH")
	skipVenvStr := envStr("SPACESHIP_SKIP_PYTHON_VENV")
	if pythonPath != "" || skipVenvStr != "" {
		fc.Python = &PythonYAML{}
		if pythonPath != "" {
			fc.Python.Path = &pythonPath
		}
		if skipVenvStr != "" {
			sv := strings.EqualFold(skipVenvStr, "true")
			fc.Python.SkipVenv = &sv
		}
	}

	return fc
}

func envStr(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// ---------------------------------------------------------------------------
// Merge: flag > env > yaml
// ---------------------------------------------------------------------------

func merge(flag, env, yaml FileConfig) FileConfig {
	var out FileConfig

	out.ServerURL = coalesceStr(flag.ServerURL, env.ServerURL, yaml.ServerURL)
	out.Token = coalesceStr(flag.Token, env.Token, yaml.Token)
	out.LogLevel = coalesceStr(flag.LogLevel, env.LogLevel, yaml.LogLevel)
	out.Heartbeat = coalesceStr(flag.Heartbeat, env.Heartbeat, yaml.Heartbeat)

	out.Reconnect = mergeReconnect(flag.Reconnect, env.Reconnect, yaml.Reconnect)
	out.Python = mergePython(flag.Python, env.Python, yaml.Python)

	return out
}

func coalesceStr(values ...*string) *string {
	for _, v := range values {
		if v != nil && *v != "" {
			return v
		}
	}
	return nil
}

func coalesceBool(values ...*bool) *bool {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func mergeReconnect(layers ...*ReconnectYAML) *ReconnectYAML {
	var minDelay, maxDelay *string
	for _, l := range layers {
		if l == nil {
			continue
		}
		if minDelay == nil && l.MinDelay != nil {
			minDelay = l.MinDelay
		}
		if maxDelay == nil && l.MaxDelay != nil {
			maxDelay = l.MaxDelay
		}
	}
	if minDelay == nil && maxDelay == nil {
		return nil
	}
	return &ReconnectYAML{MinDelay: minDelay, MaxDelay: maxDelay}
}

func mergePython(layers ...*PythonYAML) *PythonYAML {
	var path *string
	var skipVenv *bool
	for _, l := range layers {
		if l == nil {
			continue
		}
		if path == nil {
			path = coalesceStr(l.Path)
		}
		if skipVenv == nil {
			skipVenv = coalesceBool(l.SkipVenv)
		}
	}
	if path == nil && skipVenv == nil {
		return nil
	}
	return &PythonYAML{Path: path, SkipVenv: skipVenv}
}

// ---------------------------------------------------------------------------
// Build final Config from merged FileConfig
// ---------------------------------------------------------------------------

func buildConfig(fc FileConfig, yamlSource string) (Config, error) {
	// Auto-generate node ID from machine identifier.
	nodeID, err := machineid.NodeID()
	if err != nil {
		return Config{}, fmt.Errorf("failed to generate node_id: %w", err)
	}

	cfg := Config{
		ServerURL:         derefStr(fc.ServerURL),
		NodeID:            nodeID,
		Token:             derefStr(fc.Token),
		LogLevel:          derefStrOr(fc.LogLevel, "info"),
		HeartbeatInterval: defaultHeartbeatInterval,
		ReconnectMinDelay: defaultReconnectMinDelay,
		ReconnectMaxDelay: defaultReconnectMaxDelay,
		Platform:          runtime.GOOS,
		Arch:              runtime.GOARCH,
	}

	// Heartbeat interval
	if fc.Heartbeat != nil {
		d, err := time.ParseDuration(*fc.Heartbeat)
		if err != nil {
			return Config{}, fmt.Errorf("invalid heartbeat_interval %q: %w", *fc.Heartbeat, err)
		}
		cfg.HeartbeatInterval = d
	}

	// Reconnect delays
	if fc.Reconnect != nil {
		if fc.Reconnect.MinDelay != nil {
			d, err := time.ParseDuration(*fc.Reconnect.MinDelay)
			if err != nil {
				return Config{}, fmt.Errorf("invalid reconnect min_delay %q: %w", *fc.Reconnect.MinDelay, err)
			}
			cfg.ReconnectMinDelay = d
		}
		if fc.Reconnect.MaxDelay != nil {
			d, err := time.ParseDuration(*fc.Reconnect.MaxDelay)
			if err != nil {
				return Config{}, fmt.Errorf("invalid reconnect max_delay %q: %w", *fc.Reconnect.MaxDelay, err)
			}
			cfg.ReconnectMaxDelay = d
		}
	}

	// Python
	if fc.Python != nil {
		if fc.Python.Path != nil {
			cfg.PythonPath = *fc.Python.Path
		}
		if fc.Python.SkipVenv != nil {
			cfg.SkipPythonVenv = *fc.Python.SkipVenv
		}
	}

	// Defaults / validation
	if cfg.ReconnectMinDelay <= 0 {
		cfg.ReconnectMinDelay = defaultReconnectMinDelay
	}
	if cfg.ReconnectMaxDelay < cfg.ReconnectMinDelay {
		cfg.ReconnectMaxDelay = cfg.ReconnectMinDelay
	}
	if cfg.ServerURL == "" {
		return Config{}, errors.New("server_url is required (set via --server, SPACESHIP_SERVER_URL, or config file)")
	}
	if cfg.Token == "" {
		return Config{}, errors.New("token is required (set via --token, SPACESHIP_NODE_TOKEN, or config file)")
	}

	// Config source for logging
	if yamlSource != "" {
		cfg.ConfigSource = yamlSource
	} else {
		cfg.ConfigSource = "env"
	}

	return cfg, nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefStrOr(p *string, fallback string) string {
	if p == nil || *p == "" {
		return fallback
	}
	return *p
}

// ---------------------------------------------------------------------------
// .env loader (backward compatibility)
// ---------------------------------------------------------------------------

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
