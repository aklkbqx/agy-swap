package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, privateDirMode()); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(path, 0o700)
	}
	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := ensurePrivateDir(dir); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if runtime.GOOS != "windows" {
		if err := tmp.Chmod(mode); err != nil {
			_ = tmp.Close()
			return err
		}
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := replaceFile(tmpName, path); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, mode); err != nil {
			return err
		}
		if dirHandle, err := os.Open(dir); err == nil {
			_ = dirHandle.Sync()
			_ = dirHandle.Close()
		}
	}
	return nil
}

func atomicWriteJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return atomicWrite(path, data, 0o600)
}

type fileSnapshot map[string][]byte

func snapshotFiles(paths ...string) (fileSnapshot, error) {
	snapshot := make(fileSnapshot, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			snapshot[path] = nil
			continue
		}
		if err != nil {
			return nil, err
		}
		snapshot[path] = data
	}
	return snapshot, nil
}

func restoreFiles(snapshot fileSnapshot) bool {
	ok := true
	for path, data := range snapshot {
		if data == nil {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				ok = false
			}
			continue
		}
		if err := atomicWrite(path, data, 0o600); err != nil {
			ok = false
		}
	}
	return ok
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
