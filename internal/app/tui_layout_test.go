package app

import "testing"

func TestLayoutPlanFollowsWidthForArrangementAndSpaceForGaugeDetail(t *testing.T) {
	cases := []struct {
		name          string
		width, height int
		want          tuiLayoutPlan
	}{
		{"wide with room", 120, 30, tuiLayoutPlan{arrangement: arrWide, listLines: 2, panels: panelsSide, digits: digitsFive}},
		{"wide but short", 120, 22, tuiLayoutPlan{arrangement: arrWide, listLines: 2, panels: panelsSide, digits: digitsThree}},
		{"wide and very short", 120, 18, tuiLayoutPlan{arrangement: arrWide, listLines: 2, panels: panelsSide, digits: digitsNone}},
		{"wide tight stacks the panels", 96, 28, tuiLayoutPlan{arrangement: arrWide, listLines: 2, panels: panelsStack, digits: digitsThree}},
		{"wide tight and short drops the digits", 96, 22, tuiLayoutPlan{arrangement: arrWide, listLines: 2, panels: panelsStack, digits: digitsNone}},
		{"stacked with room", 80, 30, tuiLayoutPlan{arrangement: arrStacked, listLines: 2, panels: panelsSide, digits: digitsThree}},
		{"stacked at the classic 80x24", 80, 24, tuiLayoutPlan{arrangement: arrStacked, listLines: 1, panels: panelsStack, digits: digitsNone}},
		{"compact with room", 63, 24, tuiLayoutPlan{arrangement: arrCompact, listLines: 1, panels: panelsStack, digits: digitsThree}},
		{"compact and short", 63, 16, tuiLayoutPlan{arrangement: arrCompact, listLines: 1, panels: panelsStack, digits: digitsNone}},
		{"narrow with room", 40, 28, tuiLayoutPlan{arrangement: arrNarrow, listLines: 2, panels: panelsStack, digits: digitsNone}},
		{"narrow and short", 40, 20, tuiLayoutPlan{arrangement: arrNarrow, listLines: 1, panels: panelsStack, digits: digitsNone}},
		{"the smallest supported window", 28, 12, tuiLayoutPlan{arrangement: arrNarrow, listLines: 1, panels: panelsStack, digits: digitsNone}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tuiLayoutPlanFor(tc.width, tc.height); got != tc.want {
				t.Fatalf("plan(%dx%d) = %+v, want %+v", tc.width, tc.height, got, tc.want)
			}
		})
	}
}

func TestLayoutPlanBreakpointsAreExactlyWhereTheTableSays(t *testing.T) {
	arrangement := func(width int) tuiArrangement { return tuiLayoutPlanFor(width, 30).arrangement }
	for width, want := range map[int]tuiArrangement{
		28: arrNarrow, 40: arrNarrow, 41: arrCompact, 63: arrCompact,
		64: arrStacked, 91: arrStacked, 92: arrWide, 200: arrWide,
	} {
		if got := arrangement(width); got != want {
			t.Fatalf("width %d: arrangement %v, want %v", width, got, want)
		}
	}
	if tuiLayoutPlanFor(119, 30).panels != panelsStack || tuiLayoutPlanFor(120, 30).panels != panelsSide {
		t.Fatal("side-by-side panels start at 120 columns")
	}
}

func TestLayoutPlanNeverFailsForAnySize(t *testing.T) {
	for width := 1; width <= 220; width++ {
		for height := 1; height <= 70; height++ {
			plan := tuiLayoutPlanFor(width, height)
			if plan.listLines != 1 && plan.listLines != 2 {
				t.Fatalf("%dx%d: listLines = %d", width, height, plan.listLines)
			}
		}
	}
}
