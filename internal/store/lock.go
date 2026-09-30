package store

import "io"

// FileLock represents an advisory lock held on a specific file path.
type FileLock interface {
	io.Closer
}
