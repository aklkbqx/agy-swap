package app

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestDemoSeedsIndependentAccountsAndSwitchesLocally(t *testing.T) {
	firstHome := filepath.Join(t.TempDir(), "first")
	secondHome := filepath.Join(t.TempDir(), "second")
	first, err := NewDemo("2.6.0", &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, DemoOptions{Home: firstHome})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDemo("2.6.0", &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, DemoOptions{Home: secondHome})
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := first.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts.Order) != 3 || accounts.Order[0] != "alpha@example.invalid" || accounts.Order[1] != "beta@example.invalid" {
		t.Fatalf("unexpected demo accounts: %v", accounts.Order)
	}
	settings, err := first.loadSettings()
	if err != nil || len(settings.Profiles) == 0 {
		t.Fatalf("demo profiles missing: %v", err)
	}
	if got := localActiveEmail(accounts, first.credentials.Current(context.Background())); got != "alpha@example.invalid" {
		t.Fatalf("initial active account = %q", got)
	}
	beta, err := first.accountToken(context.Background(), accounts.ByEmail["beta@example.invalid"])
	if err != nil || !first.applyAccount(context.Background(), beta, "beta@example.invalid") {
		t.Fatalf("switch failed: %v", err)
	}
	if got := localActiveEmail(accounts, first.credentials.Current(context.Background())); got != "beta@example.invalid" {
		t.Fatalf("active after switch = %q", got)
	}
	other, _ := second.store.Load(false)
	if got := localActiveEmail(other, second.credentials.Current(context.Background())); got != "alpha@example.invalid" {
		t.Fatalf("second session changed: %q", got)
	}
}

func TestDemoPolicyRejectsExternalEffects(t *testing.T) {
	application, err := NewDemo("2.6.0", &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, DemoOptions{Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"login", "run-now", "update", "backup-export", "history-export", "doctor", "vault-migrate", "target", "binding"} {
		if err := application.allowDemoAction(action); err == nil || !strings.Contains(err.Error(), "Unavailable in demo") {
			t.Errorf("%s was allowed: %v", action, err)
		}
	}
	for _, action := range []string{"switch", "refresh", "profile-create", "tags"} {
		if err := application.allowDemoAction(action); err != nil {
			t.Errorf("%s was blocked: %v", action, err)
		}
	}
}
