package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestIsHomebrewManaged(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "cellar path",
			path: "/opt/homebrew/Cellar/agy-swap/2.11.0/bin/agy-swap",
			want: true,
		},
		{
			name: "linux brew cellar path",
			path: "/home/linuxbrew/.linuxbrew/Cellar/agy-swap/2.11.0/bin/agy-swap",
			want: true,
		},
		{
			name: "local bin path",
			path: "/Users/user/.local/bin/agy-swap",
			want: false,
		},
		{
			name: "custom bin path",
			path: "/usr/local/bin/agy-swap",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHomebrewManaged(tt.path)
			if got != tt.want {
				t.Errorf("isHomebrewManaged(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCleanShadowedLocalBinary(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	localBinDir := filepath.Join(tempHome, ".local", "bin")
	if err := os.MkdirAll(localBinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(localBinDir, "agy-swap")
	if err := os.WriteFile(binaryPath, []byte("dummy"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cleanShadowedLocalBinary(&out, makePalette(false))

	if _, err := os.Stat(binaryPath); !os.IsNotExist(err) {
		t.Fatalf("expected binary at %s to be deleted, got err=%v", binaryPath, err)
	}
	if !strings.Contains(out.String(), "Removed shadowing binary") {
		t.Fatalf("expected output to mention removing shadowing binary, got %q", out.String())
	}
}

func TestFormatUpdateDownloadErrorStripsSignedQuery(t *testing.T) {
	rawURL := "https://release-assets.githubusercontent.com/github-production-release-asset/1295161353/8f4b8121-1e1e-4f06-8e13-542da9a1302b?sp=r&sv=2018-11-09&jwt=eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.secret&filename=checksums.txt"
	err := &url.Error{
		Op:  "Get",
		URL: rawURL,
		Err: errors.New(`tls: failed to verify certificate: x509: “10.1.104.2” certificate is not standards compliant`),
	}
	got := formatUpdateDownloadError(err)
	if strings.Contains(got, "jwt=") || strings.Contains(got, "sp=r") {
		t.Fatalf("expected query string to be stripped from error, got %q", got)
	}
	if !strings.Contains(got, "https://release-assets.githubusercontent.com/github-production-release-asset/1295161353/8f4b8121-1e1e-4f06-8e13-542da9a1302b") {
		t.Fatalf("expected host and path to be preserved, got %q", got)
	}
	if !strings.Contains(got, "10.1.104.2") {
		t.Fatalf("expected underlying TLS error to be preserved, got %q", got)
	}
}

func TestDiagnoseUpdateDownloadErrorDetectsPrivateIPInterception(t *testing.T) {
	interceptionErr := errors.New(`Get "https://release-assets.githubusercontent.com/asset": tls: failed to verify certificate: x509: “10.1.104.2” certificate is not standards compliant`)
	hint := diagnoseUpdateDownloadError(interceptionErr)
	if !strings.Contains(hint, "10.1.104.2") || !strings.Contains(hint, "AGY_SWAP_RELEASE_MIRROR") {
		t.Fatalf("expected private IP firewall/captive-portal hint, got %q", hint)
	}

	caErr := errors.New(`Get "https://release-assets.githubusercontent.com/asset": tls: failed to verify certificate: x509: certificate signed by unknown authority`)
	caHint := diagnoseUpdateDownloadError(caErr)
	if !strings.Contains(caHint, "AGY_SWAP_CA_BUNDLE") {
		t.Fatalf("expected custom CA bundle hint, got %q", caHint)
	}
}

func TestFilterPublicDoHIPsRejectsPrivateSinkholes(t *testing.T) {
	ips := filterPublicDoHIPs([]string{
		"10.1.104.2",
		"192.168.1.1",
		"172.16.0.1",
		"127.0.0.1",
		"169.254.1.1",
		"not-an-ip",
		"185.199.108.133",
		"185.199.109.133",
	})
	if len(ips) != 2 || ips[0] != "185.199.108.133" || ips[1] != "185.199.109.133" {
		t.Fatalf("expected only public GitHub IPs, got %v", ips)
	}
}

func TestHTTPServiceTrustsCustomCABundle(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok-custom-ca"))
	}))
	defer ts.Close()

	cert, err := x509.ParseCertificate(ts.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	caFile := filepath.Join(t.TempDir(), "custom-ca.pem")
	if writeErr := os.WriteFile(caFile, pemBytes, 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	t.Setenv("AGY_SWAP_CA_BUNDLE", caFile)

	var errBuf bytes.Buffer
	svc := NewHTTPService(&errBuf)
	data, status, err := svc.getBytes(context.Background(), ts.URL, nil, 5*time.Second, 1024)
	if err != nil || status != http.StatusOK || string(data) != "ok-custom-ca" {
		t.Fatalf("expected custom CA request to succeed, got status=%d data=%q err=%v", status, string(data), err)
	}
	if strings.Contains(errBuf.String(), "AGY_SWAP_INSECURE_TLS") {
		t.Fatalf("expected verified TLS without insecure warning, got %q", errBuf.String())
	}
}

func TestResolveHostViaDoHFiltersPrivateIPs(t *testing.T) {
	dohSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "release-assets.githubusercontent.com" {
			http.Error(w, "unexpected name", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Status":0,"Answer":[{"type":5,"data":"github.map.fastly.net."},{"type":1,"data":"10.1.104.2"},{"type":1,"data":"185.199.108.133"},{"type":1,"data":"185.199.109.133"}]}`))
	}))
	defer dohSrv.Close()

	t.Setenv("AGY_SWAP_DOH_URL", dohSrv.URL)
	ips, err := resolveHostViaDoH(context.Background(), dohSrv.Client(), "release-assets.githubusercontent.com")
	if err != nil {
		t.Fatalf("expected DoH resolution to succeed, got err=%v", err)
	}
	if len(ips) != 2 || ips[0] != "185.199.108.133" || ips[1] != "185.199.109.133" {
		t.Fatalf("expected filtered public IPs, got %v", ips)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestHTTPServiceDoRetriesViaDoHOnTLSError(t *testing.T) {
	var errBuf bytes.Buffer
	svc := NewHTTPService(&errBuf)
	var primaryCalls, dohCalls atomic.Int32

	svc.client = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			primaryCalls.Add(1)
			return nil, &url.Error{
				Op:  "Get",
				URL: r.URL.String(),
				Err: errors.New(`tls: failed to verify certificate: x509: “10.1.104.2” certificate is not standards compliant`),
			}
		}),
	}
	svc.dohClient = &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			dohCalls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("recovered-via-doh")),
				Header:     make(http.Header),
			}, nil
		}),
	}

	data, status, err := svc.getBytes(context.Background(), "https://release-assets.githubusercontent.com/github-production-release-asset/1295161353/asset", nil, 5*time.Second, 1024)
	if err != nil || status != http.StatusOK || string(data) != "recovered-via-doh" {
		t.Fatalf("expected DoH retry to recover from TLS sinkhole, got status=%d data=%q err=%v", status, string(data), err)
	}
	if primaryCalls.Load() != 1 || dohCalls.Load() != 1 {
		t.Fatalf("expected 1 primary call and 1 DoH call, got primary=%d doh=%d", primaryCalls.Load(), dohCalls.Load())
	}
}

