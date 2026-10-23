package app

import (
	"strings"
	"testing"
	"time"
)

func TestTheDetailDoesNotNarrateBackgroundRefreshes(t *testing.T) {
	a, state := selectionState()
	plan := tuiLayoutPlan{arrangement: arrWide, panels: panelsStack, digits: digitsNone}
	text := strings.Join(a.tuiGaugeDetail(state, 60, 0, plan), "\n")
	for _, gone := range []string{"Session token", "updated just now", "updated"} {
		if strings.Contains(text, gone) {
			t.Fatalf("fresh detail still says %q:\n%s", gone, text)
		}
	}
	account := state.accounts.ByEmail["beta@example.invalid"]
	getMap(account["quota_snapshot"])["observed_at"] = poolNow.Add(-20 * time.Minute).Format(time.RFC3339)
	text = strings.Join(a.tuiGaugeDetail(state, 60, 0, plan), "\n")
	if !strings.Contains(text, "updated 20m") || !strings.Contains(text, "ago") {
		t.Fatalf("old numbers must say how old they are:\n%s", text)
	}
}

func TestOnlyARefreshTheUserAskedForReportsSuccess(t *testing.T) {
	if message, _, animate := refreshNotice(true, nil); message != "" || animate {
		t.Fatalf("background refresh says %q (animate %v)", message, animate)
	}
	if message, kind, animate := refreshNotice(false, nil); message != "Usage refreshed" || kind != "success" || !animate {
		t.Fatalf("manual refresh says %q %q %v", message, kind, animate)
	}
	for _, quiet := range []bool{true, false} {
		if message, kind, _ := refreshNotice(quiet, map[string]string{"a@example.invalid": "boom"}); kind != "warning" || !strings.Contains(message, "1 warning") {
			t.Fatalf("quiet=%v: a failed refresh says %q %q", quiet, message, kind)
		}
	}
}
