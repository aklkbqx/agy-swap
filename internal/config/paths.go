package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	MaxLimitDuration = 7 * 24 * time.Hour
	MaxTokenBytes    = 1024 * 1024
	LogScanBytes     = 8 * 1024 * 1024
	LogTotalBytes    = 64 * 1024 * 1024
	QuotaSchema      = 2
	StateSchema      = 1
	HistorySchema    = 1
	MaxHistoryBytes  = 8 * 1024 * 1024
	QuotaCache       = 60 * time.Second
	TUIAutoRefresh   = 60 * time.Second
	CloudCodeAPI     = "https://daily-cloudcode-pa.googleapis.com/v1internal:"
	OAuthTokenURL    = "https://oauth2.googleapis.com/token"
	GithubRepo       = "aklkbqx/agy-swap"
)

const DefaultOAuthClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"

var OAuthClientSecrets = map[string]string{
	DefaultOAuthClientID: "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",
	"884354919052-36trc1jjb3tguiac32ov6cod268c5blh.apps.googleusercontent.com": "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",
}

var TierNames = map[string]string{
	"free-tier":          "Free",
	"g1-pro-tier":        "Google AI Pro",
	"g1-ultra-tier":      "Google AI Ultra",
	"g1-ultra-lite-tier": "Google AI Ultra Lite",
}

// Paths contains resolved filesystem locations for agy-swap storage and Google Antigravity tokens.
type Paths struct {
	Home             string
	ConfigDir        string
	Accounts         string
	AccountsBackup   string
	AccountsLock     string
	SessionLock      string
	LogCache         string
	Settings         string
	History          string
	RuntimeState     string
	JournalDir       string
	OAuthToken       string
	OAuthCredentials string
	GoogleAccounts   string
}

// DefaultPaths resolves user home and configuration file paths across platforms.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return Paths{}, errors.New("cannot determine user home directory")
	}
	config := filepath.Join(home, ".gemini", "agy-swap")
	return Paths{
		Home:             home,
		ConfigDir:        config,
		Accounts:         filepath.Join(config, "accounts.json"),
		AccountsBackup:   filepath.Join(config, "accounts.json.bak"),
		AccountsLock:     filepath.Join(config, ".accounts.lock"),
		SessionLock:      filepath.Join(config, ".session.lock"),
		LogCache:         filepath.Join(config, "log-cache-v1.json"),
		Settings:         filepath.Join(config, "config.json"),
		History:          filepath.Join(config, "history-v1.jsonl"),
		RuntimeState:     filepath.Join(config, "runtime-state.json"),
		JournalDir:       filepath.Join(config, "journals"),
		OAuthToken:       filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token"),
		OAuthCredentials: filepath.Join(home, ".gemini", "oauth_creds.json"),
		GoogleAccounts:   filepath.Join(home, ".gemini", "google_accounts.json"),
	}, nil
}

// PrivateDirMode returns restrictive directory permissions (0700 on POSIX, 0777 on Windows).
func PrivateDirMode() os.FileMode {
	if runtime.GOOS == "windows" {
		return 0o777
	}
	return 0o700
}
