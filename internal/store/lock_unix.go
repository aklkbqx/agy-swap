//go:build !windows

package store

import (
	"os"

	"golang.org/x/sys/unix"
)

type posixFileLock struct{ file *os.File }

// AcquireFileLock obtains an exclusive POSIX advisory lock on the target file.
func AcquireFileLock(path string) (FileLock, error) {
	if err := EnsurePrivateDir(filepathDir(path)); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	_ = file.Chmod(0o600)
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &posixFileLock{file: file}, nil
}

func (l *posixFileLock) Close() error {
	_ = unix.Flock(int(l.file.Fd()), unix.LOCK_UN)
	return l.file.Close()
}

func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return "."
}
