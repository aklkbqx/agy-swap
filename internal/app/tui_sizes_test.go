package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func sizesAccounts(count int) *Accounts {
	accounts := NewAccounts()
	for i := 0; i < count; i++ {
		email := fmt.Sprintf("user%02d@example.invalid", i)
		account := namedPoolAccount(fmt.Sprintf("User %02d", i),
			poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.9-float64(i%5)*0.2, 100*time.Hour), poolBucket("5h", 0.5, 3*time.Hour)),
			poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", float64(i%3)*0.4, 22*time.Hour)))
		account["email"] = email
		accounts.Set(email, account)
	}
	return accounts
}

// Every terminal size from the smallest supported to a large one must give a
// frame of exactly that size with no broken rows, however many accounts.
func TestEveryTerminalSizeProducesAnExactFrame(t *testing.T) {
	a := &Application{Version: "2.13.0", p: palette{}, renderClock: func() time.Time { return poolNow }}
	for _, count := range []int{0, 1, 3, 12} {
		state := newTUIState(sizesAccounts(count), "")
		if count > 0 {
			state.selectedEmail = fmt.Sprintf("user%02d@example.invalid", count/2)
		}
		for _, size := range sizeGrid() {
			width, height := size[0], size[1]
			{
				lines := a.tuiLines(state, width-2, height)
				if len(lines) != height {
					t.Fatalf("%d accounts, %dx%d: %d rows", count, width, height, len(lines))
				}
				for i, line := range lines {
					if got := visibleWidth(line); got != width {
						t.Fatalf("%d accounts, %dx%d: row %d is %d wide: %q", count, width, height, i, got, line)
					}
					if strings.ContainsRune(line, '\n') || strings.ContainsRune(line, '�') {
						t.Fatalf("%d accounts, %dx%d: row %d is broken: %q", count, width, height, i, line)
					}
				}
				if first, last := []rune(lines[0]), []rune(lines[height-1]); first[0] != '╭' || last[len(last)-1] != '╯' {
					t.Fatalf("%d accounts, %dx%d: frame corners are %q ... %q", count, width, height, lines[0], lines[height-1])
				}
			}
		}
	}
}

// sizeGrid is a coarse sweep of every layout boundary plus the common sizes.
func sizeGrid() [][2]int {
	var sizes [][2]int
	for width := 28; width <= 200; width += 7 {
		for height := 12; height <= 60; height += 4 {
			sizes = append(sizes, [2]int{width, height})
		}
	}
	for _, width := range []int{28, 40, 41, 63, 64, 80, 91, 92, 96, 107, 108, 119, 120, 160} {
		for _, height := range []int{12, 16, 18, 22, 24, 28, 30, 38} {
			sizes = append(sizes, [2]int{width, height})
		}
	}
	return sizes
}

func TestSelectedAccountStaysVisibleWithManyAccounts(t *testing.T) {
	a := &Application{Version: "2.13.0", p: palette{}, renderClock: func() time.Time { return poolNow }}
	state := newTUIState(sizesAccounts(30), "")
	for _, size := range [][2]int{{120, 30}, {80, 24}, {63, 24}, {40, 24}} {
		for _, index := range []int{0, 14, 29} {
			state.selectedEmail = fmt.Sprintf("user%02d@example.invalid", index)
			text := strings.Join(a.tuiLines(state, size[0]-2, size[1]), "\n")
			if !strings.Contains(text, fmt.Sprintf("User %02d", index)) || !strings.Contains(text, "❯") {
				t.Fatalf("%dx%d: selected account %d is not visible:\n%s", size[0], size[1], index, text)
			}
		}
	}
}
