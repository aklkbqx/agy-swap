package app

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	maxLimitDuration = 7 * 24 * time.Hour
	maxTokenBytes    = 1024 * 1024
	logScanBytes     = 8 * 1024 * 1024
	logTotalBytes    = 64 * 1024 * 1024
	quotaSchema      = 2
	stateSchema      = 1
	historySchema    = 1
	maxHistoryBytes  = 8 * 1024 * 1024
	quotaCache       = 60 * time.Second
	tuiAutoRefresh   = 60 * time.Second
	cloudCodeAPI     = "https://daily-cloudcode-pa.googleapis.com/v1internal:"
	oauthTokenURL    = "https://oauth2.googleapis.com/token"
	githubRepo       = "aklkbqx/agy-swap"
)

const defaultOAuthClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"

var oauthClientSecrets = map[string]string{
	defaultOAuthClientID: "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",
	"884354919052-36trc1jjb3tguiac32ov6cod268c5blh.apps.googleusercontent.com": "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",
}

var tierNames = map[string]string{
	"free-tier":          "Free",
	"g1-pro-tier":        "Google AI Pro",
	"g1-ultra-tier":      "Google AI Ultra",
	"g1-ultra-lite-tier": "Google AI Ultra Lite",
}

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

func defaultPaths() (Paths, error) {
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

func privateDirMode() os.FileMode {
	if runtime.GOOS == "windows" {
		return 0o777
	}
	return 0o700
}

var (
	errStoreConflict = errors.New("accounts.json changed in another process; retry the command")
	errAmbiguous     = errors.New("ambiguous account target")
)
