package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func tokenRefreshToken(tokenData string) string {
	decoded := decodeToken(tokenData)
	inner := tokenObject(decoded)
	if inner == nil {
		inner = decoded
	}
	if inner == nil {
		return ""
	}
	return strings.TrimSpace(getString(inner, "refresh_token"))
}

// hasOAuthSession reports whether token holds an access or refresh token, as
// opposed to an empty or unrelated JSON file.
func hasOAuthSession(token string) bool {
	decoded := decodeToken(token)
	inner := tokenObject(decoded)
	if inner == nil {
		inner = decoded
	}
	if inner == nil {
		return false
	}
	return strings.TrimSpace(getString(inner, "access_token")) != "" || strings.TrimSpace(getString(inner, "refresh_token")) != ""
}

// unsavedSurfaceSession returns a session from any Antigravity surface (CLI,
// IDE, Jetski) whose token belongs to no saved account, or "" when there is
// none. reconcileActiveSession leaves such a session alone, and `add` offers to
// save it.
func (a *Application) unsavedSurfaceSession(ctx context.Context, accounts *Accounts) string {
	for _, p := range a.credentials.oauthTokenPaths() {
		token := readOAuthTokenFromFile(p)
		if hasOAuthSession(token) && matchAccountByToken(ctx, accounts, a.vault, token) == "" {
			return token
		}
	}
	return ""
}

func (a *Application) hasAnySurfaceSession() bool {
	if a == nil || a.credentials == nil {
		return false
	}
	for _, p := range a.credentials.oauthTokenPaths() {
		if hasOAuthSession(readOAuthTokenFromFile(p)) {
			return true
		}
	}
	return false
}

