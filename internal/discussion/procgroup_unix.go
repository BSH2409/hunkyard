//go:build !windows

package discussion

import (
	"os/exec"
	"syscall"
)

func stopWholeRequest(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
}
