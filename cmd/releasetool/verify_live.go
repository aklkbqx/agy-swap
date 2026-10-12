package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Cloudflare Web Analytics injects this beacon into HTML at the edge.
var edgeAnalyticsBeacon = regexp.MustCompile(`<script[^>]*src="https://static\.cloudflareinsights\.com/beacon\.min\.js[^"]*"[^>]*></script>\s*`)

func liveClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func fetch(client *http.Client, url string) (int, []byte, http.Header, error) {
	resp, err := client.Get(url)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return resp.StatusCode, body, resp.Header, err
}

// verifyLive checks a deployed Site against the local build of its release
// tag and reports every fault found, not only the first.
func verifyLive(releaseVersion, dist, baseURL string, client *http.Client) error {
	releaseVersion = strings.TrimPrefix(releaseVersion, "v")
	baseURL = strings.TrimRight(baseURL, "/")
	// Never follow redirects: installer endpoints must answer 302 themselves.
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client = &noRedirect
	var faults []string
	fault := func(format string, args ...any) { faults = append(faults, fmt.Sprintf(format, args...)) }
	expect := func(path string, want int) []byte {
		status, body, _, err := fetch(client, baseURL+path)
		if err != nil {
			fault("%s: %v", path, err)
			return nil
		}
		if status != want {
			fault("%s returned %d, want %d", path, status, want)
		}
		return body
	}

	expect("/healthz", http.StatusOK)
	for _, path := range []string{"/install.html", "/robots.txt", "/sitemap.xml", "/llms.txt"} {
		expect(path, http.StatusOK)
	}
	if home := expect("/", http.StatusOK); !bytes.Contains(home, []byte(`"softwareVersion": "v`+releaseVersion+`"`)) {
		fault("homepage softwareVersion is not v%s", releaseVersion)
	}
	expect("/demo/ws", http.StatusNotFound)

	for _, installer := range []string{"install.sh", "install.ps1"} {
		status, _, header, err := fetch(client, baseURL+"/"+installer)
		if err != nil || status != http.StatusFound || header.Get("Location") == "" {
			fault("/%s: want a 302 to the promoted installer (status %d, err %v)", installer, status, err)
			continue
		}
		status, script, _, err := fetch(client, header.Get("Location"))
		if err != nil || status != http.StatusOK {
			fault("/%s target %s: status %d, err %v", installer, header.Get("Location"), status, err)
			continue
		}
		if !installerHasVersion(string(script), releaseVersion) {
			fault("/%s promotes an installer without version %s", installer, releaseVersion)
		}
	}

	err := filepath.WalkDir(dist, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dist, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "sitemap.xml" { // carries its build date
			return nil
		}
		local, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		status, live, _, err := fetch(client, baseURL+"/"+rel)
		if strings.HasSuffix(rel, ".html") {
			live = edgeAnalyticsBeacon.ReplaceAll(live, nil)
		}
		switch {
		case err != nil:
			fault("%s: %v", rel, err)
		case status != http.StatusOK:
			fault("%s returned %d", rel, status)
		case !bytes.Equal(local, live):
			fault("%s differs from the build (%d live bytes, %d built)", rel, len(live), len(local))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(faults) > 0 {
		return errors.New("live site does not match release v" + releaseVersion + ":\n  " + strings.Join(faults, "\n  "))
	}
	return nil
}
