//go:build darwin && cgo

package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

// A keychain item carries a partition list derived from the code identity that
// created it, and no ACL entry overrides it. Reading through a helper binary
// therefore blocks on the authorization dialog forever. These checks run
// against the real keychain, so they stay opt-in:
//
//	AGY_SWAP_KEYCHAIN_PROBE=1 go test -run TestKeychain ./internal/app
func probeName(t *testing.T) string {
	t.Helper()
	// A leftover item would be updated in place, keeping the identity that
	// created it and hiding the very thing under test.
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatalf("cannot draw a unique probe name: %v", err)
	}
	return "probe-" + hex.EncodeToString(suffix[:])
}

func requireProbe(t *testing.T) {
	t.Helper()
	if os.Getenv("AGY_SWAP_KEYCHAIN_PROBE") != "1" {
		t.Skip("set AGY_SWAP_KEYCHAIN_PROBE=1 to touch the real keychain")
	}
}

func TestKeychainRoundTripsInProcess(t *testing.T) {
	requireProbe(t)
	const service, value = "agy-swap-probe", "probe-value"
	account := probeName(t)
	ctx := context.Background()
	t.Cleanup(func() { keychainDelete(ctx, service, account) })

	if !keychainSet(ctx, service, account, value) {
		t.Fatal("keychainSet reported failure")
	}
	if got := keychainGet(ctx, service, account); got != value {
		t.Fatalf("keychainGet returned %q, want %q", got, value)
	}
	if !keychainDelete(ctx, service, account) {
		t.Fatal("keychainDelete reported failure")
	}
	if got := keychainGet(ctx, service, account); got != "" {
		t.Fatalf("keychainGet returned %q after delete, want empty", got)
	}
}
