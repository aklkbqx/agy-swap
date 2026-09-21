//go:build !windows

package app

import (
	"bytes"
	"context"
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
	if runtime.GOOS == "darwin" {
		return keychainGet(ctx, "agy-swap", ref)
	}
	var command *exec.Cmd
	if runtime.GOOS == "linux" {
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
		return keychainSet(ctx, "agy-swap", ref, token)
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
	if runtime.GOOS == "darwin" {
		return keychainDelete(ctx, "agy-swap", ref)
	}
	var command *exec.Cmd
	if runtime.GOOS == "linux" {
		command = exec.CommandContext(ctx, "secret-tool", "clear", "service", "agy-swap", "username", ref)
	} else {
		return false
	}
	if command.Run() == nil {
		return true
	}
	return platformVaultGet(parent, ref) == ""
}
