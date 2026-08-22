package app

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type palette struct {
	Orange, Green, Blue, Red, Yellow, Cyan, Gray, DarkGray, White, Bold, Reset string
}

func makePalette(color bool) palette {
	if !color || os.Getenv("NO_COLOR") != "" {
		return palette{}
	}
	return palette{Orange: "\x1b[38;5;208m", Green: "\x1b[38;5;78m", Blue: "\x1b[38;5;75m", Red: "\x1b[38;5;203m", Yellow: "\x1b[38;5;220m", Cyan: "\x1b[38;5;86m", Gray: "\x1b[38;5;244m", DarkGray: "\x1b[38;5;238m", White: "\x1b[38;5;255m", Bold: "\x1b[1m", Reset: "\x1b[0m"}
}

func formatDuration(seconds float64) string {
	minutes := int(math.Floor(seconds / 60))
	if minutes < 0 {
		minutes = 0
	}
	days := minutes / (24 * 60)
	minutes %= 24 * 60
	hours := minutes / 60
	minutes %= 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

var durationPattern = regexp.MustCompile(`^\s*(?:(\d+)\s*d)?\s*(?:(\d+)\s*h)?\s*(?:(\d+)\s*m)?\s*(?:(\d+)\s*s)?\s*$`)

func parseDuration(value string) (time.Duration, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if oneOf(value, "reset", "clear", "0", "none") {
		return 0, true
	}
	if value == "" {
		return 0, false
	}
	if allDigits(value) {
		var mins int64
		if _, err := fmt.Sscan(value, &mins); err != nil {
			return 0, false
		}
		d := time.Duration(mins) * time.Minute
		return d, d <= maxLimitDuration
	}
	match := durationPattern.FindStringSubmatch(value)
	if match == nil || strings.Join(match[1:], "") == "" {
		return 0, false
	}
	var values [4]int64
	for i := range values {
		if match[i+1] != "" {
			_, _ = fmt.Sscan(match[i+1], &values[i])
		}
	}
	d := time.Duration(values[0])*24*time.Hour + time.Duration(values[1])*time.Hour + time.Duration(values[2])*time.Minute + time.Duration(values[3])*time.Second
	return d, d >= 0 && d <= maxLimitDuration
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

type ActiveLimit struct {
	Remaining time.Duration
	Limit     map[string]any
}

func activeLimits(account Account, now time.Time) []ActiveLimit {
	var result []ActiveLimit
	for _, raw := range getMap(account["quota_limits"]) {
		limit := getMap(raw)
		reset, err := parseUTC(getString(limit, "reset_at"))
		if err != nil {
			continue
		}
		remaining := reset.Sub(now)
		if remaining > 0 {
			result = append(result, ActiveLimit{remaining, limit})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Remaining < result[j].Remaining })
	return result
}

func quotaGroups(account Account) []any {
	snapshot := getMap(account["quota_snapshot"])
	return getSlice(snapshot["groups"])
}

func quotaWait(account Account, now time.Time, family string) (time.Duration, bool) {
	groups := quotaGroups(account)
	if len(groups) == 0 {
		return 0, false
	}
	groupID := ""
	if family == "gemini" {
		groupID = "gemini"
	} else if family == "claude" || family == "gpt" {
		groupID = "third_party"
	}
	matched := false
	var maxWait time.Duration
	for _, raw := range groups {
		group := getMap(raw)
		if groupID != "" && getString(group, "id") != groupID {
			continue
		}
		matched = true
		for _, rb := range getSlice(group["buckets"]) {
			b := getMap(rb)
			f, _ := getFloat(b["remaining_fraction"])
			if f > 0 {
				continue
			}
			reset, e := parseUTC(getString(b, "reset_at"))
			if e == nil && reset.Sub(now) > maxWait {
				maxWait = max(0, reset.Sub(now))
			}
		}
	}
	return maxWait, matched
}

func tierBadge(account Account, p palette) string {
	snapshot := getMap(account["quota_snapshot"])
	tier := getMap(snapshot["tier"])
	if name := cleanText(getString(tier, "name")); name != "" {
		c := p.Orange
		if getString(tier, "id") == "free-tier" {
			c = p.Gray
		}
		return c + name + p.Reset
	}
	plan := strings.Title(strings.ToLower(cleanText(firstString(account["plan"], getString(account, "tier")))))
	if getString(account, "tier_source") != "manual" || !oneOf(plan, "Pro", "Starter", "Free") {
		return p.Gray + "Unknown" + p.Reset
	}
	c := p.Gray
	if plan == "Pro" {
		c = p.Orange
	}
	return c + plan + " (manual)" + p.Reset
}

type quotaGroupHealth struct {
	id       string
	label    string
	bucket   string
	fraction float64
	resetAt  time.Time
	found    bool
}

func quotaGroupHealths(account Account) []quotaGroupHealth {
	groups := quotaGroups(account)
	result := make([]quotaGroupHealth, 0, len(groups))
	for _, rawGroup := range groups {
		group := getMap(rawGroup)
		health := quotaGroupHealth{
			id:    strings.ToLower(cleanText(getString(group, "id"))),
			label: quotaGroupLabel(group),
		}
		for _, rawBucket := range getSlice(group["buckets"]) {
			bucket := getMap(rawBucket)
			fraction, ok := getFloat(bucket["remaining_fraction"])
			if !ok {
				continue
			}
			fraction = max(0, min(1, fraction))
			if !health.found || fraction < health.fraction {
				health.found = true
				health.fraction = fraction
				health.bucket = cleanText(getString(bucket, "name"))
				health.resetAt, _ = parseUTC(getString(bucket, "reset_at"))
			}
		}
		if health.found {
			result = append(result, health)
		}
	}
	return result
}

func quotaGroupLabel(group map[string]any) string {
	switch strings.ToLower(cleanText(getString(group, "id"))) {
	case "gemini":
		return "Gemini"
	case "third_party":
		return "Claude/GPT"
	}
	name := cleanText(getString(group, "name"))
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(name, " Models"), " models"))
	return firstString(name, "Quota")
}

