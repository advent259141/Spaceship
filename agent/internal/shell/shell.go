package shell

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

type ExecRequest struct {
	Command    string
	CWD        string
	Env        map[string]string
	Timeout    time.Duration
	Background bool
}

type ExecResult struct {
	Stdout    string
	Stderr    string
	ExitCode  int
	PID       int
	TimedOut  bool
	Cancelled bool
	Duration  time.Duration
}

// OutputChunk represents a streaming output fragment.
type OutputChunk struct {
	Stream string // "stdout" or "stderr"
	Data   string
}

// StreamCallback is called for each output chunk during streaming execution.
type StreamCallback func(chunk OutputChunk)

type Runner struct {
	logger *slog.Logger
}

func NewRunner(logger *slog.Logger) Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return Runner{logger: logger}
}

func (r Runner) Exec(ctx context.Context, request ExecRequest) (ExecResult, error) {
	if request.Command == "" {
		return ExecResult{}, errors.New("command is required")
	}

	workingDir := request.CWD
	if workingDir == "" {
		workingDir = "."
	}
	logger := r.logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("executing shell command",
		"cwd", workingDir,
		"timeout", request.Timeout.String(),
		"background", request.Background,
		"env_keys", len(request.Env),
		"command", request.Command,
	)

	startedAt := time.Now()
	if request.Timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, request.Timeout)
		defer timeoutCancel()
	}

	command := buildCommand(ctx, request.Command)
	command.Dir = request.CWD
	command.Env = mergeEnv(request.Env)

	if request.Background {
		if err := command.Start(); err != nil {
			logger.Error("failed to start shell command",
				"cwd", workingDir,
				"background", true,
				"error", err,
			)
			return ExecResult{}, err
		}
		result := ExecResult{
			PID:      command.Process.Pid,
			ExitCode: 0,
			Duration: time.Since(startedAt),
		}
		logger.Info("shell command started in background",
			"cwd", workingDir,
			"pid", result.PID,
			"duration_ms", result.Duration.Milliseconds(),
		)
		return result, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err := command.Run()
	cancelled := errors.Is(ctx.Err(), context.Canceled)
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)

	// If the context expired, kill the entire process tree.
	if timedOut || cancelled {
		killProcessTree(command, logger)
	}

	result := ExecResult{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		ExitCode:  exitCode(err),
		Duration:  time.Since(startedAt),
		TimedOut:  timedOut,
		Cancelled: cancelled,
	}

	if err != nil && result.ExitCode == 0 && !timedOut && !cancelled {
		logger.Error("shell command failed before exit status was available",
			"cwd", workingDir,
			"duration_ms", result.Duration.Milliseconds(),
			"error", err,
		)
		return result, err
	}

	level := slog.LevelInfo
	if timedOut || cancelled || result.ExitCode != 0 {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, "shell command completed",
		"cwd", workingDir,
		"exit_code", result.ExitCode,
		"timed_out", result.TimedOut,
		"cancelled", result.Cancelled,
		"stdout_bytes", len(result.Stdout),
		"stderr_bytes", len(result.Stderr),
		"duration_ms", result.Duration.Milliseconds(),
	)
	return result, nil
}

// ExecStream executes a command and streams output chunks via callback in real time.
// Unlike Exec, stdout and stderr are not buffered in memory; each line is delivered
// immediately through onChunk. The returned ExecResult has empty Stdout/Stderr fields
// because all output has already been streamed.
func (r Runner) ExecStream(ctx context.Context, request ExecRequest, onChunk StreamCallback) (ExecResult, error) {
	if request.Command == "" {
		return ExecResult{}, errors.New("command is required")
	}
	if onChunk == nil {
		return r.Exec(ctx, request)
	}

	workingDir := request.CWD
	if workingDir == "" {
		workingDir = "."
	}
	logger := r.logger
	if logger == nil {
		logger = slog.Default()
	}

	logger.Info("executing shell command (streaming)",
		"cwd", workingDir,
		"timeout", request.Timeout.String(),
		"env_keys", len(request.Env),
		"command", request.Command,
	)

	startedAt := time.Now()
	if request.Timeout > 0 {
		var timeoutCancel context.CancelFunc
		ctx, timeoutCancel = context.WithTimeout(ctx, request.Timeout)
		defer timeoutCancel()
	}

	command := buildCommand(ctx, request.Command)
	command.Dir = request.CWD
	command.Env = mergeEnv(request.Env)

	stdoutPipe, err := command.StdoutPipe()
	if err != nil {
		return ExecResult{}, err
	}
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return ExecResult{}, err
	}

	if err := command.Start(); err != nil {
		logger.Error("failed to start shell command (streaming)",
			"cwd", workingDir,
			"error", err,
		)
		return ExecResult{}, err
	}

	// Monitor context: kill the entire process tree when context expires.
	// This ensures child processes (e.g. sleep) are killed and pipe readers
	// unblock promptly instead of waiting for children to exit naturally.
	processDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			killProcessTree(command, logger)
		case <-processDone:
			// Process exited normally; nothing to kill.
		}
	}()

	var wg sync.WaitGroup
	var stdoutBytes, stderrBytes int64

	wg.Add(2)
	go func() {
		defer wg.Done()
		stdoutBytes = streamPipe(stdoutPipe, "stdout", onChunk)
	}()
	go func() {
		defer wg.Done()
		stderrBytes = streamPipe(stderrPipe, "stderr", onChunk)
	}()

	// Wait for all pipe readers to finish before calling Wait.
	wg.Wait()

	waitErr := command.Wait()

	// Signal the kill goroutine that the process has exited.
	close(processDone)

	cancelled := errors.Is(ctx.Err(), context.Canceled)
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)

	result := ExecResult{
		Stdout:    "", // already streamed
		Stderr:    "", // already streamed
		ExitCode:  exitCode(waitErr),
		Duration:  time.Since(startedAt),
		TimedOut:  timedOut,
		Cancelled: cancelled,
	}

	if waitErr != nil && result.ExitCode == 0 && !timedOut && !cancelled {
		logger.Error("shell command failed before exit status was available (streaming)",
			"cwd", workingDir,
			"duration_ms", result.Duration.Milliseconds(),
			"error", waitErr,
		)
		return result, waitErr
	}

	level := slog.LevelInfo
	if timedOut || cancelled || result.ExitCode != 0 {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, "shell command completed (streaming)",
		"cwd", workingDir,
		"exit_code", result.ExitCode,
		"timed_out", result.TimedOut,
		"cancelled", result.Cancelled,
		"stdout_bytes", stdoutBytes,
		"stderr_bytes", stderrBytes,
		"duration_ms", result.Duration.Milliseconds(),
	)
	return result, nil
}

// streamPipe reads from a pipe line-by-line and sends each line as an OutputChunk.
// Returns the total bytes read.
func streamPipe(pipe io.ReadCloser, stream string, onChunk StreamCallback) int64 {
	var total int64
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1MB per line
	for scanner.Scan() {
		line := scanner.Text() + "\n"
		total += int64(len(line))
		onChunk(OutputChunk{Stream: stream, Data: line})
	}
	return total
}

func buildCommand(ctx context.Context, command string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-lc", command)
	}
	// Disable CommandContext's default kill behavior.
	// We handle killing the entire process tree ourselves via killProcessTree.
	cmd.Cancel = func() error { return nil }
	// Set platform-specific process attributes for process group management.
	setProcAttr(cmd)
	return cmd
}

func mergeEnv(customEnv map[string]string) []string {
	merged := os.Environ()
	for key, value := range customEnv {
		merged = append(merged, key+"="+value)
	}
	return merged
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
