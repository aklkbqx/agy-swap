//go:build !windows

package app

import (
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func credentialContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

func platformCredentialGet(parent context.Context) string {
	ctx, cancel := credentialContext(parent, 5*time.Second)
	defer cancel()
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.CommandContext(ctx, "security", "find-generic-password", "-a", "antigravity", "-s", "gemini", "-w")
	} else if runtime.GOOS == "linux" {
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
		// macOS security's stdin form always opens /dev/tty and prompts. -X
		// avoids that prompt; hex encoding keeps the bearer token out of plain
		// text in the short-lived process argument list.
		passwordData := hex.EncodeToString([]byte(token))
		command = exec.CommandContext(ctx, "security", "add-generic-password", "-U", "-a", "antigravity", "-s", "gemini", "-X", passwordData)
		command.Stdout = io.Discard
		command.Stderr = io.Discard
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
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.CommandContext(ctx, "security", "delete-generic-password", "-a", "antigravity", "-s", "gemini")
	} else if runtime.GOOS == "linux" {
		command = exec.CommandContext(ctx, "secret-tool", "clear", "service", "gemini", "username", "antigravity")
	} else {
		return false
	}
	if command.Run() == nil {
		return true
	}
	return platformCredentialGet(parent) == ""
}
