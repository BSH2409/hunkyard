package feedback

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func privateDirectory(path string) error {
	if err := makeDirectories(path); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

func makeDirectories(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err := makeDirectories(filepath.Dir(path)); err != nil {
			return err
		}
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private state directory must not be a symlink or non-directory")
	}
	return nil
}

func withLock(path string, fn func() error) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("state lock must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := lockFile(file); err != nil {
		return err
	}
	defer unlockFile(file)
	return fn()
}

func atomicFile(path string, content []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	_, err = temp.Write(content)
	if err == nil {
		err = temp.Sync()
	}
	err = errors.Join(err, temp.Close())
	if err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
