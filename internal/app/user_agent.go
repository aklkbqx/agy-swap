package app

import (
	"os"
	"runtime"
	"strings"
)

// cloudClientVersion and cloudClientCL align Code Assist requests with the
// current Antigravity hub client User-Agent format.
const (
	cloudClientVersion = "2.35.2"
	cloudClientCL      = "1030252320"
)

// cloudUserAgent returns the User-Agent for Code Assist requests.
// AGY_SWAP_USER_AGENT replaces it when Google starts keying answers on a newer client.
func cloudUserAgent() string {
	if value := strings.TrimSpace(os.Getenv("AGY_SWAP_USER_AGENT")); value != "" {
		return value
	}
	return "antigravity/hub/" + cloudClientVersion + " (aidev_client; os_type=" + runtime.GOOS + "; arch=" + runtime.GOARCH + "; cl=" + cloudClientCL + ")"
}