func TestCmdUpdateFailsFastOnChecksumsAndReportsActualAsset(t *testing.T) {
	assetName := "agy-swap_v9.9.9_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	var binaryRequests atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			rel := map[string]any{
				"tag_name": "v9.9.9",
				"html_url": srv.URL + "/release-notes",
				"assets": []map[string]any{
					{
						"name":                 "checksums.txt",
						"browser_download_url": srv.URL + "/download/checksums.txt?sp=r&jwt=secret-token",
					},
					{
						"name":                 assetName,
						"browser_download_url": srv.URL + "/download/" + assetName + "?sp=r&jwt=secret-token",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		case "/download/checksums.txt":
			http.Error(w, "blocked", http.StatusBadGateway)
		default:
			binaryRequests.Add(1)
			http.Error(w, "should not be called", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("AGY_SWAP_UPDATE_URL", srv.URL+"/releases/latest")
	t.Setenv("AGY_SWAP_NO_GO_FALLBACK", "1")

	var outBuf, errBuf bytes.Buffer
	app := &Application{
		Version: "2.17.2",
		Out:     &outBuf,
		Err:     &errBuf,
		http:    NewHTTPService(&errBuf),
		p:       makePalette(false),
	}
	code := app.cmdUpdate(context.Background(), cliArgs{})
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if binaryRequests.Load() != 0 {
		t.Fatalf("expected binary download not to be attempted when checksums.txt fails, got %d requests", binaryRequests.Load())
	}
	if !strings.Contains(errBuf.String(), "Download failed for checksums.txt") {
		t.Fatalf("expected error to name checksums.txt, got %q", errBuf.String())
	}
}

func TestDownloadReleaseAssetFallsBackAcrossCandidates(t *testing.T) {
	payload := []byte("verified-release-payload")
	sum := sha256.Sum256(payload)
	expectedHex := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/primary/asset":
			http.Error(w, "blocked by firewall", http.StatusForbidden)
		case "/mirror/v9.9.9/asset":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("AGY_SWAP_RELEASE_MIRROR", srv.URL+"/mirror")
	var errBuf bytes.Buffer
	svc := NewHTTPService(&errBuf)
	candidates := assetCandidateURLs("v9.9.9", "asset", releaseAsset{
		Name:   "asset",
		URL:    srv.URL + "/primary/asset",
		APIURL: srv.URL + "/api/asset",
	})
	data, err := svc.downloadReleaseAsset(context.Background(), candidates, 5*time.Second, 1024*1024)
	if err != nil {
		t.Fatalf("expected fallback candidate to succeed, got err=%v", err)
	}
	gotSum := sha256.Sum256(data)
	if hex.EncodeToString(gotSum[:]) != expectedHex {
		t.Fatalf("unexpected payload checksum")
	}
	_ = net.IPv4zero
}

func TestFetchReleaseMetadataFallsBackToSiteSoftwareVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api-latest":
			http.NotFound(w, r)
		case "/site":
			_, _ = w.Write([]byte(`<!doctype html><html><head><script type="application/ld+json">{"softwareVersion": "v2.20.1"}</script></head></html>`))
		case "/download/v2.20.1/checksums.txt":
			_, _ = w.Write([]byte("abc  agy-swap_v2.20.1_" + runtime.GOOS + "_" + runtime.GOARCH + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("AGY_SWAP_UPDATE_URL", srv.URL+"/api-latest")
	t.Setenv("AGY_SWAP_SITE_URL", srv.URL+"/site")
	previousBase := releaseDownloadBase
	releaseDownloadBase = srv.URL + "/download"
	t.Cleanup(func() { releaseDownloadBase = previousBase })

	var outBuf, errBuf bytes.Buffer
	app := &Application{
		Version: "2.20.1",
		Out:     &outBuf,
		Err:     &errBuf,
		http:    NewHTTPService(&errBuf),
		p:       makePalette(false),
	}
	code := app.cmdUpdate(context.Background(), cliArgs{})
	if code != 0 {
		t.Fatalf("expected exit code 0 via site metadata fallback, got %d (err=%q)", code, errBuf.String())
	}
	if !strings.Contains(outBuf.String(), "Already up to date (v2.20.1)") {
		t.Fatalf("expected up-to-date message from site metadata fallback, got %q", outBuf.String())
	}
}

func TestSiteVersionFallbackNeedsPublishedStableRelease(t *testing.T) {
	cases := []struct {
		name      string
		site      string
		checksums bool
		mirror    bool
		wantTag   string
	}{
		{"published release", "2.99.0", true, false, "v2.99.0"},
		{"site ahead of the release", "2.99.0", false, false, ""},
		{"prerelease", "2.99.0-rc.1", true, false, ""},
		{"checksums only on the mirror", "2.99.0", false, true, "v2.99.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/site":
					_, _ = w.Write([]byte(`{"softwareVersion": "v` + tc.site + `"}`))
				case r.URL.Path == "/download/v"+tc.site+"/checksums.txt" && tc.checksums:
					_, _ = w.Write([]byte("abc  agy-swap\n"))
				case r.URL.Path == "/mirror/v"+tc.site+"/checksums.txt" && tc.mirror:
					_, _ = w.Write([]byte("abc  agy-swap\n"))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			t.Setenv("AGY_SWAP_UPDATE_URL", srv.URL+"/api-latest")
			t.Setenv("AGY_SWAP_SITE_URL", srv.URL+"/site")
			if tc.mirror {
				t.Setenv("AGY_SWAP_RELEASE_MIRROR", srv.URL+"/mirror")
			}
			previousBase := releaseDownloadBase
			releaseDownloadBase = srv.URL + "/download"
			t.Cleanup(func() { releaseDownloadBase = previousBase })

			release, err := NewHTTPService(io.Discard).fetchLatestRelease(context.Background())
			if tc.wantTag == "" {
				if err == nil {
					t.Fatalf("fallback announced %q, want an error", release.Tag)
				}
				return
			}
			if err != nil || release.Tag != tc.wantTag {
				t.Fatalf("release = %q, err = %v; want %q", release.Tag, err, tc.wantTag)
			}
		})
	}
}

func TestGoFallbackLdflagsMarkTheBuild(t *testing.T) {
	flags := goFallbackLdflags("2.99.0")
	if !strings.Contains(flags, "-X main.version=2.99.0") || !strings.Contains(flags, "-X main.buildID=go-install") {
		t.Fatalf("ldflags = %q", flags)
	}
	if strings.Contains(flags, "buildID=release") {
		t.Fatalf("a go install build must not claim to be a release build: %q", flags)
	}
}

func TestGoFallbackDoesNotClaimVerification(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fallback does not run on Windows")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(dir, "agy-swap")
	if err := os.WriteFile(current, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AGY_SWAP_GO_FALLBACK", "1")
	var out bytes.Buffer
	a := &Application{Version: "2.20.1", Out: &out, Err: io.Discard, p: makePalette(false)}
	if a.tryGoToolchainFallback(context.Background(), "2.99.0", current, "", errors.New("download failed")) {
		t.Fatal("fallback reported success with a failing go command")
	}
	if strings.Contains(out.String(), "verified") || !strings.Contains(out.String(), "not checked against the release checksum") {
		t.Fatalf("fallback message = %q", out.String())
	}
}
