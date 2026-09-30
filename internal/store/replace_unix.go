//go:build !windows

package store

import "os"

// ReplaceFile atomically replaces target file with source using POSIX rename.
func ReplaceFile(source, target string) error {
	return os.Rename(source, target)
}
