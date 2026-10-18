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

// tuiLimitCaption sits under the big figure and names what the percentage is
// of: the limit that is tighter, or the one that is used up.
func tuiLimitCaption(pool poolView) string {
	window := map[string]string{"weekly": "weekly", "5h": "5-hour"}[pool.limitingWindow]
	if pool.state == poolUsedUp {
		return "% left · " + window + " limit used up"
	}
	return "% of the " + window + " limit left"
}

// tuiPoolPanelLines is the body of one pool's gauge panel, every row exactly
// width columns wide. The border is added by tuiBox.
func tuiPoolPanelLines(p palette, pool poolView, now time.Time, width int, digits tuiDigits) []string {
	return tuiPoolPanelScaled(p, pool, now, width, digits, 1, 0, 1)
}

// tuiPoolPanelScaled is tuiPoolPanelLines with the digits and bars grown by
// scale, gap extra blank rows between the sections and bars thick rows tall.
func tuiPoolPanelScaled(p palette, pool poolView, now time.Time, width int, digits tuiDigits, scale, gap, thick int) []string {
	scale, gap, thick = maxInt(1, scale), maxInt(0, gap), maxInt(1, thick)
	blank := func(lines *[]string, pad func(string) string) {
		for i := 0; i <= gap; i++ {
			*lines = append(*lines, pad(""))
		}
	}
	color := poolColor(p, pool.state)
	pad := func(value string) string { return fitVisible(value, width, p) }
	var lines []string
	title := p.Bold + p.White + strings.ToUpper(pool.label) + p.Reset
	lines = append(lines, pad(rowSpread(title, tuiChip(p, color, pool.state.word()), width, p)))
	if digits != tuiDigits(0) {
		// Big digits are for what needs attention: a pool with plenty left
		// gets the smaller figure.
		rows := 5
		if digits == digitsThree || (scale == 1 && pool.state == poolPlenty) {
			rows = 3
		}
		lines = append(lines, pad(""))
		// The % is not drawn big (the 3x5 sign is unreadable); the caption says it.
		digitsOnly := strings.TrimSuffix(poolPercent(pool.limiting.fraction), "%")
		figure := bigNumber(digitsOnly, rows)
		if rows == 5 && scale > 1 {
			figure = bigNumberScaled(digitsOnly, scale)
		}
		caption := tuiLimitCaption(pool)
		indent := ""
		if scale > 1 {
			// A big figure sits in the middle of its panel.
			indent = strings.Repeat(" ", maxInt(0, (width-visibleWidth(figure[0]))/2))
		}
		for _, row := range figure {
			lines = append(lines, pad(indent+color+row+p.Reset))
		}
		if scale > 1 {
			indent = strings.Repeat(" ", maxInt(0, (width-len([]rune(caption)))/2))
		}
		lines = append(lines, pad(indent+p.Gray+caption+p.Reset))
	}
	blank(&lines, pad)
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
		if scale > 1 || thick > 1 {
			for _, row := range renderThickMeter(p, l.fraction, width, maxInt(1, thick), tone) {
				lines = append(lines, pad(row))
			}
		} else {
			lines = append(lines, pad(renderMeter(tuiMeterStyle, l.fraction, width, tone, p.Track, p.Reset)))
		}
		if reset := tuiResetText(l, now); reset != "" {
			lines = append(lines, pad(p.Gray+reset+p.Reset))
		}
	}
	limit("Weekly", pool.weekly, pool.fiveHour)
	if pool.weekly.present && (pool.fiveHour.present || pool.paused) {
		blank(&lines, pad)
	}
	limit("5-hour", pool.fiveHour, pool.weekly)
	switch {
	case pool.state == poolUsedUp && pool.backIn > 0:
		blank(&lines, pad)
		lines = append(lines, pad(p.Bold+p.Red+"back in "+formatDuration(pool.backIn.Seconds())+p.Reset))
	case pool.state == poolUsedUp:
		blank(&lines, pad)
		lines = append(lines, pad(p.Bold+p.Red+"used up"+p.Reset))
	case pool.state != poolPlenty:
		// The chip already says PLENTY; only a pool in trouble needs the reassurance.
		blank(&lines, pad)
		lines = append(lines, pad(p.Bold+color+"still usable"+p.Reset))
	}
	return lines
}

