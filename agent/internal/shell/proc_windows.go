//go:build windows

package shell

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
)

// setProcAttr is a no-op on Windows (no process group setup needed).
func setProcAttr(cmd *exec.Cmd) {
	// Windows does not support Setpgid.
}

// killProcessTree kills the process directly on Windows.
func killProcessTree(cmd *exec.Cmd, logger *slog.Logger) {
	if cmd.Process == nil {
		return
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		logger.Warn("failed to kill process", "pid", cmd.Process.Pid, "error", err)
	}
}
