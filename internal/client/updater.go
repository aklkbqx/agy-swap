package client

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ReleaseAsset represents an attached binary or checksum asset in a GitHub Release.
type ReleaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

// GithubRelease represents release metadata returned by the GitHub API.
type GithubRelease struct {
	Tag     string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []ReleaseAsset `json:"assets"`
}

// NormalizedReleaseTag ensures the release tag begins with a 'v' prefix.
func NormalizedReleaseTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag != "" && !strings.HasPrefix(tag, "v") {
		return "v" + tag
	}
	return tag
}

// ExpectedChecksum extracts the SHA-256 checksum for a binary asset name from a checksums manifest.
func ExpectedChecksum(manifest []byte, name string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(manifest)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == name {
			sum := strings.ToLower(fields[0])
			if len(sum) == 64 {
				if _, err := hex.DecodeString(sum); err == nil {
					return sum, nil
				}
			}
		}
	}
	return "", fmt.Errorf("checksum for %s not found in manifest", name)
}

// FetchLatestRelease queries the GitHub releases API for the specified repository.
func FetchLatestRelease(ctx context.Context, httpClient *http.Client, repo string) (*GithubRelease, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "agy-swap")

	requestCtx, cancel := context.WithTimeout(request.Context(), 10*time.Second)
	defer cancel()

	response, err := httpClient.Do(request.WithContext(requestCtx))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("GitHub API returned HTTP %d", response.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1024*1024 {
		return nil, fmt.Errorf("release payload exceeds 1MB")
	}

	var release GithubRelease
	if err := json.Unmarshal(data, &release); err != nil {
		return nil, fmt.Errorf("invalid release JSON: %w", err)
	}
	if release.Tag == "" {
		return nil, fmt.Errorf("release has no tag")
	}

	return &release, nil
}
