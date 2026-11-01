//go:build windows

package hooks

import "os/exec"

// stopWholeHook keeps the default kill on Windows; WaitDelay bounds the wait.
func stopWholeHook(*exec.Cmd) {}
