package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		if form.Kind == "profile-edit" && form.OriginalKey != "" && form.OriginalKey != name {
			if prev, ok := settings.Profiles[form.OriginalKey]; ok {
				profile = prev
			}
			delete(settings.Profiles, form.OriginalKey)
			for i := range settings.Bindings {
				if settings.Bindings[i].Profile == form.OriginalKey {
					settings.Bindings[i].Profile = name
				}
			}
		}
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
		if !oneOf(mode, "prompt", "recommend", "auto", "disabled") {
			return "", errors.New("mode must be prompt, recommend, auto, or disabled")
		}
		settings, err := a.loadSettings()
		if err != nil {
			return "", err
		}
		if _, ok := settings.Profiles[profile]; !ok {
			return "", fmt.Errorf("unknown profile %q", profile)
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
	return a.collectDoctorChecks(ctx, refresh)
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
	count, migrated, err := a.importBackup(ctx, strings.TrimSpace(path), passphrase, merge)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Imported %d account(s); migrated %d secret(s)", count, migrated), nil
}

func (a *Application) tuiVerifyBackup(path, passphrase string) (string, error) {
	if _, err := readBackup(strings.TrimSpace(path), passphrase); err != nil {
		return "", err
	}
	return "Backup is valid", nil
}
