//go:build !windows

package app

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func credentialContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

// securityBinary is the code identity that owns the gemini/antigravity item.
// The agy CLI reads that item by spawning this tool. A partition list admits
// only the identity that created the item, so this slot stays on /usr/bin/security.
// The agy-swap vault uses keychainSet instead and never this binary.
var securityBinary = "/usr/bin/security"

func geminiSecurityArgs(action, token string) []string {
	switch action {
	case "set":
		return []string{"add-generic-password", "-U", "-a", "antigravity", "-s", "gemini", "-w", token, "-A"}
	case "get":
		return []string{"find-generic-password", "-a", "antigravity", "-s", "gemini", "-w"}
	case "delete":
		return []string{"delete-generic-password", "-a", "antigravity", "-s", "gemini"}
	default:
		return nil
	}
}

func runGeminiSecurity(ctx context.Context, args []string) ([]byte, error) {
	command := exec.CommandContext(ctx, securityBinary, args...)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	err := command.Run()
	return stdout.Bytes(), err
}

func platformCredentialGet(parent context.Context) string {
	ctx, cancel := credentialContext(parent, 5*time.Second)
	defer cancel()
	if runtime.GOOS == "darwin" {
		output, err := runGeminiSecurity(ctx, geminiSecurityArgs("get", ""))
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(output))
	}
	var command *exec.Cmd
	if runtime.GOOS == "linux" {
		command = exec.CommandContext(ctx, "secret-tool", "lookup", "service", "gemini", "username", "antigravity")
	} else {
		return ""
	}
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func platformCredentialSet(parent context.Context, token string) bool {
	ctx, cancel := credentialContext(parent, 10*time.Second)
	defer cancel()
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		if token == "" {
			return false
		}
		_, err := runGeminiSecurity(ctx, geminiSecurityArgs("set", token))
		return err == nil
	} else if runtime.GOOS == "linux" {
		command = exec.CommandContext(ctx, "secret-tool", "store", "--label=gemini", "service", "gemini", "username", "antigravity")
		command.Stdin = bytes.NewBufferString(token)
	} else {
		return false
	}
	return command.Run() == nil
}

func platformCredentialDelete(parent context.Context) bool {
	ctx, cancel := credentialContext(parent, 5*time.Second)
	defer cancel()
	if runtime.GOOS == "darwin" {
		_, err := runGeminiSecurity(ctx, geminiSecurityArgs("delete", ""))
		return err == nil
	}
	var command *exec.Cmd
	if runtime.GOOS == "linux" {
		command = exec.CommandContext(ctx, "secret-tool", "clear", "service", "gemini", "username", "antigravity")
	} else {
		return false
	}
	if command.Run() == nil {
		return true
	}
	return platformCredentialGet(parent) == ""
}
