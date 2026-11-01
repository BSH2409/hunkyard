//go:build windows

package main

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// addressInUse reports whether listening failed because the port is taken.
// Windows reports it as WSAEADDRINUSE, not syscall.EADDRINUSE.
func addressInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, windows.WSAEADDRINUSE)
}
