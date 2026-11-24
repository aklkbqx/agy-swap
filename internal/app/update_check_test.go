package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var nginxToolUARegex = regexp.MustCompile(`^agy-swap/([0-9]+\.[0-9]+\.[0-9]+) \((darwin|linux|windows); (amd64|arm64); (brew|binary|go-install)\)$`)

func withUpdateCheckTestHooks(
	t *testing.T,
	execFn func() (string, error),
	evalFn func(string) (string, error),
	buildFn func() (*debug.BuildInfo, bool),
	nowFn func() time.Time,
) {
	t.Helper()
	origExec := executablePath
	origEval := evalSymlinks
	origBuild := readBuildInfo
	origNow := updateCheckNow
	if execFn != nil {
		executablePath = execFn
	}
	if evalFn != nil {
		evalSymlinks = evalFn
	}
	if buildFn != nil {
		readBuildInfo = buildFn
	}
	if nowFn != nil {
		updateCheckNow = nowFn
	}
	t.Cleanup(func() {
		executablePath = origExec
		evalSymlinks = origEval
		readBuildInfo = origBuild
		updateCheckNow = origNow
	})
}

func newTestUpdateCheckApp(t *testing.T, buildID string) (*Application, *bytes.Buffer) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	var errBuf bytes.Buffer
	app, err := New("2.25.0", strings.NewReader(""), io.Discard, &errBuf)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	app.BuildID = buildID
	return app, &errBuf
}

func TestUpdateCheckUserAgentMatchesNginxRegex(t *testing.T) {
	t.Run("homebrew symlink detection", func(t *testing.T) {
		withUpdateCheckTestHooks(
			t,
			func() (string, error) { return "/opt/homebrew/bin/agy-swap", nil },
			func(string) (string, error) { return "/opt/homebrew/Cellar/agy-swap/2.25.0/bin/agy-swap", nil },
			func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{}, true },
			nil,
		)
		ua := updateCheckUserAgent("v2.25.0-dev", "release-v2.25.0")
		m := nginxToolUARegex.FindStringSubmatch(ua)
		if m == nil {
			t.Fatalf("User-Agent %q does not match nginx regex", ua)
		}
		if m[1] != "2.25.0" || m[4] != "brew" {
			t.Fatalf("unexpected capture groups %v from %q", m, ua)
		}
	})

	t.Run("go install clean build info", func(t *testing.T) {
		withUpdateCheckTestHooks(
			t,
			func() (string, error) { return "/Users/dev/go/bin/agy-swap", nil },
			func(p string) (string, error) { return p, nil },
			func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{Main: debug.Module{Version: "v2.25.0"}}, true
			},
			nil,
		)
		ua := updateCheckUserAgent("2.25.0", "dev")
		m := nginxToolUARegex.FindStringSubmatch(ua)
		if m == nil {
			t.Fatalf("User-Agent %q does not match nginx regex", ua)
		}
		if m[4] != "go-install" {
			t.Fatalf("got channel %q, want go-install", m[4])
		}
	})

	t.Run("direct release binary", func(t *testing.T) {
		withUpdateCheckTestHooks(
			t,
			func() (string, error) { return "/Users/dev/.local/bin/agy-swap", nil },
			func(p string) (string, error) { return p, nil },
			func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{Main: debug.Module{Version: "v2.25.0"}}, true
			},
			nil,
		)
		ua := updateCheckUserAgent("2.25.0", "release-v2.25.0")
		m := nginxToolUARegex.FindStringSubmatch(ua)
		if m == nil {
			t.Fatalf("User-Agent %q does not match nginx regex", ua)
		}
		if m[4] != "binary" {
			t.Fatalf("got channel %q, want binary", m[4])
		}
	})
}

