package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type storyFrameSet struct {
	Version string                         `json:"version"`
	Frames  map[string]map[string][]string `json:"frames"`
}

// The website's still frames are produced by the same renderer as the live TUI.
// Run AGY_SWAP_UPDATE_STORY_FRAMES=1 go test ./internal/app -run TestSiteStoryFrames
// after changing the TUI renderer or demo data.
func TestSiteStoryFrames(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	siteDir := filepath.Join("..", "..", "site")
	packageData, err := os.ReadFile(filepath.Join(siteDir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(packageData, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version == "" {
		t.Fatal("site package version is empty")
	}
	clock := time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC)
	application, err := NewDemo(manifest.Version, &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, DemoOptions{Home: t.TempDir(), Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	application.color = true
	application.p = makePalette(true)
	accounts, err := application.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	frames := storyFrameSet{Version: manifest.Version, Frames: map[string]map[string][]string{}}
	for _, scene := range []string{"hero", "accounts", "quota", "switch"} {
		frames.Frames[scene] = map[string][]string{}
		for layout, size := range map[string]struct{ width, height int }{
			"desktop": {120, 24},
			"tablet":  {80, 22},
			"mobile":  {48, 22},
			"narrow":  {40, 22},
		} {
			switch scene {
			case "hero":
				if layout == "desktop" {
					size.width = 72
				}
				size.height = 20
			case "accounts":
				if layout == "mobile" || layout == "narrow" {
					size.height = 16
				}
			case "switch":
				if layout == "mobile" || layout == "narrow" {
					size.height = 18
				}
			}
			state := newTUIState(accounts, application.credentials.Current(context.Background()))
			state.motionEnabled = false
			switch scene {
			case "accounts":
				state.selectedEmail = "beta@example.invalid"
			case "quota":
				state.view = tuiViewQuota
				state.selectedEmail = "beta@example.invalid"
			case "switch":
				state.selectedEmail = "beta@example.invalid"
				state.active = "beta@example.invalid"
				state.showToast("Switched to beta@example.invalid", "success")
				state.toastUntil = time.Time{}
			}
			lines := application.tuiLines(state, size.width-2, size.height)
			if len(lines) != size.height {
				t.Fatalf("%s %s: got %d rows, want %d", scene, layout, len(lines), size.height)
			}
			for _, line := range lines {
				if got := visibleWidth(line); got != size.width {
					t.Fatalf("%s %s: got %d columns, want %d", scene, layout, got, size.width)
				}
			}
			frames.Frames[scene][layout] = lines
		}
	}
	data, err := json.MarshalIndent(frames, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if bytes.Contains(data, []byte("demo-only")) || strings.Contains(string(data), "go-keyring-base64") {
		t.Fatal("story frames contain a demo credential")
	}
	fixturePath := filepath.Join(siteDir, "src", "generated", "tui-story-frames.json")
	if os.Getenv("AGY_SWAP_UPDATE_STORY_FRAMES") == "1" {
		if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixturePath, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	committed, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, committed) {
		t.Fatal("site TUI frames differ from the Go renderer; regenerate with AGY_SWAP_UPDATE_STORY_FRAMES=1 go test ./internal/app -run TestSiteStoryFrames")
	}
}
