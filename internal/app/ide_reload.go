package app

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/aklkbqx/agy-swap/v2/internal/config"
)

// ideHubDescriptor describes a live local `agy --hub` server discovered from
// ~/.gemini/antigravity/daemon/hub_<pid>.json.
type ideHubDescriptor struct {
	PID       int
	Port      int
	CSRFToken string
	Path      string
}

var ideRunningCheck = detectAntigravityIDERunning

var allowIDEHubReloadInTests bool

func isAntigravityIDERunning() bool {
	if ideRunningCheck != nil {
		return ideRunningCheck()
	}
	return detectAntigravityIDERunning()
}

// discoverRunningIDEHubs scans ~/.gemini/antigravity/daemon/hub_*.json and
// returns live local hub endpoints whose PID is still active.
func discoverRunningIDEHubs(home string) []ideHubDescriptor {
	home = strings.TrimSpace(home)
	if home == "" {
		return nil
	}
	daemonDir := filepath.Join(home, ".gemini", "antigravity", "daemon")
	entries, err := os.ReadDir(daemonDir)
	if err != nil {
		return nil
	}
	var hubs []ideHubDescriptor
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "hub_") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(daemonDir, entry.Name())
		raw, readErr := os.ReadFile(path)
		if readErr != nil || len(raw) == 0 {
			continue
		}
		var doc struct {
			PID           int    `json:"pid"`
			HTTPPort      int    `json:"httpPort"`
			LocalHTTPPort int    `json:"localHttpPort"`
			CSRFToken     string `json:"csrfToken"`
		}
		if json.Unmarshal(raw, &doc) != nil || doc.PID <= 0 {
			continue
		}
		if !isProcessAlive(doc.PID) {
			continue
		}
		port := doc.HTTPPort
		if port <= 0 {
			port = doc.LocalHTTPPort
		}
		if port <= 0 || port > 65535 {
			continue
		}
		hubs = append(hubs, ideHubDescriptor{
			PID:       doc.PID,
			Port:      port,
			CSRFToken: strings.TrimSpace(doc.CSRFToken),
			Path:      path,
		})
	}
	return hubs
}

