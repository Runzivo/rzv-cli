//go:build !windows

package config

import (
	"errors"
	"os"
	"syscall"
)

// openNoFollow refuses a lock file that is a symbolic link.
const openNoFollow = syscall.O_NOFOLLOW

func tryLockFile(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return false, nil
	}
	return err == nil, err
}

func unlockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
