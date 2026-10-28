package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaRecordingPromotesOnlyCompleteValidatedSet(t *testing.T) {
	for _, failure := range []string{"", "dashboard", "vhs", "poster"} {
		t.Run("failure="+failure, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range []string{"scripts/tapes", "bin", "docs/assets", "dist/media", "site/public/media", "promo"} {
				if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			script, readErr := os.ReadFile("../../scripts/record-media.sh")
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err := os.WriteFile(filepath.Join(root, "scripts/record-media.sh"), script, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "Makefile"), []byte("VERSION ?= 2.0.0\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, clip := range []string{"cli", "dashboard", "switch"} {
				if err := os.WriteFile(filepath.Join(root, "scripts/tapes", clip+".tape"), []byte("fixture"), 0o644); err != nil {
					t.Fatal(err)
				}
				for _, file := range []string{"docs/assets/" + clip + ".gif", "dist/media/" + clip + ".mp4", "site/public/media/" + clip + ".mp4", "site/public/media/" + clip + ".png"} {
					if err := os.WriteFile(filepath.Join(root, file), []byte("previous"), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			// Preserve older local copies without creating new duplicates beside README GIFs.
			legacyMP4 := filepath.Join(root, "docs/assets/dashboard.mp4")
			if err := os.WriteFile(legacyMP4, []byte("legacy-local"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, ext := range []string{"gif", "mp4"} {
				if err := os.WriteFile(filepath.Join(root, "promo/agy-swap-demo."+ext), []byte("previous"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			tools := map[string]string{
				"go": `#!/bin/sh
[ -f Makefile ] || exit 1
while [ "$#" -gt 0 ]; do
 if [ "$1" = -o ]; then shift; printf '#!/bin/sh\n[ -z "${NO_COLOR:-}" ] && [ "$COLORTERM" = truecolor ] || exit 9\nexit 0\n' > "$1"; chmod +x "$1"; break; fi
 shift
done
`,
				"ttyd": "#!/bin/sh\nexit 0\n",
				"vhs": `#!/bin/sh
clip=$(basename "$1" .tape)
printf recorded > "docs/assets/$clip.gif"
printf recorded > "docs/assets/$clip.mp4"
if [ "$MEDIA_TEST_FAIL" = vhs ]; then exit 1; fi
if [ "$MEDIA_TEST_FAIL" = "$clip" ]; then
 # Replace just the demo executable, leaving the wrapper and validation real.
 printf '#!/bin/sh\nexit 7\n' > "$(dirname "$(command -v agy-swap)")/agy-swap-demo"
fi
agy-swap list || true
`,
				"ffmpeg": `#!/bin/sh
[ "$MEDIA_TEST_FAIL" != poster ] || exit 1
for output do :; done
printf poster > "$output"
`,
			}
			for name, body := range tools {
				if err := os.WriteFile(filepath.Join(root, "bin", name), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("bash", filepath.Join(root, "scripts/record-media.sh"))
			cmd.Env = append(os.Environ(), "PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"), "MEDIA_TEST_FAIL="+failure, "NO_COLOR=1", "COLORTERM=disabled")
			out, err := cmd.CombinedOutput()
			if (err != nil) != (failure != "") {
				t.Fatalf("unexpected result: %v\n%s", err, out)
			}
			for _, clip := range []string{"cli", "dashboard", "switch"} {
				localMP4 := filepath.Join(root, "docs/assets", clip+".mp4")
				if clip == "dashboard" {
					data, err := os.ReadFile(localMP4)
					if err != nil || string(data) != "legacy-local" {
						t.Fatalf("legacy local MP4 changed: %q, %v", data, err)
					}
				} else if _, err := os.Stat(localMP4); !os.IsNotExist(err) {
					t.Fatalf("recording created an unnecessary Tool MP4 at %s: %v", localMP4, err)
				}
				for _, file := range []string{"docs/assets/" + clip + ".gif", "dist/media/" + clip + ".mp4", "site/public/media/" + clip + ".mp4", "site/public/media/" + clip + ".png"} {
					data, err := os.ReadFile(filepath.Join(root, file))
					if err != nil {
						t.Fatal(err)
					}
					if failure != "" && string(data) != "previous" {
						t.Fatalf("failed recording replaced %s with %q", file, data)
					}
					if failure == "" && !strings.Contains(string(data), "recorded") && string(data) != "poster" {
						t.Fatalf("new recording missing: %s", file)
					}
				}
			}
			for _, ext := range []string{"gif", "mp4"} {
				data, err := os.ReadFile(filepath.Join(root, "promo/agy-swap-demo."+ext))
				if err != nil {
					t.Fatal(err)
				}
				want := "recorded"
				if failure != "" {
					want = "previous"
				}
				if string(data) != want {
					t.Fatalf("promo %s = %q, want %q", ext, data, want)
				}
			}
		})
	}
}
