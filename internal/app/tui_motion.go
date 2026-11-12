package app

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Motion is brief and only where it helps the eye follow a change: a value
// eases from where it was to where it is going, and an effect runs once from
// the moment its change happened. Nothing moves while the screen is idle, and
// AGY_SWAP_REDUCED_MOTION=1 draws every end state at once.

const (
	tuiFrameInterval = 16 * time.Millisecond // about 60 frames a second while moving
	tuiMotionSlide   = 150 * time.Millisecond
	tuiMotionMeter   = 240 * time.Millisecond
	tuiMotionModal   = 160 * time.Millisecond
	tuiMotionToast   = 160 * time.Millisecond
	tuiMotionPulse   = 600 * time.Millisecond
	tuiMotionSweep   = 250 * time.Millisecond
	tuiMotionFlash   = 160 * time.Millisecond
	tuiMotionStatus  = 450 * time.Millisecond
	tuiMotionStart   = 360 * time.Millisecond
	tuiMotionView    = 160 * time.Millisecond
	tuiCaretBlink    = 530 * time.Millisecond
	tuiRefreshSweep  = 1100 * time.Millisecond // one pass of the light while a refresh runs
	tuiBreathPeriod  = 3 * time.Second         // one breath of the active dot
	tuiAmbientFrame  = 250 * time.Millisecond  // frames while only the dot breathes
)

type tuiTween struct {
	from, to float64
	start    time.Time
	dur      time.Duration
}

func easeOutCubic(t float64) float64 {
	t = max(0, min(1, t))
	u := 1 - t
	return 1 - u*u*u
}

func (t *tuiTween) at(now time.Time) float64 {
	if t.dur <= 0 {
		return t.to
	}
	elapsed := now.Sub(t.start)
	if elapsed >= t.dur {
		return t.to
	}
	if elapsed <= 0 {
		return t.from
	}
	return t.from + (t.to-t.from)*easeOutCubic(float64(elapsed)/float64(t.dur))
}

func (t *tuiTween) done(now time.Time) bool {
	return t.dur <= 0 || !now.Before(t.start.Add(t.dur))
}

// follow is the value to draw for key: the target once it has settled, or an
// eased value on the way from the previous target. A key seen for the first
// time starts settled, and a new target mid-flight starts from where the value
// is, so a held key never queues motion.
func (s *tuiState) follow(key string, target float64, dur time.Duration, now time.Time) float64 {
	if s.tweens == nil {
		s.tweens = map[string]*tuiTween{}
	}
	tween, ok := s.tweens[key]
	if !ok || !s.motionEnabled {
		s.tweens[key] = &tuiTween{from: target, to: target, start: now}
		return target
	}
	if tween.to != target {
		*tween = tuiTween{from: tween.at(now), to: target, start: now, dur: dur}
	}
	return tween.at(now)
}

// settle puts a tween at value with no motion, for a change the eye should
// not follow.
func (s *tuiState) settle(key string, value float64) {
	if s.tweens == nil {
		s.tweens = map[string]*tuiTween{}
	}
	s.tweens[key] = &tuiTween{from: value, to: value}
}

// tweenPools eases each pool's numbers toward their values, under keys that
// start with scope. Only the drawn numbers move: a pool's state, and so its
// colour and the account's status word, follow the real value at once.
func (s *tuiState) tweenPools(scope string, pools []poolView, now time.Time) []poolView {
	eased := make([]poolView, len(pools))
	for i, pool := range pools {
		key := scope + ":" + pool.id
		pool.limiting.fraction = s.follow(key+":limit", pool.limiting.fraction, tuiMotionMeter, now)
		if pool.weekly.present {
			pool.weekly.fraction = s.follow(key+":weekly", pool.weekly.fraction, tuiMotionMeter, now)
		}
		if pool.fiveHour.present {
			pool.fiveHour.fraction = s.follow(key+":5h", pool.fiveHour.fraction, tuiMotionMeter, now)
		}
		eased[i] = pool
	}
	return eased
}

// fxProgress is how far an effect that began at start has run, from 0 to 1.
// An effect that never began, one dated after now (clock skew) and every
// effect under reduced motion count as finished.
func (s *tuiState) fxProgress(start time.Time, dur time.Duration, now time.Time) float64 {
	if !s.motionEnabled || start.IsZero() || dur <= 0 {
		return 1
	}
	elapsed := now.Sub(start)
	if elapsed < 0 || elapsed >= dur {
		return 1
	}
	return float64(elapsed) / float64(dur)
}

// moving says whether the next frame would differ from this one, so the frame
// timer runs only while something is in motion.
func (s *tuiState) moving(now time.Time) bool {
	if !s.motionEnabled {
		return false
	}
	for _, tween := range s.tweens {
		if !tween.done(now) {
			return true
		}
	}
	effects := []struct {
		start time.Time
		dur   time.Duration
	}{
		{s.startedAt, tuiMotionStart},
		{s.viewChangedAt, tuiMotionView},
		{s.modalOpenedAt, tuiMotionModal},
		{s.toastShownAt, tuiMotionToast},
		{s.activeChangedAt, tuiMotionPulse},
		{s.keyFlashAt, tuiMotionFlash},
	}
	for _, fx := range effects {
		if s.fxProgress(fx.start, fx.dur, now) < 1 {
			return true
		}
	}
	for _, at := range s.statusChangedAt {
		if s.fxProgress(at, tuiMotionStatus, now) < 1 {
			return true
		}
	}
	// The caret blinks, and the light crosses the screen while a refresh runs.
	return s.mode == tuiSearch || s.refreshing
}

