package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func tokenRefreshToken(tokenData string) string {
	inner := tokenObject(decodeToken(tokenData))
	if inner == nil {
		return ""
	}
	return strings.TrimSpace(getString(inner, "refresh_token"))
}

// matchAccountByToken identifies which saved account owns token using local
// metadata and vault entries (including matching by refresh_token after
// Antigravity refreshes access_token in place) without network requests.
func matchAccountByToken(ctx context.Context, accounts *Accounts, vault AccountVault, token string) string {
	token = strings.TrimSpace(token)
	if token == "" || accounts == nil || accounts.Len() == 0 {
		return ""
	}
	if local := localActiveEmail(accounts, token); local != "" {
		if _, ok := accounts.Get(local); ok {
			return normalizeEmail(local)
		}
	}
	currentHash := hashToken(token)
	for _, email := range accounts.Order {
		acc := accounts.ByEmail[email]
		stored, err := accountToken(ctx, acc, vault)
		if err != nil || stored == "" {
			continue
		}
		if stored == token || (currentHash != "" && hashToken(stored) == currentHash) {
			return normalizeEmail(email)
		}
	}
	currentRefresh := tokenRefreshToken(token)
	if currentRefresh == "" {
		return ""
	}
	matchedEmail := ""
	for _, email := range accounts.Order {
		acc := accounts.ByEmail[email]
		stored, err := accountToken(ctx, acc, vault)
		if err != nil || stored == "" {
			continue
		}
		if tokenRefreshToken(stored) == currentRefresh {
			if matchedEmail != "" && !strings.EqualFold(matchedEmail, email) {
				return ""
			}
			matchedEmail = normalizeEmail(email)
		}
	}
	return matchedEmail
}

func shouldHarvestToken(existingToken, candidateToken string) bool {
	if strings.TrimSpace(candidateToken) == "" {
		return false
	}
	if strings.TrimSpace(existingToken) == "" {
		return true
	}
	if hashToken(existingToken) == hashToken(candidateToken) {
		return false
	}
	candInner := tokenObject(decodeToken(candidateToken))
	if candInner == nil || getString(candInner, "access_token") == "" {
		return false
	}
	existInner := tokenObject(decodeToken(existingToken))
	if existInner == nil {
		return true
	}
	existExp, existOK := tokenExpiry(existInner)
	candExp, candOK := tokenExpiry(candInner)
	if existOK && candOK && candExp.Before(existExp) {
		return false
	}
	return true
}

// reconcileActiveSession keeps the file-backed Antigravity OAuth session aligned
// with the account selected in agy-swap while harvesting in-place token
// refreshes written by running agy processes.
func (a *Application) reconcileActiveSession(ctx context.Context) {
	if a == nil || a.credentials == nil || a.store == nil || os.Getenv("AGY_SWAP_SKIP_SYNC") == "1" {
		return
	}
	a.credentials.ensureKeyringBypassMarker()
	a.ensureAgyLauncherGuard()

	lock, err := tryAcquireFileLock(a.paths.SessionLock)
	if err != nil {
		return
	}
	defer func() { _ = lock.Close() }()

	diskToken := a.credentials.readOAuthToken()
	if diskToken == "" {
		return
	}
	accounts, err := a.store.Load(false)
	if err != nil || accounts.Len() == 0 {
		return
	}
	ownerEmail := matchAccountByToken(ctx, accounts, a.vault, diskToken)
	if ownerEmail == "" {
		return
	}

	if ownerAcc, ok := accounts.Get(ownerEmail); ok {
		storedOwnerToken, _ := a.accountToken(ctx, ownerAcc)
		if shouldHarvestToken(storedOwnerToken, diskToken) {
			oldRef, _ := a.saveAccountSecret(ctx, ownerAcc, diskToken)
			accounts.Set(ownerEmail, ownerAcc)
			if saveErr := a.store.Save(accounts); saveErr == nil && oldRef != "" {
				a.deleteReplacedSecrets(ctx, []string{oldRef})
			}
		}
	}

	intendedEmail := a.credentials.StoredActiveEmail()
	if intendedEmail == "" || strings.EqualFold(intendedEmail, ownerEmail) {
		return
	}
	intendedAcc, ok := accounts.Get(intendedEmail)
	if !ok {
		return
	}
	intendedToken, err := a.accountToken(ctx, intendedAcc)
	if err != nil || intendedToken == "" {
		return
	}
	_ = a.credentials.writeOAuthFiles(intendedToken, intendedEmail)
	a.credentials.ensureKeyringBypassMarker()
}

