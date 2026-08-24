package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AppSettings is deliberately small and versioned. Unknown future fields are
// ignored on read, while all writes are atomic and private.
type AppSettings struct {
	Schema        int                     `json:"schema"`
	Aliases       map[string]string       `json:"aliases,omitempty"`
	Tags          map[string][]string     `json:"tags,omitempty"`
	Profiles      map[string]Profile      `json:"profiles,omitempty"`
	Bindings      []Binding               `json:"bindings,omitempty"`
	Policy        PolicyConfig            `json:"policy,omitempty"`
	Notifications NotificationConfig      `json:"notifications,omitempty"`
	History       HistoryConfig           `json:"history,omitempty"`
	Statusline    StatuslineConfig        `json:"statusline,omitempty"`
	Targets       map[string]TargetConfig `json:"targets,omitempty"`
	UpdatedAt     string                  `json:"updated_at,omitempty"`
}

type Profile struct {
	Account         string   `json:"account"`
	Family          string   `json:"family,omitempty"`
	Policy          string   `json:"policy,omitempty"`
	ReserveAccounts []string `json:"reserve_accounts,omitempty"`
	NotifyThreshold int      `json:"notify_threshold,omitempty"`
}

type Binding struct {
	Path    string `json:"path"`
	Profile string `json:"profile"`
	Mode    string `json:"mode,omitempty"` // prompt, recommend, disabled
}

type PolicyConfig struct {
	Name            string `json:"name,omitempty"`
	MinRemainingPct int    `json:"min_remaining_pct,omitempty"`
	PreferFamily    string `json:"prefer_family,omitempty"`
	AllowApply      bool   `json:"allow_apply,omitempty"`
}

type NotificationConfig struct {
	Enabled         bool `json:"enabled,omitempty"`
	Threshold       int  `json:"threshold,omitempty"`
	Reset           bool `json:"reset,omitempty"`
	AuthFailure     bool `json:"auth_failure,omitempty"`
	Stale           bool `json:"stale,omitempty"`
	CooldownSeconds int  `json:"cooldown_seconds,omitempty"`
}

type HistoryConfig struct {
	Enabled       bool `json:"enabled,omitempty"`
	RetentionDays int  `json:"retention_days,omitempty"`
	MaxBytes      int  `json:"max_bytes,omitempty"`
}

type StatuslineConfig struct {
	Installed bool   `json:"installed,omitempty"`
	Command   string `json:"command,omitempty"`
}

type TargetConfig struct {
	Command string `json:"command"`
	Enabled bool   `json:"enabled,omitempty"`
}

func defaultSettings() AppSettings {
	return AppSettings{
		Schema:        stateSchema,
		Aliases:       map[string]string{},
		Tags:          map[string][]string{},
		Profiles:      map[string]Profile{},
		Bindings:      []Binding{},
		Policy:        PolicyConfig{Name: "sticky", MinRemainingPct: 10},
		Notifications: NotificationConfig{Threshold: 20, CooldownSeconds: 1800},
		History:       HistoryConfig{Enabled: true, RetentionDays: 30, MaxBytes: maxHistoryBytes},
		Targets:       map[string]TargetConfig{},
	}
}

