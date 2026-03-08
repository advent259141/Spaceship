package executor

import (
	"fmt"
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
	runner  shell.Runner
	fileops fileops.Service
}

func NewDispatcher(runner shell.Runner) Dispatcher {
	return Dispatcher{runner: runner, fileops: fileops.Service{}}
}

func (d Dispatcher) Dispatch(task protocol.TaskSpec) (Result, error) {
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
			return Result{}, err
		}
		return Result{
			Stdout:     result.Stdout,
			Stderr:     result.Stderr,
			ExitCode:   result.ExitCode,
			PID:        result.PID,
			TimedOut:   result.TimedOut,
			Truncated:  false,
			DurationMS: result.Duration.Milliseconds(),
		}, nil
	case "read_file":
		content, truncated, err := d.fileops.Read(fileops.ReadRequest{
			Path:     stringArg(task.Args, "path"),
			MaxBytes: intArg(task.Args, "max_bytes", task.MaxOutputBytes),
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Stdout: content, Truncated: truncated}, nil
	case "list_dir":
		content, truncated, err := d.fileops.ListDir(fileops.ListDirRequest{
			Path:       stringArg(task.Args, "path"),
			Recursive:  boolArg(task.Args, "recursive"),
			ShowHidden: boolArg(task.Args, "show_hidden"),
			Limit:      intArg(task.Args, "limit", 200),
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Stdout: content, Truncated: truncated}, nil
	case "write_file":
		content, err := d.fileops.Write(fileops.WriteRequest{
			Path:       stringArg(task.Args, "path"),
			Content:    stringArg(task.Args, "content"),
			Append:     boolArg(task.Args, "append"),
			CreateDirs: boolArgWithDefault(task.Args, "create_dirs", true),
		})
		if err != nil {
			return Result{}, err
		}
		return Result{Stdout: content}, nil
	default:
		return Result{}, fmt.Errorf("unsupported task type: %s", task.TaskType)
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
