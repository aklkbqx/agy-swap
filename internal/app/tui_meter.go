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
