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

func moveOrCopyExecutable(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		_ = os.Chmod(dst, 0o755)
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Chmod(dst, 0o755)
	_ = os.Remove(src)
	return nil
}

func shellQuoteDouble(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`")
	return `"` + replacer.Replace(value) + `"`
}

func buildAgyLauncherScriptForPaths(realCommand, preferredSwapCommand string, skipPermsByDefault bool) string {
	realExpr := `"$HOME/.local/libexec/agy"`
	if strings.TrimSpace(realCommand) != "" {
		realExpr = shellQuoteDouble(realCommand)
	}
	swapExpr := `"$HOME/.local/bin/agy-swap"`
	if strings.TrimSpace(preferredSwapCommand) != "" {
		swapExpr = shellQuoteDouble(preferredSwapCommand)
	}
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
real_command=` + realExpr + `
swap_command=` + swapExpr + `
if [ ! -x "$swap_command" ]; then
  for candidate in "$HOME/.local/bin/agy-swap" /opt/homebrew/bin/agy-swap /usr/local/bin/agy-swap "$HOME/go/bin/agy-swap"; do
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

func currentSwapExecutable() string {
	self, err := os.Executable()
	if err != nil || strings.TrimSpace(self) == "" {
		return ""
	}
	base := strings.ToLower(filepath.Base(self))
	if strings.Contains(base, ".test") || strings.Contains(self, os.TempDir()) {
		return ""
	}
	return self
}

func (a *Application) ensureAgyLauncherGuardForPath(agyBin string) {
	if a == nil || runtime.GOOS == "windows" || strings.TrimSpace(agyBin) == "" {
		return
	}
	lstat, err := os.Lstat(agyBin)
	if err != nil {
		return
	}
	home := strings.TrimSpace(a.paths.Home)
	defaultBin := ""
	defaultLibexec := ""
	if home != "" {
		defaultBin = filepath.Join(home, ".local", "bin", "agy")
		defaultLibexec = filepath.Join(home, ".local", "libexec", "agy")
	}
	preferredSwap := currentSwapExecutable()

	// If agyBin is a symlink to a native binary elsewhere, resolve the real target
	// and replace the symlink with a wrapper pointing directly to the resolved binary
	// so relative symlinks are never broken by moving them across directories.
	if lstat.Mode()&os.ModeSymlink != 0 {
		resolved, evalErr := filepath.EvalSymlinks(agyBin)
		if evalErr != nil || strings.TrimSpace(resolved) == "" || filepath.Clean(resolved) == filepath.Clean(agyBin) {
			return
		}
		targetData, readErr := os.ReadFile(resolved)
		if readErr != nil || !isNativeExecutable(targetData) {
			return
		}
		_ = os.Remove(agyBin)
		script := buildAgyLauncherScriptForPaths(resolved, preferredSwap, false)
		_ = os.WriteFile(agyBin, []byte(script), 0o755)
		return
	}

	data, err := os.ReadFile(agyBin)
	if err != nil || len(data) == 0 {
		return
	}

	libexecBin := defaultLibexec
	realRef := ""
	if filepath.Clean(agyBin) != filepath.Clean(defaultBin) || libexecBin == "" {
		libexecBin = filepath.Join(filepath.Dir(agyBin), ".agy.real")
		realRef = libexecBin
	}

	if isNativeExecutable(data) {
		if err := moveOrCopyExecutable(agyBin, libexecBin); err != nil {
			return
		}
		script := buildAgyLauncherScriptForPaths(realRef, preferredSwap, false)
		_ = os.WriteFile(agyBin, []byte(script), 0o755)
		return
	}

	text := string(data)
	if !strings.HasPrefix(text, "#!") || (!strings.Contains(text, ".local/libexec/agy") && !strings.Contains(text, "Managed by agy-swap session guard")) {
		return
	}
	if strings.Contains(text, "--sync-session") && strings.Contains(text, "WSL_INTEROP") && !strings.Contains(text, "python3 -") {
		return
	}
	skipPerms := strings.Contains(text, "AGY_NORMAL_MODE") || strings.Contains(text, "--dangerously-skip-permissions")
	script := buildAgyLauncherScriptForPaths(realRef, preferredSwap, skipPerms)
	_ = os.WriteFile(agyBin, []byte(script), 0o755)
}

// ensureAgyLauncherGuard ensures ~/.local/bin/agy (and any configured custom agy
// path) delegates to its backing binary and runs agy-swap --sync-session before
// and after each session so background token refreshes never clobber newly opened workspaces.
func (a *Application) ensureAgyLauncherGuard() {
	if a == nil || runtime.GOOS == "windows" || strings.TrimSpace(a.paths.Home) == "" {
		return
	}
	defaultBin := filepath.Join(a.paths.Home, ".local", "bin", "agy")
	a.ensureAgyLauncherGuardForPath(defaultBin)

	if envBin := strings.TrimSpace(os.Getenv("AGY_SWAP_AGY_BIN")); envBin != "" && filepath.Clean(envBin) != filepath.Clean(defaultBin) {
		a.ensureAgyLauncherGuardForPath(envBin)
	}
	if a.store != nil {
		if settings, err := a.loadSettings(); err == nil {
			if target, ok := settings.Targets["agy"]; ok && target.Enabled {
				cmd := strings.TrimSpace(target.Command)
				if cmd != "" && cmd != "agy" && filepath.IsAbs(cmd) && filepath.Clean(cmd) != filepath.Clean(defaultBin) {
					a.ensureAgyLauncherGuardForPath(cmd)
				}
			}
		}
	}
}
