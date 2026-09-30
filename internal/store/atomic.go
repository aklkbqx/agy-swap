package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/aklkbqx/agy-swap/internal/config"
)

// EnsurePrivateDir creates a directory with private permissions (0700 on POSIX).
func EnsurePrivateDir(path string) error {
	if err := os.MkdirAll(path, config.PrivateDirMode()); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return os.Chmod(path, 0o700)
	}
	return nil
}

// AtomicWrite writes data to a temporary file in the target directory, fsyncs, and replaces atomically.
func AtomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, config.PrivateDirMode()); err != nil {
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
	if err := ReplaceFile(tmpName, path); err != nil {
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

// AtomicWriteJSON marshals value to JSON and performs an atomic write with 0600 mode.
func AtomicWriteJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return AtomicWrite(path, data, 0o600)
}

// FileSnapshot maps file paths to their backup byte content (or nil if non-existent).
type FileSnapshot map[string][]byte

// SnapshotFiles takes in-memory snapshots of the specified files.
func SnapshotFiles(paths ...string) (FileSnapshot, error) {
	snapshot := make(FileSnapshot, len(paths))
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

// RestoreFiles writes backed up content back or removes files created after the snapshot.
func RestoreFiles(snapshot FileSnapshot) bool {
	ok := true
	for path, data := range snapshot {
		if data == nil {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				ok = false
			}
			continue
		}
		if err := AtomicWrite(path, data, 0o600); err != nil {
			ok = false
		}
	}
	return ok
}