func bestAvailableQuotaGroup(groups []quotaGroupHealth) (quotaGroupHealth, bool) {
	var best quotaGroupHealth
	found := false
	for _, group := range groups {
		if group.fraction <= 0 {
			continue
		}
		if !found || group.fraction > best.fraction {
			best, found = group, true
		}
	}
	return best, found
}

func limitedQuotaGroupLabels(groups []quotaGroupHealth) []string {
	labels := make([]string, 0, len(groups))
	for _, group := range groups {
		if group.fraction <= 0 {
			labels = append(labels, group.label)
		}
	}
	return labels
}

func accountStatus(account Account, p palette, now time.Time) string {
	tier := tierBadge(account, p)
	groups := quotaGroupHealths(account)
	if best, ok := bestAvailableQuotaGroup(groups); ok {
		color := p.Green
		if best.fraction <= .1 {
			color = p.Red
		} else if best.fraction <= .3 {
			color = p.Yellow
		}
		status := fmt.Sprintf("[%s] %sReady%s · %s %.2f%% available", tier, color, p.Reset, best.label, best.fraction*100)
		if limited := limitedQuotaGroupLabels(groups); len(limited) > 0 {
			status += " · " + strings.Join(limited, "/") + " limited"
		}
		return status
	}
	if len(groups) > 0 {
		limited := groups[0]
		for _, group := range groups[1:] {
			if !limited.resetAt.IsZero() && !group.resetAt.IsZero() && group.resetAt.Before(limited.resetAt) {
				limited = group
			}
		}
		reset := ""
		if !limited.resetAt.IsZero() && limited.resetAt.After(now) {
			reset = fmt.Sprintf(" · %s resets in %s", limited.bucket, formatDuration(limited.resetAt.Sub(now).Seconds()))
		}
		return fmt.Sprintf("[%s] %sLimited%s%s", tier, p.Red, p.Reset, reset)
	}
	limits := activeLimits(account, now)
	if len(limits) > 0 {
		parts := make([]string, 0, len(limits))
		for _, l := range limits {
			parts = append(parts, fmt.Sprintf("%s! %s (%s)%s", p.Red, getString(l.Limit, "model"), formatDuration(l.Remaining.Seconds()), p.Reset))
		}
		return "[" + tier + "] " + strings.Join(parts, " ")
	}
	return fmt.Sprintf("[%s] %sUsage unavailable · no recent cooldown error%s", tier, p.Gray, p.Reset)
}

func formatQuotaBar(bucket map[string]any, p palette, now time.Time, width int) string {
	fraction, _ := getFloat(bucket["remaining_fraction"])
	fraction = max(0, min(1, fraction))
	filled := int(math.Round(fraction * float64(width)))
	color := p.Green
	if fraction <= .1 {
		color = p.Red
	} else if fraction <= .3 {
		color = p.Yellow
	}
	bar := color + strings.Repeat("█", filled) + p.DarkGray + strings.Repeat("░", width-filled) + p.Reset
	reset := ""
	if at, err := parseUTC(getString(bucket, "reset_at")); err == nil {
		remaining := at.Sub(now)
		if remaining > 0 {
			reset = " · resets in " + formatDuration(remaining.Seconds())
		} else {
			reset = " · refresh due"
		}
	}
	return fmt.Sprintf("[%s] %.2f%% remaining%s", bar, fraction*100, reset)
}

