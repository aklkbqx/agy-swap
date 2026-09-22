package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func (v *fileAccountVault) lockPath() string {
	return v.path + ".lock"
}

// fileLock serializes readers and writers across processes. create is set for
// mutations so the config directory exists; lookups leave a missing directory
// untouched and report the entry as absent.
func (v *fileAccountVault) fileLock(create bool) (*fileLock, error) {
	dir := filepath.Dir(v.path)
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		_ = os.Chmod(dir, 0o700)
	} else if _, err := os.Stat(dir); err != nil {
		return nil, err
	}
	return acquireFileLock(v.lockPath())
}

// readMap distinguishes a missing vault from a vault that cannot be trusted.
// A missing or empty file is an empty map. Corrupt JSON and permission errors
// fail closed so a later write cannot replace every saved token.
func (v *fileAccountVault) readMap() (map[string]string, error) {
	data, err := os.ReadFile(v.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]string{}, nil
	}
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	if decoded == nil {
		return map[string]string{}, nil
	}
	return decoded, nil
}

func (v *fileAccountVault) writeMap(m map[string]string) error {
	if m == nil {
		m = map[string]string{}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(v.path, data, 0o600)
}

func (v *fileAccountVault) Get(_ context.Context, ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	lock, err := v.fileLock(false)
	if err != nil {
		return "", false
	}
	defer lock.Close()
	m, err := v.readMap()
	if err != nil {
		return "", false
	}
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
	lock, err := v.fileLock(true)
	if err != nil {
		return false
	}
	defer lock.Close()
	m, err := v.readMap()
	if err != nil {
		return false
	}
	m[ref] = token
	return v.writeMap(m) == nil
}

func (v *fileAccountVault) Delete(_ context.Context, ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return true
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	lock, err := v.fileLock(true)
	if err != nil {
		return false
	}
	defer lock.Close()
	m, err := v.readMap()
	if err != nil {
		return false
	}
	if _, ok := m[ref]; !ok {
		return true
	}
	delete(m, ref)
	return v.writeMap(m) == nil
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

func rememberTokenExpiry(account Account, token string) {
	inner := tokenObject(decodeToken(token))
	if inner == nil {
		delete(account, "access_expires_at")
		return
	}
	expiry, ok := tokenExpiry(inner)
	if !ok {
		delete(account, "access_expires_at")
		return
	}
	account["access_expires_at"] = isoTime(expiry)
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

// saveAccountSecret writes token into the deterministic vault entry and updates
// account metadata. The returned reference is the previous vault entry, which
// the caller must delete only after accounts.json has been saved. A false
// result means the token stayed in token_data and nothing should be deleted.
func (a *Application) saveAccountSecret(ctx context.Context, account Account, token string) (string, bool) {
	email := getString(account, "email")
	token = strings.TrimSpace(token)
	if email == "" || a.vault == nil || token == "" {
		if token != "" {
			account["token_data"] = token
			account["token_hash"] = hashToken(token)
			rememberTokenExpiry(account, token)
		}
		delete(account, "secret_ref")
		return "", false
	}
	oldRef := getString(account, "secret_ref")
	ref := accountSecretRef(email)
	if !a.vault.Set(ctx, ref, token) {
		account["token_data"] = token
		account["token_hash"] = hashToken(token)
		rememberTokenExpiry(account, token)
		delete(account, "secret_ref")
		return "", false
	}
	account["secret_ref"] = ref
	account["token_hash"] = hashToken(token)
	rememberTokenExpiry(account, token)
	delete(account, "token_data")
	if oldRef != "" && oldRef != ref {
		return oldRef, true
	}
	return "", true
}

func (a *Application) deleteReplacedSecrets(ctx context.Context, refs []string) {
	if a == nil || a.vault == nil {
		return
	}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		_ = a.vault.Delete(ctx, ref)
	}
}

func captureSecretFields(account Account) map[string]any {
	saved := make(map[string]any, 4)
	for _, key := range []string{"secret_ref", "token_hash", "token_data", "access_expires_at"} {
		if value, ok := account[key]; ok {
			saved[key] = value
		}
	}
	return saved
}

func restoreSecretFields(account Account, saved map[string]any) {
	for _, key := range []string{"secret_ref", "token_hash", "token_data", "access_expires_at"} {
		if value, ok := saved[key]; ok {
			account[key] = value
			continue
		}
		delete(account, key)
	}
}

func (a *Application) migrateVaultAccounts(ctx context.Context, accounts *Accounts) bool {
	if accounts == nil || accounts.Len() == 0 || a.vault == nil {
		return false
	}
	changed := false
	var replaced []string
	previous := make(map[string]map[string]any, accounts.Len())
	for _, email := range accounts.Order {
		acc := accounts.ByEmail[email]
		detRef := accountSecretRef(email)
		oldRef := getString(acc, "secret_ref")
		tokenHash := getString(acc, "token_hash")

		if oldRef != "" && (oldRef != detRef || tokenHash == "") {
			token, err := a.accountToken(ctx, acc)
			if err != nil || token == "" {
				continue
			}
			// Keep the live reference until the new copy and accounts.json both succeed.
			if !a.vault.Set(ctx, detRef, token) {
				continue
			}
			previous[email] = captureSecretFields(acc)
			acc["secret_ref"] = detRef
			acc["token_hash"] = hashToken(token)
			rememberTokenExpiry(acc, token)
			delete(acc, "token_data")
			changed = true
			if oldRef != detRef {
				replaced = append(replaced, oldRef)
			}
			continue
		}
		if oldRef == "" {
			token := getString(acc, "token_data")
			if token == "" {
				continue
			}
			previous[email] = captureSecretFields(acc)
			old, ok := a.saveAccountSecret(ctx, acc, token)
			if !ok {
				restoreSecretFields(acc, previous[email])
				delete(previous, email)
				continue
			}
			changed = true
			if old != "" {
				replaced = append(replaced, old)
			}
		}
	}
	if !changed {
		return false
	}
	if a.store == nil {
		for email, saved := range previous {
			restoreSecretFields(accounts.ByEmail[email], saved)
		}
		return false
	}
	if err := a.store.Save(accounts); err != nil {
		for email, saved := range previous {
			restoreSecretFields(accounts.ByEmail[email], saved)
		}
		return false
	}
	a.deleteReplacedSecrets(ctx, replaced)
	return true
}
