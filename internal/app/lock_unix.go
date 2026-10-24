//go:build !windows

package app

import "github.com/aklkbqx/agy-swap/v2/internal/store"

type fileLock struct{ inner store.FileLock }

func acquireFileLock(path string) (*fileLock, error) {
	lock, err := store.AcquireFileLock(path)
	if err != nil {
		return nil, err
	}
	return &fileLock{inner: lock}, nil
}

func tryAcquireFileLock(path string) (*fileLock, error) {
	lock, err := store.TryAcquireFileLock(path)
	if err != nil {
		return nil, err
	}
	return &fileLock{inner: lock}, nil
}

func (l *fileLock) Close() error {
	if l == nil || l.inner == nil {
		return nil
	}
	return l.inner.Close()
}
