package app

import (
	"errors"
	"time"

	"github.com/aklkbqx/agy-swap/internal/config"
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

// Paths is shared with internal/store through internal/config.
type Paths = config.Paths

func defaultPaths() (Paths, error) { return config.DefaultPaths() }

var (
	errStoreConflict = errors.New("accounts.json changed in another process; retry the command")
	errAmbiguous     = errors.New("ambiguous account target")
)