// tuiPoolInlineLines is a pool with the 3-row digits beside its bars: a header
// and four rows, every row exactly width columns. It fits where the stacked
// digit panel does not.
func tuiPoolInlineLines(p palette, pool poolView, now time.Time, width int) []string {
	const digitWidth = 14
	color := poolColor(p, pool.state)
	pad := func(value string) string { return fitVisible(value, width, p) }
	title := p.Bold + p.White + strings.ToUpper(pool.label) + p.Reset
	lines := []string{pad(rowSpread(title, tuiChip(p, color, pool.state.word()), width, p))}
	right := width - digitWidth - 1
	digits := bigNumber(strings.TrimSuffix(poolPercent(pool.limiting.fraction), "%"), 3)
	rowsRight := make([]string, 0, 4)
	limit := func(name string, l poolLimit, isLimiting bool) {
		tone := poolColor(p, poolStateFor(l.fraction))
		percent := fmt.Sprintf("%4s", poolPercent(l.fraction))
		label := fmt.Sprintf("%-8s", name)
		bar := renderMeter(tuiMeterStyle, l.fraction, maxInt(4, right-8-1-len(percent)), tone, p.Track, p.Reset)
		rowsRight = append(rowsRight, label+bar+" "+tone+percent+p.Reset)
		reset, resetColor := tuiResetText(l, now), p.Gray
		if isLimiting && pool.state == poolUsedUp && pool.backIn > 0 {
			reset, resetColor = "back in "+formatDuration(pool.backIn.Seconds()), p.Bold+p.Red
		}
		rowsRight = append(rowsRight, strings.Repeat(" ", 8)+resetColor+reset+p.Reset)
	}
	if pool.weekly.present {
		limit("Weekly", pool.weekly, pool.limitingWindow == "weekly")
	}
	switch {
	case pool.fiveHour.present:
		limit("5-hour", pool.fiveHour, pool.limitingWindow == "5h")
	case pool.paused:
		rowsRight = append(rowsRight, fmt.Sprintf("%-8s", "5-hour")+p.Gray+"paused"+p.Reset, strings.Repeat(" ", 8)+p.Gray+"until weekly resets"+p.Reset)
	}
	for i := 0; i < 4; i++ {
		left := strings.Repeat(" ", digitWidth)
		if i < 3 {
			left = fitVisible(color+digits[i]+p.Reset, digitWidth, p)
			if i == 2 {
				left = fitVisible(color+digits[i]+" %"+p.Reset, digitWidth, p)
			}
		}
		rest := ""
		if i < len(rowsRight) {
			rest = rowsRight[i]
		}
		lines = append(lines, pad(left+" "+rest))
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

// tuiPoolCompactLines is a pool without a box or big digits, for small screens:
// a header, then one line per limit (the reset wraps to its own line when the
// bar would get too short). A used-up pool says when it is back.
func tuiPoolCompactLines(p palette, pool poolView, now time.Time, width int) []string {
	const labelWidth = 8
	color := poolColor(p, pool.state)
	pad := func(value string) string { return fitVisible(value, width, p) }
	title := p.Bold + p.White + strings.ToUpper(pool.label) + p.Reset
	lines := []string{pad(rowSpread(title, tuiChip(p, color, pool.state.word()), width, p))}
	limit := func(name string, l poolLimit, isLimiting bool) {
		tone := poolColor(p, poolStateFor(l.fraction))
		reset := tuiResetText(l, now)
		resetColor := p.Gray
		if isLimiting && pool.state == poolUsedUp && pool.backIn > 0 {
			reset, resetColor = "back in "+formatDuration(pool.backIn.Seconds()), p.Bold+p.Red
		}
		percent := fmt.Sprintf("%4s", poolPercent(l.fraction))
		label := fmt.Sprintf("%-*s", labelWidth, name)
		oneLine := width - labelWidth - 1 - len(percent) - 2 - len(reset)
		if reset == "" || oneLine < 10 {
			bar := renderMeter(tuiMeterStyle, l.fraction, maxInt(6, width-labelWidth-1-len(percent)), tone, p.Track, p.Reset)
			lines = append(lines, pad(label+bar+" "+tone+percent+p.Reset))
			if reset != "" {
				lines = append(lines, pad(strings.Repeat(" ", labelWidth)+resetColor+reset+p.Reset))
			}
			return
		}
		bar := renderMeter(tuiMeterStyle, l.fraction, oneLine, tone, p.Track, p.Reset)
		lines = append(lines, pad(label+bar+" "+tone+percent+p.Reset+"  "+resetColor+reset+p.Reset))
	}
	if pool.weekly.present {
		limit("Weekly", pool.weekly, pool.limitingWindow == "weekly")
	}
	switch {
	case pool.fiveHour.present:
		limit("5-hour", pool.fiveHour, pool.limitingWindow == "5h")
	case pool.paused:
		const reason = "paused until weekly resets"
		if width-labelWidth >= len(reason) {
			lines = append(lines, pad(fmt.Sprintf("%-*s", labelWidth, "5-hour")+p.Gray+reason+p.Reset))
		} else {
			lines = append(lines, pad(fmt.Sprintf("%-*s", labelWidth, "5-hour")+p.Gray+"paused"+p.Reset))
			lines = append(lines, pad(strings.Repeat(" ", labelWidth)+p.Gray+"until weekly resets"+p.Reset))
		}
	}
	return lines
}
