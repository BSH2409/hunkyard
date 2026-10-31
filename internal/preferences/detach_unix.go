//go:build !windows

package preferences

import (
	"os/exec"
	"syscall"
)

func detachProcess(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
