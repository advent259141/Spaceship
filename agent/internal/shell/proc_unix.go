//go:build !windows

package shell

import (
	"errors"
	"log/slog"
	"os/exec"
	"syscall"
)

// setProcAttr creates a new process group so we can kill all child processes on timeout.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessTree sends SIGKILL to the entire process group.
func killProcessTree(cmd *exec.Cmd, logger *slog.Logger) {
	if cmd.Process == nil {
		return
	}
	pgid := -cmd.Process.Pid
	if err := syscall.Kill(pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		logger.Warn("failed to kill process group", "pgid", pgid, "error", err)
	}
}
