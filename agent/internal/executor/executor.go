package executor

import (
	"context"
	"encoding/json"
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
	Cancelled  bool
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

func (d Dispatcher) Dispatch(ctx context.Context, task protocol.TaskSpec) (Result, error) {
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
		result, err := d.runner.Exec(ctx, request)
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
			Cancelled:  result.Cancelled,
			Truncated:  false,
			DurationMS: result.Duration.Milliseconds(),
		}
		d.logger.Info("task dispatch completed",
			"task_id", task.TaskID,
			"task_type", task.TaskType,
			"exit_code", final.ExitCode,
			"timed_out", final.TimedOut,
			"cancelled", final.Cancelled,
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
	case "edit_file":
		edits, err := parseEdits(task.Args)
		if err != nil {
			d.logger.Error("edit_file task failed: invalid edits",
				"task_id", task.TaskID,
				"error", err,
			)
			return Result{}, err
		}
		request := fileops.EditRequest{
			Path:  stringArg(task.Args, "path"),
			Edits: edits,
		}
		content, err := d.fileops.EditFile(request)
		if err != nil {
			d.logger.Error("edit_file task failed",
				"task_id", task.TaskID,
				"path", request.Path,
				"error", err,
			)
			return Result{}, err
		}
		result := Result{Stdout: content}
		d.logger.Info("edit_file task completed",
			"task_id", task.TaskID,
			"path", request.Path,
			"edits_count", len(edits),
			"result_bytes", len(content),
		)
		return result, nil
	case "grep":
		request := fileops.GrepRequest{
			Pattern:         stringArg(task.Args, "pattern"),
			Path:            stringArg(task.Args, "path"),
			IsRegex:         boolArg(task.Args, "is_regex"),
			CaseInsensitive: boolArg(task.Args, "case_insensitive"),
			IncludeGlobs:    stringSliceArg(task.Args, "include_globs"),
			ExcludeGlobs:    stringSliceArg(task.Args, "exclude_globs"),
			MaxMatches:      intArg(task.Args, "max_matches", 100),
		}
		content, err := d.fileops.Grep(request)
		if err != nil {
			d.logger.Error("grep task failed",
				"task_id", task.TaskID,
				"path", request.Path,
				"pattern", request.Pattern,
				"error", err,
			)
			return Result{}, err
		}
		result := Result{Stdout: content}
		d.logger.Info("grep task completed",
			"task_id", task.TaskID,
			"path", request.Path,
			"pattern", request.Pattern,
			"is_regex", request.IsRegex,
			"result_bytes", len(content),
		)
		return result, nil
	case "delete_file":
		request := fileops.DeleteRequest{
			Path:      stringArg(task.Args, "path"),
			Recursive: boolArg(task.Args, "recursive"),
		}
		content, err := d.fileops.Delete(request)
		if err != nil {
			d.logger.Error("delete_file task failed",
				"task_id", task.TaskID,
				"path", request.Path,
				"error", err,
			)
			return Result{}, err
		}
		result := Result{Stdout: content}
		d.logger.Info("delete_file task completed",
			"task_id", task.TaskID,
			"path", request.Path,
			"recursive", request.Recursive,
		)
		return result, nil
	case "move_file":
		request := fileops.MoveRequest{
			Src:       stringArg(task.Args, "src"),
			Dst:       stringArg(task.Args, "dst"),
			Overwrite: boolArg(task.Args, "overwrite"),
		}
		content, err := d.fileops.Move(request)
		if err != nil {
			d.logger.Error("move_file task failed",
				"task_id", task.TaskID,
				"src", request.Src,
				"dst", request.Dst,
				"error", err,
			)
			return Result{}, err
		}
		result := Result{Stdout: content}
		d.logger.Info("move_file task completed",
			"task_id", task.TaskID,
			"src", request.Src,
			"dst", request.Dst,
		)
		return result, nil
	case "copy_file":
		request := fileops.CopyRequest{
			Src:       stringArg(task.Args, "src"),
			Dst:       stringArg(task.Args, "dst"),
			Recursive: boolArg(task.Args, "recursive"),
		}
		content, err := d.fileops.Copy(request)
		if err != nil {
			d.logger.Error("copy_file task failed",
				"task_id", task.TaskID,
				"src", request.Src,
				"dst", request.Dst,
				"error", err,
			)
			return Result{}, err
		}
		result := Result{Stdout: content}
		d.logger.Info("copy_file task completed",
			"task_id", task.TaskID,
			"src", request.Src,
			"dst", request.Dst,
			"recursive", request.Recursive,
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

// DispatchStream dispatches tasks with streaming output support.
// For exec tasks, output is streamed in real time via onChunk.
// For all other task types, falls back to Dispatch and delivers the
// complete result as a single chunk through onChunk.
func (d Dispatcher) DispatchStream(ctx context.Context, task protocol.TaskSpec, onChunk shell.StreamCallback) (Result, error) {
	if task.TaskType != "exec" {
		// Non-exec tasks produce output in one shot; dispatch normally
		// and deliver the result as bulk chunks.
		result, err := d.Dispatch(ctx, task)
		if err != nil {
			return result, err
		}
		if onChunk != nil {
			if result.Stdout != "" {
				onChunk(shell.OutputChunk{Stream: "stdout", Data: result.Stdout})
			}
			if result.Stderr != "" {
				onChunk(shell.OutputChunk{Stream: "stderr", Data: result.Stderr})
			}
		}
		// Clear the fields since they have been forwarded via callback.
		result.Stdout = ""
		result.Stderr = ""
		return result, nil
	}

	// exec task: stream shell output in real time.
	d.logger.Info("dispatching task (streaming)",
		"task_id", task.TaskID,
		"task_type", task.TaskType,
		"timeout_sec", task.TimeoutSec,
	)

	request := shell.ExecRequest{
		Command:    stringArg(task.Args, "command"),
		CWD:        stringArg(task.Args, "cwd"),
		Env:        stringMapArg(task.Args, "env"),
		Timeout:    time.Duration(task.TimeoutSec) * time.Second,
		Background: boolArg(task.Args, "background"),
	}
	result, err := d.runner.ExecStream(ctx, request, onChunk)
	if err != nil {
		d.logger.Error("task dispatch (streaming) failed",
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
		Cancelled:  result.Cancelled,
		Truncated:  false,
		DurationMS: result.Duration.Milliseconds(),
	}
	d.logger.Info("task dispatch (streaming) completed",
		"task_id", task.TaskID,
		"task_type", task.TaskType,
		"exit_code", final.ExitCode,
		"timed_out", final.TimedOut,
		"cancelled", final.Cancelled,
		"duration_ms", final.DurationMS,
	)
	return final, nil
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

func stringSliceArg(args map[string]any, key string) []string {
	value, ok := args[key]
	if !ok || value == nil {
		return nil
	}
	rawSlice, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(rawSlice))
	for _, item := range rawSlice {
		text, _ := item.(string)
		if text != "" {
			result = append(result, text)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func parseEdits(args map[string]any) ([]fileops.EditOp, error) {
	raw, ok := args["edits"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("edits argument is required")
	}

	// The edits arrive as []any from JSON unmarshal.
	// Re-marshal and unmarshal to get typed structs.
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to encode edits: %w", err)
	}
	var edits []fileops.EditOp
	if err := json.Unmarshal(encoded, &edits); err != nil {
		return nil, fmt.Errorf("failed to parse edits: %w", err)
	}
	if len(edits) == 0 {
		return nil, fmt.Errorf("edits array is empty")
	}
	return edits, nil
}
