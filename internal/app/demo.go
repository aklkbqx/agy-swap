package app

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"time"
)

type DemoOptions struct {
	Home string
}

// NewDemo uses the production TUI with an isolated, disposable home and fake credentials.
func NewDemo(version string, in io.Reader, out, errOut io.Writer, options DemoOptions) (*Application, error) {
	if options.Home == "" || !filepath.IsAbs(options.Home) {
		return nil, errors.New("demo home must be an absolute path")
	}
	paths := demoPaths(options.Home)
	store := NewStore(paths)
	accounts := NewAccounts()
	now := time.Now().UTC()
	for i, fixture := range []struct {
		email, name string
		remaining   float64
	}{
		{"alpha@example.invalid", "Alpha User", 0.86},
		{"beta@example.invalid", "Beta User", 0.43},
		{"gamma@example.invalid", "Gamma User", 0.09},
	} {
		token := demoToken(fixture.email)
		accounts.Set(fixture.email, Account{
			"email": fixture.email, "name": fixture.name,
			"token_data": token, "token_hash": hashToken(token),
			"quota_snapshot": map[string]any{
				"observed_at": now.Format(time.RFC3339),
				"tier":        map[string]any{"id": "pro-tier", "name": "Pro"},
				"groups": []any{map[string]any{
					"id": "gemini", "name": "Gemini Models",
					"buckets": []any{map[string]any{
						"id": "gemini-weekly", "name": "Weekly", "window": "weekly",
						"remaining_fraction": fixture.remaining,
						"reset_at":           now.Add(time.Duration(i+1) * 8 * time.Hour).Format(time.RFC3339),
					}},
				}},
			},
		})
	}
	if err := store.Save(accounts); err != nil {
		return nil, err
	}
	settings := defaultSettings()
	settings.Profiles["personal"] = Profile{Account: "alpha@example.invalid", Family: "gemini", Policy: "sticky"}
	settings.Profiles["work"] = Profile{Account: "beta@example.invalid", Family: "claude", Policy: "balanced"}
	if err := store.SaveSettings(settings); err != nil {
		return nil, err
	}
	credentials := &Credentials{paths: paths, backend: &demoCredentialBackend{token: demoToken("alpha@example.invalid")}}
	var reader *bufio.Reader
	if in != nil {
		reader = bufio.NewReader(in)
	}
	httpService := NewHTTPService(errOut)
	application := &Application{
		Version: version, BuildID: "demo", In: in, Out: out, Err: errOut,
		lineReader: reader, paths: paths, store: store, credentials: credentials,
		http: httpService, stdinTTY: readerTerminal(in), stdoutTTY: writerTerminal(out),
		demo: true, color: writerTerminal(out), renderClock: time.Now,
	}
	application.p = makePalette(application.color)
	if err := application.appendHistory("switch", "alpha@example.invalid", nil); err != nil {
		return nil, err
	}
	return application, nil
}

func demoPaths(home string) Paths {
	config := filepath.Join(home, ".gemini", "agy-swap")
	return Paths{
		Home: home, ConfigDir: config,
		Accounts: filepath.Join(config, "accounts.json"), AccountsBackup: filepath.Join(config, "accounts.json.bak"),
		AccountsLock: filepath.Join(config, ".accounts.lock"), SessionLock: filepath.Join(config, ".session.lock"),
		LogCache: filepath.Join(config, "log-cache-v1.json"), Settings: filepath.Join(config, "config.json"),
		History: filepath.Join(config, "history-v1.jsonl"), RuntimeState: filepath.Join(config, "runtime-state.json"),
		JournalDir:       filepath.Join(config, "journals"),
		OAuthToken:       filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token"),
		OAuthCredentials: filepath.Join(home, ".gemini", "oauth_creds.json"),
		GoogleAccounts:   filepath.Join(home, ".gemini", "google_accounts.json"),
	}
}

func demoToken(email string) string {
	data, _ := json.Marshal(map[string]any{"email": email, "token": map[string]any{"access_token": "demo-" + email, "refresh_token": "demo-only"}})
	return "go-keyring-base64:" + base64.StdEncoding.EncodeToString(data)
}

type demoCredentialBackend struct{ token string }

func (b *demoCredentialBackend) Get(_ context.Context) string { return b.token }
func (b *demoCredentialBackend) Set(_ context.Context, token string) bool {
	b.token = token
	return true
}
func (b *demoCredentialBackend) Delete(_ context.Context) bool { b.token = ""; return true }
