package shell

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestRunner() Runner {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewRunner(logger)
}

func TestExecStream_CollectsChunks(t *testing.T) {
	runner := newTestRunner()

	var mu sync.Mutex
	var chunks []OutputChunk

	callback := func(chunk OutputChunk) {
		mu.Lock()
		defer mu.Unlock()
		chunks = append(chunks, chunk)
	}

	// Echo multiple lines to generate multiple chunks.
	result, err := runner.ExecStream(context.Background(), ExecRequest{
		Command: "echo line1 && echo line2 && echo line3",
		Timeout: 10 * time.Second,
	}, callback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", result.ExitCode)
	}

	mu.Lock()
	defer mu.Unlock()

	// Should have received at least 3 stdout chunks (one per line).
	stdoutChunks := 0
	var combined strings.Builder
	for _, chunk := range chunks {
		if chunk.Stream == "stdout" {
			stdoutChunks++
			combined.WriteString(chunk.Data)
		}
	}
	if stdoutChunks < 3 {
		t.Fatalf("expected at least 3 stdout chunks, got %d", stdoutChunks)
	}
	if !strings.Contains(combined.String(), "line1") ||
		!strings.Contains(combined.String(), "line2") ||
		!strings.Contains(combined.String(), "line3") {
		t.Fatalf("missing expected lines in output: %q", combined.String())
	}

	// Stdout and Stderr in result should be empty (already streamed).
	if result.Stdout != "" {
		t.Fatalf("expected empty Stdout in result, got %q", result.Stdout)
	}
	if result.Stderr != "" {
		t.Fatalf("expected empty Stderr in result, got %q", result.Stderr)
	}
}

func TestExecStream_NilCallbackFallsBack(t *testing.T) {
	runner := newTestRunner()

	// With nil callback, should fall back to regular Exec behavior.
	result, err := runner.ExecStream(context.Background(), ExecRequest{
		Command: "echo fallback",
		Timeout: 10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Stdout, "fallback") {
		t.Fatalf("expected 'fallback' in stdout, got %q", result.Stdout)
	}
}

func TestExecStream_StderrChunks(t *testing.T) {
	runner := newTestRunner()

	var mu sync.Mutex
	var chunks []OutputChunk

	callback := func(chunk OutputChunk) {
		mu.Lock()
		defer mu.Unlock()
		chunks = append(chunks, chunk)
	}

	// Write to stderr.
	result, err := runner.ExecStream(context.Background(), ExecRequest{
		Command: "echo error_output 1>&2",
		Timeout: 10 * time.Second,
	}, callback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// On Windows cmd, exit code may still be 0.
	_ = result

	mu.Lock()
	defer mu.Unlock()

	stderrChunks := 0
	for _, chunk := range chunks {
		if chunk.Stream == "stderr" {
			stderrChunks++
		}
	}
	if stderrChunks < 1 {
		t.Fatalf("expected at least 1 stderr chunk, got %d", stderrChunks)
	}
}

func TestExecStream_EmptyCommand(t *testing.T) {
	runner := newTestRunner()
	_, err := runner.ExecStream(context.Background(), ExecRequest{}, nil)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}
