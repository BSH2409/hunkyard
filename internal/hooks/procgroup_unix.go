//go:build !windows

package hooks

import (
	"os/exec"
	"syscall"
)

// stopWholeHook runs the hook in its own process group and makes a timeout
// kill that group, so a shell's children (`sleep`, a script's subprocesses)
// stop with it instead of holding the output pipe open.
func stopWholeHook(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