func (a *Application) matchAccountByToken(ctx context.Context, accounts *Accounts, token string) string {
	return matchAccountByToken(ctx, accounts, a.vault, token)
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
		if strings.TrimSpace(getString(acc, "token_hash")) != "" {
			continue
		}
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
		if h := strings.TrimSpace(getString(acc, "refresh_hash")); h != "" {
			if h == hashToken(currentRefresh) {
				if matchedEmail != "" && !strings.EqualFold(matchedEmail, email) {
					return ""
				}
				matchedEmail = normalizeEmail(email)
			}
			continue
		}
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

	surfacePaths := a.credentials.oauthTokenPaths()
	diskTokens := make([]string, 0, len(surfacePaths))
	for _, p := range surfacePaths {
		if tok := readOAuthTokenFromFile(p); tok != "" {
			diskTokens = append(diskTokens, tok)
		}
	}
	if len(diskTokens) == 0 {
		// No session file is left: the user signed out. Do not sign them back in.
		return
	}
	accounts, err := a.store.Load(false)
	if err != nil || accounts.Len() == 0 {
		return
	}

	var matchedOwners []string
	unsavedSession := false
	for _, diskToken := range diskTokens {
		ownerEmail := matchAccountByToken(ctx, accounts, a.vault, diskToken)
		if ownerEmail == "" {
			if hasOAuthSession(diskToken) {
				unsavedSession = true
			}
			continue
		}
		matchedOwners = append(matchedOwners, ownerEmail)
		if ownerAcc, ok := accounts.Get(ownerEmail); ok {
			if storedHash := strings.TrimSpace(getString(ownerAcc, "token_hash")); storedHash == "" || storedHash != hashToken(diskToken) {
				storedOwnerToken, _ := a.accountToken(ctx, ownerAcc)
				if shouldHarvestToken(storedOwnerToken, diskToken) {
					oldRef, _ := a.saveAccountSecret(ctx, ownerAcc, diskToken)
					accounts.Set(ownerEmail, ownerAcc)
					if saveErr := a.store.Save(accounts); saveErr == nil && oldRef != "" {
						a.deleteReplacedSecrets(ctx, []string{oldRef})
					}
				}
			}
		}
	}
	if unsavedSession {
		// A sign-in for an account agy-swap has not saved (for example in the
		// IDE). Leave every surface alone so that token is not lost;
		// `agy-swap add` offers to save it.
		return
	}

	intendedEmail := a.credentials.StoredActiveEmail()
	if intendedEmail == "" && len(matchedOwners) > 0 {
		intendedEmail = matchedOwners[0]
	}
	if intendedEmail == "" {
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
	needsSync := !strings.EqualFold(a.credentials.StoredActiveEmail(), intendedEmail)
	if !needsSync {
		for _, p := range surfacePaths {
			if readOAuthTokenFromFile(p) != intendedToken {
				needsSync = true
				break
			}
		}
	}
	if needsSync {
		_ = a.credentials.writeOAuthFiles(intendedToken, intendedEmail)
		a.credentials.ensureKeyringBypassMarker()
		if a.credentials.shouldMirrorOSKeyring() {
			_ = a.credentials.Set(ctx, intendedToken)
		}
	}
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

// buildAgyLauncherScriptForPaths returns the session-guard wrapper. For the
// IDE's --hub service it restarts the hub only when reloadMarker changed while
// the hub ran (agy-swap rewrites it right before it stops hubs after a switch),
// and gives up after ten restarts within ten seconds.
func buildAgyLauncherScriptForPaths(realCommand, preferredSwapCommand, reloadMarker string, skipPermsByDefault bool) string {
	realExpr := `"$HOME/.local/libexec/agy"`
	if strings.TrimSpace(realCommand) != "" {
		realExpr = shellQuoteDouble(realCommand)
	}
	swapExpr := `"$HOME/.local/bin/agy-swap"`
	if strings.TrimSpace(preferredSwapCommand) != "" {
		swapExpr = shellQuoteDouble(preferredSwapCommand)
	}
	markerExpr := `""`
	if strings.TrimSpace(reloadMarker) != "" {
		markerExpr = shellQuoteDouble(reloadMarker)
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
case "${1:-}" in
  --version|-v|--help)
    exec "$real_command" "$@"
    ;;
esac
swap_command=` + swapExpr + `
reload_marker=` + markerExpr + `
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
case " $* " in
  *" --hub "*)
    stopping=0
    hub_pid=""
    window_start="$(date +%s)"
    window_restarts=0
    trap 'stopping=1; if [ -n "$hub_pid" ]; then kill -TERM "$hub_pid" 2>/dev/null || true; wait "$hub_pid" 2>/dev/null || true; fi; exit 0' TERM INT HUP
    while [ "$stopping" -eq 0 ]; do
      marker_before="$(cat "$reload_marker" 2>/dev/null || true)"
      "$real_command" "$@" &
      hub_pid=$!
      wait "$hub_pid"
      status=$?
      hub_pid=""
      if [ "$stopping" -ne 0 ]; then
        exit 0
      fi
      marker_after="$(cat "$reload_marker" 2>/dev/null || true)"
      if [ -z "$reload_marker" ] || [ "$marker_after" = "$marker_before" ]; then
        exit "$status"
      fi
      now="$(date +%s)"
      if [ $((now - window_start)) -ge 10 ]; then
        window_start="$now"
        window_restarts=0
      fi
      window_restarts=$((window_restarts + 1))
      if [ "$window_restarts" -ge 10 ]; then
        exit "$status"
      fi
      sleep 0.1
      if [ "${AGY_SWAP_SKIP_SYNC:-0}" != "1" ] && [ -n "$swap_command" ] && [ -x "$swap_command" ]; then
        "$swap_command" --sync-session >/dev/null 2>&1 || true
      fi
    done
    exit 0
    ;;
esac
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

// syncAgyBinarySidecar updates Antigravity IDE's agy.version.json sidecar (when
// present next to agyBin) so doesSidecarMatchStats sees the wrapper's size,
// mtimeMs, and ino=0 while preserving the cached version and sha512.
func syncAgyBinarySidecar(agyBin string) {
	sidecarPath := strings.TrimSpace(agyBin) + ".version.json"
	if sidecarPath == ".version.json" {
		return
	}
	raw, err := os.ReadFile(sidecarPath)
	if err != nil || len(raw) == 0 {
		return
	}
	var sidecar map[string]any
	if json.Unmarshal(raw, &sidecar) != nil || sidecar == nil {
		return
	}
	info, err := os.Stat(agyBin)
	if err != nil || info.IsDir() {
		return
	}
	wantSize := info.Size()
	wantMtime := info.ModTime().UnixMilli()
	gotSize, sizeOK := sidecar["size"].(float64)
	gotMtime, mtimeOK := sidecar["mtimeMs"].(float64)
	gotIno, inoOK := sidecar["ino"].(float64)
	if sizeOK && int64(gotSize) == wantSize && mtimeOK && int64(gotMtime) == wantMtime && inoOK && int64(gotIno) == 0 {
		return
	}
	sidecar["size"] = wantSize
	sidecar["mtimeMs"] = wantMtime
	sidecar["ino"] = 0
	_ = atomicWriteJSON(sidecarPath, sidecar)
}

func readHeaderBytes(path string, maxBytes int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, maxBytes)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:n], nil
}

