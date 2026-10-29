//go:build !windows

package preferences

import (
	"errors"
	"os"
	"syscall"
)

func lockFile(file *os.File, nonblocking bool) error {
	mode := syscall.LOCK_EX
	if nonblocking {
		mode |= syscall.LOCK_NB
	}
	return syscall.Flock(int(file.Fd()), mode)
}
func unlockFile(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
func lockBusy(err error) bool        { return errors.Is(err, syscall.EWOULDBLOCK) }
func ownedFile(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}
func syncDirectoryFile(file *os.File) error {
	err := file.Sync()
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.EISDIR) {
		return nil
	}
	return err
}
