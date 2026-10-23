package app

import "strings"

// tuiUseASCII says whether the screen should avoid box, block and symbol
// glyphs. AGY_SWAP_ASCII=1 or 0 decides; otherwise a Linux console, a dumb
// terminal or a non-UTF-8 locale selects ASCII. An unset locale is treated as
// UTF-8.
func tuiUseASCII(env func(string) string) bool {
	switch env("AGY_SWAP_ASCII") {
	case "1":
		return true
	case "0":
		return false
	}
	switch env("TERM") {
	case "linux", "dumb":
		return true
	}
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if value := env(key); value != "" {
			lower := strings.ToLower(value)
			return !strings.Contains(lower, "utf-8") && !strings.Contains(lower, "utf8")
		}
	}
	return false
}

var asciiGlyphs = strings.NewReplacer(
	"█", "#", "▉", "#", "▊", "#", "▋", "#", "▌", "#", "▍", "#", "▎", "#", "▏", "#",
	"░", ".", "▀", "#", "▄", "#", "▖", "=", "▗", "=", "▂", ".", "▃", ".",
	"▔", "-", "▁", "_", "╲", "\\",
	"╭", "+", "╮", "+", "╰", "+", "╯", "+", "├", "+", "┤", "+", "┬", "+", "┴", "+", "┼", "+",
	"─", "-", "│", "|", "━", "-",
	"❯", ">", "┃", ">", "●", "*", "◉", "o", "·", ".", "⚠", "!", "✓", "v", "✕", "x", "⬢", "*", "↑↓", "^v", "←→", "<>", "…", "~",
	"—", "-", "–", "-", "→", ">", "◐", "o", "◓", "o", "◑", "o", "◒", "o", "⌁", "~", "›", ">",
)

// asciiLines swaps every non-ASCII glyph for a one-column ASCII stand-in,
// leaving color sequences alone, so each row keeps its width.
func asciiLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = asciiLine(line)
	}
	return out
}

func asciiLine(line string) string {
	var b strings.Builder
	for len(line) > 0 {
		if line[0] == '\x1b' {
			if size := ansiPrefixLen(line); size > 0 {
				b.WriteString(line[:size])
				line = line[size:]
				continue
			}
		}
		next := strings.IndexByte(line, '\x1b')
		if next < 0 {
			next = len(line)
		} else if next == 0 {
			b.WriteByte(line[0])
			line = line[1:]
			continue
		}
		for _, r := range asciiGlyphs.Replace(line[:next]) {
			if r < 127 {
				b.WriteRune(r)
				continue
			}
			// Anything unlisted keeps its column width as question marks.
			b.WriteString(strings.Repeat("?", maxInt(1, terminalRuneWidth(r))))
		}
		line = line[next:]
	}
	return b.String()
}
