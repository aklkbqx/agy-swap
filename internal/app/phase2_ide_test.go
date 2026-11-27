package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPhase2DiscoverHubAndLocalQuotaFastPathAndRPCFast(t *testing.T) {
	var authCalls, quotaCalls atomic.Int32
	var gotCSRF string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCSRF = r.Header.Get("x-codeium-csrf-token")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/exa.language_server_pb.LanguageServerService/GetAuthStatus":
			authCalls.Add(1)
			fmt.Fprint(w, `{"email":"active@example.invalid","tier":{"id":"g1-pro-tier","name":"Google AI Pro"}}`)
		case "/exa.language_server_pb.LanguageServerService/RetrieveUserQuotaSummary":
			quotaCalls.Add(1)
			fmt.Fprint(w, `{"groups":[{"buckets":[{"bucketId":"gemini-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.64,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(u.Port())

	home := t.TempDir()
	daemonDir := filepath.Join(home, ".gemini", "antigravity", "daemon")
	if err := os.MkdirAll(daemonDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Write a live hub discovery file using current test PID so liveness check passes,
	// plus a dead PID entry that must be skipped.
	liveDesc := map[string]any{
		"pid":       os.Getpid(),
		"httpPort":  port,
		"csrfToken": "csrf-secret-123",
	}
	liveRaw, _ := json.Marshal(liveDesc)
	if err := os.WriteFile(filepath.Join(daemonDir, fmt.Sprintf("hub_%d.json", os.Getpid())), liveRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	deadRaw, _ := json.Marshal(map[string]any{"pid": 999999, "httpPort": 1, "csrfToken": "dead"})
	if err := os.WriteFile(filepath.Join(daemonDir, "hub_999999.json"), deadRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	hubs := discoverRunningIDEHubs(home)
	if len(hubs) != 1 || hubs[0].PID != os.Getpid() || hubs[0].Port != port {
		t.Fatalf("expected 1 live hub on port %d, got %+v", port, hubs)
	}

	// 1. Local hub quota fast path returns normalized snapshot with zero Google calls.
	snap, ok := fetchLocalHubQuota(context.Background(), home, "active@example.invalid")
	if !ok || snap == nil {
		t.Fatal("expected local hub quota fast path to succeed")
	}
	if gotCSRF != "csrf-secret-123" {
		t.Fatalf("expected x-codeium-csrf-token header, got %q", gotCSRF)
	}
	if quotaCalls.Load() != 1 {
		t.Fatalf("expected 1 local RetrieveUserQuotaSummary call, got %d", quotaCalls.Load())
	}

	// Mismatched email must NOT reuse the active hub's quota!
	if snapOther, okOther := fetchLocalHubQuota(context.Background(), home, "other@example.invalid"); okOther || snapOther != nil {
		t.Fatalf("expected mismatched email to skip local hub quota fast path, got %v", snapOther)
	}

	// 2. rpc-fast calls GetAuthStatus on discovered hub.
	before := authCalls.Load()
	if n := triggerHubAuthStatusRPC(context.Background(), home); n != 1 || authCalls.Load() != before+1 {
		t.Fatalf("expected rpc-fast to trigger 1 hub, got n=%d calls=%d", n, authCalls.Load()-before)
	}
}

func TestPhase2InFlightTurnGateDetectsActiveStreamAndWaits(t *testing.T) {
	home := t.TempDir()
	logDir := filepath.Join(home, ".gemini", "antigravity-cli", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	ts := now.Format("0102 15:04:05.000000")
	logFile := filepath.Join(logDir, "cli-test.log")

	// Unpaired Sending user message -> in-flight!
	inFlight := fmt.Sprintf("I%s 1 conv.go:1] Sending user message to conversation conv-abc-123\n", ts)
	if err := os.WriteFile(logFile, []byte(inFlight), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasInFlightIDETurn(home, now) {
		t.Fatal("expected unpaired Sending user message to be detected as in-flight")
	}

	// Completing the stream clears the in-flight gate.
	done := inFlight + fmt.Sprintf("I%s 1 conv.go:2] Stream completed for conv-abc-123\n", ts)
	if err := os.WriteFile(logFile, []byte(done), 0o600); err != nil {
		t.Fatal(err)
	}
	if hasInFlightIDETurn(home, now) {
		t.Fatal("expected completed stream not to block in-flight turn gate")
	}
}

func TestPhase2StateVSCDBNeverMutatedWhileIDERunningAndSyncsBothFieldsOffline(t *testing.T) {
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not installed")
	}
	home := t.TempDir()
	dbDir := filepath.Join(home, "Library", "Application Support", "Antigravity IDE", "User", "globalStorage")
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Also create Linux path so test passes on both darwin and linux.
	linuxDir := filepath.Join(home, ".config", "Antigravity IDE", "User", "globalStorage")
	if err := os.MkdirAll(linuxDir, 0o700); err != nil {
		t.Fatal(err)
	}
	initDB := func(dir string) string {
		db := filepath.Join(dir, "state.vscdb")
		sql := `CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT); INSERT INTO ItemTable VALUES ('google.google-antigravity', '{"antigravity-storage":{"agy.onboarding.lastLoginUsername":"old@example.invalid","agy.onboarding.lastLoginIsGcpTos":false}}');`
		if out, runErr := exec.Command(sqliteBin, db, sql).CombinedOutput(); runErr != nil {
			t.Fatalf("sqlite3 init failed: %v %s", runErr, out)
		}
		return db
	}
	darwinDB := initDB(dbDir)
	linuxDB := initDB(linuxDir)

	origCheck := ideRunningCheck
	defer func() { ideRunningCheck = origCheck }()

	// 1. When IDE is running, syncIDEOnboardingStateOffline MUST NOT touch state.vscdb!
	ideRunningCheck = func() bool { return true }
	syncIDEOnboardingStateOffline(home, "new@example.invalid", true)
	for _, db := range []string{darwinDB, linuxDB} {
		out, _ := exec.Command(sqliteBin, db, "SELECT value FROM ItemTable WHERE key='google.google-antigravity';").Output()
		if !strings.Contains(string(out), `"old@example.invalid"`) {
			t.Fatalf("state.vscdb was mutated while IDE was running: %s", out)
		}
	}

	// 2. When IDE is closed, syncIDEOnboardingStateOffline updates BOTH lastLoginUsername AND lastLoginIsGcpTos!
	ideRunningCheck = func() bool { return false }
	syncIDEOnboardingStateOffline(home, "gcp-user@example.invalid", true)
	activeDB := darwinDB
	if _, statErr := os.Stat(ideStateDBPaths(home)[0]); statErr == nil {
		activeDB = ideStateDBPaths(home)[0]
	}
	out, _ := exec.Command(sqliteBin, activeDB, "SELECT value FROM ItemTable WHERE key='google.google-antigravity';").Output()
	if !strings.Contains(string(out), `"gcp-user@example.invalid"`) || !strings.Contains(string(out), `"agy.onboarding.lastLoginIsGcpTos":true`) {
		t.Fatalf("expected offline sync to update both username and isGcpTos, got: %s", out)
	}
}

func TestPhase2AppRestartGracefulSequenceAbortsIfIDEDoesNotExit(t *testing.T) {
	home := t.TempDir()
	var quitCalled, launchCalled, syncedOffline bool

	// Case 1: IDE refuses to exit (e.g., dirty Save dialog) -> must NOT relaunch or write state.vscdb!
	err := runGracefulIDERestart(context.Background(), ideRestartHooks{
		isRunning:    func() bool { return true },
		gracefulQuit: func(context.Context) error { quitCalled = true; return nil },
		syncOffline:  func() { syncedOffline = true },
		relaunch:     func(context.Context) error { launchCalled = true; return nil },
		waitTimeout:  60 * time.Millisecond,
		pollEvery:    10 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected error when IDE does not exit cleanly")
	}
	if !quitCalled || syncedOffline || launchCalled {
		t.Fatalf("expected abort without sync/relaunch when IDE stays open: quit=%v sync=%v launch=%v", quitCalled, syncedOffline, launchCalled)
	}

	// Case 2: IDE exits cleanly after quit -> writes offline state.vscdb -> relaunches!
	_ = home
	var steps []string
	var running atomic.Bool
	running.Store(true)
	err = runGracefulIDERestart(context.Background(), ideRestartHooks{
		isRunning: running.Load,
		gracefulQuit: func(context.Context) error {
			steps = append(steps, "quit")
			running.Store(false)
			return nil
		},
		syncOffline: func() { steps = append(steps, "sync") },
		relaunch:    func(context.Context) error { steps = append(steps, "relaunch"); return nil },
		waitTimeout: time.Second,
		pollEvery:   10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("expected graceful restart success, got %v", err)
	}
	if strings.Join(steps, "->") != "quit->sync->relaunch" {
		t.Fatalf("unexpected restart sequence: %v", steps)
	}

	// Case 3: IDE is NOT running -> never cold-launches IDE, only syncs offline state!
	steps = nil
	err = runGracefulIDERestart(context.Background(), ideRestartHooks{
		isRunning:    func() bool { return false },
		gracefulQuit: func(context.Context) error { steps = append(steps, "quit"); return nil },
		syncOffline:  func() { steps = append(steps, "sync") },
		relaunch:     func(context.Context) error { steps = append(steps, "relaunch"); return nil },
		waitTimeout:  time.Second,
		pollEvery:    10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("unexpected error when IDE closed: %v", err)
	}
	if strings.Join(steps, "->") != "sync" {
		t.Fatalf("closed IDE must never cold-launch, got steps: %v", steps)
	}
	_ = io.Discard
}

func TestPhase2QuotaServiceUsesLocalHubFastPathWithZeroGoogleCalls(t *testing.T) {
	var remoteHits atomic.Int32
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		remoteHits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer remote.Close()

	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/exa.language_server_pb.LanguageServerService/GetAuthStatus":
			fmt.Fprint(w, `{"email":"active@example.invalid","tier":{"id":"g1-ultra-tier","name":"Google AI Ultra"}}`)
		case "/exa.language_server_pb.LanguageServerService/RetrieveUserQuotaSummary":
			fmt.Fprint(w, `{"groups":[{"buckets":[{"bucketId":"claude-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.82,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer hub.Close()

	u, _ := url.Parse(hub.URL)
	port, _ := strconv.Atoi(u.Port())
	home := t.TempDir()
	daemonDir := filepath.Join(home, ".gemini", "antigravity", "daemon")
	if err := os.MkdirAll(daemonDir, 0o700); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "httpPort": port, "csrfToken": "tok"})
	if err := os.WriteFile(filepath.Join(daemonDir, fmt.Sprintf("hub_%d.json", os.Getpid())), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	httpSvc := NewHTTPService(io.Discard)
	httpSvc.cloudAPI = remote.URL
	q := NewQuotaService(httpSvc, nil)
	q.localHubHome = home

	acc := Account{"email": "active@example.invalid", "token_data": sampleTokenWithRefresh("acc-tok", "ref-tok", time.Now().Add(time.Hour))}
	snap, err := q.Fetch(context.Background(), acc)
	if err != nil || snap == nil {
		t.Fatalf("expected QuotaService.Fetch to use local hub fast path, err=%v", err)
	}
	if remoteHits.Load() != 0 {
		t.Fatalf("expected 0 remote Google HTTP calls when local hub matches active email, got %d", remoteHits.Load())
	}

	// Verify CLI flags --restart-ide and --no-restart-ide parse cleanly.
	parsedRestart, err := parseCLI([]string{"switch", "active@example.invalid", "--restart-ide"})
	if err != nil || !parsedRestart.restartIDE || reloadOverrideFromArgs(parsedRestart) != "app-restart" {
		t.Fatalf("expected --restart-ide override, got %+v err=%v", parsedRestart, err)
	}
	parsedManual, err := parseCLI([]string{"next", "--no-restart-ide"})
	if err != nil || !parsedManual.noRestartIDE || reloadOverrideFromArgs(parsedManual) != "manual" {
		t.Fatalf("expected --no-restart-ide override, got %+v err=%v", parsedManual, err)
	}
}
