package app

import (
	"strings"
	"testing"
)

func TestDashboardFooterDropsTheLeastUsefulKeysFirstAndNeverOverflows(t *testing.T) {
	a, state := headerApp()
	cases := []struct {
		width int
		want  []string
		gone  []string
	}{
		{118, []string{"[↑↓]", "[Enter]", "[^K]", "[?]", "[q]"}, nil},
		{50, []string{"[↑↓]", "[Enter]", "[?]", "[q]"}, nil},
		{36, []string{"[Enter]", "[q]"}, []string{"Resize"}},
		{26, []string{"[q]"}, nil},
	}
	for _, tc := range cases {
		row := a.tuiFooterLines(state, tc.width)[1]
		if visibleWidth(row) != tc.width+2 {
			t.Fatalf("width %d: footer row is %d wide: %q", tc.width, visibleWidth(row), row)
		}
		for _, want := range tc.want {
			if !strings.Contains(row, want) {
				t.Fatalf("width %d: footer lost %q: %q", tc.width, want, row)
			}
		}
		for _, gone := range tc.gone {
			if strings.Contains(row, gone) {
				t.Fatalf("width %d: footer kept %q: %q", tc.width, gone, row)
			}
		}
	}
}
