package app

// A layout plan answers two questions for a terminal size: how the screen is
// arranged (decided by width) and how much detail the gauges can carry
// (decided by the rows left over). It only reads the size.

type tuiArrangement uint8

const (
	arrWide    tuiArrangement = iota // list on the left, gauges on the right
	arrStacked                       // list on top, gauges below
	arrCompact                       // one-line rows, pools stacked below
	arrNarrow                        // 40 columns or fewer
)

type tuiPanels uint8

const (
	panelsSide  tuiPanels = iota // two pool panels next to each other
	panelsStack                  // pool panels one above the other
)

type tuiDigits uint8

const (
	digitsNone   tuiDigits = iota // limits as plain lines
	digitsThree                   // 3-row half-block digits
	digitsInline                  // 3-row digits beside the bars, about 7 rows a pool
	digitsFive                    // 5-row block digits
)

type tuiLayoutPlan struct {
	arrangement tuiArrangement
	listLines   int // lines each account takes in the list: 1 or 2
	panels      tuiPanels
	digits      tuiDigits
	// tight drops the freshness line and the blank rows between pools, for
	// terminals that are too short to show both pools any other way.
	tight bool
	// scale grows the 5-row digits and the bars with the window (1 = normal),
	// gap adds blank rows between the sections of a panel and thick is how many
	// rows tall each bar is.
	scale, gap, thick int
}

// tuiHeaderAirMinHeight is the window height from which the header gets a
// blank row between the logo and the active account line. From 32 rows the
// body still has the 22 rows the richest layout asks for; shorter windows
// keep that row for the list.
const tuiHeaderAirMinHeight = 32

// tuiChromeRows is what the header and footer bars take: the header's padding,
// logo and active line, the body's leading blank row, and the footer's
// padding, status line and keys.
const tuiChromeRows = 9

// tuiPlanChromeRows is the chrome the layout plan is measured against. Plans
// stay tied to the window height they always had; when the padded bars leave a
// row less, tuiFitPlan gives up detail instead of overflowing.
const tuiPlanChromeRows = 8

func tuiHeaderAir(height int) bool { return height >= tuiHeaderAirMinHeight }

// tuiBodyRowsFor is the rows left for the body once the header, status line,
// footer and rules are taken out. It matches newTUIGeometry.
func tuiBodyRowsFor(height int) int {
	chrome := tuiChromeRows
	if tuiHeaderAir(height) {
		chrome++
	}
	return maxInt(1, height-chrome)
}

func tuiLayoutPlanFor(width, height int) tuiLayoutPlan {
	return tuiLayoutPlanForRows(width, maxInt(1, height-tuiPlanChromeRows))
}

// tuiLayoutPlanForRows picks the plan for a body of this many rows.
func tuiLayoutPlanForRows(width, rows int) tuiLayoutPlan {
	switch {
	case width >= 92:
		plan := tuiLayoutPlan{arrangement: arrWide, listLines: 2, panels: panelsStack}
		if width >= 120 {
			plan.panels = panelsSide
		}
		switch {
		case plan.panels == panelsSide && rows >= 22:
			plan.digits = digitsFive
		case plan.panels == panelsSide && rows >= 13:
			plan.digits = digitsThree
		case plan.panels == panelsStack && rows >= 16:
			// One gauge column: digits beside the bars keep a pool to about
			// seven rows instead of a tall digit block over the bars.
			plan.digits = digitsInline
		}
		return plan
	case width >= 64:
		if rows >= 22 {
			return tuiLayoutPlan{arrangement: arrStacked, listLines: 2, panels: panelsSide, digits: digitsThree}
		}
		return tuiLayoutPlan{arrangement: arrStacked, listLines: 1, panels: panelsStack}
	case width >= 41:
		plan := tuiLayoutPlan{arrangement: arrCompact, listLines: 1, panels: panelsStack}
		if rows >= 15 {
			plan.digits = digitsThree
		}
		return plan
	}
	plan := tuiLayoutPlan{arrangement: arrNarrow, listLines: 1, panels: panelsStack}
	if rows >= 18 {
		plan.listLines = 2
	}
	return plan
}
