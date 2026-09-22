//go:build !darwin

package app

import "context"

func cleanOrphanKeychainItems(_ context.Context, _ []string) int {
	return 0
}
