package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadYAML_FullConfig(t *testing.T) {
	content := `
server_url: ws://example.com/ws
token: secret123
log_level: debug
heartbeat_interval: 15s
reconnect:
  min_delay: 2s
  max_delay: 60s
python:
  path: /usr/bin/python3
  skip_venv: true
`
	path := writeTempYAML(t, content)
	fc, err := loadYAML(path)
	if err != nil {
		t.Fatalf("loadYAML failed: %v", err)
	}

	assertStrPtr(t, "ServerURL", fc.ServerURL, "ws://example.com/ws")
	assertStrPtr(t, "Token", fc.Token, "secret123")
	assertStrPtr(t, "LogLevel", fc.LogLevel, "debug")
	assertStrPtr(t, "Heartbeat", fc.Heartbeat, "15s")

	if fc.Reconnect == nil {
		t.Fatal("Reconnect should not be nil")
	}
	assertStrPtr(t, "Reconnect.MinDelay", fc.Reconnect.MinDelay, "2s")
	assertStrPtr(t, "Reconnect.MaxDelay", fc.Reconnect.MaxDelay, "60s")

	if fc.Python == nil {
		t.Fatal("Python should not be nil")
	}
	assertStrPtr(t, "Python.Path", fc.Python.Path, "/usr/bin/python3")
	if fc.Python.SkipVenv == nil || !*fc.Python.SkipVenv {
		t.Fatal("Python.SkipVenv should be true")
	}
}

func TestLoadYAML_PartialConfig(t *testing.T) {
	content := `
server_url: ws://partial.example.com/ws
token: tok
`
	path := writeTempYAML(t, content)
	fc, err := loadYAML(path)
	if err != nil {
		t.Fatalf("loadYAML failed: %v", err)
	}

	assertStrPtr(t, "ServerURL", fc.ServerURL, "ws://partial.example.com/ws")
	assertStrPtr(t, "Token", fc.Token, "tok")

	if fc.Reconnect != nil {
		t.Fatal("Reconnect should be nil")
	}
	if fc.Python != nil {
		t.Fatal("Python should be nil")
	}
}

func TestLoadYAML_FileNotFound(t *testing.T) {
	_, err := loadYAML("/nonexistent/spaceship.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestBuildConfig_FromYAML(t *testing.T) {
	content := `
server_url: ws://build.example.com/ws
token: build-token
heartbeat_interval: 10s
reconnect:
  min_delay: 3s
  max_delay: 45s
`
	path := writeTempYAML(t, content)
	fc, err := loadYAML(path)
	if err != nil {
		t.Fatalf("loadYAML failed: %v", err)
	}

	cfg, err := buildConfig(fc, path)
	if err != nil {
		t.Fatalf("buildConfig failed: %v", err)
	}

	if cfg.ServerURL != "ws://build.example.com/ws" {
		t.Fatalf("ServerURL = %q", cfg.ServerURL)
	}
	if cfg.NodeID == "" {
		t.Fatal("NodeID should be auto-generated, got empty")
	}
	if cfg.Token != "build-token" {
		t.Fatalf("Token = %q", cfg.Token)
	}
	if cfg.HeartbeatInterval != 10*time.Second {
		t.Fatalf("HeartbeatInterval = %v", cfg.HeartbeatInterval)
	}
	if cfg.ReconnectMinDelay != 3*time.Second {
		t.Fatalf("ReconnectMinDelay = %v", cfg.ReconnectMinDelay)
	}
	if cfg.ReconnectMaxDelay != 45*time.Second {
		t.Fatalf("ReconnectMaxDelay = %v", cfg.ReconnectMaxDelay)
	}
	if cfg.ConfigSource != path {
		t.Fatalf("ConfigSource = %q, want %q", cfg.ConfigSource, path)
	}
}

func TestBuildConfig_NodeIDDeterministic(t *testing.T) {
	url := "ws://test/ws"
	tok := "tok"
	fc := FileConfig{
		ServerURL: &url,
		Token:     &tok,
	}
	cfg1, err := buildConfig(fc, "")
	if err != nil {
		t.Fatalf("buildConfig 1st call failed: %v", err)
	}
	cfg2, err := buildConfig(fc, "")
	if err != nil {
		t.Fatalf("buildConfig 2nd call failed: %v", err)
	}
	if cfg1.NodeID != cfg2.NodeID {
		t.Fatalf("NodeID should be deterministic: %q != %q", cfg1.NodeID, cfg2.NodeID)
	}
	if cfg1.NodeID == "" {
		t.Fatal("NodeID should not be empty")
	}
}

func TestMerge_PriorityOrder(t *testing.T) {
	yamlVal := "yaml-value"
	envVal := "env-value"
	flagVal := "flag-value"

	// YAML only
	result := merge(
		FileConfig{},
		FileConfig{},
		FileConfig{ServerURL: &yamlVal},
	)
	assertStrPtr(t, "yaml-only", result.ServerURL, "yaml-value")

	// env overrides yaml
	result = merge(
		FileConfig{},
		FileConfig{ServerURL: &envVal},
		FileConfig{ServerURL: &yamlVal},
	)
	assertStrPtr(t, "env>yaml", result.ServerURL, "env-value")

	// flag overrides env and yaml
	result = merge(
		FileConfig{ServerURL: &flagVal},
		FileConfig{ServerURL: &envVal},
		FileConfig{ServerURL: &yamlVal},
	)
	assertStrPtr(t, "flag>env>yaml", result.ServerURL, "flag-value")
}

func TestBuildConfig_MissingRequired(t *testing.T) {
	cases := []struct {
		name string
		fc   FileConfig
	}{
		{"no server_url", FileConfig{Token: strPtr("t")}},
		{"no token", FileConfig{ServerURL: strPtr("ws://x")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildConfig(tc.fc, "")
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "spaceship.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	return path
}

func assertStrPtr(t *testing.T, name string, got *string, want string) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: expected %q, got nil", name, want)
	}
	if *got != want {
		t.Fatalf("%s: expected %q, got %q", name, want, *got)
	}
}

func strPtr(s string) *string {
	return &s
}
