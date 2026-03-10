package executor

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"spaceship/agent/internal/protocol"
	"spaceship/agent/internal/shell"
)

func newTestDispatcher() Dispatcher {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewDispatcher(logger, shell.NewRunner(logger))
}

// ──────────────────────────────────────
// parseEdits tests
// ──────────────────────────────────────

func TestParseEdits_Valid(t *testing.T) {
	args := map[string]any{
		"edits": []any{
			map[string]any{"search": "foo", "replace": "bar"},
			map[string]any{"search": "baz", "replace": "qux"},
		},
	}

	edits, err := parseEdits(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(edits) != 2 {
		t.Fatalf("expected 2 edits, got %d", len(edits))
	}
	if edits[0].Search != "foo" || edits[0].Replace != "bar" {
		t.Fatalf("edit[0] mismatch: %+v", edits[0])
	}
	if edits[1].Search != "baz" || edits[1].Replace != "qux" {
		t.Fatalf("edit[1] mismatch: %+v", edits[1])
	}
}

func TestParseEdits_MissingKey(t *testing.T) {
	args := map[string]any{}
	_, err := parseEdits(args)
	if err == nil {
		t.Fatal("expected error for missing edits key")
	}
}

func TestParseEdits_NilValue(t *testing.T) {
	args := map[string]any{"edits": nil}
	_, err := parseEdits(args)
	if err == nil {
		t.Fatal("expected error for nil edits")
	}
}

func TestParseEdits_EmptyArray(t *testing.T) {
	args := map[string]any{"edits": []any{}}
	_, err := parseEdits(args)
	if err == nil {
		t.Fatal("expected error for empty edits array")
	}
}

func TestParseEdits_InvalidStructure(t *testing.T) {
	args := map[string]any{"edits": "not_an_array"}
	_, err := parseEdits(args)
	if err == nil {
		t.Fatal("expected error for non-array edits")
	}
}

// ──────────────────────────────────────
// Helper argument parsers tests
// ──────────────────────────────────────

func TestStringArg(t *testing.T) {
	args := map[string]any{"key": "value", "empty": ""}
	if v := stringArg(args, "key"); v != "value" {
		t.Fatalf("expected 'value', got %q", v)
	}
	if v := stringArg(args, "missing"); v != "" {
		t.Fatalf("expected empty for missing key, got %q", v)
	}
	if v := stringArg(args, "empty"); v != "" {
		t.Fatalf("expected empty string, got %q", v)
	}
}

func TestBoolArg(t *testing.T) {
	args := map[string]any{"flag": true, "off": false}
	if v := boolArg(args, "flag"); !v {
		t.Fatal("expected true")
	}
	if v := boolArg(args, "off"); v {
		t.Fatal("expected false")
	}
	if v := boolArg(args, "missing"); v {
		t.Fatal("expected false for missing key")
	}
}

func TestBoolArgWithDefault(t *testing.T) {
	args := map[string]any{"flag": false}
	if v := boolArgWithDefault(args, "flag", true); v {
		t.Fatal("expected false (explicit value), got true")
	}
	if v := boolArgWithDefault(args, "missing", true); !v {
		t.Fatal("expected true (default) for missing key")
	}
}

func TestIntArg(t *testing.T) {
	args := map[string]any{
		"int":     42,
		"float64": float64(99),
	}
	if v := intArg(args, "int", 0); v != 42 {
		t.Fatalf("expected 42, got %d", v)
	}
	if v := intArg(args, "float64", 0); v != 99 {
		t.Fatalf("expected 99, got %d", v)
	}
	if v := intArg(args, "missing", 10); v != 10 {
		t.Fatalf("expected fallback 10, got %d", v)
	}
}

func TestStringMapArg(t *testing.T) {
	args := map[string]any{
		"env": map[string]any{"FOO": "bar", "BAZ": "qux"},
	}
	m := stringMapArg(args, "env")
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if m["FOO"] != "bar" {
		t.Fatalf("expected FOO=bar, got FOO=%s", m["FOO"])
	}

	// Missing key returns nil
	if m2 := stringMapArg(args, "missing"); m2 != nil {
		t.Fatal("expected nil for missing key")
	}
}

// ──────────────────────────────────────
// Dispatch integration tests (file ops)
// ──────────────────────────────────────

func TestDispatch_ReadFile(t *testing.T) {
	path := writeTemp(t, "read_me")
	dispatcher := newTestDispatcher()
	task := protocol.TaskSpec{
		TaskID:   "test-001",
		TaskType: "read_file",
		Args:     map[string]any{"path": path},
	}
	result, err := dispatcher.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stdout != "read_me" {
		t.Fatalf("expected 'read_me', got %q", result.Stdout)
	}
}

func TestDispatch_WriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dispatch_write.txt")
	dispatcher := newTestDispatcher()
	task := protocol.TaskSpec{
		TaskID:   "test-002",
		TaskType: "write_file",
		Args: map[string]any{
			"path":    path,
			"content": "dispatch_written",
		},
	}
	result, err := dispatcher.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stdout == "" {
		t.Fatal("expected non-empty result")
	}
	assertFileContent(t, path, "dispatch_written")
}

