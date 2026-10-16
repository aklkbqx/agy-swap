package app

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func (state poolState) word() string {
	switch state {
	case poolLow:
		return "LOW"
	case poolNearlyOut:
		return "NEARLY OUT"
	case poolUsedUp:
		return "USED UP"
	}
	return "PLENTY"
}

func poolColor(p palette, state poolState) string {
	switch state {
	case poolLow:
		return p.Yellow
	case poolNearlyOut, poolUsedUp:
		return p.Red
	}
	return p.Green
}

func poolPercent(fraction float64) string {
	return fmt.Sprintf("%d%%", int(math.Round(max(0, min(1, fraction))*100)))
}

// rowSpread puts left and right on one row of exactly width columns.
func rowSpread(left, right string, width int, p palette) string {
	gap := width - visibleWidth(left) - visibleWidth(right)
	if gap < 1 {
		return fitVisible(left, width, p)
	}
	return left + strings.Repeat(" ", gap) + right
}

func tuiResetText(limit poolLimit, now time.Time) string {
	if limit.resetAt.IsZero() {
		return ""
	}
	if left := limit.resetAt.Sub(now); left > 0 {
		return "resets in " + formatDuration(left.Seconds())
	}
	return "refresh due"
}

// tuiPoolPanelLines is the body of one pool's gauge panel, every row exactly
// width columns wide. The border is added by tuiBox.
func tuiPoolPanelLines(p palette, pool poolView, now time.Time, width int, digits tuiDigits) []string {
	color := poolColor(p, pool.state)
	pad := func(value string) string { return fitVisible(value, width, p) }
	var lines []string
	title := p.Bold + p.White + strings.ToUpper(pool.label) + p.Reset
	lines = append(lines, pad(rowSpread(title, tuiChip(p, color, pool.state.word()), width, p)))
	if digits != tuiDigits(0) {
		rows := 5
		if digits == digitsThree {
			rows = 3
		}
		lines = append(lines, pad(""))
		for _, row := range bigNumber(poolPercent(pool.limiting.fraction), rows) {
			lines = append(lines, pad(color+row+p.Reset))
		}
		caption := "weekly is the tighter limit"
		switch {
		case pool.state == poolUsedUp:
			caption = "weekly limit used up"
		case pool.limitingWindow == "5h":
			caption = "5-hour is the tighter limit"
		}
		if pool.state == poolUsedUp && pool.limitingWindow == "5h" {
			caption = "5-hour limit used up"
		}
		lines = append(lines, pad(p.Gray+caption+p.Reset))
	}
	lines = append(lines, pad(""))
	limit := func(name string, l poolLimit, other poolLimit) {
		if !l.present {
			if name == "5-hour" && pool.paused {
				lines = append(lines, pad(rowSpread(name, p.Gray+"paused"+p.Reset, width, p)))
				lines = append(lines, pad(p.Gray+"paused until weekly resets"+p.Reset))
			}
			return
		}
		tone := poolColor(p, poolStateFor(l.fraction))
		lines = append(lines, pad(rowSpread(name, p.Bold+tone+poolPercent(l.fraction)+p.Reset, width, p)))
		lines = append(lines, pad(renderFractionalBar(l.fraction, width, tone, p.DarkGray, p.Reset)))
		if reset := tuiResetText(l, now); reset != "" {
			lines = append(lines, pad(p.Gray+reset+p.Reset))
		}
	}
	limit("Weekly", pool.weekly, pool.fiveHour)
	if pool.weekly.present && (pool.fiveHour.present || pool.paused) {
		lines = append(lines, pad(""))
	}
	limit("5-hour", pool.fiveHour, pool.weekly)
	lines = append(lines, pad(""))
	switch {
	case pool.state == poolUsedUp && pool.backIn > 0:
		lines = append(lines, pad(p.Bold+p.Red+"back in "+formatDuration(pool.backIn.Seconds())+p.Reset))
	case pool.state == poolUsedUp:
		lines = append(lines, pad(p.Bold+p.Red+"used up"+p.Reset))
	default:
		lines = append(lines, pad(p.Bold+color+"usable right now"+p.Reset))
	}
	return lines
}

// tuiBox wraps lines in a rounded border of exactly width columns. Each line
// is fitted to the inside (width-4: border and one space on each side).
func tuiBox(p palette, lines []string, width int, borderColor string) []string {
	inner := maxInt(1, width-4)
	if borderColor == "" {
		borderColor = p.DarkGray
	}
	edge := func(left, right string) string {
		return borderColor + left + strings.Repeat("─", width-2) + right + p.Reset
	}
	box := []string{edge("╭", "╮")}
	for _, line := range lines {
		box = append(box, borderColor+"│"+p.Reset+" "+fitVisible(line, inner, p)+" "+borderColor+"│"+p.Reset)
	}
	return append(box, edge("╰", "╯"))
}