func callLocalHubRPC(ctx context.Context, hub ideHubDescriptor, method string) (map[string]any, error) {
	if hub.Port <= 0 || hub.Port > 65535 {
		return nil, errors.New("invalid local hub port")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/exa.language_server_pb.LanguageServerService/%s", hub.Port, method)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if hub.CSRFToken != "" {
		req.Header.Set("x-codeium-csrf-token", hub.CSRFToken)
	}
	tr := &http.Transport{DisableKeepAlives: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("local hub %s returned HTTP %d", method, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func extractHubAuthEmail(resp map[string]any) string {
	if resp == nil {
		return ""
	}
	if email := normalizeEmail(getString(resp, "email")); email != "" {
		return email
	}
	for _, key := range []string{"userStatus", "user_status", "userInfo", "user_info", "account"} {
		if sub := getMap(resp[key]); sub != nil {
			if email := normalizeEmail(getString(sub, "email")); email != "" {
				return email
			}
		}
	}
	return ""
}

func firstNonEmptyText(values ...any) string {
	for _, v := range values {
		if s, ok := v.(string); ok {
			if cleaned := cleanText(s); cleaned != "" {
				return cleaned
			}
		}
	}
	return ""
}

func extractHubTier(resp map[string]any) (string, string) {
	if resp == nil {
		return "", ""
	}
	for _, container := range []map[string]any{resp, getMap(resp["userStatus"]), getMap(resp["user_status"]), getMap(resp["planStatus"])} {
		if container == nil {
			continue
		}
		if tier := getMap(container["tier"]); tier != nil {
			id := cleanText(getString(tier, "id"))
			name := firstNonEmptyText(tier["name"], tierNames[id], id)
			if id != "" {
				return id, name
			}
		}
		if id := firstNonEmptyText(container["userTier"], container["user_tier"], container["tierId"]); id != "" {
			name := firstNonEmptyText(container["tierDisplayName"], container["tier_display_name"], tierNames[id], id)
			return id, name
		}
	}
	return "", ""
}

// fetchLocalHubQuota reads the active account's quota snapshot directly from a
// running local `agy --hub` instance (<20 ms, zero extra Google HTTP calls),
// verifying the hub's active email matches expectedEmail before accepting it.
func fetchLocalHubQuota(ctx context.Context, home, expectedEmail string) (map[string]any, bool) {
	expectedEmail = normalizeEmail(expectedEmail)
	if expectedEmail == "" || strings.TrimSpace(home) == "" {
		return nil, false
	}
	hubs := discoverRunningIDEHubs(home)
	for _, hub := range hubs {
		authResp, err := callLocalHubRPC(ctx, hub, "GetAuthStatus")
		if err != nil {
			continue
		}
		if hubEmail := extractHubAuthEmail(authResp); hubEmail == "" || !strings.EqualFold(hubEmail, expectedEmail) {
			continue
		}
		summary, err := callLocalHubRPC(ctx, hub, "RetrieveUserQuotaSummary")
		if err != nil || summary == nil {
			continue
		}
		groups, _ := quotaGroupsFromSummary(summary)
		if len(groups) == 0 {
			continue
		}
		tierID, tierName := extractHubTier(authResp)
		if tierID == "" {
			tierID, tierName = "unknown", "Antigravity"
		}
		snap, err := normalizeQuotaSnapshot(map[string]any{
			"observed_at": isoTime(time.Now().UTC()),
			"tier":        map[string]any{"id": tierID, "name": tierName},
			"groups":      groups,
		}, expectedEmail)
		if err != nil {
			continue
		}
		return snap, true
	}
	return nil, false
}

// triggerHubAuthStatusRPC invokes GetAuthStatus on each discovered live hub
// (used exclusively for opt-in `ide.reload_mode = "rpc-fast"`).
func triggerHubAuthStatusRPC(ctx context.Context, home string) int {
	hubs := discoverRunningIDEHubs(home)
	triggered := 0
	for _, hub := range hubs {
		if _, err := callLocalHubRPC(ctx, hub, "GetAuthStatus"); err == nil {
			triggered++
		}
	}
	return triggered
}

var (
	ideTurnSendRE     = regexp.MustCompile(`Sending user message to conversation\s+([A-Za-z0-9_.:-]+)`)
	ideTurnCompleteRE = regexp.MustCompile(`Stream completed for\s+([A-Za-z0-9_.:-]+)`)
	ideTurnExitedRE   = regexp.MustCompile(`Stream goroutine exited for\s+([A-Za-z0-9_.:-]+)`)
)

const inFlightTurnIdleStale = 15 * time.Second

// hasInFlightIDETurn checks recent Antigravity logs for an unpaired
// "Sending user message to conversation <id>" without a subsequent completion,
// stream goroutine exit, error termination, or >15s of inactivity.
func hasInFlightIDETurn(home string, now time.Time) bool {
	home = strings.TrimSpace(home)
	if home == "" {
		return false
	}
	scanner := NewLogScanner(Paths{Home: home})
	candidates := scanner.selected()
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	cutoff := now.Add(-5 * time.Minute)
	type openEntry struct {
		opened time.Time
		seq    int64
	}
	for _, c := range candidates {
		if c.info != nil && c.info.ModTime().Before(cutoff) {
			continue
		}
		raw, err := os.ReadFile(c.path)
		if err != nil || len(raw) == 0 {
			continue
		}
		// Inspect up to the last 256 KiB of each active log file.
		if len(raw) > 256*1024 {
			raw = raw[len(raw)-256*1024:]
		}
		modTime := now
		if c.info != nil {
			modTime = c.info.ModTime()
		}
		openStreams := map[string]openEntry{}
		var seq int64
		var lastActivity time.Time
		for _, line := range strings.Split(string(raw), "\n") {
			ts, hasTS := parseLogTimestamp(line, modTime)
			if m := ideTurnSendRE.FindStringSubmatch(line); len(m) == 2 {
				seq++
				entryTS := ts
				if !hasTS {
					entryTS = modTime
				}
				openStreams[m[1]] = openEntry{opened: entryTS, seq: seq}
				if entryTS.After(lastActivity) {
					lastActivity = entryTS
				}
				continue
			}
			if m := ideTurnCompleteRE.FindStringSubmatch(line); len(m) == 2 {
				delete(openStreams, m[1])
				continue
			}
			if m := ideTurnExitedRE.FindStringSubmatch(line); len(m) == 2 {
				delete(openStreams, m[1])
				continue
			}
			if containsASCIIFold(line, "agent executor error:") || containsASCIIFold(line, "resource_exhausted") || containsASCIIFold(line, "resourceexhausted, http 429") {
				latestID := ""
				var latestSeq int64
				for id, entry := range openStreams {
					if entry.seq > latestSeq {
						latestSeq = entry.seq
						latestID = id
					}
				}
				if latestID != "" {
					delete(openStreams, latestID)
				}
				continue
			}
			if hasTS && (strings.Contains(line, "streamGenerateContent") || strings.Contains(line, "llm_monitor.go") || strings.Contains(line, "runner.go")) {
				if ts.After(lastActivity) {
					lastActivity = ts
				}
			}
		}
		if len(openStreams) == 0 {
			continue
		}
		latest := lastActivity
		for _, entry := range openStreams {
			if entry.opened.After(latest) {
				latest = entry.opened
			}
		}
		if latest.IsZero() {
			latest = modTime
		}
		if now.Sub(latest) <= inFlightTurnIdleStale {
			return true
		}
	}
	return false
}

// waitForIDEIdle waits up to timeout for any in-flight Antigravity stream to
// complete before signaling a hub or restarting the IDE.
func waitForIDEIdle(ctx context.Context, home string, timeout time.Duration) bool {
	if !hasInFlightIDETurn(home, time.Now()) {
		return true
	}
	if timeout <= 0 {
		return false
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return false
		case now := <-ticker.C:
			if !hasInFlightIDETurn(home, now) {
				return true
			}
			if now.After(deadline) {
				return false
			}
		}
	}
}

func ideStateDBPaths(home string) []string {
	home = strings.TrimSpace(home)
	if home == "" {
		return nil
	}
	seen := map[string]bool{}
	var paths []string
	add := func(dir string) {
		if dir == "" {
			return
		}
		db := filepath.Clean(filepath.Join(dir, "User", "globalStorage", "state.vscdb"))
		if seen[db] {
			return
		}
		seen[db] = true
		paths = append(paths, db)
	}
	for _, dir := range config.IDEUserDataDirs(home) {
		add(dir)
	}
	// Preserve legacy cross-path checks for tests & non-standard installs.
	add(filepath.Join(home, "Library", "Application Support", "Antigravity IDE"))
	add(filepath.Join(home, ".config", "Antigravity IDE"))
	return paths
}

// syncIDEOnboardingStateOffline updates both agy.onboarding.lastLoginUsername
// and agy.onboarding.lastLoginIsGcpTos in state.vscdb ONLY when Antigravity IDE
// is verified not running, preventing SQLite lock races and Electron overwrites.
func syncIDEOnboardingStateOffline(home, email string, isGCPTos bool) {
	email = normalizeEmail(email)
	if email == "" || strings.TrimSpace(home) == "" {
		return
	}
	if isAntigravityIDERunning() {
		return
	}
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil || sqliteBin == "" {
		return
	}
	for _, dbPath := range ideStateDBPaths(home) {
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		out, err := exec.CommandContext(ctx, sqliteBin, "-cmd", ".timeout 1500", dbPath,
			"SELECT value FROM ItemTable WHERE key = 'google.google-antigravity';").Output()
		cancel()
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(string(out))
		if idx := strings.IndexByte(raw, '{'); idx > 0 {
			raw = raw[idx:]
		}
		if raw == "" {
			continue
		}
		var outer map[string]any
		if json.Unmarshal([]byte(raw), &outer) != nil || outer == nil {
			continue
		}
		storage, ok := outer["antigravity-storage"].(map[string]any)
		if !ok || storage == nil {
			continue
		}
		curEmail := getString(storage, "agy.onboarding.lastLoginUsername")
		curGcp, hasGcp := storage["agy.onboarding.lastLoginIsGcpTos"].(bool)
		if strings.EqualFold(curEmail, email) && hasGcp && curGcp == isGCPTos {
			continue
		}
		storage["agy.onboarding.lastLoginUsername"] = email
		storage["agy.onboarding.lastLoginIsGcpTos"] = isGCPTos
		outer["antigravity-storage"] = storage
		updatedJSON, err := json.Marshal(outer)
		if err != nil {
			continue
		}
		hexVal := hex.EncodeToString(updatedJSON)
		updateSQL := "UPDATE ItemTable SET value = CAST(X'" + hexVal + "' AS TEXT) WHERE key = 'google.google-antigravity';"
		uCtx, uCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = exec.CommandContext(uCtx, sqliteBin, "-cmd", ".timeout 1500", dbPath, updateSQL).Run()
		uCancel()
	}
}

// clearIDEOnboardingStateOffline clears cached onboarding email and GCP ToS
// flags in state.vscdb only when Antigravity IDE is closed.
func clearIDEOnboardingStateOffline(home string) {
	if strings.TrimSpace(home) == "" || isAntigravityIDERunning() {
		return
	}
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil || sqliteBin == "" {
		return
	}
	for _, dbPath := range ideStateDBPaths(home) {
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		out, err := exec.CommandContext(ctx, sqliteBin, "-cmd", ".timeout 1500", dbPath,
			"SELECT value FROM ItemTable WHERE key = 'google.google-antigravity';").Output()
		cancel()
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(string(out))
		if idx := strings.IndexByte(raw, '{'); idx > 0 {
			raw = raw[idx:]
		}
		if raw == "" {
			continue
		}
		var outer map[string]any
		if json.Unmarshal([]byte(raw), &outer) != nil || outer == nil {
			continue
		}
		storage, ok := outer["antigravity-storage"].(map[string]any)
		if !ok || storage == nil {
			continue
		}
		_, hasUser := storage["agy.onboarding.lastLoginUsername"]
		_, hasGcp := storage["agy.onboarding.lastLoginIsGcpTos"]
		if !hasUser && !hasGcp {
			continue
		}
		delete(storage, "agy.onboarding.lastLoginUsername")
		delete(storage, "agy.onboarding.lastLoginIsGcpTos")
		outer["antigravity-storage"] = storage
		updatedJSON, err := json.Marshal(outer)
		if err != nil {
			continue
		}
		hexVal := hex.EncodeToString(updatedJSON)
		updateSQL := "UPDATE ItemTable SET value = CAST(X'" + hexVal + "' AS TEXT) WHERE key = 'google.google-antigravity';"
		uCtx, uCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = exec.CommandContext(uCtx, sqliteBin, "-cmd", ".timeout 1500", dbPath, updateSQL).Run()
		uCancel()
	}
}

type ideRestartHooks struct {
	isRunning    func() bool
	gracefulQuit func(context.Context) error
	syncOffline  func()
	relaunch     func(context.Context) error
	waitTimeout  time.Duration
	pollEvery    time.Duration
}

// runGracefulIDERestart executes the safe IDE restart sequence:
//  1. If IDE is not running, sync state.vscdb offline and NEVER cold-launch IDE.
//  2. Send graceful quit only (never SIGKILL/taskkill /F).
//  3. Wait for IDE to exit; abort cleanly without relaunch if user cancels/blocks quit.
//  4. Sync state.vscdb offline once SQLite lock is released.
//  5. Relaunch IDE in background without stealing focus.
func runGracefulIDERestart(ctx context.Context, hooks ideRestartHooks) error {
	if hooks.isRunning == nil || !hooks.isRunning() {
		if hooks.syncOffline != nil {
			hooks.syncOffline()
		}
		return nil
	}
	if hooks.gracefulQuit == nil {
		return errors.New("graceful IDE quit is not available on this platform")
	}
	if err := hooks.gracefulQuit(ctx); err != nil {
		return fmt.Errorf("graceful IDE quit failed: %w", err)
	}
	waitTimeout := hooks.waitTimeout
	if waitTimeout <= 0 {
		waitTimeout = 10 * time.Second
	}
	pollEvery := hooks.pollEvery
	if pollEvery <= 0 {
		pollEvery = 200 * time.Millisecond
	}
	deadline := time.Now().Add(waitTimeout)
	for hooks.isRunning() {
		if time.Now().After(deadline) {
			return errors.New("graceful restart aborted: Antigravity IDE did not exit within timeout (possibly waiting on an unsaved dialog)")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollEvery):
		}
	}
	if hooks.syncOffline != nil {
		hooks.syncOffline()
	}
	if hooks.relaunch != nil {
		if err := hooks.relaunch(ctx); err != nil {
			return fmt.Errorf("relaunch of Antigravity IDE failed: %w", err)
		}
	}
	return nil
}

// reloadIDEForSwitch applies the configured ide.reload_mode after credentials
// have been written.
func (a *Application) reloadIDEForSwitch(ctx context.Context, mode string, waitTimeout time.Duration, email string, isGCPTos bool) {
	if a == nil || a.credentials == nil || a.demo {
		return
	}
	if strings.TrimSpace(mode) == "" {
		mode = defaultIDEReloadMode()
	}
	if !allowIDEHubReloadInTests && !a.credentials.shouldReloadIDEHubs() && runtime.GOOS != "windows" {
		return
	}
	home := a.paths.Home
	switch mode {
	case "manual":
		syncIDEOnboardingStateOffline(home, email, isGCPTos)
	case "rpc-fast":
		_ = triggerHubAuthStatusRPC(ctx, home)
		syncIDEOnboardingStateOffline(home, email, isGCPTos)
	case "app-restart":
		if !isAntigravityIDERunning() {
			syncIDEOnboardingStateOffline(home, email, isGCPTos)
			return
		}
		if !waitForIDEIdle(ctx, home, waitTimeout) {
			if a.Err != nil {
				fmt.Fprintln(a.Err, "Antigravity IDE has an active turn; skipped IDE restart.")
			}
			return
		}
		if err := spawnDetachedIDERestart(email, isGCPTos); err != nil && a.Err != nil {
			fmt.Fprintf(a.Err, "Could not schedule detached IDE restart: %v\n", err)
		}
	default: // "hub-restart"
		if !isAntigravityIDERunning() {
			syncIDEOnboardingStateOffline(home, email, isGCPTos)
		}
		if runtime.GOOS == "windows" {
			return
		}
		if len(discoverRunningIDEHubs(home)) == 0 {
			return
		}
		if !waitForIDEIdle(ctx, home, waitTimeout) {
			if a.Err != nil {
				fmt.Fprintln(a.Err, "Antigravity IDE has an active turn; deferred hub reload.")
			}
			return
		}
		a.credentials.reloadRunningIDEHubsTargeted(home)
	}
}

// runInternalRestartIDE executes inside the detached child process so an IDE
// restart triggered from Antigravity IDE's own integrated terminal survives the
// IDE closing and cleanly completes state.vscdb sync + background relaunch.
func (a *Application) runInternalRestartIDE(ctx context.Context, argv []string) int {
	email := ""
	isGCPTos := false
	for i := 0; i < len(argv); i++ {
		switch {
		case argv[i] == "--email" && i+1 < len(argv):
			i++
			email = normalizeEmail(argv[i])
		case argv[i] == "--gcp-tos=true":
			isGCPTos = true
		case argv[i] == "--gcp-tos=false":
			isGCPTos = false
		}
	}
	exePath := captureWindowsIDEExecutable()
	err := runGracefulIDERestart(ctx, ideRestartHooks{
		isRunning:    isAntigravityIDERunning,
		gracefulQuit: gracefulQuitAntigravityIDE,
		syncOffline:  func() { syncIDEOnboardingStateOffline(a.paths.Home, email, isGCPTos) },
		relaunch:     func(rctx context.Context) error { return relaunchAntigravityIDE(rctx, exePath) },
		waitTimeout:  10 * time.Second,
		pollEvery:    200 * time.Millisecond,
	})
	if err != nil {
		return 1
	}
	return 0
}
