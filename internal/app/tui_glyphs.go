package app

import "strings"

// Big numbers come from one 3x5 pixel bitmap per glyph. Five rows draw each
// pixel as a block; three rows pack two pixel rows into one with half blocks.
var bigGlyphs = map[rune][5]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"010", "110", "010", "010", "111"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "111", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "001", "001", "001"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
	'%': {"101", "001", "010", "100", "101"},
}

// bigNumber draws text such as "36%" in 3 or 5 rows. Other row counts return nil.
func bigNumber(text string, rows int) []string {
	if rows != 3 && rows != 5 {
		return nil
	}
	lines := make([]strings.Builder, rows)
	for index, r := range []rune(text) {
		bitmap, ok := bigGlyphs[r]
		if !ok {
			bitmap = [5]string{"000", "000", "000", "000", "000"}
		}
		for row := 0; row < rows; row++ {
			if index > 0 {
				// Terminal cells are about twice as tall as wide, so a 5-row
				// pixel is two cells wide to stay square.
				lines[row].WriteString(pick(rows == 5, "  ", " "))
			}
			for col := 0; col < 3; col++ {
				if rows == 5 {
					lines[row].WriteString(pick(bitmap[row][col] == '1', "██", "  "))
					continue
				}
				top := bitmap[row*2][col] == '1'
				bottom := row*2+1 < 5 && bitmap[row*2+1][col] == '1'
				switch {
				case top && bottom:
					lines[row].WriteString("█")
				case top:
					lines[row].WriteString("▀")
				case bottom:
					lines[row].WriteString("▄")
				default:
					lines[row].WriteString(" ")
				}
			}
		}
	}
	result := make([]string, rows)
	for i := range lines {
		result[i] = lines[i].String()
	}
	return result
}

// bigNumberScaled draws text in 5*scale rows: every pixel of the 3x5 bitmap is
// a block 2*scale cells wide and scale rows tall, so the digits stay square
// and grow with the window.
func bigNumberScaled(text string, scale int) []string {
	scale = maxInt(1, scale)
	lines := make([]strings.Builder, 5*scale)
	for index, r := range []rune(text) {
		bitmap, ok := bigGlyphs[r]
		if !ok {
			bitmap = [5]string{"000", "000", "000", "000", "000"}
		}
		for row := 0; row < 5; row++ {
			for sub := 0; sub < scale; sub++ {
				out := &lines[row*scale+sub]
				if index > 0 {
					out.WriteString(strings.Repeat(" ", 2*scale))
				}
				for col := 0; col < 3; col++ {
					out.WriteString(pick(bitmap[row][col] == '1', strings.Repeat("█", 2*scale), strings.Repeat(" ", 2*scale)))
				}
			}
		}
	}
	result := make([]string, len(lines))
	for i := range lines {
		result[i] = lines[i].String()
	}
	return result
}

// bigNumberWidth is the columns bigNumberScaled needs for glyphs characters.
func bigNumberWidth(glyphs, scale int) int {
	if glyphs <= 0 {
		return 0
	}
	return glyphs*6*scale + (glyphs-1)*2*scale
}

func pick(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}

// tuiChip is a short state word. With color it is a filled chip (reverse video
// in the state color); without color it keeps brackets so the word still stands out.
func tuiChip(p palette, color, word string) string {
	if p.Reset == "" {
		return "[ " + word + " ]"
	}
	return color + "\x1b[7m " + word + " " + p.Reset
}

// tuiWordmark is the website's wordmark in terminal glyphs: agy, an orange
// step line, swap.
func tuiWordmark(p palette) string {
	return p.White + p.Bold + "agy" + p.Reset + p.Orange + "▔▔╲▁▁" + p.Reset + p.White + p.Bold + "swap" + p.Reset
}
