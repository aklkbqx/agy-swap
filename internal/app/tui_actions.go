package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func (a *Application) applyTUIForm(ctx context.Context, state *tuiState) (string, error) {
	if state == nil || state.form == nil {
		return "", errors.New("no form is active")
	}
	form := state.form
	switch form.Kind {
	case "profile-create", "profile-edit":
		name := cleanText(formField(form, "name"))
		if err := validateAliasName(name); err != nil {
			return "", err
		}
		accounts, err := a.store.Load(false)
		if err != nil {
			return "", err
		}
		settings, err := a.loadSettings()
		if err != nil {
			return "", err
		}
		email, err := resolveConfiguredTarget(formField(form, "account"), accounts, settings)
		if err != nil || email == "" {
			if err == nil {
				err = errors.New("account not found")
			}
			return "", err
		}
		profile := settings.Profiles[name]
		profile.Account = email
		profile.Family = cleanText(formField(form, "family"))
		profile.Policy = firstString(profile.Policy, settings.Policy.Name)
		if threshold := strings.TrimSpace(formField(form, "threshold")); threshold != "" {
			value, parseErr := strconv.Atoi(threshold)
			if parseErr != nil || value < 0 || value > 100 {
				return "", errors.New("notify threshold must be between 0 and 100")
			}
			profile.NotifyThreshold = value
		}
		settings.Profiles[name] = profile
		if err := a.store.SaveSettings(settings); err != nil {
			return "", err
		}
		return "Saved profile " + name, nil

	case "tags":
		email, _, ok := state.selectedAccount()
		if !ok {
			return "", errors.New("select an account first")
		}
		settings, err := a.loadSettings()
		if err != nil {
			return "", err
		}
		parts := strings.Split(formField(form, "tags"), ",")
		settings.Tags[email] = uniqueClean(parts)
		if len(settings.Tags[email]) == 0 {
			delete(settings.Tags, email)
		}
		if err := a.store.SaveSettings(settings); err != nil {
			return "", err
		}
		return "Updated tags for " + email, nil

	case "settings":
		settings, err := a.loadSettings()
		if err != nil {
			return "", err
		}
		for _, field := range form.Fields {
			value := field.Value
			if field.Value == "on" || field.Value == "off" {
				value = strconv.FormatBool(formBool(form, field.Key))
			}
			if err := setConfigValue(&settings, field.Key, value); err != nil {
				return "", fmt.Errorf("%s: %w", field.Label, err)
			}
		}
		if err := a.store.SaveSettings(settings); err != nil {
			return "", err
		}
		return "Settings saved", nil

	case "alias":
		name, target := cleanText(formField(form, "name")), cleanText(formField(form, "target"))
		if err := validateAliasName(name); err != nil {
			return "", err
		}
		if target == "" {
			return "", errors.New("target is required")
		}
		settings, err := a.loadSettings()
		if err != nil {
			return "", err
		}
		settings.Aliases[name] = target
		if err := a.store.SaveSettings(settings); err != nil {
			return "", err
		}
		return "Saved alias " + name, nil

	case "binding":
		path := cleanBindingPath(formField(form, "path"))
		profile := cleanText(formField(form, "profile"))
		mode := firstString(cleanText(formField(form, "mode")), "prompt")
		if path == "" || profile == "" {
			return "", errors.New("path and profile are required")
		}
		if !oneOf(mode, "prompt", "recommend", "disabled") {
			return "", errors.New("mode must be prompt, recommend, or disabled")
		}
		settings, err := a.loadSettings()
		if err != nil {
			return "", err
		}
		replaced := false
		for index := range settings.Bindings {
			if settings.Bindings[index].Path == path {
				settings.Bindings[index] = Binding{Path: path, Profile: profile, Mode: mode}
				replaced = true
			}
		}
		if !replaced {
			settings.Bindings = append(settings.Bindings, Binding{Path: path, Profile: profile, Mode: mode})
		}
		if err := a.store.SaveSettings(settings); err != nil {
			return "", err
		}
		return "Saved project binding", nil

	case "target":
		name, command := cleanText(formField(form, "name")), strings.TrimSpace(formField(form, "command"))
		if err := validateAliasName(name); err != nil {
			return "", err
		}
		if command == "" || strings.ContainsAny(command, "\t\r\n") {
			return "", errors.New("target command must be one executable path or name")
		}
		settings, err := a.loadSettings()
		if err != nil {
			return "", err
		}
		settings.Targets[name] = TargetConfig{Command: command, Enabled: true}
		if err := a.store.SaveSettings(settings); err != nil {
			return "", err
		}
		return "Saved target " + name, nil
	default:
		return "", fmt.Errorf("unsupported TUI form %q", form.Kind)
	}
}

