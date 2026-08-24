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

func vaultContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, timeout)
}

func platformVaultGet(parent context.Context, ref string) string {
	ctx, cancel := vaultContext(parent, 5*time.Second)
	defer cancel()
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.CommandContext(ctx, "security", "find-generic-password", "-a", ref, "-s", "agy-swap", "-w")
	} else if runtime.GOOS == "linux" {
		command = exec.CommandContext(ctx, "secret-tool", "lookup", "service", "agy-swap", "username", ref)
	} else {
		return ""
	}
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func platformVaultSet(parent context.Context, ref, token string) bool {
	ctx, cancel := vaultContext(parent, 10*time.Second)
	defer cancel()
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		if token == "" {
			return false
		}
		passwordData := hex.EncodeToString([]byte(token))
		command = exec.CommandContext(ctx, "security", "add-generic-password", "-U", "-a", ref, "-s", "agy-swap", "-X", passwordData)
		command.Stdout = io.Discard
		command.Stderr = io.Discard
	} else if runtime.GOOS == "linux" {
		command = exec.CommandContext(ctx, "secret-tool", "store", "--label=agy-swap account token", "service", "agy-swap", "username", ref)
		command.Stdin = bytes.NewBufferString(token)
	} else {
		return false
	}
	return command.Run() == nil
}

func platformVaultDelete(parent context.Context, ref string) bool {
	ctx, cancel := vaultContext(parent, 5*time.Second)
	defer cancel()
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.CommandContext(ctx, "security", "delete-generic-password", "-a", ref, "-s", "agy-swap")
	} else if runtime.GOOS == "linux" {
		command = exec.CommandContext(ctx, "secret-tool", "clear", "service", "agy-swap", "username", ref)
	} else {
		return false
	}
	if command.Run() == nil {
		return true
	}
	return platformVaultGet(parent, ref) == ""
}
