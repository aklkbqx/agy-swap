//go:build !windows

package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGeminiSecurityArgsUpdateInPlace(t *testing.T) {
	set := geminiSecurityArgs("set", "stub-token")
	if strings.Join(set, " ") != "add-generic-password -U -a antigravity -s gemini -w stub-token -A" {
		t.Fatalf("set args = %q", set)
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
	if len(calls) != 3 {
		t.Fatalf("security invocations = %d, want 3\n%s", len(calls), data)
	}
	if calls[0] != "add-generic-password\n-U\n-a\nantigravity\n-s\ngemini\n-w\nstub-token\n-A" {
		t.Fatalf("set invocation =\n%s", calls[0])
	}
	if strings.Contains(calls[0], "delete-generic-password") {
		t.Fatal("set invocation deletes the item")
	}
	if calls[1] != "find-generic-password\n-a\nantigravity\n-s\ngemini\n-w" {
		t.Fatalf("get invocation =\n%s", calls[1])
	}
	if calls[2] != "delete-generic-password\n-a\nantigravity\n-s\ngemini" {
		t.Fatalf("delete invocation =\n%s", calls[2])
	}
}

func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", "'\\''") + "'"
}