func normalizeSettings(s AppSettings) (AppSettings, error) {
	defaults := defaultSettings()
	if s.Schema == 0 {
		s.Schema = defaults.Schema
	}
	if s.Schema != stateSchema {
		return AppSettings{}, fmt.Errorf("unsupported config schema %d", s.Schema)
	}
	if s.Aliases == nil {
		s.Aliases = defaults.Aliases
	}
	if s.Tags == nil {
		s.Tags = defaults.Tags
	}
	if s.Profiles == nil {
		s.Profiles = defaults.Profiles
	}
	if s.Bindings == nil {
		s.Bindings = defaults.Bindings
	}
	if s.Targets == nil {
		s.Targets = defaults.Targets
	}
	if s.Policy.Name == "" {
		s.Policy.Name = defaults.Policy.Name
	}
	if s.Policy.MinRemainingPct < 0 || s.Policy.MinRemainingPct > 100 {
		return AppSettings{}, errors.New("policy min_remaining_pct must be between 0 and 100")
	}
	if s.Notifications == (NotificationConfig{}) {
		s.Notifications = defaults.Notifications
	}
	if s.Notifications.Threshold < 0 || s.Notifications.Threshold > 100 {
		return AppSettings{}, errors.New("notification threshold must be between 0 and 100")
	}
	if s.Notifications.CooldownSeconds == 0 && !s.Notifications.Enabled && !s.Notifications.Reset && !s.Notifications.AuthFailure {
		s.Notifications.CooldownSeconds = defaults.Notifications.CooldownSeconds
	}
	if s.History.RetentionDays == 0 {
		s.History.RetentionDays = defaults.History.RetentionDays
	}
	if s.History.MaxBytes == 0 {
		s.History.MaxBytes = defaults.History.MaxBytes
	}
	if s.History.RetentionDays < 1 || s.History.RetentionDays > 3650 {
		return AppSettings{}, errors.New("history retention_days must be between 1 and 3650")
	}
	if s.History.MaxBytes < 64*1024 || s.History.MaxBytes > 256*1024*1024 {
		return AppSettings{}, errors.New("history max_bytes is outside the supported range")
	}
	for name, profile := range s.Profiles {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(profile.Account) == "" {
			return AppSettings{}, errors.New("profiles require a name and account")
		}
		profile.Account = normalizeEmail(profile.Account)
		if profile.Family != "" && !oneOf(profile.Family, "claude", "gemini", "gpt") {
			return AppSettings{}, fmt.Errorf("profile %s has invalid family", name)
		}
		if profile.Policy == "" {
			profile.Policy = s.Policy.Name
		}
		if profile.NotifyThreshold < 0 || profile.NotifyThreshold > 100 {
			return AppSettings{}, fmt.Errorf("profile %s has invalid notify threshold", name)
		}
		s.Profiles[name] = profile
	}
	for i := range s.Bindings {
		s.Bindings[i].Path = filepath.Clean(strings.TrimSpace(s.Bindings[i].Path))
		if s.Bindings[i].Path == "." || s.Bindings[i].Path == "" {
			return AppSettings{}, errors.New("binding path must be non-empty")
		}
		if s.Bindings[i].Mode == "" {
			s.Bindings[i].Mode = "prompt"
		}
		if !oneOf(s.Bindings[i].Mode, "prompt", "recommend", "disabled") {
			return AppSettings{}, errors.New("binding mode must be prompt, recommend, or disabled")
		}
		if strings.TrimSpace(s.Bindings[i].Profile) == "" {
			return AppSettings{}, errors.New("binding profile must be non-empty")
		}
	}
	for name, target := range s.Targets {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, " /\\\t\r\n") || strings.TrimSpace(target.Command) == "" || strings.ContainsAny(target.Command, "\t\r\n") {
			return AppSettings{}, fmt.Errorf("target %s has an invalid executable mapping", name)
		}
		target.Command = strings.TrimSpace(target.Command)
		s.Targets[name] = target
	}
	return s, nil
}

func (s *Store) LoadSettings() (AppSettings, error) {
	data, err := os.ReadFile(s.paths.Settings)
	if errors.Is(err, os.ErrNotExist) {
		return defaultSettings(), nil
	}
	if err != nil {
		return AppSettings{}, fmt.Errorf("cannot read %s: %w", s.paths.Settings, err)
	}
	var settings AppSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return AppSettings{}, fmt.Errorf("cannot parse %s: %w", s.paths.Settings, err)
	}
	return normalizeSettings(settings)
}

func (s *Store) SaveSettings(settings AppSettings) error {
	settings, err := normalizeSettings(settings)
	if err != nil {
		return err
	}
	settings.UpdatedAt = isoTime(time.Now().UTC())
	return atomicWriteJSON(s.paths.Settings, settings)
}

func (s *Store) UpdateSettings(fn func(*AppSettings) error) (AppSettings, error) {
	settings, err := s.LoadSettings()
	if err != nil {
		return AppSettings{}, err
	}
	if err := fn(&settings); err != nil {
		return AppSettings{}, err
	}
	if err := s.SaveSettings(settings); err != nil {
		return AppSettings{}, err
	}
	return settings, nil
}
