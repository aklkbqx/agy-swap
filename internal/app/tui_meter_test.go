package app

import "testing"

func TestMeterIsAlwaysExactlyItsWidthInEveryStyle(t *testing.T) {
	for _, style := range []meterStyle{meterBlocks, meterHalf, meterLine} {
		for width := 1; width <= 40; width++ {
			for _, fraction := range []float64{0, 0.01, 0.1, 0.36, 0.5, 0.93, 0.999, 1} {
				if got := visibleWidth(renderMeter(style, fraction, width, "", "", "")); got != width {
					t.Fatalf("style %d width %d fraction %v: drew %d cells", style, width, fraction, got)
				}
			}
		}
	}
}

func TestHalfHeightMeterFillsInHalfCellsAndKeepsAVisibleRail(t *testing.T) {
	// Fill and rail colors are escape prefixes; here they are visible markers.
	if got := renderMeter(meterHalf, 0.25, 4, "[", "|", "]"); got != "[▄|▄▄▄]" {
		t.Fatalf("a quarter of four cells = %q", got)
	}
	if got := renderMeter(meterHalf, 0.375, 4, "[", "|", "]"); got != "[▄▖|▄▄]" {
		t.Fatalf("a half cell step = %q", got)
	}
	if got := renderMeter(meterHalf, 0, 3, "[", "|", "]"); got != "|▄▄▄]" {
		t.Fatalf("empty = %q", got)
	}
	if got := renderMeter(meterLine, 0.3, 10, "", "", ""); got != "━━━───────" {
		t.Fatalf("line = %q", got)
	}
	if got := renderMeter(meterLine, 0.35, 10, "", "", ""); got != "━━━╸──────" {
		t.Fatalf("line with a half cell = %q", got)
	}
}
