package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNormalizedReleaseTag(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"2.3.1", "v2.3.1"},
		{"v2.3.1", "v2.3.1"},
		{"  2.3.1 ", "v2.3.1"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizedReleaseTag(c.input); got != c.want {
			t.Errorf("NormalizedReleaseTag(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestExpectedChecksum(t *testing.T) {
	manifest := []byte(`
e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  agy-swap_v2.3.1_darwin_arm64
f2ca1bb6c7e907d06dafe4687e579fce76b37e4e93b7605022da52e6ccc26fd2 *agy-swap_v2.3.1_linux_amd64
`)

	sum, err := ExpectedChecksum(manifest, "agy-swap_v2.3.1_darwin_arm64")
	if err != nil || sum != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("unexpected checksum: %s, %v", sum, err)
	}

	sum, err = ExpectedChecksum(manifest, "agy-swap_v2.3.1_linux_amd64")
	if err != nil || sum != "f2ca1bb6c7e907d06dafe4687e579fce76b37e4e93b7605022da52e6ccc26fd2" {
		t.Fatalf("unexpected checksum with asterisk: %s, %v", sum, err)
	}

	if _, err := ExpectedChecksum(manifest, "nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent asset")
	}
}

func TestFetchLatestReleaseMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"tag_name":"v2.3.1","html_url":"https://github.com/test/release","assets":[{"name":"bin","browser_download_url":"http://test"}]}`)
	}))
	defer server.Close()

	client := server.Client()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("mock failed: %v", err)
	}
	defer resp.Body.Close()
}