func extractWrapperRealCommand(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "real_command=") {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, "real_command="))
		val = strings.TrimSuffix(strings.TrimPrefix(val, `"`), `"`)
		unquoted := strings.NewReplacer(`\"`, `"`, `\\`, `\`, `\$`, `$`, "\\`", "`").Replace(val)
		if unquoted == "$HOME/.local/libexec/agy" {
			return ""
		}
		return unquoted
	}
	return ""
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
		targetData, readErr := readHeaderBytes(resolved, 8192)
		if readErr != nil || len(targetData) == 0 {
			return
		}
		if !isNativeExecutable(targetData) {
			if strings.HasPrefix(string(targetData), "#!") {
				a.ensureAgyLauncherGuardForPath(resolved)
			}
			return
		}
		script := buildAgyLauncherScriptForPaths(resolved, preferredSwap, a.credentials.hubReloadMarkerPath(), false)
		_ = writeExecutableAtomically(agyBin, []byte(script))
		syncAgyBinarySidecar(agyBin)
		return
	}

	data, err := readHeaderBytes(agyBin, 8192)
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
		script := buildAgyLauncherScriptForPaths(realRef, preferredSwap, a.credentials.hubReloadMarkerPath(), false)
		_ = writeExecutableAtomically(agyBin, []byte(script))
		syncAgyBinarySidecar(agyBin)
		return
	}

	text := string(data)
	if !strings.HasPrefix(text, "#!") || (!strings.Contains(text, ".local/libexec/agy") && !strings.Contains(text, "Managed by agy-swap session guard")) {
		return
	}
	if strings.Contains(text, "--sync-session") && strings.Contains(text, "WSL_INTEROP") && strings.Contains(text, "hub_pid") && strings.Contains(text, "reload_marker=") && !strings.Contains(text, "python3 -") {
		syncAgyBinarySidecar(agyBin)
		return
	}
	if existingReal := extractWrapperRealCommand(text); existingReal != "" {
		if realRef == "" {
			realRef = existingReal
		} else if _, statErr := os.Stat(realRef); statErr != nil {
			realRef = existingReal
		}
	}
	skipPerms := strings.Contains(text, "AGY_NORMAL_MODE") || strings.Contains(text, "--dangerously-skip-permissions")
	script := buildAgyLauncherScriptForPaths(realRef, preferredSwap, a.credentials.hubReloadMarkerPath(), skipPerms)
	_ = writeExecutableAtomically(agyBin, []byte(script))
	syncAgyBinarySidecar(agyBin)
}

func writeExecutableAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".agy-swap-wrapper-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// ensureAgyLauncherGuard ensures ~/.local/bin/agy, ~/.gemini/bin/agy (used by
// Antigravity IDE), and any configured custom agy path delegate to their
// backing binary and run agy-swap --sync-session before and after each session.
func (a *Application) ensureAgyLauncherGuard() {
	if a == nil || runtime.GOOS == "windows" || strings.TrimSpace(a.paths.Home) == "" {
		return
	}
	defaultBin := filepath.Join(a.paths.Home, ".local", "bin", "agy")
	ideBin := filepath.Join(a.paths.Home, ".gemini", "bin", "agy")
	a.ensureAgyLauncherGuardForPath(defaultBin)
	a.ensureAgyLauncherGuardForPath(ideBin)

	if envBin := strings.TrimSpace(os.Getenv("AGY_SWAP_AGY_BIN")); envBin != "" &&
		filepath.Clean(envBin) != filepath.Clean(defaultBin) &&
		filepath.Clean(envBin) != filepath.Clean(ideBin) {
		a.ensureAgyLauncherGuardForPath(envBin)
	}
	if a.store != nil {
		if settings, err := a.loadSettings(); err == nil {
			if target, ok := settings.Targets["agy"]; ok && target.Enabled {
				cmd := strings.TrimSpace(target.Command)
				if cmd != "" && cmd != "agy" && filepath.IsAbs(cmd) &&
					filepath.Clean(cmd) != filepath.Clean(defaultBin) &&
					filepath.Clean(cmd) != filepath.Clean(ideBin) {
					a.ensureAgyLauncherGuardForPath(cmd)
				}
			}
		}
	}
}
