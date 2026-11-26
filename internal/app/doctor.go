package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (a *Application) collectDoctorChecks(ctx context.Context, refresh bool) ([]doctorCheck, bool) {
	checks := make([]doctorCheck, 0, 16)
	add := func(name, status, message string) {
		checks = append(checks, doctorCheck{Name: name, Status: status, Message: message})
	}
	if err := ensurePrivateDir(a.paths.ConfigDir); err != nil {
		add("config_dir", "error", err.Error())
	} else {
		add("config_dir", "ok", a.paths.ConfigDir)
	}
	if settings, err := a.loadSettings(); err != nil {
		add("config", "error", err.Error())
	} else {
		add("config", "ok", fmt.Sprintf("schema %d", settings.Schema))
	}
	accounts, err := a.store.Load(false)
	if err != nil {
		add("accounts", "error", err.Error())
	} else {
		add("accounts", "ok", fmt.Sprintf("%d account(s)", accounts.Len()))
		plaintext := 0
		missing := 0
		for _, email := range accounts.Order {
			account := accounts.ByEmail[email]
			if getString(account, "token_data") != "" {
				plaintext++
			}
			if _, tokenErr := a.accountToken(ctx, account); tokenErr != nil {
				missing++
			}
		}
		if plaintext > 0 {
			add("vault_migration", "warning", fmt.Sprintf("%d account(s) still use legacy token_data; run account migrate --force", plaintext))
		}
		if missing > 0 {
			add("vault_entries", "error", fmt.Sprintf("%d account secret(s) cannot be read", missing))
		}
		add("vault", "ok", describeVault(a.vault))
	}
	if runtime.GOOS != "windows" {
		vaultFile := filepath.Join(a.paths.ConfigDir, "vault.json")
		if info, statErr := os.Stat(vaultFile); statErr == nil && !info.IsDir() {
			if perm := info.Mode().Perm(); perm&0o077 != 0 {
				add("vault_perms", "warning", fmt.Sprintf("vault.json permissions are %04o; expected 0600", perm))
			} else {
				add("vault_perms", "ok", "vault.json permissions 0600")
			}
		}
	}
	current := a.credentials.Current(ctx)
	switch {
	case current == "":
		add("active_session", "warning", "no active Antigravity credential detected")
	case decodeToken(current) == nil:
		add("active_session", "error", "active credential is not a recognized OAuth token")
	default:
		add("active_session", "ok", "OAuth credential detected")
	}
	if agyPath := a.resolveAgyExecutable(); agyPath != "" {
		add("agy_cli", "ok", agyPath)
	} else {
		add("agy_cli", "warning", "Antigravity CLI ('agy') not found; run 'agy-swap install-agy'")
	}
	if runtime.GOOS != "windows" && strings.TrimSpace(a.paths.Home) != "" {
		a.checkLauncherGuards(add)
		missingHelpers := make([]string, 0, 3)
		for _, tool := range []string{"sqlite3", "pkill"} {
			if _, lookErr := exec.LookPath(tool); lookErr != nil {
				missingHelpers = append(missingHelpers, tool)
			}
		}
		if runtime.GOOS == "linux" {
			if _, lookErr := exec.LookPath("secret-tool"); lookErr != nil {
				missingHelpers = append(missingHelpers, "secret-tool")
			}
		}
		if len(missingHelpers) > 0 {
			add("helper_tools", "warning", "optional helper(s) missing: "+strings.Join(missingHelpers, ", "))
		} else {
			add("helper_tools", "ok", "sqlite3 and pkill available")
		}
	}
	if runtime.GOOS == "windows" {
		add("platform", "ok", runtime.GOOS+"/"+runtime.GOARCH+" uses Credential Manager and PowerShell installer")
	} else {
		add("platform", "ok", runtime.GOOS+"/"+runtime.GOARCH)
	}
	if _, err := os.Stat(a.paths.History); err == nil {
		add("history", "ok", a.paths.History)
	} else {
		add("history", "ok", "not created yet")
	}
	if refresh {
		if release, releaseErr := a.releaseAssetCheck(ctx); releaseErr != nil {
			add("update_asset", "warning", releaseErr.Error())
		} else if available, _ := release["available"].(bool); available {
			add("update_asset", "ok", fmt.Sprintf("v%v asset %v is available", release["latest"], release["asset"]))
		} else {
			add("update_asset", "error", fmt.Sprintf("v%v is missing asset %v", release["latest"], release["asset"]))
		}
	}
	healthy := true
	for _, check := range checks {
		if check.Status == "error" {
			healthy = false
			break
		}
	}
	return checks, healthy
}

