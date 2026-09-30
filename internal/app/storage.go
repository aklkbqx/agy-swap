package app

import (
	"fmt"
	"io"
	"os"

	"github.com/aklkbqx/agy-swap/internal/store"
)

func ensurePrivateDir(path string) error {
	return store.EnsurePrivateDir(path)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	return store.AtomicWrite(path, data, mode)
}

func atomicWriteJSON(path string, value any) error {
	return store.AtomicWriteJSON(path, value)
}

type fileSnapshot = store.FileSnapshot

func snapshotFiles(paths ...string) (fileSnapshot, error) {
	return store.SnapshotFiles(paths...)
}

func restoreFiles(snapshot fileSnapshot) bool {
	return store.RestoreFiles(snapshot)
}

func readLimited(path string, max int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("file exceeds %d bytes", max)
	}
	return data, nil
}
