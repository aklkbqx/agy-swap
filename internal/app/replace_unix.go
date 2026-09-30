//go:build !windows

package app

import "github.com/aklkbqx/agy-swap/internal/store"

func replaceFile(source, target string) error { return store.ReplaceFile(source, target) }
