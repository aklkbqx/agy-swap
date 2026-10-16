package app

import (
	"strings"
	"testing"
)

func TestBigNumberDrawsFiveRowDigitsFromOneBitmap(t *testing.T) {
	want := []string{
		"███ ███ █ █",
		"  █ █     █",
		"███ ███  █ ",
		"  █ █ █ █  ",
		"███ ███ █ █",
	}
	got := bigNumber("36%", 5)
	if len(got) != 5 {
		t.Fatalf("rows = %d", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBigNumberPacksThreeRowDigitsWithHalfBlocks(t *testing.T) {
	want := []string{"▀▀█ █▀▀ ▀ █", "▀▀█ █▀█ ▄▀ ", "▀▀▀ ▀▀▀ ▀ ▀"}
	got := bigNumber("36%", 3)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
	zero := bigNumber("0%", 3)
	if zero[0] != "█▀█ ▀ █" || zero[1] != "█ █ ▄▀ " || zero[2] != "▀▀▀ ▀ ▀" {
		t.Fatalf("0%% = %q", zero)
	}
}

func TestBigNumberRowsHaveOneWidthAndUnknownRunesAreBlank(t *testing.T) {
	for _, text := range []string{"0%", "9%", "36%", "100%", "7"} {
		for _, rows := range []int{3, 5} {
			lines := bigNumber(text, rows)
			if len(lines) != rows {
				t.Fatalf("%q rows = %d", text, len(lines))
			}
			for _, line := range lines {
				if visibleWidth(line) != visibleWidth(lines[0]) {
					t.Fatalf("%q has ragged rows: %q", text, lines)
				}
			}
		}
	}
	if got := bigNumber("?", 3); strings.TrimSpace(strings.Join(got, "")) != "" {
		t.Fatalf("unknown rune drew %q", got)
	}
	if bigNumber("5", 4) != nil {
		t.Fatal("only 3 and 5 rows exist")
	}
}

func TestChipIsFilledWithColorAndBracketedWithout(t *testing.T) {
	colored := makePalette(true)
	if colored.Reset == "" {
		t.Skip("palette has no color in this environment")
	}
	chip := tuiChip(colored, colored.Yellow, "LOW")
	if !strings.Contains(chip, "\x1b[7m") || !strings.Contains(chip, " LOW ") || visibleWidth(chip) != 5 {
		t.Fatalf("chip = %q (width %d)", chip, visibleWidth(chip))
	}
	plain := tuiChip(palette{}, "", "USED UP")
	if plain != "[ USED UP ]" {
		t.Fatalf("plain chip = %q", plain)
	}
}

func TestWordmarkIsTwelveColumnsAndReadsAgySwap(t *testing.T) {
	plain := tuiWordmark(palette{})
	if plain != "agy▔▔╲▁▁swap" || visibleWidth(plain) != 12 {
		t.Fatalf("wordmark = %q (width %d)", plain, visibleWidth(plain))
	}
	colored := makePalette(true)
	if colored.Reset != "" {
		if got := tuiWordmark(colored); visibleWidth(got) != 12 || !strings.Contains(got, colored.Orange) {
			t.Fatalf("colored wordmark = %q", got)
		}
	}
}
