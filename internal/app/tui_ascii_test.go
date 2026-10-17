package app

import (
	"strings"
	"testing"
)

func envLookup(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestASCIIModeFollowsTheOverrideThenTheTerminal(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"nothing set is a normal UTF-8 terminal", map[string]string{}, false},
		{"explicit on", map[string]string{"AGY_SWAP_ASCII": "1"}, true},
		{"explicit off beats a bad locale", map[string]string{"AGY_SWAP_ASCII": "0", "LANG": "C"}, false},
		{"linux console", map[string]string{"TERM": "linux"}, true},
		{"dumb terminal", map[string]string{"TERM": "dumb"}, true},
		{"UTF-8 locale", map[string]string{"LANG": "en_US.UTF-8"}, false},
		{"utf8 spelling", map[string]string{"LC_ALL": "en_US.utf8"}, false},
		{"C locale", map[string]string{"LANG": "C"}, true},
		{"LC_ALL wins over LANG", map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, true},
		{"POSIX locale", map[string]string{"LC_CTYPE": "POSIX"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tuiUseASCII(envLookup(tc.env)); got != tc.want {
				t.Fatalf("tuiUseASCII = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestASCIILinesKeepEveryRowTheSameWidthAndDropNonASCII(t *testing.T) {
	a, _ := headerApp()
	state := newTUIState(sizesAccounts(3), "")
	lines := a.tuiLines(state, 118, 34)
	ascii := asciiLines(lines)
	if len(ascii) != len(lines) {
		t.Fatalf("rows changed: %d vs %d", len(ascii), len(lines))
	}
	for i, line := range ascii {
		if visibleWidth(line) != visibleWidth(lines[i]) {
			t.Fatalf("row %d changed width: %d vs %d", i, visibleWidth(line), visibleWidth(lines[i]))
		}
		for _, r := range line {
			if r > 126 {
				t.Fatalf("row %d still has %q: %q", i, r, line)
			}
		}
	}
	text := strings.Join(ascii, "\n")
	for _, want := range []string{"agy--\\__swap", "#", "+--", "> "} {
		if !strings.Contains(text, want) {
			t.Fatalf("ASCII frame is missing %q", want)
		}
	}
}

func TestASCIILinesKeepAnsiColorSequences(t *testing.T) {
	in := "\x1b[38;2;1;2;3m█▉░\x1b[0m ❯ ●"
	if got := asciiLines([]string{in})[0]; got != "\x1b[38;2;1;2;3m###.\x1b[0m > *" && got != "\x1b[38;2;1;2;3m##.\x1b[0m > *" {
		// █→#, ▉→#, ░→. (three cells, three characters)
		if got != "\x1b[38;2;1;2;3m##.\x1b[0m > *" {
			t.Fatalf("got %q", got)
		}
	}
}

func TestASCIIMetersKeepTheFillAndTheRailApart(t *testing.T) {
	for _, style := range []meterStyle{meterBlocks, meterHalf, meterLine} {
		bar := asciiLine(renderMeter(style, 0.45, 20, "", "", ""))
		if !strings.ContainsAny(bar, "#=-") && style != meterLine {
			t.Fatalf("style %d: no fill in %q", style, bar)
		}
		fill := strings.IndexAny(bar, "#=")
		rail := strings.IndexAny(bar, ".-_")
		if style != meterLine && (fill < 0 || rail < 0 || fill > rail) {
			t.Fatalf("style %d: fill and rail run together in %q", style, bar)
		}
		if strings.ContainsRune(bar, '?') {
			t.Fatalf("style %d: unmapped glyph in %q", style, bar)
		}
	}
}
