package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aklkbqx/agy-swap/v2/internal/app"
)

func TestDemoMainRunsSubcommands(t *testing.T) {
	home := t.TempDir()
	var out, errOut bytes.Buffer
	application, err := app.NewDemo(version, &bytes.Buffer{}, &out, &errOut, app.DemoOptions{Home: home})
	if err != nil {
		t.Fatal(err)
	}
	code := application.Run(context.Background(), []string{"version"})
	if code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !strings.Contains(out.String(), "agy-swap v"+version) {
		t.Fatalf("unexpected version output: %s", out.String())
	}
}
