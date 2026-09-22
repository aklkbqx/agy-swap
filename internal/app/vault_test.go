package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memoryVault struct {
	data map[string]string
}

func newMemoryVault() *memoryVault {
	return &memoryVault{data: make(map[string]string)}
}

func (m *memoryVault) Get(_ context.Context, ref string) (string, bool) {
	v, ok := m.data[ref]
	return v, ok
}

func (m *memoryVault) Set(_ context.Context, ref, token string) bool {
	m.data[ref] = token
	return true
}

func (m *memoryVault) Delete(_ context.Context, ref string) bool {
	delete(m.data, ref)
	return true
}

func TestFileAccountVault(t *testing.T) {
	tempDir := t.TempDir()
	vaultPath := filepath.Join(tempDir, "vault.json")
	v := NewFileAccountVault(vaultPath)
	ctx := context.Background()

	// Initial get on empty vault
	if val, ok := v.Get(ctx, "account:user@example.com"); ok || val != "" {
		t.Fatalf("expected empty vault, got val=%q ok=%v", val, ok)
	}

	// Set value
	token := `{"access_token":"test-token","refresh_token":"ref-token"}`
	if !v.Set(ctx, "account:user@example.com", token) {
		t.Fatalf("v.Set failed")
	}

	// Verify file permissions
	info, err := os.Stat(vaultPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("expected permissions 0600, got %o", perm)
	}

	// Get value
	got, ok := v.Get(ctx, "account:user@example.com")
	if !ok || got != token {
		t.Fatalf("expected %q, got %q (ok=%v)", token, got, ok)
	}

	// Delete value
	if !v.Delete(ctx, "account:user@example.com") {
		t.Fatalf("v.Delete failed")
	}
	if val, ok := v.Get(ctx, "account:user@example.com"); ok || val != "" {
		t.Fatalf("expected deleted value, got val=%q", val)
	}
}

func TestHybridVaultFallbackAndPromotion(t *testing.T) {
	primary := newMemoryVault()
	fallback := newMemoryVault()
	h := &hybridVault{primary: primary, fallback: fallback}
	ctx := context.Background()

	// Seed fallback with a token
	ref := "account:legacy@example.com"
	secret := "legacy-secret-token"
	fallback.Set(ctx, ref, secret)

	// Get from hybrid should read fallback and promote to primary
	got, ok := h.Get(ctx, ref)
	if !ok || got != secret {
		t.Fatalf("expected %q, got %q", secret, got)
	}

	// Primary should now have it directly
	if pVal, pOk := primary.Get(ctx, ref); !pOk || pVal != secret {
		t.Fatalf("primary was not populated on fallback read: %q, %v", pVal, pOk)
	}

	// Set on hybrid should write primary and remove from fallback
	newSecret := "new-secret-token"
	if !h.Set(ctx, ref, newSecret) {
		t.Fatalf("h.Set failed")
	}
	if pVal, _ := primary.Get(ctx, ref); pVal != newSecret {
		t.Fatalf("primary not updated: %q", pVal)
	}
	if _, fOk := fallback.Get(ctx, ref); fOk {
		t.Fatalf("fallback was not cleaned on set")
	}
}

func TestDeterministicRefAndTokenHash(t *testing.T) {
	email := "USER.Name+tag@Example.Com"
	ref := accountSecretRef(email)
	if ref != "account:user.name+tag@example.com" {
		t.Fatalf("unexpected ref: %q", ref)
	}
	// Verify deterministic: calling multiple times returns identical string (no random nonce)
	for i := 0; i < 5; i++ {
		if r := accountSecretRef(email); r != ref {
			t.Fatalf("ref is not deterministic: %q vs %q", r, ref)
		}
	}

	token := "dummy-oauth-bearer-token-12345"
	hash1 := hashToken(token)
	hash2 := hashToken(token)
	if hash1 == "" || !strings.HasPrefix(hash1, "sha256:") {
		t.Fatalf("invalid hashToken output: %q", hash1)
	}
	if hash1 != hash2 {
		t.Fatalf("hashToken is not deterministic")
	}
}

func TestLocalActiveEmailWithTokenHash(t *testing.T) {
	current := "bearer-token-active-account"
	tokenHash := hashToken(current)

	accounts := NewAccounts()
	accounts.Order = []string{"user1@example.com", "user2@example.com"}
	accounts.ByEmail["user1@example.com"] = Account{
		"email":      "user1@example.com",
		"secret_ref": "account:user1@example.com",
		"token_hash": "sha256:differenthash",
	}
	accounts.ByEmail["user2@example.com"] = Account{
		"email":      "user2@example.com",
		"secret_ref": "account:user2@example.com",
		"token_hash": tokenHash,
	}

	active := localActiveEmail(accounts, current)
	if active != "user2@example.com" {
		t.Fatalf("expected user2@example.com, got %q", active)
	}
}

func TestMigrateVaultAccounts(t *testing.T) {
	tempDir := t.TempDir()
	vaultPath := filepath.Join(tempDir, "vault.json")
	v := NewFileAccountVault(vaultPath)
	ctx := context.Background()

	oldRef := "account:user@example.com:0123456789abcdef0123456789abcdef"
	v.Set(ctx, oldRef, "migrated-token-val")

	accounts := NewAccounts()
	accounts.Order = []string{"user@example.com"}
	accounts.ByEmail["user@example.com"] = Account{
		"email":      "user@example.com",
		"secret_ref": oldRef,
	}

	app := &Application{
		vault: v,
		store: NewStore(Paths{Accounts: filepath.Join(tempDir, "accounts.json")}),
	}

	migrated := app.migrateVaultAccounts(ctx, accounts)
	if !migrated {
		t.Fatalf("expected accounts to be migrated")
	}

	acc := accounts.ByEmail["user@example.com"]
	detRef := "account:user@example.com"
	if r := getString(acc, "secret_ref"); r != detRef {
		t.Fatalf("expected secret_ref %q, got %q", detRef, r)
	}
	if h := getString(acc, "token_hash"); h != hashToken("migrated-token-val") {
		t.Fatalf("expected token_hash %q, got %q", hashToken("migrated-token-val"), h)
	}

	// Verify old ref is deleted from vault
	if _, ok := v.Get(ctx, oldRef); ok {
		t.Fatalf("old nonce ref was not deleted from vault")
	}
	// Verify deterministic ref is in vault
	if val, ok := v.Get(ctx, detRef); !ok || val != "migrated-token-val" {
		t.Fatalf("deterministic ref missing from vault: %q, %v", val, ok)
	}
}
