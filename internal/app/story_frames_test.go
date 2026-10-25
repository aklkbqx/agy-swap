package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type storyFrameSet struct {
	Version     string                         `json:"version"`
	Frames      map[string]map[string][]string `json:"frames"`
	Transitions map[string]map[string][]string `json:"transitions,omitempty"`
}

func TestSiteStoryFramesTerminalEnvironment(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for name, vars := range map[string][]string{
		"truecolor": {"TERM=xterm-256color", "COLORTERM=truecolor"},
		"direct":    {"TERM=xterm-direct"},
		"256color":  {"TERM=xterm-256color"},
		"no-color":  {"TERM=xterm-256color", "COLORTERM=24bit", "NO_COLOR=1"},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(binary, "-test.run=^TestSiteStoryFrames$", "-test.count=1")
			for _, entry := range os.Environ() {
				key, _, _ := strings.Cut(entry, "=")
				switch key {
				case "TERM", "COLORTERM", "NO_COLOR", "AGY_SWAP_UPDATE_STORY_FRAMES":
					continue
				}
				cmd.Env = append(cmd.Env, entry)
			}
			cmd.Env = append(cmd.Env, vars...)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("fixture changed under %s: %v\n%s", name, err, output)
			}
		})
	}
}

// The website's still frames are produced by the same renderer as the live TUI.
// Run AGY_SWAP_UPDATE_STORY_FRAMES=1 go test ./internal/app -run TestSiteStoryFrames
// after changing the TUI renderer or demo data.
func TestSiteStoryFrames(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "")
	siteDir := filepath.Join("..", "..", "site")
	packageData, err := os.ReadFile(filepath.Join(siteDir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if decodeErr := json.Unmarshal(packageData, &manifest); decodeErr != nil {
		t.Fatal(decodeErr)
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
	frames := storyFrameSet{
		Version:     manifest.Version,
		Frames:      map[string]map[string][]string{},
		Transitions: map[string]map[string][]string{},
	}
	transitionOffsets := []time.Duration{
		16 * time.Millisecond,
		48 * time.Millisecond,
		90 * time.Millisecond,
		140 * time.Millisecond,
		195 * time.Millisecond,
	}
	for _, scene := range []string{"hero", "accounts", "quota", "switch"} {
		frames.Frames[scene] = map[string][]string{}
		frames.Transitions[scene] = map[string][]string{}
		for layout, size := range map[string]struct{ width, height int }{
			"desktop": {120, 34},
			"tablet":  {80, 24},
			"stage":   {80, 38},
			"mobile":  {48, 24},
			"narrow":  {40, 24},
		} {
			application.renderClock = func() time.Time { return clock }
			state := newTUIState(accounts, application.credentials.Current(context.Background()))
			state.motionEnabled = false
			switch scene {
			case "hero":
				state.selectedEmail = "alpha@example.invalid"
			case "accounts":
				state.selectedEmail = "beta@example.invalid"
			case "quota":
				state.selectedEmail = "gamma@example.invalid"
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

			// Build deterministic transition diff patches from the preceding beat.
			animState := newTUIState(accounts, application.credentials.Current(context.Background()))
			animState.motionEnabled = true
			animState.startedAt = time.Time{}
			switch scene {
			case "hero":
				animState.selectedEmail = "beta@example.invalid"
			case "accounts":
				animState.selectedEmail = "alpha@example.invalid"
			case "quota":
				animState.selectedEmail = "beta@example.invalid"
			case "switch":
				animState.selectedEmail = "gamma@example.invalid"
			}
			application.renderClock = func() time.Time { return clock }
			_ = application.tuiLines(animState, size.width-2, size.height)
			switch scene {
			case "hero":
				animState.selectedEmail = "alpha@example.invalid"
			case "accounts":
				animState.selectedEmail = "beta@example.invalid"
			case "quota":
				animState.selectedEmail = "gamma@example.invalid"
			case "switch":
				animState.selectedEmail = "beta@example.invalid"
				animState.active = "beta@example.invalid"
				animState.showToast("Switched to beta@example.invalid", "success")
				animState.toastShownAt = clock
				animState.toastUntil = time.Time{}
			}
			// Prime tweens at t=0 before sampling the 60fps ease steps.
			application.renderClock = func() time.Time { return clock }
			prev := application.tuiLines(animState, size.width-2, size.height)
			patches := make([]string, 0, len(transitionOffsets)+1)
			patches = append(patches, tuiPaintDiff(lines, prev, size.width))
			for _, d := range transitionOffsets[:len(transitionOffsets)-1] {
				application.renderClock = func() time.Time { return clock.Add(d) }
				stepLines := application.tuiLines(animState, size.width-2, size.height)
				patches = append(patches, tuiPaintDiff(prev, stepLines, size.width))
				prev = stepLines
			}
			patches = append(patches, tuiPaintDiff(prev, lines, size.width))
			frames.Transitions[scene][layout] = patches
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
		if mkdirErr := os.MkdirAll(filepath.Dir(fixturePath), 0o755); mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
		if writeErr := os.WriteFile(fixturePath, data, 0o644); writeErr != nil {
			t.Fatal(writeErr)
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
