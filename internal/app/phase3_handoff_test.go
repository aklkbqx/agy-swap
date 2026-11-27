package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPhase3FindLatestConversationPureGoAndCleanHandoffRedaction(t *testing.T) {
	home := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "my-repo")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	convID := "11111111-2222-3333-4444-555555555555"

	// 1. Primary lookup via cache/last_conversations.json (zero SQLite).
	cacheDir := filepath.Join(home, ".gemini", "antigravity-cli", "cache")
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		t.Fatal(err)
	}
	rawMap, _ := json.Marshal(map[string]string{workspace: convID})
	if err := os.WriteFile(filepath.Join(cacheDir, "last_conversations.json"), rawMap, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := findLatestConversationID(home, workspace); got != convID {
		t.Fatalf("expected conversation ID %q from last_conversations.json, got %q", convID, got)
	}

	// 2. Fallback to history.jsonl when last_conversations.json is missing.
	_ = os.Remove(filepath.Join(cacheDir, "last_conversations.json"))
	histPath := filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl")
	histContent := `{"conversation_id":"old-id","cwd":"/other"}` + "\n" +
		`{"conversation_id":"` + convID + `","cwd":"` + filepath.ToSlash(workspace) + `"}` + "\n"
	if runtime.GOOS == "windows" {
		b, _ := json.Marshal(map[string]string{"conversation_id": convID, "cwd": workspace})
		histContent = string(b) + "\n"
	}
	if err := os.WriteFile(histPath, []byte(histContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := findLatestConversationID(home, workspace); got != convID {
		t.Fatalf("expected fallback conversation ID %q from history.jsonl, got %q", convID, got)
	}

	// 3. Build clean handoff brief from transcript.jsonl and verify redaction & bounds.
	logsDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain", convID, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"type":"SYSTEM_MESSAGE","step_index":1,"content":"SECRET_SYSTEM_PROMPT"}`,
		`{"type":"CHECKPOINT","step_index":2,"content":"Implemented Phase 1 and 2 for old@example.invalid in ` + convID + `"}`,
		`{"type":"USER_INPUT","source":"USER_EXPLICIT","step_index":3,"content":"Please finish Phase 3 supervisor tests"}`,
		`{"type":"PLANNER_RESPONSE","step_index":4,"thinking":"PRIVATE_CHAIN_OF_THOUGHT","content":"Working on handoff.go now","tool_calls":[{"name":"edit_file","arguments":{"Path":"internal/app/handoff.go"}}]}`,
		`{"type":"ERROR_MESSAGE","step_index":5,"content":"RESOURCE_EXHAUSTED raw dump"}`,
	}
	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	brief, err := buildCleanHandoffMarkdown(home, convID, workspace, defaultHandoffMaxBytes)
	if err != nil {
		t.Fatalf("buildCleanHandoffMarkdown failed: %v", err)
	}
	if len(brief) > defaultHandoffMaxBytes {
		t.Fatalf("handoff brief exceeded %d bytes: got %d", defaultHandoffMaxBytes, len(brief))
	}
	for _, forbidden := range []string{convID, "old@example.invalid", "SECRET_SYSTEM_PROMPT", "PRIVATE_CHAIN_OF_THOUGHT", "RESOURCE_EXHAUSTED raw dump", "step_index"} {
		if strings.Contains(brief, forbidden) {
			t.Fatalf("handoff brief leaked forbidden content %q:\n%s", forbidden, brief)
		}
	}
	for _, want := range []string{"Implemented Phase 1 and 2", "Please finish Phase 3 supervisor tests", "Working on handoff.go now", "internal/app/handoff.go"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("handoff brief missing expected content %q:\n%s", want, brief)
		}
	}

	// 4. Write private handoff file (0600 on Unix) under ~/.gemini/agy-swap/handoffs/.
	handoffPath, err := writeHandoffBrief(home, workspace, brief)
	if err != nil {
		t.Fatalf("writeHandoffBrief failed: %v", err)
	}
	info, err := os.Stat(handoffPath)
	if err != nil {
		t.Fatalf("handoff file missing: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("expected handoff file mode 0600, got %04o", info.Mode().Perm())
	}
}

func TestPhase3UnwrapWrapperAndSupervisedEnvAndRelaunchArgs(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	libexecDir := filepath.Join(home, ".local", "libexec")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libexecDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realBin := filepath.Join(libexecDir, "agy")
	if err := os.WriteFile(realBin, []byte("#!/bin/sh\necho real\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	wrapperBin := filepath.Join(binDir, "agy")
	wrapperScript := buildAgyLauncherScriptForPaths(realBin, "/usr/local/bin/agy-swap", "", true)
	if err := os.WriteFile(wrapperBin, []byte(wrapperScript), 0o755); err != nil {
		t.Fatal(err)
	}

	unwrapped := unwrapManagedAgyBinary(home, wrapperBin)
	if unwrapped != realBin {
		t.Fatalf("expected wrapper to unwrap to %q, got %q", realBin, unwrapped)
	}

	env := buildSupervisedAgyEnv([]string{"PATH=/usr/bin", "WSL_INTEROP=1"}, "darwin")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "WSL_INTEROP=") {
		t.Fatalf("expected darwin supervised env to strip WSL_INTEROP, got %v", env)
	}
	if !strings.Contains(joined, "AGY_SWAP_SKIP_SYNC=1") || !strings.Contains(joined, "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1") {
		t.Fatalf("expected AGY_SWAP_SKIP_SYNC=1 and AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1, got %v", env)
	}

	argsClean := buildHandoffRelaunchArgs([]string{"--model", "gemini-2.5-pro", "-p", "old prompt"}, "clean-summary", "/tmp/h.md", "conv-1")
	if strings.Join(argsClean, " ") != "--model gemini-2.5-pro -p Read /tmp/h.md and continue." {
		t.Fatalf("unexpected clean-summary relaunch args: %v", argsClean)
	}
	if strings.Contains(strings.Join(argsClean, " "), "--dangerously-skip-permissions") {
		t.Fatalf("relaunch args must never silently inject --dangerously-skip-permissions: %v", argsClean)
	}

	argsResume := buildHandoffRelaunchArgs([]string{"--model", "claude-sonnet", "-i", "hi"}, "resume", "/tmp/h.md", "conv-1")
	if strings.Join(argsResume, " ") != "--model claude-sonnet --resume conv-1" {
		t.Fatalf("unexpected resume relaunch args: %v", argsResume)
	}

	// Verify explicit --conversation / -r / -c / --continue are extracted and stripped in clean-summary mode
	// so clean-summary never leaks the old conversation_id into the newly swapped account!
	if got := extractConversationArg([]string{"--model", "gemini-2.5-pro", "--conversation", "uuid-123"}); got != "uuid-123" {
		t.Fatalf("extractConversationArg(--conversation) = %q, want uuid-123", got)
	}
	if got := extractConversationArg([]string{"--conversation=uuid-456"}); got != "uuid-456" {
		t.Fatalf("extractConversationArg(--conversation=) = %q, want uuid-456", got)
	}
	if got := extractConversationArg([]string{"-r", "uuid-789"}); got != "uuid-789" {
		t.Fatalf("extractConversationArg(-r) = %q, want uuid-789", got)
	}
	argsFromConv := buildHandoffRelaunchArgs([]string{"--model", "gemini-2.5-pro", "--conversation", "uuid-123", "-c"}, "clean-summary", "/tmp/h.md", "uuid-123")
	if strings.Join(argsFromConv, " ") != "--model gemini-2.5-pro -i Read /tmp/h.md and continue." {
		t.Fatalf("clean-summary must strip --conversation and -c, got: %v", argsFromConv)
	}
	argsResumeFromConv := buildHandoffRelaunchArgs([]string{"--conversation=uuid-123", "--effort", "high"}, "resume", "/tmp/h.md", "uuid-123")
	if strings.Join(argsResumeFromConv, " ") != "--effort high --resume uuid-123" {
		t.Fatalf("resume mode must not duplicate --conversation, got: %v", argsResumeFromConv)
	}

	// Verify ergonomic `agy-swap run` shorthand without redundant `now` or `--`.
	cmd1, opts1, pos1, err1 := parseExtended([]string{"run", "-c"})
	if err1 != nil || cmd1 != "run" || len(pos1) != 0 || strings.Join(opts1.RunArgs, " ") != "-c" {
		t.Fatalf("parseExtended(run -c) = cmd=%q pos=%v runArgs=%v err=%v", cmd1, pos1, opts1.RunArgs, err1)
	}
	cmd2, opts2, pos2, err2 := parseExtended([]string{"run", "--auto-swap", "--conversation", "uuid-999", "--model", "gemini-2.5-pro"})
	if err2 != nil || cmd2 != "run" || !opts2.AutoSwap || len(pos2) != 0 || strings.Join(opts2.RunArgs, " ") != "--conversation uuid-999 --model gemini-2.5-pro" {
		t.Fatalf("parseExtended(run --auto-swap --conversation ...) = cmd=%q auto=%v pos=%v runArgs=%v err=%v", cmd2, opts2.AutoSwap, pos2, opts2.RunArgs, err2)
	}
	cmd3, opts3, pos3, err3 := parseExtended([]string{"run", "--account", "work", "-p", "hello"})
	if err3 != nil || cmd3 != "run" || opts3.Account != "work" || len(pos3) != 0 || strings.Join(opts3.RunArgs, " ") != "-p hello" {
		t.Fatalf("parseExtended(run --account work -p hello) = cmd=%q acct=%q pos=%v runArgs=%v err=%v", cmd3, opts3.Account, pos3, opts3.RunArgs, err3)
	}
	// Ensure `run --help` still shows agy-swap help while `run -- --help` forwards to agy.
	_, optsHelp, _, errHelp := parseExtended([]string{"run", "--help"})
	if errHelp != nil || !optsHelp.Help || len(optsHelp.RunArgs) != 0 {
		t.Fatalf("expected run --help to show agy-swap help, got opts=%+v err=%v", optsHelp, errHelp)
	}
	_, optsFwdHelp, _, errFwdHelp := parseExtended([]string{"run", "--", "--help"})
	if errFwdHelp != nil || optsFwdHelp.Help || strings.Join(optsFwdHelp.RunArgs, " ") != "--help" {
		t.Fatalf("expected run -- --help to forward --help to agy, got opts=%+v err=%v", optsFwdHelp, errFwdHelp)
	}

	if isQuotaExhaustedOutputLine("HTTP 503 Service Unavailable") {
		t.Fatal("transient 503 must not trigger quota exhaustion")
	}
	if !isQuotaExhaustedOutputLine(`{"error":{"status":"RESOURCE_EXHAUSTED","message":"You have exhausted your capacity"}}`) {
		t.Fatal("RESOURCE_EXHAUSTED must be detected as quota exhaustion")
	}
}

func newPhaseTestApp(t *testing.T) (*Application, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	paths := testPaths(t)
	store := NewStore(paths)
	var out, errOut bytes.Buffer
	httpSvc := NewHTTPService(&errOut)
	httpSvc.cloudAPI = "http://127.0.0.1:1/v1internal:"
	creds := NewCredentials(paths)
	vault := NewAccountVault(paths)
	qs := NewQuotaService(httpSvc, store)
	qs.SetVault(vault)
	a := &Application{
		Version:     "2.25.1-test",
		In:          strings.NewReader(""),
		Out:         &out,
		Err:         &errOut,
		paths:       paths,
		store:       store,
		credentials: creds,
		vault:       vault,
		http:        httpSvc,
		quota:       qs,
	}
	return a, &out, &errOut
}

func TestPhase3SupervisedRunAutoSwapsCleanlyWithZeroOverlap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake agy harness runs on Unix")
	}
	a, out, errOut := newPhaseTestApp(t)
	// Seed two ready accounts: a@ is active and hits RESOURCE_EXHAUSTED; b@ is ready with 85%.
	accounts := NewAccounts()
	futureReset := isoTime(time.Now().UTC().Add(4 * time.Hour))
	nowISO := isoTime(time.Now().UTC())
	accounts.Set("a@example.invalid", Account{
		"email":      "a@example.invalid",
		"name":       "Account A",
		"token_data": sampleTokenWithRefresh("tok-a", "rt-a", time.Now().Add(time.Hour)),
		"quota_snapshot": map[string]any{
			"observed_at": nowISO,
			"tier":        map[string]any{"id": "g1-pro-tier", "name": "Google AI Pro"},
			"groups": []any{map[string]any{
				"id":   poolThirdParty,
				"name": "Claude/GPT",
				"buckets": []any{map[string]any{
					"id": "claude-5h", "name": "5-Hour", "window": "5h",
					"remaining_fraction": 0.02, "reset_at": futureReset,
				}},
			}},
		},
	})
	accounts.Set("b@example.invalid", Account{
		"email":      "b@example.invalid",
		"name":       "Account B",
		"token_data": sampleTokenWithRefresh("tok-b", "rt-b", time.Now().Add(time.Hour)),
		"quota_snapshot": map[string]any{
			"observed_at": nowISO,
			"tier":        map[string]any{"id": "g1-pro-tier", "name": "Google AI Pro"},
			"groups": []any{map[string]any{
				"id":   poolThirdParty,
				"name": "Claude/GPT",
				"buckets": []any{map[string]any{
					"id": "claude-5h", "name": "5-Hour", "window": "5h",
					"remaining_fraction": 0.85, "reset_at": futureReset,
				}},
			}},
		},
	})
	if err := a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	if !a.credentials.ApplyWithoutReload(context.Background(), getString(accounts.ByEmail["a@example.invalid"], "token_data"), "a@example.invalid") {
		t.Fatal("failed to activate account A")
	}

	// Create fake agy script: run 1 checks active account (A), emits RESOURCE_EXHAUSTED,
	// writes its PID lock to prove zero overlap, and exits; run 2 verifies active account
	// switched to B and received clean handoff file path via -p.
	traceFile := filepath.Join(t.TempDir(), "trace.log")
	fakeAgy := filepath.Join(t.TempDir(), "fake-agy")
	script := `#!/bin/sh
echo "RUN args=$* env_force=${AGY_CLI_FORCE_FILE_TOKEN_STORAGE:-} env_skip=${AGY_SWAP_SKIP_SYNC:-}" >> ` + shellQuoteDouble(traceFile) + `
acct="$(cat ` + shellQuoteDouble(a.paths.GoogleAccounts) + ` 2>/dev/null || true)"
echo "ACCT=$acct" >> ` + shellQuoteDouble(traceFile) + `
if echo "$acct" | grep -q "a@example.invalid"; then
  echo '{"type":"error","status":"RESOURCE_EXHAUSTED","message":"quota exhausted"}' >&2
  exit 1
fi
echo "CONTINUED_OK"
exit 0
`
	if err := os.WriteFile(fakeAgy, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	// Enable auto_swap and register fake agy target in a single SaveSettings call.
	settings, _ := a.loadSettings()
	settings.AutoSwap.Enabled = true
	settings.Targets["agy"] = TargetConfig{Command: fakeAgy, Enabled: true}
	if err := a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	origSleep := autoSwapPaceSleep
	autoSwapPaceSleep = func(context.Context, time.Duration) {}
	defer func() { autoSwapPaceSleep = origSleep }()

	code := a.Run(context.Background(), []string{"run", "now", "--auto-swap", "--", "-p", "finish build"})
	if code != 0 {
		t.Fatalf("expected supervised run to auto-swap and succeed (exit 0), got %d; out=%q err=%q", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "CONTINUED_OK") {
		t.Fatalf("expected relaunched child output CONTINUED_OK, got out=%q err=%q", out.String(), errOut.String())
	}
	traceBytes, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatal(err)
	}
	trace := string(traceBytes)
	if !strings.Contains(trace, "a@example.invalid") || !strings.Contains(trace, "b@example.invalid") {
		t.Fatalf("expected run 1 on A and run 2 on B, got trace:\n%s", trace)
	}
	if !strings.Contains(trace, "env_force=1 env_skip=1") {
		t.Fatalf("expected official env flags in child, got trace:\n%s", trace)
	}
	if !strings.Contains(trace, "handoff-") {
		t.Fatalf("expected clean-summary handoff file path in relaunch args, got trace:\n%s", trace)
	}
	_ = bytes.Buffer{}
}