func TestDispatch_ListDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "item.txt"), []byte("x"), 0o644)

	dispatcher := newTestDispatcher()
	task := protocol.TaskSpec{
		TaskID:   "test-003",
		TaskType: "list_dir",
		Args:     map[string]any{"path": dir},
	}
	result, err := dispatcher.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &parsed); err != nil {
		t.Fatalf("failed to parse list_dir result: %v", err)
	}
	entries := parsed["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
}

func TestDispatch_EditFile(t *testing.T) {
	path := writeTemp(t, "original content")
	dispatcher := newTestDispatcher()
	task := protocol.TaskSpec{
		TaskID:   "test-004",
		TaskType: "edit_file",
		Args: map[string]any{
			"path": path,
			"edits": []any{
				map[string]any{"search": "original", "replace": "modified"},
			},
		},
	}
	result, err := dispatcher.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Stdout == "" {
		t.Fatal("expected non-empty result")
	}
	assertFileContent(t, path, "modified content")
}

func TestDispatch_EditFile_InvalidEdits(t *testing.T) {
	dispatcher := newTestDispatcher()
	task := protocol.TaskSpec{
		TaskID:   "test-005",
		TaskType: "edit_file",
		Args:     map[string]any{"path": "some_file.txt"},
	}
	_, err := dispatcher.Dispatch(context.Background(), task)
	if err == nil {
		t.Fatal("expected error when edits arg is missing")
	}
}

func TestDispatch_UnsupportedType(t *testing.T) {
	dispatcher := newTestDispatcher()
	task := protocol.TaskSpec{
		TaskID:   "test-006",
		TaskType: "unknown_type",
		Args:     map[string]any{},
	}
	_, err := dispatcher.Dispatch(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for unsupported task type")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected 'unsupported' in error, got: %v", err)
	}
}

func TestDispatch_Exec_SimpleCommand(t *testing.T) {
	dispatcher := newTestDispatcher()
	task := protocol.TaskSpec{
		TaskID:   "test-007",
		TaskType: "exec",
		Args: map[string]any{
			"command": "echo hello_test",
		},
		TimeoutSec: 10,
	}
	result, err := dispatcher.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Stdout, "hello_test") {
		t.Fatalf("expected stdout to contain 'hello_test', got %q", result.Stdout)
	}
}

func TestDispatch_Grep(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "searchme.txt"), []byte("target line\nother line\n"), 0o644)

	dispatcher := newTestDispatcher()
	task := protocol.TaskSpec{
		TaskID:   "test-008",
		TaskType: "grep",
		Args: map[string]any{
			"pattern": "target",
			"path":    dir,
		},
	}
	result, err := dispatcher.Dispatch(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result.Stdout), &parsed); err != nil {
		t.Fatalf("failed to parse grep result: %v", err)
	}
	count := parsed["count"].(float64)
	if count != 1 {
		t.Fatalf("expected 1 match, got %v", count)
	}
}

