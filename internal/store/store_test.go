package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteAndReplace(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "test.txt")

	data1 := []byte("hello world")
	if err := AtomicWrite(target, data1, 0o600); err != nil {
		t.Fatalf("AtomicWrite failed: %v", err)
	}

	read, err := os.ReadFile(target)
	if err != nil || string(read) != "hello world" {
		t.Fatalf("read mismatch: %s, %v", string(read), err)
	}

	// Update atomically
	data2 := []byte("updated content")
	if err := AtomicWrite(target, data2, 0o600); err != nil {
		t.Fatalf("AtomicWrite update failed: %v", err)
	}

	read, err = os.ReadFile(target)
	if err != nil || string(read) != "updated content" {
		t.Fatalf("read update mismatch: %s, %v", string(read), err)
	}
}

func TestSnapshotAndRestore(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "f1.txt")
	f2 := filepath.Join(dir, "f2.txt")

	if err := os.WriteFile(f1, []byte("original f1"), 0o600); err != nil {
		t.Fatal(err)
	}

	snap, err := SnapshotFiles(f1, f2)
	if err != nil {
		t.Fatalf("SnapshotFiles failed: %v", err)
	}

	// Modify f1, create f2
	if err := os.WriteFile(f1, []byte("corrupted f1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("extra f2"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Restore
	if !RestoreFiles(snap) {
		t.Fatal("RestoreFiles reported failure")
	}

	read1, _ := os.ReadFile(f1)
	if string(read1) != "original f1" {
		t.Fatalf("f1 not restored: %s", string(read1))
	}

	if _, err := os.Stat(f2); !os.IsNotExist(err) {
		t.Fatalf("f2 was not removed during restore: %v", err)
	}
}

func TestFileLock(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, ".test.lock")

	lock1, err := AcquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("AcquireFileLock failed: %v", err)
	}

	if err := lock1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Should be able to acquire again
	lock2, err := AcquireFileLock(lockPath)
	if err != nil {
		t.Fatalf("second AcquireFileLock failed: %v", err)
	}
	_ = lock2.Close()
}
