package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aklkbqx/agy-swap/v2/internal/config"
)

func TestPhase1aPerAccountIsolatedTransportDoesNotReuseTCPSockets(t *testing.T) {
	var mu sync.Mutex
	addrsByAuth := map[string]map[string]bool{}
	var seenUserAgents []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		auth := r.Header.Get("Authorization")
		if addrsByAuth[auth] == nil {
			addrsByAuth[auth] = map[string]bool{}
		}
		addrsByAuth[auth][r.RemoteAddr] = true
		seenUserAgents = append(seenUserAgents, r.Header.Get("User-Agent"))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			fmt.Fprint(w, `{"cloudaicompanionProject":"proj-1","currentTier":{"id":"g1-pro-tier"}}`)
		case strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary"):
			fmt.Fprint(w, `{"groups":[{"buckets":[{"bucketId":"gemini-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.8,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	httpSvc := NewHTTPService(io.Discard)
	httpSvc.cloudAPI = server.URL + "/v1internal:"
	store := NewStore(testPaths(t))
	qs := NewQuotaService(httpSvc, store)

	accounts := NewAccounts()
	accounts.Set("a@example.invalid", Account{
		"email":      "a@example.invalid",
		"token_data": sampleTokenWithRefresh("tok-a", "rt-a", time.Now().Add(time.Hour)),
	})
	accounts.Set("b@example.invalid", Account{
		"email":      "b@example.invalid",
		"token_data": sampleTokenWithRefresh("tok-b", "rt-b", time.Now().Add(time.Hour)),
	})

	errs := qs.Refresh(context.Background(), accounts, true, nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected refresh errors: %v", errs)
	}

	mu.Lock()
	defer mu.Unlock()
	addrsA := addrsByAuth["Bearer tok-a"]
	addrsB := addrsByAuth["Bearer tok-b"]
	if len(addrsA) == 0 || len(addrsB) == 0 {
		t.Fatalf("expected requests from both accounts, got a=%v b=%v", addrsA, addrsB)
	}
	for addr := range addrsA {
		if addrsB[addr] {
			t.Fatalf("accounts A and B shared TCP socket %s (addrsA=%v, addrsB=%v)", addr, addrsA, addrsB)
		}
	}
	wantUA := "antigravity/hub/2.35.2 (aidev_client; os_type=" + runtime.GOOS + "; arch=" + runtime.GOARCH + "; cl=1030252320)"
	for _, ua := range seenUserAgents {
		if ua != wantUA {
			t.Fatalf("cloud User-Agent = %q, want %q", ua, wantUA)
		}
	}
}

func TestPhase1aOAuthNoMozillaUAAndWIFAndGCPRouting(t *testing.T) {
	var lastUA string
	var lastForm string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		lastUA = r.Header.Get("User-Agent")
		lastForm = string(body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"fresh-tok","expires_in":3600}`)
	}))
	defer server.Close()

	httpSvc := NewHTTPService(io.Discard)
	httpSvc.oauthURL = server.URL + "/token"

	// 1. GCP token without id_token aud uses GCP client ID & omits Mozilla/5.0
	gcpRaw, _ := json.Marshal(map[string]any{
		"auth_method": "gcp",
		"project_id":  "my-gcp-proj",
		"token": map[string]any{
			"access_token":  "expired",
			"refresh_token": "rt-gcp",
			"expiry_date":   time.Now().Add(-time.Hour).UnixMilli(),
		},
	})
	gcpBlob := "go-keyring-base64:" + base64.StdEncoding.EncodeToString(gcpRaw)
	if _, _, err := httpSvc.accessTokenData(context.Background(), gcpBlob); err != nil {
		t.Fatalf("gcp refresh failed: %v", err)
	}
	if strings.Contains(lastUA, "Mozilla/5.0") {
		t.Fatalf("OAuth refresh must not send fake Mozilla/5.0 UA, got %q", lastUA)
	}
	if !strings.Contains(lastForm, "client_id="+config.GCPOAuthClientID) {
		t.Fatalf("expected GCP OAuth client ID in form, got %q", lastForm)
	}
	if got := httpSvc.cloudEndpointFor("gcp"); got != config.GCPCloudCodeAPI {
		t.Fatalf("cloudEndpointFor(gcp) = %q, want %q", got, config.GCPCloudCodeAPI)
	}
	if got := httpSvc.cloudEndpointFor("consumer"); got != config.CloudCodeAPI {
		t.Fatalf("cloudEndpointFor(consumer) = %q, want %q", got, config.CloudCodeAPI)
	}

	// 2. WIF token uses public PKCE WIF client ID and omits client_secret
	wifRaw, _ := json.Marshal(map[string]any{
		"wif_provider": "projects/123/locations/global/workloadIdentityPools/p/providers/g",
		"token": map[string]any{
			"access_token":  "expired",
			"refresh_token": "rt-wif",
			"expiry_date":   time.Now().Add(-time.Hour).UnixMilli(),
		},
	})
	wifBlob := "go-keyring-base64:" + base64.StdEncoding.EncodeToString(wifRaw)
	if _, _, err := httpSvc.accessTokenData(context.Background(), wifBlob); err != nil {
		t.Fatalf("wif refresh failed: %v", err)
	}
	if !strings.Contains(lastForm, "client_id="+config.WIFOAuthClientID) {
		t.Fatalf("expected WIF client ID in form, got %q", lastForm)
	}
	if strings.Contains(lastForm, "client_secret=") {
		t.Fatalf("WIF public PKCE refresh must not send client_secret, got %q", lastForm)
	}
}