func TestUpdateCheckSkipsDemoOptOutCIAndDevBuilds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"version":"2.26.0"}`))
	}))
	defer srv.Close()
	t.Setenv("AGY_SWAP_UPDATE_CHECK_URL", srv.URL+"/version.json")

	clearEnv := func(t *testing.T) {
		t.Helper()
		for _, k := range []string{"AGY_SWAP_NO_UPDATE_CHECK", "DO_NOT_TRACK", "CI", "GITHUB_ACTIONS"} {
			t.Setenv(k, "")
		}
	}

	t.Run("local vcs modified dev build skips", func(t *testing.T) {
		clearEnv(t)
		withUpdateCheckTestHooks(
			t,
			func() (string, error) { return "/tmp/agy-swap", nil },
			func(p string) (string, error) { return p, nil },
			func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{
					Main:     debug.Module{Version: "(devel)"},
					Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}},
				}, true
			},
			nil,
		)
		app, _ := newTestUpdateCheckApp(t, "dev")
		if got := app.checkForUpdate(context.Background()); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})

	t.Run("demo optout and CI skip", func(t *testing.T) {
		withUpdateCheckTestHooks(
			t,
			func() (string, error) { return "/tmp/agy-swap", nil },
			func(p string) (string, error) { return p, nil },
			func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{}, true },
			nil,
		)
		for _, env := range []struct{ key, val string }{
			{"AGY_SWAP_NO_UPDATE_CHECK", "1"},
			{"DO_NOT_TRACK", "1"},
			{"CI", "true"},
			{"GITHUB_ACTIONS", "true"},
		} {
			clearEnv(t)
			t.Setenv(env.key, env.val)
			app, _ := newTestUpdateCheckApp(t, "release-v2.25.0")
			if got := app.checkForUpdate(context.Background()); got != "" {
				t.Fatalf("%s=%s returned %q, want empty", env.key, env.val, got)
			}
		}
		clearEnv(t)
		app, _ := newTestUpdateCheckApp(t, "release-v2.25.0")
		app.demo = true
		if got := app.checkForUpdate(context.Background()); got != "" {
			t.Fatalf("demo returned %q, want empty", got)
		}
	})

	if hits.Load() != 0 {
		t.Fatalf("expected 0 HTTP requests for skipped checks, got %d", hits.Load())
	}

	t.Run("clean go install runs check", func(t *testing.T) {
		clearEnv(t)
		withUpdateCheckTestHooks(
			t,
			func() (string, error) { return "/Users/dev/go/bin/agy-swap", nil },
			func(p string) (string, error) { return p, nil },
			func() (*debug.BuildInfo, bool) {
				return &debug.BuildInfo{Main: debug.Module{Version: "v2.25.0"}}, true
			},
			nil,
		)
		app, _ := newTestUpdateCheckApp(t, "dev")
		if got := app.checkForUpdate(context.Background()); got != "2.26.0" {
			t.Fatalf("clean go install got %q, want 2.26.0", got)
		}
		if hits.Load() != 1 {
			t.Fatalf("expected 1 HTTP request, got %d", hits.Load())
		}
	})
}

func TestUpdateCheckCachesFor24HoursAndHandlesErrorsSilently(t *testing.T) {
	for _, k := range []string{"AGY_SWAP_NO_UPDATE_CHECK", "DO_NOT_TRACK", "CI", "GITHUB_ACTIONS"} {
		t.Setenv(k, "")
	}

	var hits atomic.Int32
	var seenUA atomic.Value
	var statusCode atomic.Int32
	var payload atomic.Value
	statusCode.Store(http.StatusOK)
	payload.Store(`{"version":"2.26.0"}`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		seenUA.Store(r.Header.Get("User-Agent"))
		w.WriteHeader(int(statusCode.Load()))
		_, _ = w.Write([]byte(payload.Load().(string)))
	}))
	defer srv.Close()
	t.Setenv("AGY_SWAP_UPDATE_CHECK_URL", srv.URL+"/version.json")

	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	withUpdateCheckTestHooks(
		t,
		func() (string, error) { return "/usr/local/bin/agy-swap", nil },
		func(p string) (string, error) { return p, nil },
		func() (*debug.BuildInfo, bool) { return &debug.BuildInfo{}, true },
		func() time.Time { return now },
	)

	app, errBuf := newTestUpdateCheckApp(t, "release-v2.25.0")

	if got := app.checkForUpdate(context.Background()); got != "2.26.0" {
		t.Fatalf("first check got %q, want 2.26.0", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected 1 hit, got %d", hits.Load())
	}
	if ua, _ := seenUA.Load().(string); !nginxToolUARegex.MatchString(ua) {
		t.Fatalf("sent invalid User-Agent %q", ua)
	}

	cachePath := filepath.Join(app.paths.ConfigDir, updateCheckFileName)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(cachePath)
		if err != nil {
			t.Fatalf("stat cache: %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0o600 {
			t.Fatalf("cache mode = %o, want 0600", mode)
		}
	}

	// Within 24h: zero additional HTTP hits.
	now = now.Add(23 * time.Hour)
	if got := app.checkForUpdate(context.Background()); got != "2.26.0" {
		t.Fatalf("cached check got %q, want 2.26.0", got)
	}
	if hits.Load() != 1 {
		t.Fatalf("expected cached hit count 1, got %d", hits.Load())
	}

	// Expired (>24h) + server HTTP 500: quiet failure, preserves cache CheckedAt.
	now = now.Add(2 * time.Hour)
	beforeCheckedAt := now.Add(-25 * time.Hour)
	statusCode.Store(http.StatusInternalServerError)
	payload.Store(`upstream error`)
	if got := app.checkForUpdate(context.Background()); got != "" {
		t.Fatalf("HTTP 500 check got %q, want empty", got)
	}
	if errBuf.Len() != 0 {
		t.Fatalf("expected empty stderr on failure, got %q", errBuf.String())
	}
	if hits.Load() != 2 {
		t.Fatalf("expected 2 hits after expiry, got %d", hits.Load())
	}
	raw, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var cached updateCheckCache
	if err := json.Unmarshal(raw, &cached); err != nil {
		t.Fatalf("unmarshal cache: %v", err)
	}
	if !cached.CheckedAt.Equal(beforeCheckedAt) {
		t.Fatalf("cache CheckedAt mutated on HTTP 500: got %v want %v", cached.CheckedAt, beforeCheckedAt)
	}

	// Same/older version returns "" and refreshes cache for 24h.
	statusCode.Store(http.StatusOK)
	payload.Store(`{"version":"2.25.0"}`)
	if got := app.checkForUpdate(context.Background()); got != "" {
		t.Fatalf("same version got %q, want empty", got)
	}
	if hits.Load() != 3 {
		t.Fatalf("expected 3 hits, got %d", hits.Load())
	}
	now = now.Add(1 * time.Hour)
	if got := app.checkForUpdate(context.Background()); got != "" {
		t.Fatalf("cached same version got %q, want empty", got)
	}
	if hits.Load() != 3 {
		t.Fatalf("expected same-version check to be cached for 24h, hits=%d", hits.Load())
	}
}

func TestTUIHeaderShowsAvailableUpdateWithoutDisturbingDefault(t *testing.T) {
	app, _ := newTestUpdateCheckApp(t, "release-v2.25.0")
	baseLines := app.tuiHeaderLinesWithVersion("2.25.0", &tuiState{}, 100)
	baseText := stripEscapes(strings.Join(baseLines, "\n"))
	if strings.Contains(baseText, "↑") {
		t.Fatalf("default header unexpectedly showed update arrow: %q", baseText)
	}

	updatedLines := app.tuiHeaderLinesWithVersion("2.25.0", &tuiState{availableUpdate: "2.26.0"}, 100)
	updatedText := stripEscapes(strings.Join(updatedLines, "\n"))
	if !strings.Contains(updatedText, "v2.25.0") || !strings.Contains(updatedText, "v2.26.0 ↑") {
		t.Fatalf("header missing expected version hint: %q", updatedText)
	}
}