// formatQuotaBarResponsive keeps the value and reset window visible when a
// stacked/compact detail pane cannot fit the full desktop copy. The bar is
// still rendered with the same palette and fraction; only the copy gets
// progressively shorter.
func formatQuotaBarResponsive(bucket map[string]any, p palette, now time.Time, width, maxWidth int) string {
	width = maxInt(1, width)
	maxWidth = maxInt(1, maxWidth)
	fraction, _ := getFloat(bucket["remaining_fraction"])
	fraction = max(0, min(1, fraction))
	filled := int(math.Round(fraction * float64(width)))
	color := p.Green
	if fraction <= .1 {
		color = p.Red
	} else if fraction <= .3 {
		color = p.Yellow
	}
	bar := color + strings.Repeat("█", filled) + p.DarkGray + strings.Repeat("░", width-filled) + p.Reset
	remaining := ""
	if at, err := parseUTC(getString(bucket, "reset_at")); err == nil {
		if left := at.Sub(now); left > 0 {
			remaining = formatDuration(left.Seconds())
		} else {
			remaining = "due"
		}
	}
	full := fmt.Sprintf("[%s] %.2f%% remaining", bar, fraction*100)
	if remaining != "" {
		full += " · resets in " + remaining
	}
	if visibleWidth(full) <= maxWidth {
		return full
	}
	if remaining != "" && maxWidth >= width+16 {
		short := remaining
		if fields := strings.Fields(short); len(fields) > 2 {
			short = strings.Join(fields[:2], " ")
		}
		candidate := fmt.Sprintf("[%s] %.1f%% · %s", bar, fraction*100, short)
		if visibleWidth(candidate) <= maxWidth {
			return candidate
		}
	}
	return fmt.Sprintf("[%s] %.0f%%", bar, fraction*100)
}

func formatCooldownBar(limit map[string]any, p palette, now time.Time, width int) string {
	observed, e1 := parseUTC(getString(limit, "observed_at"))
	reset, e2 := parseUTC(getString(limit, "reset_at"))
	if e1 != nil || e2 != nil || !reset.After(observed) {
		return ""
	}
	ratio := max(0, min(1, reset.Sub(now).Seconds()/reset.Sub(observed).Seconds()))
	filled := int(math.Round(ratio * float64(width)))
	bar := p.Red + strings.Repeat("█", filled) + p.DarkGray + strings.Repeat("░", width-filled) + p.Reset
	return fmt.Sprintf("[%s] %.1f%% time left", bar, ratio*100)
}

func avatar(name, email string, color bool) string {
	parts := strings.Fields(cleanText(name))
	initials := "GU"
	if len(parts) >= 2 {
		initials = strings.ToUpper(firstRune(parts[0]) + firstRune(parts[1]))
	} else if len(parts) == 1 {
		r := []rune(parts[0])
		if len(r) > 2 {
			r = r[:2]
		}
		initials = strings.ToUpper(string(r))
	} else if len(email) >= 2 {
		initials = strings.ToUpper(email[:2])
	}
	label := "[" + initials + "]"
	if !color || os.Getenv("NO_COLOR") != "" {
		return label
	}
	colors := []int{166, 172, 64, 71, 133, 167, 31, 68}
	sum := 0
	for _, b := range []byte(email) {
		sum += int(b)
	}
	// Keep the avatar ASCII-shaped. Background-color blocks are attractive in
	// some terminals but are captured as replacement glyphs or bleed into the
	// next column in others, which makes the TUI table look corrupted.
	return fmt.Sprintf("\x1b[38;5;%dm\x1b[1m%s\x1b[0m", colors[sum%len(colors)], label)
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

var ansiPattern = regexp.MustCompile(`\x1B(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~])`)

func visibleWidth(s string) int {
	n := 0
	for _, r := range ansiPattern.ReplaceAllString(s, "") {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if isWide(r) {
			n += 2
		} else {
			n++
		}
	}
	return n
}
func isWide(r rune) bool {
	return r >= 0x1100 && (r <= 0x115f || r == 0x2329 || r == 0x232a || r >= 0x2e80 && r <= 0xa4cf || r >= 0xac00 && r <= 0xd7a3 || r >= 0xf900 && r <= 0xfaff || r >= 0xfe10 && r <= 0xfe19 || r >= 0xfe30 && r <= 0xfe6f || r >= 0xff00 && r <= 0xff60 || r >= 0xffe0 && r <= 0xffe6 || r >= 0x1f300)
}
func truncateVisible(s string, width int, p palette) string {
	if width <= 0 {
		return ""
	}
	if visibleWidth(s) <= width {
		return s
	}
	var b strings.Builder
	used := 0
	limit := maxInt(0, width-1)
	for len(s) > 0 {
		if match := ansiPattern.FindStringIndex(s); match != nil && match[0] == 0 {
			b.WriteString(s[:match[1]])
			s = s[match[1]:]
			continue
		}
		r, size := utf8.DecodeRuneInString(s)
		if size == 0 {
			break
		}
		w := 1
		if unicode.Is(unicode.Mn, r) {
			w = 0
		} else if isWide(r) {
			w = 2
		}
		if used+w > limit {
			break
		}
		b.WriteString(s[:size])
		used += w
		s = s[size:]
	}
	return b.String() + "…" + p.Reset
}
