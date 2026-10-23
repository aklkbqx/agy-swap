//go:build !windows

package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestGeminiSecurityArgsUpdateInPlace(t *testing.T) {
	set := geminiSecurityArgs("set", "stub-token")
	if strings.Join(set, " ") != "add-generic-password -U -a antigravity -s gemini -w stub-token" {
		t.Fatalf("set args = %q (an update must keep the item's access list: -A replaces it and macOS asks for the keychain password)", set)
	}
	if got := strings.Join(geminiSecurityArgs("create", "stub-token"), " "); got != "add-generic-password -a antigravity -s gemini -w stub-token -A" {
		t.Fatalf("create args = %q", got)
	}
	if got := strings.Join(geminiSecurityArgs("exists", ""), " "); got != "find-generic-password -a antigravity -s gemini" {
		t.Fatalf("exists args = %q (it must not read the secret)", got)
	}
	if strings.Contains(strings.Join(set, " "), "delete-generic-password") {
		t.Fatal("set rewrites the item by deleting it first")
	}
	if got := strings.Join(geminiSecurityArgs("get", ""), " "); got != "find-generic-password -a antigravity -s gemini -w" {
		t.Fatalf("get args = %q", got)
	}
	if got := strings.Join(geminiSecurityArgs("delete", ""), " "); got != "delete-generic-password -a antigravity -s gemini" {
		t.Fatalf("delete args = %q", got)
	}
}

func TestPlatformCredentialSecurityStubFailure(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("gemini session credential uses /usr/bin/security on darwin")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	stub := filepath.Join(dir, "security")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> " + shellQuote(logPath) + "\nprintf '\\n' >> " + shellQuote(logPath) + "\nexit 1\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := securityBinary
	securityBinary = stub
	t.Cleanup(func() { securityBinary = previous })

	if platformCredentialSet(context.Background(), "stub-token") {
		t.Fatal("set reported success")
	}
	if got := platformCredentialGet(context.Background()); got != "" {
		t.Fatalf("get returned %q", got)
	}
	if platformCredentialDelete(context.Background()) {
		t.Fatal("delete reported success")
	}
	if platformCredentialSet(context.Background(), "") {
		t.Fatal("empty token was sent to security")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	rawCalls := strings.Split(strings.TrimRight(string(data), "\n"), "\n\n")
	calls := make([]string, 0, len(rawCalls))
	for _, call := range rawCalls {
		if call != "" {
			calls = append(calls, call)
		}
	}
	// The failing stub reports the item missing, so set tries to create it.
	if len(calls) != 4 {
		t.Fatalf("security invocations = %d, want 4\n%s", len(calls), data)
	}
	if calls[0] != "find-generic-password\n-a\nantigravity\n-s\ngemini" {
		t.Fatalf("exists invocation =\n%s", calls[0])
	}
	if calls[1] != "add-generic-password\n-a\nantigravity\n-s\ngemini\n-w\nstub-token\n-A" {
		t.Fatalf("create invocation =\n%s", calls[1])
	}
	if calls[2] != "find-generic-password\n-a\nantigravity\n-s\ngemini\n-w" {
		t.Fatalf("get invocation =\n%s", calls[2])
	}
	if calls[3] != "delete-generic-password\n-a\nantigravity\n-s\ngemini" {
		t.Fatalf("delete invocation =\n%s", calls[3])
	}
}

// securityStub replaces /usr/bin/security with a script that logs its
// arguments and answers find-generic-password with findStatus.
func securityStub(t *testing.T, findStatus int) func() []string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	stub := filepath.Join(dir, "security")
	script := "#!/bin/sh\nprintf '%s ' \"$@\" >> " + shellQuote(logPath) + "\nprintf '\\n' >> " + shellQuote(logPath) +
		"\nif [ \"$1\" = find-generic-password ]; then exit " + strconv.Itoa(findStatus) + "; fi\nexit 0\n"
	if err := os.WriteFile(stub, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := securityBinary
	securityBinary = stub
	t.Cleanup(func() { securityBinary = previous })
	return func() []string {
		data, _ := os.ReadFile(logPath)
		var calls []string
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			calls = append(calls, strings.TrimSpace(line))
		}
		return calls
	}
}

func TestSwitchUpdatesAnExistingSessionItemWithoutTouchingItsAccess(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("gemini session credential uses /usr/bin/security on darwin")
	}
	calls := securityStub(t, 0)
	if !platformCredentialSet(context.Background(), "stub-token") {
		t.Fatal("set reported failure")
	}
	got := calls()
	want := []string{"find-generic-password -a antigravity -s gemini", "add-generic-password -U -a antigravity -s gemini -w stub-token"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("security calls = %q, want %q", got, want)
	}
}

func TestSwitchCreatesAMissingSessionItemOpenToAntigravity(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("gemini session credential uses /usr/bin/security on darwin")
	}
	calls := securityStub(t, 44) // security's "item not found"
	if !platformCredentialSet(context.Background(), "stub-token") {
		t.Fatal("set reported failure")
	}
	got := calls()
	want := []string{"find-generic-password -a antigravity -s gemini", "add-generic-password -a antigravity -s gemini -w stub-token -A"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("security calls = %q, want %q", got, want)
	}
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}
