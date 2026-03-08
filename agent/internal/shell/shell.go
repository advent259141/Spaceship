package shell

import (
	"bytes"
	"context"
	"errors"
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

type Runner struct{}

func (Runner) Exec(request ExecRequest) (ExecResult, error) {
	if request.Command == "" {
		return ExecResult{}, errors.New("command is required")
	}

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
			return ExecResult{}, err
		}
		return ExecResult{
			PID:      command.Process.Pid,
			ExitCode: 0,
			Duration: time.Since(startedAt),
		}, nil
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
		return result, err
	}
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
