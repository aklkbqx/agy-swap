package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestCloudRequestsSendClientUserAgent(t *testing.T) {
	for _, tc := range []struct {
		name, env, want string
	}{
		{"default", "", "antigravity/hub/" + cloudClientVersion + " " + runtime.GOOS + "/" + runtime.GOARCH},
		{"override", "antigravity/9.9.9 test", "antigravity/9.9.9 test"},
		{"blank override", "   ", "antigravity/hub/" + cloudClientVersion + " " + runtime.GOOS + "/" + runtime.GOARCH},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AGY_SWAP_USER_AGENT", tc.env)
			var got []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = append(got, r.Header.Get("User-Agent"))
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"done":true}`)
			}))
			defer server.Close()
			service := NewHTTPService(io.Discard)
			service.cloudAPI = server.URL + "/v1internal:"
			if _, err := service.cloudPost(context.Background(), "token", "loadCodeAssist", map[string]any{}); err != nil {
				t.Fatal(err)
			}
			if _, err := service.cloudGet(context.Background(), "token", "operations/x"); err != nil {
				t.Fatal(err)
			}
			if len(got) != 2 || got[0] != tc.want || got[1] != tc.want {
				t.Fatalf("User-Agent = %q, want %q on both requests", got, tc.want)
			}
		})
	}
}
