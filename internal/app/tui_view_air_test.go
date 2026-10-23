package app

import (
	"strings"
	"testing"
)

// Every page keeps the dashboard's air: a blank row above and below its
// content, as the two columns either side do.
func TestEveryPageKeepsARowOfAirAboveAndBelow(t *testing.T) {
	a, state := listApp()
	state.settingsLoaded = true
	for _, view := range []tuiView{tuiViewProfiles, tuiViewHistory, tuiViewSettings, tuiViewDoctor, tuiViewBackup} {
		state.view = view
		for _, height := range []int{6, 12, 30} {
			body := a.tuiViewBody(state, 98, height)
			if len(body) != height {
				t.Fatalf("view %d height %d: %d body rows", view, height, len(body))
			}
			if first := strings.TrimSpace(stripEscapes(body[0])); first != "" {
				t.Fatalf("view %d height %d: content starts on the first body row: %q", view, height, first)
			}
			if last := strings.TrimSpace(stripEscapes(body[height-1])); last != "" {
				t.Fatalf("view %d height %d: content reaches the last body row: %q", view, height, last)
			}
			if title := stripEscapes(body[1]); !strings.HasPrefix(title, "  "+tuiViewTitle(view)) {
				t.Fatalf("view %d height %d: title is not on the second body row: %q", view, height, title)
			}
		}
	}
}