func (a *Application) tuiDoctorSnapshot(ctx context.Context, refresh bool) ([]doctorCheck, bool) {
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
		plaintext, missing := 0, 0
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
			add("vault_migration", "warning", fmt.Sprintf("%d account(s) still use legacy token_data", plaintext))
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
		add("platform", "ok", runtime.GOOS+"/"+runtime.GOARCH+" uses Credential Manager")
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

func (a *Application) tuiExportBackup(ctx context.Context, path, passphrase string, includeSecrets bool) (string, error) {
	path = firstString(strings.TrimSpace(path), "agy-swap-backup.json")
	plaintext, err := a.backupDocument(ctx, includeSecrets)
	if err != nil {
		return "", err
	}
	output := plaintext
	if includeSecrets {
		envelope, encryptErr := encryptBackup(passphrase, plaintext)
		if encryptErr != nil {
			return "", encryptErr
		}
		output, err = json.MarshalIndent(envelope, "", "  ")
		if err != nil {
			return "", err
		}
	}
	if err := atomicWrite(path, append(output, '\n'), 0o600); err != nil {
		return "", err
	}
	return fmt.Sprintf("Backup written to %s", path), nil
}

func (a *Application) tuiImportBackup(ctx context.Context, path, passphrase string, merge bool) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("backup path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var envelope encryptedBackup
	var object map[string]any
	if json.Unmarshal(raw, &envelope) == nil && envelope.Encrypted {
		plaintext, decryptErr := decryptBackup(passphrase, envelope)
		if decryptErr != nil {
			return "", decryptErr
		}
		if err := json.Unmarshal(plaintext, &object); err != nil {
			return "", errors.New("decrypted backup is invalid")
		}
	} else if err := json.Unmarshal(raw, &object); err != nil {
		return "", fmt.Errorf("invalid backup: %w", err)
	}
	if _, ok := object["accounts"]; !ok {
		return "", errors.New("backup has no accounts")
	}
	accountData, err := json.Marshal(object["accounts"])
	if err != nil {
		return "", err
	}
	incoming, err := decodeOrderedAccounts(accountData)
	if err != nil {
		return "", err
	}
	if merge {
		existing, loadErr := a.store.Load(false)
		if loadErr != nil {
			return "", loadErr
		}
		for _, email := range incoming.Order {
			existing.Set(email, incoming.ByEmail[email])
		}
		incoming = existing
	}
	migrated := 0
	for _, email := range incoming.Order {
		account := incoming.ByEmail[email]
		if token := getString(account, "token_data"); token != "" && a.saveAccountSecret(ctx, account, token) {
			migrated++
		}
	}
	if err := a.store.Save(incoming); err != nil {
		return "", err
	}
	if rawSettings, ok := object["settings"]; ok {
		var settings AppSettings
		if err := json.Unmarshal(mustJSON(rawSettings), &settings); err == nil {
			if err := a.store.SaveSettings(settings); err != nil {
				return "", err
			}
		}
	}
	return fmt.Sprintf("Imported %d account(s); migrated %d secret(s)", incoming.Len(), migrated), nil
}

func (a *Application) tuiVerifyBackup(path, passphrase string) (string, error) {
	raw, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	var envelope encryptedBackup
	if json.Unmarshal(raw, &envelope) == nil && envelope.Encrypted {
		if _, err := decryptBackup(passphrase, envelope); err != nil {
			return "", err
		}
	} else {
		var object map[string]any
		if err := json.Unmarshal(raw, &object); err != nil {
			return "", err
		}
	}
	return "Backup is valid", nil
}
