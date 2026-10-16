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
}

// tuiBodyRowsFor is the rows left for the body once the header, status line,
// footer and rules are taken out. It matches newTUIGeometry.
func tuiBodyRowsFor(height int) int { return maxInt(1, height-8) }

func tuiLayoutPlanFor(width, height int) tuiLayoutPlan {
	rows := tuiBodyRowsFor(height)
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
			plan.digits = digitsThree
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
