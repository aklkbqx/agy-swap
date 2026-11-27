package app

import (
	"errors"
	"time"

	"github.com/aklkbqx/agy-swap/v2/internal/config"
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
	quotaCache       = config.QuotaCache
	tuiAutoRefresh   = config.TUIAutoRefresh
	cloudCodeAPI     = config.CloudCodeAPI
	gcpCloudCodeAPI  = config.GCPCloudCodeAPI
	oauthTokenURL    = config.OAuthTokenURL
	githubRepo       = config.GithubRepo
)

const (
	defaultOAuthClientID = config.DefaultOAuthClientID
	gcpOAuthClientID     = config.GCPOAuthClientID
	wifOAuthClientID     = config.WIFOAuthClientID
)

var oauthClientSecrets = config.OAuthClientSecrets

var tierNames = config.TierNames

// Paths is shared with internal/store through internal/config.
type Paths = config.Paths

func defaultPaths() (Paths, error) { return config.DefaultPaths() }

var (
	errStoreConflict = errors.New("accounts.json changed in another process; retry the command")
	errAmbiguous     = errors.New("ambiguous account target")
)
