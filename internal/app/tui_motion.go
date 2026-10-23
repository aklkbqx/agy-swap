package app

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

// Motion is brief and only where it helps the eye follow a change: a value
// eases from where it was to where it is going, and an effect runs once from
// the moment its change happened. Nothing moves while the screen is idle, and
// AGY_SWAP_REDUCED_MOTION=1 draws every end state at once.

const (
	tuiFrameInterval = 33 * time.Millisecond // about 30 frames a second while moving
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
	tuiAmbientFrame  = 100 * time.Millisecond  // frames while only the dot breathes
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

// tuiRefreshSweep draws the light that crosses the screen while a refresh
// the user asked for runs: a soft band, brightest in its middle, leaning so
// lower rows trail, looping until the answer arrives.
func (a *Application) tuiRefreshSweep(state *tuiState, lines []string, width int, now time.Time) []string {
	if !state.refreshing || !state.motionEnabled || a.p.BodyBg == "" || a.p.KeyBg == "" {
		return lines
	}
	elapsed := max(0, now.Sub(state.refreshStartedAt))
	levels := []float64{0.25, 0.6, 1, 0.6, 0.25}
	shades := make([]string, len(levels))
	for i, level := range levels {
		shades[i] = lerpColor(a.p.BodyBg, a.p.KeyBg, level)
	}
	span := width + len(lines)/2 + len(levels)
	head := int(float64(elapsed%tuiRefreshSweep)/float64(tuiRefreshSweep)*float64(span)) - len(levels)
	for i, line := range lines {
		start := head - i/2
		lines[i] = tintColumns(line, func(col int) string {
			if k := col - start; k >= 0 && k < len(shades) {
				return shades[k]
			}
			return ""
		})
	}
	return lines
}

// lerpColor mixes two truecolor codes of the same kind (foreground or
// background). Anything else, a 256-colour code or none, gives the end colour.
func lerpColor(from, to string, t float64) string {
	var fr, fg, fb, tr, tg, tb int
	var fk, tk string
	if n, _ := fmt.Sscanf(strings.TrimPrefix(from, "\x1b["), "%2s;2;%d;%d;%dm", &fk, &fr, &fg, &fb); n != 4 {
		return to
	}
	if n, _ := fmt.Sscanf(strings.TrimPrefix(to, "\x1b["), "%2s;2;%d;%d;%dm", &tk, &tr, &tg, &tb); n != 4 || fk != tk {
		return to
	}
	if t >= 1 {
		return to
	}
	t = max(0, t)
	mix := func(a, b int) int { return a + int(math.Round(float64(b-a)*t)) }
	return fmt.Sprintf("\x1b[%s;2;%d;%d;%dm", tk, mix(fr, tr), mix(fg, tg), mix(fb, tb))
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
