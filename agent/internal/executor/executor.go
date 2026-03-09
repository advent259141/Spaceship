package executor

import (
	"fmt"
	"log/slog"
	"time"

	"spaceship/agent/internal/fileops"
	"spaceship/agent/internal/protocol"
	"spaceship/agent/internal/shell"
)

type Result struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	PID        int
	TimedOut   bool
	Truncated  bool
	DurationMS int64
}

type Dispatcher struct {
	logger  *slog.Logger
	runner  shell.Runner
	fileops fileops.Service
}

func NewDispatcher(logger *slog.Logger, runner shell.Runner) Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return Dispatcher{
		logger:  logger,
		runner:  runner,
		fileops: fileops.Service{},
	}
}

func (d Dispatcher) Dispatch(task protocol.TaskSpec) (Result, error) {
	d.logger.Info("dispatching task",
		"task_id", task.TaskID,
		"task_type", task.TaskType,
		"timeout_sec", task.TimeoutSec,
		"max_output_bytes", task.MaxOutputBytes,
	)

	switch task.TaskType {
	case "exec":
		request := shell.ExecRequest{
			Command:    stringArg(task.Args, "command"),
			CWD:        stringArg(task.Args, "cwd"),
			Env:        stringMapArg(task.Args, "env"),
			Timeout:    time.Duration(task.TimeoutSec) * time.Second,
			Background: boolArg(task.Args, "background"),
		}
		result, err := d.runner.Exec(request)
		if err != nil {
			d.logger.Error("task dispatch failed",
				"task_id", task.TaskID,
				"task_type", task.TaskType,
				"error", err,
			)
			return Result{}, err
		}
		final := Result{
			Stdout:     result.Stdout,
			Stderr:     result.Stderr,
			ExitCode:   result.ExitCode,
			PID:        result.PID,
			TimedOut:   result.TimedOut,
			Truncated:  false,
			DurationMS: result.Duration.Milliseconds(),
		}
		d.logger.Info("task dispatch completed",
			"task_id", task.TaskID,
			"task_type", task.TaskType,
			"exit_code", final.ExitCode,
			"timed_out", final.TimedOut,
			"stdout_bytes", len(final.Stdout),
			"stderr_bytes", len(final.Stderr),
			"duration_ms", final.DurationMS,
		)
		return final, nil
	case "read_file":
		request := fileops.ReadRequest{
			Path:     stringArg(task.Args, "path"),
			MaxBytes: intArg(task.Args, "max_bytes", task.MaxOutputBytes),
		}
		content, truncated, err := d.fileops.Read(request)
		if err != nil {
			d.logger.Error("read_file task failed",
				"task_id", task.TaskID,
				"path", request.Path,
				"error", err,
			)
			return Result{}, err
		}
		result := Result{Stdout: content, Truncated: truncated}
		d.logger.Info("read_file task completed",
			"task_id", task.TaskID,
			"path", request.Path,
			"bytes", len(content),
			"truncated", truncated,
		)
		return result, nil
	case "list_dir":
		request := fileops.ListDirRequest{
			Path:       stringArg(task.Args, "path"),
			Recursive:  boolArg(task.Args, "recursive"),
			ShowHidden: boolArg(task.Args, "show_hidden"),
			Limit:      intArg(task.Args, "limit", 200),
		}
		content, truncated, err := d.fileops.ListDir(request)
		if err != nil {
			d.logger.Error("list_dir task failed",
				"task_id", task.TaskID,
				"path", request.Path,
				"recursive", request.Recursive,
				"error", err,
			)
			return Result{}, err
		}
		result := Result{Stdout: content, Truncated: truncated}
		d.logger.Info("list_dir task completed",
			"task_id", task.TaskID,
			"path", request.Path,
			"recursive", request.Recursive,
			"show_hidden", request.ShowHidden,
			"limit", request.Limit,
			"bytes", len(content),
			"truncated", truncated,
		)
		return result, nil
	case "write_file":
		request := fileops.WriteRequest{
			Path:       stringArg(task.Args, "path"),
			Content:    stringArg(task.Args, "content"),
			Append:     boolArg(task.Args, "append"),
			CreateDirs: boolArgWithDefault(task.Args, "create_dirs", true),
		}
		content, err := d.fileops.Write(request)
		if err != nil {
			d.logger.Error("write_file task failed",
				"task_id", task.TaskID,
				"path", request.Path,
				"append", request.Append,
				"error", err,
			)
			return Result{}, err
		}
		result := Result{Stdout: content}
		d.logger.Info("write_file task completed",
			"task_id", task.TaskID,
			"path", request.Path,
			"append", request.Append,
			"content_bytes", len(request.Content),
			"result_bytes", len(content),
		)
		return result, nil
	default:
		err := fmt.Errorf("unsupported task type: %s", task.TaskType)
		d.logger.Error("task dispatch failed",
			"task_id", task.TaskID,
			"task_type", task.TaskType,
			"error", err,
		)
		return Result{}, err
	}
}

func stringArg(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	text, _ := value.(string)
	return text
}

func boolArg(args map[string]any, key string) bool {
	value, ok := args[key]
	if !ok || value == nil {
		return false
	}
	flag, _ := value.(bool)
	return flag
}

func boolArgWithDefault(args map[string]any, key string, fallback bool) bool {
	value, ok := args[key]
	if !ok || value == nil {
		return fallback
	}
	flag, ok := value.(bool)
	if !ok {
		return fallback
	}
	return flag
}

func intArg(args map[string]any, key string, fallback int) int {
	value, ok := args[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func stringMapArg(args map[string]any, key string) map[string]string {
	value, ok := args[key]
	if !ok || value == nil {
		return nil
	}
	rawMap, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := make(map[string]string, len(rawMap))
	for mapKey, mapValue := range rawMap {
		text, _ := mapValue.(string)
		result[mapKey] = text
	}
	return result
}
