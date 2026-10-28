package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDemoMainForwardsCLIArguments(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "agy-swap-demo")
	if out, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, command := range []string{"version", "list", "status"} {
		t.Run(command, func(t *testing.T) {
			cmd := exec.Command(binary, command)
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s failed: %v\n%s", command, err, out)
			}
			want := "alpha@example.invalid"
			if command == "version" {
				want = "agy-swap v" + version
			}
			if !strings.Contains(string(out), want) {
				t.Fatalf("%s output missing %q: %s", command, want, out)
			}
		})
	}
}
