package app

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// newRandomUUID returns a cryptographically random RFC 4122 v4 UUID string.
func newRandomUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ensureAccountIdentity returns stable per-account installation_id and
// installation_uuid values, generating and storing them on account when absent.
func ensureAccountIdentity(account Account) (string, string) {
	if account == nil {
		return "", ""
	}
	installID := strings.TrimSpace(getString(account, "installation_id"))
	if installID == "" {
		installID = newRandomUUID()
		account["installation_id"] = installID
	}
	installUUID := strings.TrimSpace(getString(account, "installation_uuid"))
	if installUUID == "" {
		installUUID = newRandomUUID()
		account["installation_uuid"] = installUUID
	}
	return installID, installUUID
}

// syncAccountIdentityFiles writes the active account's per-account
// installation_id and installation_uuid into Antigravity's local state files
// without disturbing unrelated fields in *.pbtxt files.
func syncAccountIdentityFiles(home, installID, installUUID string) {
	home = strings.TrimSpace(home)
	if home == "" {
		return
	}
	installID = strings.TrimSpace(installID)
	installUUID = strings.TrimSpace(installUUID)

	if installID != "" {
		for _, sub := range []string{"antigravity", "antigravity-cli"} {
			dir := filepath.Join(home, ".gemini", sub)
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				_ = atomicWrite(filepath.Join(dir, "installation_id"), []byte(installID+"\n"), 0o600)
			}
		}
	}

	if installUUID != "" {
		pbtxtTargets := []string{
			filepath.Join(home, ".gemini", "antigravity", "antigravity_state.pbtxt"),
			filepath.Join(home, ".gemini", "antigravity-cli", "jetski_state.pbtxt"),
		}
		for _, path := range pbtxtTargets {
			updatePbtxtFieldIfDirExists(path, "installation_uuid", installUUID)
		}
	}
}

func updatePbtxtFieldIfDirExists(path, field, value string) {
	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			line := fmt.Sprintf("%s: %q\n", field, value)
			_ = atomicWrite(path, []byte(line), 0o600)
		}
		return
	}
	lines := strings.Split(string(raw), "\n")
	prefix := field + ":"
	wantLine := fmt.Sprintf("%s: %q", field, value)
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			if trimmed == wantLine {
				return // already matches, avoid unnecessary disk write
			}
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + wantLine
			replaced = true
			break
		}
	}
	if !replaced {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = append(lines[:len(lines)-1], wantLine, "")
		} else {
			lines = append(lines, wantLine)
		}
	}
	out := strings.Join(lines, "\n")
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	_ = atomicWrite(path, []byte(out), 0o600)
}
