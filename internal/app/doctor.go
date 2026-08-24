package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

type doctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (a *Application) cmdDoctor(ctx context.Context, opts extendedOptions) int {
	checks := make([]doctorCheck, 0, 12)
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
	}
	current := a.credentials.Current(ctx)
	if current == "" {
		add("active_session", "warning", "no active Antigravity credential detected")
	} else if decodeToken(current) == nil {
		add("active_session", "error", "active credential is not a recognized OAuth token")
	} else {
		add("active_session", "ok", "OAuth credential detected")
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
	if opts.Refresh {
		if release, releaseErr := a.releaseAssetCheck(ctx); releaseErr != nil {
			add("update_asset", "warning", releaseErr.Error())
		} else if available, _ := release["available"].(bool); available {
			add("update_asset", "ok", fmt.Sprintf("v%v asset %v is available", release["latest"], release["asset"]))
		} else {
			add("update_asset", "error", fmt.Sprintf("v%v is missing asset %v", release["latest"], release["asset"]))
		}
	}
	failed := false
	for _, check := range checks {
		if check.Status == "error" {
			failed = true
		}
	}
	data := map[string]any{"healthy": !failed, "checks": checks, "paths": map[string]string{"config": a.paths.Settings, "accounts": a.paths.Accounts, "history": a.paths.History}}
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