func TestStringSliceArg(t *testing.T) {
	args := map[string]any{
		"globs": []any{"*.go", "*.py"},
	}
	result := stringSliceArg(args, "globs")
	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}

	// Missing key
	if v := stringSliceArg(args, "missing"); v != nil {
		t.Fatal("expected nil for missing key")
	}
}

// ──────────────────────────────────────
// DispatchStream tests
// ──────────────────────────────────────

func TestDispatchStream_ExecCollectsChunks(t *testing.T) {
	dispatcher := newTestDispatcher()

	var mu sync.Mutex
	var chunks []shell.OutputChunk

	callback := func(chunk shell.OutputChunk) {
		mu.Lock()
		defer mu.Unlock()
		chunks = append(chunks, chunk)
	}

	task := protocol.TaskSpec{
		TaskID:   "stream-001",
		TaskType: "exec",
		Args: map[string]any{
			"command": "echo stream_line1 && echo stream_line2",
		},
		TimeoutSec: 10,
	}

	result, err := dispatcher.DispatchStream(context.Background(), task, callback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}

	mu.Lock()
	defer mu.Unlock()

	stdoutChunks := 0
	var combined strings.Builder
	for _, chunk := range chunks {
		if chunk.Stream == "stdout" {
			stdoutChunks++
			combined.WriteString(chunk.Data)
		}
	}
	if stdoutChunks < 2 {
		t.Fatalf("expected at least 2 stdout chunks, got %d", stdoutChunks)
	}
	if !strings.Contains(combined.String(), "stream_line1") ||
		!strings.Contains(combined.String(), "stream_line2") {
		t.Fatalf("missing expected lines in streamed output: %q", combined.String())
	}

	// Result fields should be empty since output was streamed.
	if result.Stdout != "" || result.Stderr != "" {
		t.Fatalf("expected empty stdout/stderr in result, got stdout=%q stderr=%q", result.Stdout, result.Stderr)
	}
}

func TestDispatchStream_FileOpsFallback(t *testing.T) {
	path := writeTemp(t, "fallback_content")
	dispatcher := newTestDispatcher()

	var mu sync.Mutex
	var chunks []shell.OutputChunk

	callback := func(chunk shell.OutputChunk) {
		mu.Lock()
		defer mu.Unlock()
		chunks = append(chunks, chunk)
	}

	task := protocol.TaskSpec{
		TaskID:   "stream-002",
		TaskType: "read_file",
		Args:     map[string]any{"path": path},
	}

	result, err := dispatcher.DispatchStream(context.Background(), task, callback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// File ops should deliver result as a single callback chunk.
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for file ops fallback, got %d", len(chunks))
	}
	if chunks[0].Stream != "stdout" {
		t.Fatalf("expected stdout stream, got %q", chunks[0].Stream)
	}
	if chunks[0].Data != "fallback_content" {
		t.Fatalf("expected 'fallback_content', got %q", chunks[0].Data)
	}

	// Result stdout should be cleared since it was forwarded via callback.
	if result.Stdout != "" {
		t.Fatalf("expected empty stdout in result, got %q", result.Stdout)
	}
}

func TestDispatchStream_UnsupportedType(t *testing.T) {
	dispatcher := newTestDispatcher()
	task := protocol.TaskSpec{
		TaskID:   "stream-003",
		TaskType: "unknown_type",
		Args:     map[string]any{},
	}
	_, err := dispatcher.DispatchStream(context.Background(), task, nil)
	if err == nil {
		t.Fatal("expected error for unsupported task type")
	}
}

// ──────────────────────────────────────
// Helpers
// ──────────────────────────────────────

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "testfile.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func assertFileContent(t *testing.T, path string, expected string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", path, err)
	}
	if string(content) != expected {
		t.Fatalf("file content mismatch:\nexpected: %q\n     got: %q", expected, string(content))
	}
}

func assertJSONField(t *testing.T, jsonStr string, key string, expected any) {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}
	actual, ok := parsed[key]
	if !ok {
		t.Fatalf("key %q not found in JSON result", key)
	}
	if actual != expected {
		t.Fatalf("JSON field %q: expected %v, got %v", key, expected, actual)
	}
}
