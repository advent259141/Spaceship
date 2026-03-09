package shell

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
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
	Stdout   string
	Stderr   string
	ExitCode int
	PID      int
	TimedOut bool
	Duration time.Duration
}

type Runner struct {
	logger *slog.Logger
}

func NewRunner(logger *slog.Logger) Runner {
	if logger == nil {
		logger = slog.Default()
	}
	return Runner{logger: logger}
}

func (r Runner) Exec(request ExecRequest) (ExecResult, error) {
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
	ctx := context.Background()
	cancel := func() {}
	if request.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, request.Timeout)
	}
	defer cancel()

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
	result := ExecResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(err),
		Duration: time.Since(startedAt),
		TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded),
	}

	if err != nil && result.ExitCode == 0 && !result.TimedOut {
		logger.Error("shell command failed before exit status was available",
			"cwd", workingDir,
			"duration_ms", result.Duration.Milliseconds(),
			"error", err,
		)
		return result, err
	}

	level := slog.LevelInfo
	if result.TimedOut || result.ExitCode != 0 {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, "shell command completed",
		"cwd", workingDir,
		"exit_code", result.ExitCode,
		"timed_out", result.TimedOut,
		"stdout_bytes", len(result.Stdout),
		"stderr_bytes", len(result.Stderr),
		"duration_ms", result.Duration.Milliseconds(),
	)
	return result, nil
}

func buildCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/C", command)
	}
	return exec.CommandContext(ctx, "/bin/sh", "-lc", command)
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
