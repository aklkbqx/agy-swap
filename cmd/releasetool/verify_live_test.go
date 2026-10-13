package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSite serves a built dist plus the production-only endpoints; fields let a
// test break one behavior at a time.
type fakeSite struct {
	dist           string
	health         int
	demoStatus     int
	shellVersion   string
	psVersion      string
	overrides      map[string]string
	missing        map[string]bool
	notFoundStatus int
	installerHost  string
}

func (f *fakeSite) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/healthz":
		w.WriteHeader(f.health)
		_, _ = w.Write([]byte("OK\n"))
		return
	case "/demo/ws":
		w.WriteHeader(f.demoStatus)
		return
	case "/install.sh", "/install.ps1":
		http.Redirect(w, r, f.installerHost+"/raw"+r.URL.Path, http.StatusFound)
		return
	case "/raw/install.sh":
		_, _ = w.Write([]byte("#!/bin/sh\nVERSION=\"${AGY_SWAP_VERSION:-" + f.shellVersion + "}\"\n"))
		return
	case "/raw/install.ps1":
		_, _ = w.Write([]byte("if ([string]::IsNullOrWhiteSpace($Version)) { $Version = '" + f.psVersion + "' }\n"))
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if name == "install" {
		name = "install.html"
	}
	notFound := func() {
		w.WriteHeader(f.notFoundStatus)
		body, _ := os.ReadFile(filepath.Join(f.dist, "404.html"))
		if override, ok := f.overrides["404.html"]; ok {
			body = []byte(override)
		}
		_, _ = w.Write(body)
	}
	if name == "404.html" {
		notFound()
		return
	}
	if f.missing[name] {
		http.NotFound(w, r)
		return
	}
	if body, ok := f.overrides[name]; ok {
		_, _ = w.Write([]byte(body))
		return
	}
	data, err := os.ReadFile(filepath.Join(f.dist, filepath.FromSlash(name)))
	if err != nil {
		notFound()
		return
	}
	_, _ = w.Write(data)
}

func newFakeSite(t *testing.T) (*fakeSite, *httptest.Server) {
	t.Helper()
	dist := t.TempDir()
	files := map[string]string{
		"index.html":          `<script type="application/ld+json">{"softwareVersion": "v2.0.0"}</script>`,
		"install.html":        "<h1>Install</h1>",
		"robots.txt":          "User-agent: *\n",
		"sitemap.xml":         "<lastmod>2026-10-12</lastmod>",
		"llms.txt":            "go install github.com/aklkbqx/agy-swap/v2/cmd/agy-swap@latest\n",
		"404.html":            "<title>Page not found</title>",
		"assets/main-abc.js":  "console.log(1)",
		"assets/styles-x.css": "body{}",
	}
	for name, body := range files {
		path := filepath.Join(dist, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, path, body)
	}
	site := &fakeSite{dist: dist, health: 200, demoStatus: 404, shellVersion: "2.0.0", psVersion: "2.0.0", missing: map[string]bool{}, notFoundStatus: 404, overrides: map[string]string{
		// Production sitemaps carry their own build date; never byte-compared.
		"sitemap.xml": "<lastmod>2026-10-13</lastmod>",
	}}
	server := httptest.NewServer(site)
	t.Cleanup(server.Close)
	site.installerHost = server.URL
	return site, server
}

func TestVerifyLiveAcceptsMatchingDeployment(t *testing.T) {
	site, server := newFakeSite(t)
	if err := verifyLive("2.0.0", site.dist, server.URL, server.Client()); err != nil {
		t.Fatalf("matching deployment rejected: %v", err)
	}
}

func TestVerifyLiveReportsEveryDeploymentFault(t *testing.T) {
	for name, tc := range map[string]struct {
		breakSite func(*fakeSite)
		want      string
	}{
		"stale asset":          {func(f *fakeSite) { f.overrides["assets/main-abc.js"] = "console.log(0)" }, "assets/main-abc.js"},
		"missing page":         {func(f *fakeSite) { f.missing["install.html"] = true }, "install.html"},
		"old softwareVersion":  {func(f *fakeSite) { f.overrides["index.html"] = `{"softwareVersion": "v1.9.0"}` }, "softwareVersion"},
		"old shell installer":  {func(f *fakeSite) { f.shellVersion = "1.9.0" }, "install.sh"},
		"old ps installer":     {func(f *fakeSite) { f.psVersion = "1.9.0" }, "install.ps1"},
		"demo gateway exposed": {func(f *fakeSite) { f.demoStatus = 200 }, "/demo/ws"},
		"unhealthy":            {func(f *fakeSite) { f.health = 503 }, "/healthz"},
		"soft 404":             {func(f *fakeSite) { f.notFoundStatus = 200 }, "unknown path"},
		"stale 404 page":       {func(f *fakeSite) { f.overrides["404.html"] = "<title>Old</title>" }, "404 page"},
		"install route":        {func(f *fakeSite) { f.missing["install.html"] = true }, "/install"},
	} {
		t.Run(name, func(t *testing.T) {
			site, server := newFakeSite(t)
			tc.breakSite(site)
			err := verifyLive("2.0.0", site.dist, server.URL, server.Client())
			if err == nil {
				t.Fatal("faulty deployment accepted")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name %q", err, tc.want)
			}
		})
	}
}

const cloudflareBeacon = `<script type="module" src="https://static.cloudflareinsights.com/beacon.min.js/v4bc70e2c" integrity="sha512-L0ha" data-cf-beacon='{"version":"2024.11.0","token":"abc","r":1,"spa":2}' crossorigin="anonymous"></script>` + "\n"

// Cloudflare Web Analytics injects its beacon into HTML at the edge; that alone
// is not a deployment difference, but any other injected markup is.
func TestVerifyLiveIgnoresOnlyTheEdgeAnalyticsBeacon(t *testing.T) {
	site, server := newFakeSite(t)
	page, err := os.ReadFile(filepath.Join(site.dist, "install.html"))
	if err != nil {
		t.Fatal(err)
	}
	site.overrides["install.html"] = string(page) + cloudflareBeacon
	if err := verifyLive("2.0.0", site.dist, server.URL, server.Client()); err != nil {
		t.Fatalf("edge analytics beacon treated as a deployment difference: %v", err)
	}
	site.overrides["install.html"] = string(page) + `<script src="https://evil.example/x.js"></script>`
	if err := verifyLive("2.0.0", site.dist, server.URL, server.Client()); err == nil || !strings.Contains(err.Error(), "install.html") {
		t.Fatalf("other injected markup accepted: %v", err)
	}
}
