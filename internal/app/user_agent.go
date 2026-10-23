package app

import (
	"os"
	"runtime"
	"strings"
)

// cloudClientVersion is the Antigravity hub client the Code Assist API is told
// it is talking to. Google decides which quota groups to answer with from this
// version: older clients (and the bare name "antigravity") get only the
// Claude/GPT pool, 2.10 adds Gemini and the hub from 2.20 adds Argon.
const cloudClientVersion = "2.23.0"

// cloudUserAgent returns the User-Agent for Code Assist requests.
// AGY_SWAP_USER_AGENT replaces it when Google starts keying answers on a newer client.
func cloudUserAgent() string {
	if value := strings.TrimSpace(os.Getenv("AGY_SWAP_USER_AGENT")); value != "" {
		return value
	}
	return "antigravity/hub/" + cloudClientVersion + " " + runtime.GOOS + "/" + runtime.GOARCH
}
