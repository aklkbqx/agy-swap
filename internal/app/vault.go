package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// AccountVault stores long-lived account tokens outside accounts.json. The
// file keeps only metadata and a stable reference, so backups and diagnostics
// do not accidentally expose bearer credentials.
type AccountVault interface {
	Get(context.Context, string) (string, bool)
	Set(context.Context, string, string) bool
	Delete(context.Context, string) bool
}

type osAccountVault struct{}

func (osAccountVault) Get(ctx context.Context, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	value := platformVaultGet(ctx, ref)
	return value, value != ""
}

func (osAccountVault) Set(ctx context.Context, ref, token string) bool {
	return strings.TrimSpace(ref) != "" && token != "" && platformVaultSet(ctx, ref, token)
}

func (osAccountVault) Delete(ctx context.Context, ref string) bool {
	if strings.TrimSpace(ref) == "" {
		return true
	}
	return platformVaultDelete(ctx, ref)
}

type fileAccountVault struct {
	path string
	mu   sync.RWMutex
}

func NewFileAccountVault(path string) AccountVault {
	return &fileAccountVault{path: path}
}

func (v *fileAccountVault) readMapUnlocked() map[string]string {
	data, err := os.ReadFile(v.path)
	if err != nil {
		return make(map[string]string)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil || m == nil {
		return make(map[string]string)
	}
	return m
}

func (v *fileAccountVault) Get(_ context.Context, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	m := v.readMapUnlocked()
	val, ok := m[ref]
	return val, ok && val != ""
}

func (v *fileAccountVault) Set(_ context.Context, ref, token string) bool {
	ref = strings.TrimSpace(ref)
	token = strings.TrimSpace(token)
	if ref == "" || token == "" {
		return false
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	dir := filepath.Dir(v.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false
	}
	_ = os.Chmod(dir, 0700)

	m := v.readMapUnlocked()
	m[ref] = token
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return false
	}

	tmpFile := filepath.Join(dir, fmt.Sprintf(".vault-%d.tmp", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return false
	}
	_ = os.Chmod(tmpFile, 0600)
	if err := os.Rename(tmpFile, v.path); err != nil {
		_ = os.Remove(tmpFile)
		return false
	}
	_ = os.Chmod(v.path, 0600)
	return true
}

func (v *fileAccountVault) Delete(_ context.Context, ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return true
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	m := v.readMapUnlocked()
	if _, ok := m[ref]; !ok {
		return true
	}
	delete(m, ref)

	dir := filepath.Dir(v.path)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return false
	}

	tmpFile := filepath.Join(dir, fmt.Sprintf(".vault-%d.tmp", time.Now().UnixNano()))
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return false
	}
	_ = os.Chmod(tmpFile, 0600)
	if err := os.Rename(tmpFile, v.path); err != nil {
		_ = os.Remove(tmpFile)
		return false
	}
	_ = os.Chmod(v.path, 0600)
	return true
}

type hybridVault struct {
	primary  AccountVault
	fallback AccountVault
}

func (h *hybridVault) Get(ctx context.Context, ref string) (string, bool) {
	if val, ok := h.primary.Get(ctx, ref); ok && val != "" {
		return val, true
	}
	if h.fallback != nil {
		if val, ok := h.fallback.Get(ctx, ref); ok && val != "" {
			_ = h.primary.Set(ctx, ref, val)
			return val, true
		}
	}
	return "", false
}

func (h *hybridVault) Set(ctx context.Context, ref, token string) bool {
	ok := h.primary.Set(ctx, ref, token)
	if ok && h.fallback != nil {
		_ = h.fallback.Delete(ctx, ref)
	}
	return ok
}

func (h *hybridVault) Delete(ctx context.Context, ref string) bool {
	pOk := h.primary.Delete(ctx, ref)
	fOk := true
	if h.fallback != nil {
		fOk = h.fallback.Delete(ctx, ref)
	}
	return pOk || fOk
}

