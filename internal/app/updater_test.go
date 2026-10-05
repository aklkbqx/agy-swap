package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