func TestPhase1aWriteOAuthFilesPreserves11FieldsBackfillsAuthMethodAndCleansOldAccounts(t *testing.T) {
	paths := testPaths(t)
	creds := NewCredentials(paths)

	// Pre-populate google_accounts.json with another email to verify "old" is NOT accumulated.
	if err := atomicWriteJSON(paths.GoogleAccounts, map[string]any{"active": "prev@example.invalid", "old": []string{"older@example.invalid"}}); err != nil {
		t.Fatal(err)
	}

	raw, _ := json.Marshal(map[string]any{
		"id_token_email":                     "user@example.invalid",
		"saved_wif_provider":                 "",
		"project_id":                         "proj-123",
		"region":                             "us-central1",
		"user_tier":                          "g1-pro-tier",
		"tier_display_name":                  "Google AI Pro",
		"writer_id":                          "writer-1",
		"aicode_scope_sign_out_attempted_at": "2026-01-01T00:00:00Z",
		"token": map[string]any{
			"access_token":  "at-1",
			"refresh_token": "rt-1",
			"expiry_date":   time.Now().Add(time.Hour).UnixMilli(),
		},
	})
	tokenBlob := "go-keyring-base64:" + base64.StdEncoding.EncodeToString(raw)

	if !creds.writeOAuthFiles(tokenBlob, "user@example.invalid") {
		t.Fatal("writeOAuthFiles failed")
	}

	writtenBytes, err := os.ReadFile(paths.OAuthToken)
	if err != nil {
		t.Fatal(err)
	}
	var written map[string]any
	if unmarshalErr := json.Unmarshal(writtenBytes, &written); unmarshalErr != nil {
		t.Fatal(unmarshalErr)
	}
	if getString(written, "auth_method") != "consumer" {
		t.Fatalf("expected backfilled auth_method=consumer, got %q", getString(written, "auth_method"))
	}
	for _, field := range []string{"id_token_email", "project_id", "region", "user_tier", "tier_display_name", "writer_id", "aicode_scope_sign_out_attempted_at"} {
		if getString(written, field) == "" {
			t.Fatalf("expected StoredToken field %q preserved, got %#v", field, written)
		}
	}

	gaBytes, err := os.ReadFile(paths.GoogleAccounts)
	if err != nil {
		t.Fatal(err)
	}
	var ga map[string]any
	if err := json.Unmarshal(gaBytes, &ga); err != nil {
		t.Fatal(err)
	}
	if getString(ga, "active") != "user@example.invalid" {
		t.Fatalf("google_accounts active = %q", getString(ga, "active"))
	}
	if old := getSlice(ga["old"]); len(old) != 0 {
		t.Fatalf("google_accounts old must stay empty to avoid cross-account roster, got %#v", old)
	}

	// Verify all 3 keyring bypass marker paths are covered explicitly.
	markers := creds.keyringMarkerPaths()
	wantHubMarker := filepath.Join(paths.Home, ".gemini", "cache", "antigravity-keyring-unavailable")
	wantCLIMarker := filepath.Join(paths.Home, ".gemini", "antigravity-cli", "cache", "antigravity-keyring-unavailable")
	wantIDEMarker := filepath.Join(paths.Home, ".gemini", "antigravity", "cache", "antigravity-keyring-unavailable")
	for _, want := range []string{wantHubMarker, wantCLIMarker, wantIDEMarker} {
		found := false
		for _, m := range markers {
			if filepath.Clean(m) == filepath.Clean(want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("keyringMarkerPaths missing %s (got %v)", want, markers)
		}
	}
}

func TestPhase1aWrapperAndLoginUseOfficialForceFileEnvInsteadOfWSLInterop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX wrapper test")
	}
	app, _, _ := newSessionGuardTestApp(t)
	dir := filepath.Join(app.paths.Home, ".gemini", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	agyBin := filepath.Join(dir, "agy")
	legacyWrapper := "#!/bin/sh\n# Managed by agy-swap session guard\nreal_command=\"" + filepath.Join(dir, ".agy.real") + "\"\n" +
		"reload_marker=\"/tmp/m\"\n\"$swap_command\" --sync-session\nexport WSL_INTEROP=\"${WSL_INTEROP:-1}\"\nis_hub=0\nhub_pid=\"\"\n"
	if err := os.WriteFile(agyBin, []byte(legacyWrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	app.ensureAgyLauncherGuardForPath(agyBin)
	upgradedBytes, err := os.ReadFile(agyBin)
	if err != nil {
		t.Fatal(err)
	}
	upgraded := string(upgradedBytes)
	if !strings.Contains(upgraded, "AGY_CLI_FORCE_FILE_TOKEN_STORAGE") {
		t.Fatalf("expected upgraded wrapper to export AGY_CLI_FORCE_FILE_TOKEN_STORAGE, got:\n%s", upgraded)
	}
	if strings.Contains(upgraded, "WSL_INTEROP") {
		t.Fatalf("expected upgraded wrapper to stop exporting WSL_INTEROP, got:\n%s", upgraded)
	}

	cmd := app.newLoginCommand(context.Background())
	envJoined := strings.Join(cmd.Env, "\n")
	if !strings.Contains(envJoined, "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1") {
		t.Fatalf("expected login command env to include AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1")
	}
	if strings.Contains(envJoined, "WSL_INTEROP=1") {
		t.Fatalf("expected login command env not to inject WSL_INTEROP=1")
	}
}