func (a *Application) checkLauncherGuards(add func(name, status, message string)) {
	candidates := []string{
		filepath.Join(a.paths.Home, ".local", "bin", "agy"),
		filepath.Join(a.paths.Home, ".gemini", "bin", "agy"),
	}
	var issues []string
	var warnings []string
	wrapped := 0
	for _, agyBin := range candidates {
		info, err := os.Stat(agyBin)
		if err != nil || info.IsDir() {
			continue
		}
		data, err := readHeaderBytes(agyBin, 8192)
		if err != nil || len(data) == 0 {
			continue
		}
		text := string(data)
		if !strings.HasPrefix(text, "#!") || !strings.Contains(text, "Managed by agy-swap session guard") {
			continue
		}
		wrapped++
		target := extractWrapperRealCommand(text)
		if target == "" {
			target = filepath.Join(a.paths.Home, ".local", "libexec", "agy")
		}
		if _, statErr := os.Stat(target); statErr != nil {
			issues = append(issues, fmt.Sprintf("%s points to missing binary %s", agyBin, target))
		}
		sidecarPath := agyBin + ".version.json"
		if raw, readErr := os.ReadFile(sidecarPath); readErr == nil && len(raw) > 0 {
			var sidecar map[string]any
			if json.Unmarshal(raw, &sidecar) != nil || sidecar == nil {
				warnings = append(warnings, sidecarPath+" is not valid JSON")
			} else if sz, ok := sidecar["size"].(float64); ok && int64(sz) != info.Size() {
				warnings = append(warnings, sidecarPath+" size does not match wrapper")
			}
		}
	}
	switch {
	case len(issues) > 0:
		add("launcher_guard", "error", strings.Join(issues, "; "))
	case len(warnings) > 0:
		add("launcher_guard", "warning", strings.Join(warnings, "; "))
	case wrapped > 0:
		add("launcher_guard", "ok", fmt.Sprintf("%d launcher wrapper(s) verified", wrapped))
	}
}

func (a *Application) cmdDoctor(ctx context.Context, opts extendedOptions) int {
	checks, healthy := a.collectDoctorChecks(ctx, opts.Refresh)
	failed := !healthy
	data := map[string]any{"healthy": healthy, "checks": checks, "paths": map[string]string{"config": a.paths.Settings, "accounts": a.paths.Accounts, "history": a.paths.History}}
	if opts.JSON {
		if failed {
			a.writeJSON(extendedEnvelope{Schema: stateSchema, Command: "doctor", OK: false, Data: data})
			return 1
		}
		return a.extendedResult("doctor", opts, data, nil)
	}
	for _, check := range checks {
		symbol := "✓"
		if check.Status == "warning" {
			symbol = "!"
		}
		if check.Status == "error" {
			symbol = "✕"
		}
		fmt.Fprintf(a.Out, "%s %-18s %s\n", symbol, check.Name, check.Message)
	}
	if failed {
		return 1
	}
	return 0
}

func (a *Application) releaseAssetCheck(ctx context.Context) (map[string]any, error) {
	data, _, err := a.http.getBytes(ctx, "https://api.github.com/repos/"+githubRepo+"/releases/latest", map[string]string{"Accept": "application/vnd.github+json"}, 10*time.Second, 1024*1024)
	if err != nil {
		return nil, err
	}
	var release githubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return nil, err
	}
	releaseTag := normalizedReleaseTag(release.Tag)
	expected := "agy-swap_" + releaseTag + "_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		expected += ".exe"
	}
	found := false
	for _, asset := range release.Assets {
		if asset.Name == expected {
			found = true
		}
	}
	return map[string]any{"latest": strings.TrimPrefix(releaseTag, "v"), "asset": expected, "available": found, "release_url": release.HTMLURL}, nil
}