func NewAccountVault(paths ...Paths) AccountVault {
	var p Paths
	if len(paths) > 0 {
		p = paths[0]
	} else {
		p, _ = defaultPaths()
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("AGY_SWAP_VAULT")))
	if mode == "keychain" || mode == "os" {
		return osAccountVault{}
	}
	configDir := p.ConfigDir
	if configDir == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			configDir = filepath.Join(home, ".gemini", "agy-swap")
		}
	}
	vaultPath := filepath.Join(configDir, "vault.json")
	fileVault := NewFileAccountVault(vaultPath)
	if mode == "file" {
		return fileVault
	}
	return &hybridVault{primary: fileVault, fallback: osAccountVault{}}
}

func accountSecretRef(email string) string {
	email = normalizeEmail(email)
	if email == "" {
		return ""
	}
	return "account:" + email
}

func hashToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func accountToken(ctx context.Context, account Account, vault AccountVault) (string, error) {
	if ref := getString(account, "secret_ref"); ref != "" {
		if vault == nil {
			return "", fmt.Errorf("account secret vault is unavailable")
		}
		if token, ok := vault.Get(ctx, ref); ok && token != "" {
			return token, nil
		}
		// If ref has legacy nonce suffix (e.g. account:email:nonce), try deterministic ref as well
		email := getString(account, "email")
		if detRef := accountSecretRef(email); detRef != "" && detRef != ref {
			if token, ok := vault.Get(ctx, detRef); ok && token != "" {
				return token, nil
			}
		}
		return "", fmt.Errorf("secret vault entry %q is unavailable", ref)
	}
	if token := getString(account, "token_data"); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("account has no saved token")
}

func (a *Application) accountToken(ctx context.Context, account Account) (string, error) {
	return accountToken(ctx, account, a.vault)
}

func (a *Application) saveAccountSecret(ctx context.Context, account Account, token string) bool {
	email := getString(account, "email")
	token = strings.TrimSpace(token)
	if email == "" || a.vault == nil || token == "" {
		if token != "" {
			account["token_data"] = token
			account["token_hash"] = hashToken(token)
		}
		delete(account, "secret_ref")
		return false
	}
	oldRef := getString(account, "secret_ref")
	ref := accountSecretRef(email)
	if !a.vault.Set(ctx, ref, token) {
		account["token_data"] = token
		account["token_hash"] = hashToken(token)
		delete(account, "secret_ref")
		return false
	}
	if oldRef != "" && oldRef != ref {
		_ = a.vault.Delete(ctx, oldRef)
	}
	account["secret_ref"] = ref
	account["token_hash"] = hashToken(token)
	delete(account, "token_data")
	return true
}

func (a *Application) migrateVaultAccounts(ctx context.Context, accounts *Accounts) bool {
	if accounts == nil || accounts.Len() == 0 || a.vault == nil {
		return false
	}
	changed := false
	var activeRefs []string
	for _, email := range accounts.Order {
		acc := accounts.ByEmail[email]
		detRef := accountSecretRef(email)
		activeRefs = append(activeRefs, detRef)
		oldRef := getString(acc, "secret_ref")
		tokenHash := getString(acc, "token_hash")

		if oldRef != "" && (oldRef != detRef || tokenHash == "") {
			token, err := a.accountToken(ctx, acc)
			if err == nil && token != "" {
				if a.vault.Set(ctx, detRef, token) {
					if oldRef != detRef {
						_ = a.vault.Delete(ctx, oldRef)
					}
					acc["secret_ref"] = detRef
					acc["token_hash"] = hashToken(token)
					delete(acc, "token_data")
					changed = true
				}
			}
		} else if oldRef == "" {
			if token := getString(acc, "token_data"); token != "" {
				if a.saveAccountSecret(ctx, acc, token) {
					changed = true
				}
			}
		}
	}
	go func() {
		cleanCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cleanOrphanKeychainItems(cleanCtx, activeRefs)
	}()

	if changed && a.store != nil {
		_ = a.store.Save(accounts)
	}
	return changed
}