// tintColumns gives some columns of a row a background of their own, from
// bgAt (empty for none), and keeps every other cell and colour as it was.
// It walks the row once, so a column's own background comes back after the
// tinted cell.
func tintColumns(line string, bgAt func(col int) string) string {
	var out strings.Builder
	col, lastBg := 0, ""
	for len(line) > 0 {
		if size := ansiPrefixLen(line); size > 0 {
			code := line[:size]
			switch {
			case code == "\x1b[0m" || code == "\x1b[m":
				lastBg = ""
			case strings.HasPrefix(code, "\x1b[48;") || (len(code) == 5 && strings.HasPrefix(code, "\x1b[4")):
				lastBg = code
			}
			out.WriteString(code)
			line = line[size:]
			continue
		}
		r, size := utf8.DecodeRuneInString(line)
		if bg := bgAt(col); bg != "" {
			out.WriteString(bg + line[:size] + "\x1b[49m" + lastBg)
		} else {
			out.WriteString(line[:size])
		}
		col += terminalRuneWidth(r)
		line = line[size:]
	}
	return out.String()
}

// tuiRefreshSweep draws the light that crosses the header bar while a refresh
// the user asked for runs: a soft band, brightest in its middle, leaning so
// lower header rows trail, looping until the answer arrives without tainting
// body rows in tuiPaintDiff.
func (a *Application) tuiRefreshSweep(state *tuiState, lines []string, width int, now time.Time) []string {
	fromBg := firstString(a.p.BarBg, a.p.BodyBg)
	if !state.refreshing || !state.motionEnabled || fromBg == "" || a.p.KeyBg == "" {
		return lines
	}
	if oneOfMode(state.mode, tuiDialogOpen, tuiPalette, tuiForm) {
		return lines
	}
	headerRows := state.bodyTop
	if headerRows <= 0 {
		headerRows = minInt(4, len(lines))
	}
	headerRows = minInt(headerRows, len(lines))
	if headerRows == 0 {
		return lines
	}
	elapsed := max(0, now.Sub(state.refreshStartedAt))
	const halfRadius = 3.0
	span := float64(width) + float64(headerRows)*0.5 + 2*halfRadius
	head := (float64(elapsed%tuiRefreshSweep)/float64(tuiRefreshSweep))*span - halfRadius
	for i := 0; i < headerRows; i++ {
		center := head - float64(i)*0.5
		peakCol := int(math.Round(center))
		lines[i] = tintColumns(lines[i], func(col int) string {
			if col == peakCol {
				return a.p.KeyBg
			}
			dist := math.Abs(float64(col) - center)
			if dist >= halfRadius {
				return ""
			}
			level := 0.5 * (1 + math.Cos(math.Pi*dist/halfRadius))
			return lerpColor(fromBg, a.p.KeyBg, level)
		})
	}
	return lines
}

func parseTruecolorSGR(s string) (kind byte, r, g, b int, ok bool) {
	if len(s) < 13 || s[0] != 0x1b || s[1] != '[' || (s[2] != '3' && s[2] != '4') || s[3] != '8' || s[4] != ';' || s[5] != '2' || s[6] != ';' || s[len(s)-1] != 'm' {
		return 0, 0, 0, 0, false
	}
	idx := 7
	nextInt := func() (int, byte, bool) {
		if idx >= len(s)-1 {
			return 0, 0, false
		}
		val, digits := 0, 0
		for idx < len(s) {
			c := s[idx]
			idx++
			if c >= '0' && c <= '9' {
				val = val*10 + int(c-'0')
				digits++
				if val > 255 || digits > 3 {
					return 0, 0, false
				}
				continue
			}
			if digits == 0 {
				return 0, 0, false
			}
			return val, c, true
		}
		return 0, 0, false
	}
	var sep byte
	if r, sep, ok = nextInt(); !ok || sep != ';' {
		return 0, 0, 0, 0, false
	}
	if g, sep, ok = nextInt(); !ok || sep != ';' {
		return 0, 0, 0, 0, false
	}
	if b, sep, ok = nextInt(); !ok || sep != 'm' || idx != len(s) {
		return 0, 0, 0, 0, false
	}
	return s[2], r, g, b, true
}

// lerpColor mixes two truecolor codes of the same kind (foreground or
// background). Anything else, a 256-colour code or none, gives the end colour.
func lerpColor(from, to string, t float64) string {
	fk, fr, fg, fb, fok := parseTruecolorSGR(from)
	if !fok {
		return to
	}
	tk, tr, tg, tb, tok := parseTruecolorSGR(to)
	if !tok || fk != tk {
		return to
	}
	if t >= 1 {
		return to
	}
	t = max(0, t)
	mix := func(a, b int) int { return a + int(math.Round(float64(b-a)*t)) }
	var buf [24]byte
	n := copy(buf[:], "\x1b[")
	buf[n] = tk
	n++
	n += copy(buf[n:], "8;2;")
	out := strconv.AppendInt(buf[:n], int64(mix(fr, tr)), 10)
	out = append(out, ';')
	out = strconv.AppendInt(out, int64(mix(fg, tg)), 10)
	out = append(out, ';')
	out = strconv.AppendInt(out, int64(mix(fb, tb)), 10)
	out = append(out, 'm')
	return string(out)
}

// tuiPaintDiff rewrites only the rows that changed since prev. Without a
// previous frame of the same size it paints the whole frame.
func tuiPaintDiff(prev, lines []string, width int) string {
	if len(prev) != len(lines) {
		return tuiPaint(lines, width)
	}
	var out strings.Builder
	for i, line := range lines {
		if prev[i] == line {
			continue
		}
		fmt.Fprintf(&out, "\x1b[%d;1H", i+1)
		out.WriteString(line)
		if visibleWidth(line) < width {
			out.WriteString("\x1b[K")
		}
	}
	return out.String()
}
