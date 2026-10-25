package app

import (
	"math"
	"strings"
)

// meterStyle picks how a meter's bar is drawn.
type meterStyle uint8

const (
	meterBlocks meterStyle = iota // full-height eighth blocks, as the CLI list
	meterHalf                     // half-height blocks
	meterLine                     // a heavy line over a light rail
)

// The dashboard draws half-height bars: full-height blocks touch from row to
// row in a terminal and read as a wall, half-height ones keep a visible rail.
var tuiMeterStyle = meterHalf

// renderMeter draws fraction (0..1) in width cells in the chosen style. Blocks
// keep the eighth-step precision of renderFractionalBar; the thinner styles
// resolve half a cell.
func renderMeter(style meterStyle, fraction float64, width int, fill, track, reset string) string {
	width = maxInt(1, width)
	fraction = max(0, min(1, fraction))
	if style == meterBlocks {
		return renderFractionalBar(fraction, width, fill, track, reset)
	}
	full, halves := "▄", "▖"
	// The rail is thinner than the fill, so the two stay apart without color
	// and map to different characters in ASCII mode.
	rail := "▂"
	if style == meterLine {
		full, halves, rail = "━", "╸", "─"
	}
	if fill != "" && track != "" && lerpColor(track, fill, 0.5) != fill {
		exact := fraction * float64(width)
		whole := int(math.Floor(exact))
		rem := exact - float64(whole)
		if rem >= 0.995 && whole < width {
			whole, rem = whole+1, 0
		}
		var b strings.Builder
		if whole > 0 {
			b.WriteString(fill)
			b.WriteString(strings.Repeat(full, whole))
		}
		used := whole
		if rem > 0.005 && used < width {
			if rem < 0.5 {
				b.WriteString(lerpColor(track, fill, 0.35+rem*1.3))
				b.WriteString(halves)
			} else {
				b.WriteString(lerpColor(track, fill, rem))
				b.WriteString(full)
			}
			used++
		}
		if rest := width - used; rest > 0 {
			b.WriteString(track)
			b.WriteString(strings.Repeat(rail, rest))
		}
		b.WriteString(reset)
		return b.String()
	}
	cells := int(math.Round(fraction * float64(width) * 2))
	whole, half := cells/2, cells%2
	var b strings.Builder
	if whole > 0 || half > 0 {
		b.WriteString(fill)
		b.WriteString(strings.Repeat(full, whole))
		if half == 1 {
			b.WriteString(halves)
		}
	}
	used := whole
	if half == 1 {
		used++
	}
	if rest := width - used; rest > 0 {
		b.WriteString(track)
		b.WriteString(strings.Repeat(rail, rest))
	}
	b.WriteString(reset)
	return b.String()
}

// renderThickMeter is a bar thickness rows tall, drawn with full blocks so it
// grows with the window; the unfilled rail is a dark block, and without color a
// light shade. The fill resolves eighths of a cell.
func renderThickMeter(p palette, fraction float64, width, thickness int, fill string) []string {
	width = maxInt(1, width)
	fraction = max(0, min(1, fraction))
	total := fraction * float64(width)
	whole := int(math.Floor(total))
	eighths := int(math.Round((total - float64(whole)) * 8))
	if eighths >= 8 {
		whole, eighths = whole+1, 0
	}
	if whole >= width {
		whole, eighths = width, 0
	}
	var b strings.Builder
	b.WriteString(fill)
	b.WriteString(strings.Repeat("█", whole))
	used := whole
	if eighths > 0 {
		b.WriteString(fractionalBlocks[eighths])
		used++
	}
	rail, railColor := "█", p.TrackBg
	if p.Reset == "" {
		rail = "░"
	}
	if rest := width - used; rest > 0 {
		b.WriteString(railColor)
		b.WriteString(strings.Repeat(rail, rest))
	}
	b.WriteString(p.Reset)
	rows := make([]string, maxInt(1, thickness))
	for i := range rows {
		rows[i] = b.String()
	}
	return rows
}
