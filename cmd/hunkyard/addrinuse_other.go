//go:build !windows

package main

import (
	"errors"
	"syscall"
)

// addressInUse reports whether listening failed because the port is taken.
func addressInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