func isNativeExecutable(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	// ELF magic
	if bytes.Equal(data[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		return true
	}
	// Mach-O 32/64/Universal magics
	switch {
	case bytes.Equal(data[:4], []byte{0xfe, 0xed, 0xfa, 0xce}),
		bytes.Equal(data[:4], []byte{0xce, 0xfa, 0xed, 0xfe}),
		bytes.Equal(data[:4], []byte{0xfe, 0xed, 0xfa, 0xcf}),
		bytes.Equal(data[:4], []byte{0xcf, 0xfa, 0xed, 0xfe}),
		bytes.Equal(data[:4], []byte{0xca, 0xfe, 0xba, 0xbe}),
		bytes.Equal(data[:4], []byte{0xbe, 0xba, 0xfe, 0xca}):
		return true
	}
	return false
}

func buildAgyLauncherScript(skipPermsByDefault bool) string {
	execBlock := `"$real_command" "$@"`
	if skipPermsByDefault {
		execBlock = `if [ "${AGY_NORMAL_MODE:-0}" = "1" ]; then
  "$real_command" "$@"
else
  "$real_command" --dangerously-skip-permissions "$@"
fi`
	}
	return `#!/bin/sh
# Managed by agy-swap session guard
real_command="$HOME/.local/libexec/agy"
swap_command="$HOME/.local/bin/agy-swap"
if [ ! -x "$swap_command" ]; then
  for candidate in /opt/homebrew/bin/agy-swap /usr/local/bin/agy-swap; do
    if [ -x "$candidate" ]; then
      swap_command="$candidate"
      break
    fi
  done
fi
if [ ! -x "$swap_command" ]; then
  swap_command="$(command -v agy-swap 2>/dev/null || true)"
fi
if [ "${AGY_SWAP_SKIP_SYNC:-0}" != "1" ] && [ -n "$swap_command" ] && [ -x "$swap_command" ]; then
  "$swap_command" --sync-session >/dev/null 2>&1 || true
fi
export WSL_INTEROP="${WSL_INTEROP:-1}"
` + execBlock + `
status=$?
if [ "${AGY_SWAP_SKIP_SYNC:-0}" != "1" ] && [ -n "$swap_command" ] && [ -x "$swap_command" ]; then
  "$swap_command" --sync-session >/dev/null 2>&1 || true
fi
exit "$status"
`
}

// ensureAgyLauncherGuard ensures ~/.local/bin/agy (when present) delegates to
// ~/.local/libexec/agy and runs agy-swap --sync-session before and after each
// session so background token refreshes never clobber newly opened workspaces.
func (a *Application) ensureAgyLauncherGuard() {
	if a == nil || runtime.GOOS == "windows" || strings.TrimSpace(a.paths.Home) == "" {
		return
	}
	agyBin := filepath.Join(a.paths.Home, ".local", "bin", "agy")
	data, err := os.ReadFile(agyBin)
	if err != nil || len(data) == 0 {
		return
	}
	libexecBin := filepath.Join(a.paths.Home, ".local", "libexec", "agy")

	if isNativeExecutable(data) {
		if err := os.MkdirAll(filepath.Dir(libexecBin), 0o755); err != nil {
			return
		}
		if err := os.Rename(agyBin, libexecBin); err != nil {
			return
		}
		_ = os.Chmod(libexecBin, 0o755)
		script := buildAgyLauncherScript(false)
		_ = os.WriteFile(agyBin, []byte(script), 0o755)
		return
	}

	text := string(data)
	if !strings.HasPrefix(text, "#!") || !strings.Contains(text, ".local/libexec/agy") {
		return
	}
	if strings.Contains(text, "--sync-session") && strings.Contains(text, "WSL_INTEROP") && !strings.Contains(text, "python3 -") {
		return
	}
	skipPerms := strings.Contains(text, "AGY_NORMAL_MODE") || strings.Contains(text, "--dangerously-skip-permissions")
	script := buildAgyLauncherScript(skipPerms)
	_ = os.WriteFile(agyBin, []byte(script), 0o755)
}
