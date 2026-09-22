//go:build darwin

package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var acctBlobRe = regexp.MustCompile(`"acct"<blob>="([^"]+)"`)

func cleanOrphanKeychainItems(ctx context.Context, keepRefs []string) int {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return 0
	}
	keychainPath := filepath.Join(home, "Library", "Keychains", "login.keychain-db")
	dumpCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(dumpCtx, "security", "dump-keychain", keychainPath)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}

	keep := make(map[string]bool, len(keepRefs))
	for _, r := range keepRefs {
		keep[strings.TrimSpace(r)] = true
	}

	blocks := strings.Split(string(out), "keychain: ")
	deleted := 0
	for _, block := range blocks {
		if !strings.Contains(block, `"svce"<blob>="agy-swap"`) {
			continue
		}
		acctMatch := acctBlobRe.FindStringSubmatch(block)
		if len(acctMatch) < 2 {
			continue
		}
		acct := acctMatch[1]
		if !strings.HasPrefix(acct, "account:") {
			continue
		}
		parts := strings.Split(acct, ":")
		isNonceRef := len(parts) >= 3 && len(parts[2]) == 32
		if isNonceRef || !keep[acct] {
			delCtx, delCancel := context.WithTimeout(ctx, 1*time.Second)
			delCmd := exec.CommandContext(delCtx, "security", "delete-generic-password", "-s", "agy-swap", "-a", acct)
			if delCmd.Run() == nil {
				deleted++
			}
			delCancel()
		}
	}
	return deleted
}
