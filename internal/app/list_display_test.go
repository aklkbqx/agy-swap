package app

import (
	"strings"
	"testing"
	"time"
)

func TestListQuotaResetLocalTime(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, reset, suffix string
		now                 time.Time
	}{
		{"local date rollover", "2026-10-13T23:30:00Z", " · resets in 1h 30m (Wed 14 Oct 06:30 +0700)", time.Date(2026, time.October, 14, 5, 0, 0, 0, time.FixedZone("ICT", 7*60*60))},
		{"UTC", "2026-10-13T23:30:00Z", " · resets in 1h 30m (Tue 13 Oct 23:30 +0000)", time.Date(2026, time.October, 13, 22, 0, 0, 0, time.UTC)},
		{"DST fallback uses reset offset", "2026-11-01T06:30:00Z", " · resets in 1h 0m (Sun 01 Nov 01:30 -0500)", time.Date(2026, time.November, 1, 5, 30, 0, 0, time.UTC).In(newYork)},
		{"overdue", "2026-10-13T21:30:00Z", " · refresh due (Tue 13 Oct 21:30 +0000)", time.Date(2026, time.October, 13, 22, 0, 0, 0, time.UTC)},
		{"exact reset", "2026-10-13T22:00:00Z", " · refresh due (Tue 13 Oct 22:00 +0000)", time.Date(2026, time.October, 13, 22, 0, 0, 0, time.UTC)},
		{"invalid reset", "not-a-time", "", time.Now()},
		{"missing reset", "", "", time.Now()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bucket := map[string]any{"remaining_fraction": .5, "reset_at": tc.reset}
			got := formatQuotaBar(bucket, makePalette(false), tc.now, 4)
			if want := "[██░░]  50.00% remaining" + tc.suffix; got != want {
				t.Fatalf("quota row = %q, want %q", got, want)
			}
		})
	}
}

func TestListQuotaPercentAlignmentAndColors(t *testing.T) {
	p := makePalette(true)
	for _, tc := range []struct {
		fraction       float64
		percent, color string
	}{
		{-1, "  0.00%", p.Red},
		{.1, " 10.00%", p.Red},
		{.3, " 30.00%", p.Yellow},
		{.31, " 31.00%", p.Green},
		{2, "100.00%", p.Green},
	} {
		bucket := map[string]any{"remaining_fraction": tc.fraction}
		got := formatQuotaBar(bucket, p, time.Now(), 12)
		if !strings.Contains(got, "] "+tc.percent+" remaining") {
			t.Errorf("fraction %v: percentage unaligned in %q", tc.fraction, got)
		}
		// A zero-length filled segment has only the empty track's color.
		if tc.fraction > 0 && !strings.Contains(got, tc.color) {
			t.Errorf("fraction %v: lost its existing health color in %q", tc.fraction, got)
		}
	}
}

func TestListQuotaBucketLabelsUseWindow(t *testing.T) {
	for _, tc := range []struct{ window, name, want string }{
		{"weekly", "Weekly Limit Remaining", "Weekly"},
		{"5h", "Five Hour Limit Remaining", "5-Hour"},
		{"daily", "Daily quota", "Daily quota"},
		{"", "Legacy quota", "Legacy quota"},
	} {
		bucket := map[string]any{"window": tc.window, "name": tc.name}
		if got := quotaBucketLabel(bucket); got != tc.want {
			t.Errorf("window %q: label = %q, want %q", tc.window, got, tc.want)
		}
	}
}
