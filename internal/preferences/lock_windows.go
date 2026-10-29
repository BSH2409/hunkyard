//go:build windows

package preferences

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func lockBusy(err error) bool { return errors.Is(err, windows.ERROR_LOCK_VIOLATION) }

// shortcut: Windows transcript imports stay disabled until ownership is checked through ACLs.
func ownedFile(info os.FileInfo) bool { return false }

func lockFile(file *os.File, nonblocking bool) error {
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if nonblocking {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	return windows.LockFileEx(windows.Handle(file.Fd()), flags, 0, 1, 0, new(windows.Overlapped))
}
func unlockFile(file *os.File) error {
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, new(windows.Overlapped))
}
func syncDirectoryFile(file *os.File) error { return nil }
