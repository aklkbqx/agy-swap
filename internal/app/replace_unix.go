//go:build !windows

package app

import "os"

func replaceFile(source, target string) error { return os.Rename(source, target) }
