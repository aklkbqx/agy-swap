package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

type Dimensions struct {
	Cols      int `json:"cols"`
	Rows      int `json:"rows"`
	InnerCols int `json:"innerCols"`
}

type Fixture struct {
	ID         string     `json:"id"`
	Dimensions Dimensions `json:"dimensions"`
	Layout     string     `json:"layout"`
	View       string     `json:"view"`
	Mode       string     `json:"mode"`
	Active     string     `json:"active,omitempty"`
	Selected   string     `json:"selected,omitempty"`
	Lines      []string   `json:"lines"`
	Plain      []string   `json:"plain"`
}

type FixtureOutput struct {
	Schema            int       `json:"schema"`
	Renderer          string    `json:"renderer"`
	Version           string    `json:"version"`
	SourceFingerprint string    `json:"sourceFingerprint"`
	Fixtures          []Fixture `json:"fixtures"`
}

func getFingerprint(t *testing.T) string {
	t.Helper()
	files := []string{
		"tui.go",
		"tui_actions.go",
		"tui_model.go",
		"tui_render.go",
		"tui_views.go",
		"display.go",
		"model.go",
		"settings.go",
		"doctor.go",
		"history.go",
		"backup.go",
		"quota.go",
	}
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("getFingerprint: failed to read %s: %v", f, err)
		}
		h.Write([]byte(f + "\n"))
		h.Write(b)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func makeSanitizedAccount(email, name string, gemini, thirdParty float64, reset time.Time, observed time.Time) Account {
	return Account{
		"email": email,
		"name":  name,
		"quota_snapshot": map[string]any{
			"observed_at": observed.Format(time.RFC3339),
			"tier":        map[string]any{"id": "pro-tier", "name": "Pro"},
			"groups": []any{
				map[string]any{
					"id":   "gemini",
					"name": "Gemini Models",
					"buckets": []any{
						map[string]any{
							"id":                 "gemini-weekly",
							"name":               "Weekly",
							"window":             "weekly",
							"remaining_fraction": gemini,
							"reset_at":           reset.Format(time.RFC3339),
						},
					},
				},
				map[string]any{
					"id":   "third_party",
					"name": "Third Party",
					"buckets": []any{
						map[string]any{
							"id":                 "3p-weekly",
							"name":               "Weekly",
							"window":             "weekly",
							"remaining_fraction": thirdParty,
							"reset_at":           reset.Format(time.RFC3339),
						},
					},
				},
			},
		},
	}
}

func buildFixtures(t *testing.T, a *Application, accounts *Accounts, settings AppSettings, doctorChecks []doctorCheck, frozenTime, renderNow time.Time) []Fixture {
	t.Helper()

	dims := []struct {
		layout    string
		innerCols int
		rows      int
		cols      int
	}{
		{"wide", 98, 24, 100},
		{"stacked", 78, 24, 80},
		{"compact", 46, 14, 48},
	}

	createBaseStateWithActiveAndAccounts := func(acc *Accounts, view tuiView, mode tuiMode, act, sel string) *tuiState {
		s := newTUIState(acc, act)
		s.active = act
		s.view = view
		s.mode = mode
		s.selectedEmail = sel
		s.settings = settings
		s.settingsLoaded = true
		s.profileNames = []string{"personal", "work"}
		s.profileIndex = 1
		s.history = []historyEvent{
			{
				Schema: historySchema,
				At:     frozenTime.Add(-10 * time.Minute).Format(time.RFC3339),
				Kind:   "switch",
				Email:  "alpha@example.invalid",
			},
			{
				Schema: historySchema,
				At:     frozenTime.Add(-2 * time.Hour).Format(time.RFC3339),
				Kind:   "quota-refresh",
				Email:  "beta@example.invalid",
			},
			{
				Schema: historySchema,
				At:     frozenTime.Add(-24 * time.Hour).Format(time.RFC3339),
				Kind:   "profile-apply",
				Email:  "gamma@example.invalid",
			},
		}
		s.backupPath = "agy-swap-backup.json"
		return s
	}

	resetAlpha := renderNow.Add(2*time.Hour + 30*time.Minute + 45*time.Second)
	resetBeta := renderNow.Add(45*time.Minute + 45*time.Second)
	resetGamma := renderNow.Add(15*time.Minute + 45*time.Second)

	// Post-delete alpha (remaining: beta, gamma)
	accountsPostAlpha := NewAccounts()
	accountsPostAlpha.Set("beta@example.invalid", makeSanitizedAccount("beta@example.invalid", "Beta User", 0.4, 0.4, resetBeta, frozenTime.Add(-2*time.Minute)))
	accountsPostAlpha.Set("gamma@example.invalid", makeSanitizedAccount("gamma@example.invalid", "Gamma User", 0.0, 0.0, resetGamma, frozenTime.Add(-10*time.Minute)))

	// Post-delete beta (remaining: alpha, gamma)
	accountsPostBeta := NewAccounts()
	accountsPostBeta.Set("alpha@example.invalid", makeSanitizedAccount("alpha@example.invalid", "Alpha User", 1.0, 0.85, resetAlpha, frozenTime.Add(-5*time.Minute)))
	accountsPostBeta.Set("gamma@example.invalid", makeSanitizedAccount("gamma@example.invalid", "Gamma User", 0.0, 0.0, resetGamma, frozenTime.Add(-10*time.Minute)))

	// Post-delete gamma (remaining: alpha, beta)
	accountsPostGamma := NewAccounts()
	accountsPostGamma.Set("alpha@example.invalid", makeSanitizedAccount("alpha@example.invalid", "Alpha User", 1.0, 0.85, resetAlpha, frozenTime.Add(-5*time.Minute)))
	accountsPostGamma.Set("beta@example.invalid", makeSanitizedAccount("beta@example.invalid", "Beta User", 0.4, 0.4, resetBeta, frozenTime.Add(-2*time.Minute)))

	// Post-delete alpha, beta (remaining: gamma)
	accountsPostAlphaBeta := NewAccounts()
	accountsPostAlphaBeta.Set("gamma@example.invalid", makeSanitizedAccount("gamma@example.invalid", "Gamma User", 0.0, 0.0, resetGamma, frozenTime.Add(-10*time.Minute)))

	// Post-delete alpha, gamma (remaining: beta)
	accountsPostAlphaGamma := NewAccounts()
	accountsPostAlphaGamma.Set("beta@example.invalid", makeSanitizedAccount("beta@example.invalid", "Beta User", 0.4, 0.4, resetBeta, frozenTime.Add(-2*time.Minute)))

	// Post-delete beta, gamma (remaining: alpha)
	accountsPostBetaGamma := NewAccounts()
	accountsPostBetaGamma.Set("alpha@example.invalid", makeSanitizedAccount("alpha@example.invalid", "Alpha User", 1.0, 0.85, resetAlpha, frozenTime.Add(-5*time.Minute)))

	// Post-delete all (remaining: none)
	accountsEmpty := NewAccounts()

	shortNames := map[string]string{
		"":                      "none",
		"alpha@example.invalid": "alpha",
		"beta@example.invalid":  "beta",
		"gamma@example.invalid": "gamma",
	}

	accountContexts := []struct {
		contextName string
		accounts    *Accounts
		remain      []string
	}{
		{
			contextName: "",
			accounts:    accounts,
			remain:      []string{"alpha@example.invalid", "beta@example.invalid", "gamma@example.invalid"},
		},
		{
			contextName: "accounts-no-alpha",
			accounts:    accountsPostAlpha,
			remain:      []string{"beta@example.invalid", "gamma@example.invalid"},
		},
		{
			contextName: "accounts-no-beta",
			accounts:    accountsPostBeta,
			remain:      []string{"alpha@example.invalid", "gamma@example.invalid"},
		},
		{
			contextName: "accounts-no-gamma",
			accounts:    accountsPostGamma,
			remain:      []string{"alpha@example.invalid", "beta@example.invalid"},
		},
		{
			contextName: "accounts-only-gamma",
			accounts:    accountsPostAlphaBeta,
			remain:      []string{"gamma@example.invalid"},
		},
		{
			contextName: "accounts-only-beta",
			accounts:    accountsPostAlphaGamma,
			remain:      []string{"beta@example.invalid"},
		},
		{
			contextName: "accounts-only-alpha",
			accounts:    accountsPostBetaGamma,
			remain:      []string{"alpha@example.invalid"},
		},
		{
			contextName: "accounts-empty",
			accounts:    accountsEmpty,
			remain:      []string{},
		},
	}

	settingsPostRemoveWork := settings
	settingsPostRemoveWork.Profiles = map[string]Profile{
		"personal": settings.Profiles["personal"],
	}

	settingsPostRemovePersonal := settings
	settingsPostRemovePersonal.Profiles = map[string]Profile{
		"work": settings.Profiles["work"],
	}

	settingsPostRemoveAll := settings
	settingsPostRemoveAll.Profiles = map[string]Profile{}
	suffixFor := func(base, ctxName, actShort, selShort string) string {
		s := base
		if ctxName != "" {
			s += "." + ctxName
		}
		if actShort != "" {
			s += ".active-" + actShort
		}
		if selShort != "" {
			s += ".selected-" + selShort
		}
		return s
	}

	type scenario struct {
		suffix   string
		view     string
		mode     string
		active   string
		selected string
		setup    func(dim struct {
			layout    string
			innerCols int
			rows      int
			cols      int
		}) *tuiState
	}

	var scenarios []scenario

	viewsList := []struct {
		view tuiView
		name string
	}{
		{tuiViewDashboard, "dashboard"},
		{tuiViewQuota, "quota"},
		{tuiViewProfiles, "profiles"},
		{tuiViewHistory, "history"},
		{tuiViewSettings, "settings"},
		{tuiViewDoctor, "doctor"},
		{tuiViewBackup, "backup"},
	}

	editOps := []struct {
		opName  string
		keyName string
	}{
		{"char-x", "x"},
		{"backspace", "backspace"},
		{"ctrl-u", "ctrl-u"},
		{"ctrl-w", "ctrl-w"},
	}

	searchQueries := []string{
		"", "a", "al", "alp", "alpha", "b", "be", "bet", "beta", "g", "ga", "gam", "gamma", "z", "zz",
	}

	paletteQueries := []string{
		"", "p", "pr", "pro", "prof", "profile",
	}

	// Build scenarios for all account contexts and valid active/selected accounts
	for _, ac := range accountContexts {
		currentAC := ac
		activeOptions := append([]string{""}, currentAC.remain...)

		for _, act := range activeOptions {
			actShort := shortNames[act]
			currentAct := act
			firstRemain := ""
			if len(currentAC.remain) > 0 {
				firstRemain = currentAC.remain[0]
			}

			// 1. View-wide modes across all 7 views (Help, Palette, Search, Confirm-Update)
			for _, vInfo := range viewsList {
				vView := vInfo.view
				vLower := vInfo.name
				vCapital := strings.Title(vLower)

				// A. Help overlay over current view
				if vLower == "dashboard" || vLower == "quota" {
					if len(currentAC.remain) == 0 {
						scenarios = append(scenarios, scenario{
							suffix:   suffixFor(fmt.Sprintf("%s.help", vLower), currentAC.contextName, actShort, ""),
							view:     vCapital,
							mode:     "help",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								return createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiHelp, currentAct, "")
							},
						})
					} else {
						for _, sel := range currentAC.remain {
							selShort := shortNames[sel]
							currentSel := sel
							scenarios = append(scenarios, scenario{
								suffix:   suffixFor(fmt.Sprintf("%s.help", vLower), currentAC.contextName, actShort, selShort),
								view:     vCapital,
								mode:     "help",
								active:   currentAct,
								selected: currentSel,
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									return createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiHelp, currentAct, currentSel)
								},
							})
						}
					}
				} else {
					scenarios = append(scenarios, scenario{
						suffix:   suffixFor(fmt.Sprintf("%s.help", vLower), currentAC.contextName, actShort, ""),
						view:     vCapital,
						mode:     "help",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiHelp, currentAct, "")
							if vLower == "doctor" {
								s.doctorChecks = doctorChecks
							}
							return s
						},
					})
				}

				// B. Palette: queries & enabled indices
				for _, pq := range paletteQueries {
					palQ := pq
					qSlug := palQ
					if qSlug == "" {
						qSlug = "empty"
					}

					// Calculate actions for this view
					tempS := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, firstRemain)
					if vLower == "doctor" {
						tempS.doctorChecks = doctorChecks
					}
					tempS.paletteQuery = palQ
					actions := tempS.paletteActions()

					for aIdx, actItem := range actions {
						if !actItem.Enabled {
							continue
						}
						itemIndex := aIdx

						if vLower == "dashboard" || vLower == "quota" {
							if len(currentAC.remain) == 0 {
								scenarios = append(scenarios, scenario{
									suffix:   suffixFor(fmt.Sprintf("%s.palette.query-%s.index-%d", vLower, qSlug, itemIndex), currentAC.contextName, actShort, ""),
									view:     vCapital,
									mode:     "palette",
									active:   currentAct,
									selected: "",
									setup: func(d struct {
										layout                string
										innerCols, rows, cols int
									}) *tuiState {
										s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, "")
										s.paletteQuery = palQ
										s.paletteIndex = itemIndex
										return s
									},
								})
							} else {
								for _, sel := range currentAC.remain {
									selShort := shortNames[sel]
									currentSel := sel
									scenarios = append(scenarios, scenario{
										suffix:   suffixFor(fmt.Sprintf("%s.palette.query-%s.index-%d", vLower, qSlug, itemIndex), currentAC.contextName, actShort, selShort),
										view:     vCapital,
										mode:     "palette",
										active:   currentAct,
										selected: currentSel,
										setup: func(d struct {
											layout                string
											innerCols, rows, cols int
										}) *tuiState {
											s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, currentSel)
											s.paletteQuery = palQ
											s.paletteIndex = itemIndex
											return s
										},
									})
								}
							}
						} else {
							scenarios = append(scenarios, scenario{
								suffix:   suffixFor(fmt.Sprintf("%s.palette.query-%s.index-%d", vLower, qSlug, itemIndex), currentAC.contextName, actShort, ""),
								view:     vCapital,
								mode:     "palette",
								active:   currentAct,
								selected: "",
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, "")
									s.paletteQuery = palQ
									s.paletteIndex = itemIndex
									if vLower == "doctor" {
										s.doctorChecks = doctorChecks
									}
									return s
								},
							})
						}
					}
				}

				// Palette aliases (unfiltered & filtered)
				if vLower == "dashboard" || vLower == "quota" {
					if len(currentAC.remain) == 0 {
						scenarios = append(scenarios,
							scenario{
								suffix:   suffixFor(fmt.Sprintf("%s.palette.unfiltered", vLower), currentAC.contextName, actShort, ""),
								view:     vCapital,
								mode:     "palette",
								active:   currentAct,
								selected: "",
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, "")
									s.paletteQuery = ""
									s.paletteIndex = 0
									return s
								},
							},
							scenario{
								suffix:   suffixFor(fmt.Sprintf("%s.palette.filtered", vLower), currentAC.contextName, actShort, ""),
								view:     vCapital,
								mode:     "palette",
								active:   currentAct,
								selected: "",
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, "")
									s.paletteQuery = "switch"
									s.paletteIndex = 0
									return s
								},
							},
						)
					} else {
						for _, sel := range currentAC.remain {
							selShort := shortNames[sel]
							currentSel := sel
							scenarios = append(scenarios,
								scenario{
									suffix:   suffixFor(fmt.Sprintf("%s.palette.unfiltered", vLower), currentAC.contextName, actShort, selShort),
									view:     vCapital,
									mode:     "palette",
									active:   currentAct,
									selected: currentSel,
									setup: func(d struct {
										layout                string
										innerCols, rows, cols int
									}) *tuiState {
										s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, currentSel)
										s.paletteQuery = ""
										s.paletteIndex = 0
										return s
									},
								},
								scenario{
									suffix:   suffixFor(fmt.Sprintf("%s.palette.filtered", vLower), currentAC.contextName, actShort, selShort),
									view:     vCapital,
									mode:     "palette",
									active:   currentAct,
									selected: currentSel,
									setup: func(d struct {
										layout                string
										innerCols, rows, cols int
									}) *tuiState {
										s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, currentSel)
										s.paletteQuery = "switch"
										s.paletteIndex = 0
										return s
									},
								},
							)
						}
					}
				} else {
					scenarios = append(scenarios,
						scenario{
							suffix:   suffixFor(fmt.Sprintf("%s.palette.unfiltered", vLower), currentAC.contextName, actShort, ""),
							view:     vCapital,
							mode:     "palette",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, "")
								s.paletteQuery = ""
								s.paletteIndex = 0
								if vLower == "doctor" {
									s.doctorChecks = doctorChecks
								}
								return s
							},
						},
						scenario{
							suffix:   suffixFor(fmt.Sprintf("%s.palette.filtered", vLower), currentAC.contextName, actShort, ""),
							view:     vCapital,
							mode:     "palette",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiPalette, currentAct, "")
								s.paletteQuery = "switch"
								s.paletteIndex = 0
								if vLower == "doctor" {
									s.doctorChecks = doctorChecks
								}
								return s
							},
						},
					)
				}

				// C. Search queries
				for _, sq := range searchQueries {
					searchQ := sq
					qSlug := searchQ
					if qSlug == "" {
						qSlug = "empty"
					}

					if vLower == "dashboard" || vLower == "quota" {
						if len(currentAC.remain) == 0 {
							scenarios = append(scenarios, scenario{
								suffix:   suffixFor(fmt.Sprintf("%s.search.query-%s", vLower, qSlug), currentAC.contextName, actShort, ""),
								view:     vCapital,
								mode:     "search",
								active:   currentAct,
								selected: "",
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiSearch, currentAct, "")
									s.search = searchQ
									s.clampSelection()
									return s
								},
							})
						} else {
							for _, sel := range currentAC.remain {
								selShort := shortNames[sel]
								currentSel := sel
								scenarios = append(scenarios, scenario{
									suffix:   suffixFor(fmt.Sprintf("%s.search.query-%s", vLower, qSlug), currentAC.contextName, actShort, selShort),
									view:     vCapital,
									mode:     "search",
									active:   currentAct,
									selected: currentSel,
									setup: func(d struct {
										layout                string
										innerCols, rows, cols int
									}) *tuiState {
										s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiSearch, currentAct, currentSel)
										s.search = searchQ
										s.clampSelection()
										return s
									},
								})
							}
						}
					} else {
						scenarios = append(scenarios, scenario{
							suffix:   suffixFor(fmt.Sprintf("%s.search.query-%s", vLower, qSlug), currentAC.contextName, actShort, ""),
							view:     vCapital,
							mode:     "search",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiSearch, currentAct, "")
								s.search = searchQ
								if vLower == "doctor" {
									s.doctorChecks = doctorChecks
								}
								return s
							},
						})
					}
				}

				// Search aliases (match & no-match)
				if vLower == "dashboard" || vLower == "quota" {
					if len(currentAC.remain) == 0 {
						scenarios = append(scenarios,
							scenario{
								suffix:   suffixFor(fmt.Sprintf("%s.search.match", vLower), currentAC.contextName, actShort, ""),
								view:     vCapital,
								mode:     "search",
								active:   currentAct,
								selected: "",
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiSearch, currentAct, "")
									s.search = "alpha"
									s.clampSelection()
									return s
								},
							},
							scenario{
								suffix:   suffixFor(fmt.Sprintf("%s.search.no-match", vLower), currentAC.contextName, actShort, ""),
								view:     vCapital,
								mode:     "search",
								active:   currentAct,
								selected: "",
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiSearch, currentAct, "")
									s.search = "nonexistent"
									s.clampSelection()
									return s
								},
							},
						)
					} else {
						for _, sel := range currentAC.remain {
							selShort := shortNames[sel]
							currentSel := sel
							scenarios = append(scenarios, scenario{
								suffix:   suffixFor(fmt.Sprintf("%s.search.match", vLower), currentAC.contextName, actShort, selShort),
								view:     vCapital,
								mode:     "search",
								active:   currentAct,
								selected: currentSel,
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiSearch, currentAct, currentSel)
									s.search = selShort
									s.clampSelection()
									return s
								},
							})
						}
						scenarios = append(scenarios, scenario{
							suffix:   suffixFor(fmt.Sprintf("%s.search.no-match", vLower), currentAC.contextName, actShort, ""),
							view:     vCapital,
							mode:     "search",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiSearch, currentAct, "")
								s.search = "nonexistent"
								s.clampSelection()
								return s
							},
						})
					}
				} else {
					scenarios = append(scenarios,
						scenario{
							suffix:   suffixFor(fmt.Sprintf("%s.search.match", vLower), currentAC.contextName, actShort, ""),
							view:     vCapital,
							mode:     "search",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiSearch, currentAct, "")
								s.search = "alpha"
								if vLower == "doctor" {
									s.doctorChecks = doctorChecks
								}
								return s
							},
						},
						scenario{
							suffix:   suffixFor(fmt.Sprintf("%s.search.no-match", vLower), currentAC.contextName, actShort, ""),
							view:     vCapital,
							mode:     "search",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiSearch, currentAct, "")
								s.search = "nonexistent"
								if vLower == "doctor" {
									s.doctorChecks = doctorChecks
								}
								return s
							},
						},
					)
				}

				// D. Confirm update over current view
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor(fmt.Sprintf("%s.confirm-update", vLower), currentAC.contextName, actShort, ""),
					view:     vCapital,
					mode:     "confirm_action",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, vView, tuiConfirmAction, currentAct, "")
						s.confirmTitle = "Download and install the latest release"
						s.confirmAction = "update"
						if vLower == "doctor" {
							s.doctorChecks = doctorChecks
						}
						return s
					},
				})
			}

			// 2. Dashboard and Quota ready, refreshing, tags, and confirm-delete
			if len(currentAC.remain) == 0 {
				// Dashboard ready
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor("dashboard.ready", currentAC.contextName, actShort, ""),
					view:     "Dashboard",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						return createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDashboard, tuiBrowse, currentAct, "")
					},
				})

				// Quota ready
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor("quota.ready", currentAC.contextName, actShort, ""),
					view:     "Quota",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						return createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewQuota, tuiBrowse, currentAct, "")
					},
				})

				// Refreshing Dashboard & Quota
				scenarios = append(scenarios,
					scenario{
						suffix:   suffixFor("dashboard.refreshing", currentAC.contextName, actShort, ""),
						view:     "Dashboard",
						mode:     "refreshing",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDashboard, tuiBrowse, currentAct, "")
							s.refreshing = true
							s.message = "Refreshing quota…"
							s.messageType = "info"
							return s
						},
					},
					scenario{
						suffix:   suffixFor("quota.refreshing", currentAC.contextName, actShort, ""),
						view:     "Quota",
						mode:     "refreshing",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewQuota, tuiBrowse, currentAct, "")
							s.refreshing = true
							s.message = "Refreshing quota…"
							s.messageType = "info"
							return s
						},
					},
				)
			} else {
				for _, sel := range currentAC.remain {
					selShort := shortNames[sel]
					currentSel := sel

					// Dashboard ready
					scenarios = append(scenarios, scenario{
						suffix:   suffixFor("dashboard.ready", currentAC.contextName, actShort, selShort),
						view:     "Dashboard",
						mode:     "ready",
						active:   currentAct,
						selected: currentSel,
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							return createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDashboard, tuiBrowse, currentAct, currentSel)
						},
					})

					// Quota ready
					scenarios = append(scenarios, scenario{
						suffix:   suffixFor("quota.ready", currentAC.contextName, actShort, selShort),
						view:     "Quota",
						mode:     "ready",
						active:   currentAct,
						selected: currentSel,
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							return createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewQuota, tuiBrowse, currentAct, currentSel)
						},
					})

					// Refreshing Dashboard & Quota
					scenarios = append(scenarios,
						scenario{
							suffix:   suffixFor("dashboard.refreshing", currentAC.contextName, actShort, selShort),
							view:     "Dashboard",
							mode:     "refreshing",
							active:   currentAct,
							selected: currentSel,
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDashboard, tuiBrowse, currentAct, currentSel)
								s.refreshing = true
								s.message = "Refreshing quota…"
								s.messageType = "info"
								return s
							},
						},
						scenario{
							suffix:   suffixFor("quota.refreshing", currentAC.contextName, actShort, selShort),
							view:     "Quota",
							mode:     "refreshing",
							active:   currentAct,
							selected: currentSel,
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewQuota, tuiBrowse, currentAct, currentSel)
								s.refreshing = true
								s.message = "Refreshing quota…"
								s.messageType = "info"
								return s
							},
						},
					)

					// Confirm delete on Dashboard & Quota
					scenarios = append(scenarios,
						scenario{
							suffix:   suffixFor(fmt.Sprintf("dashboard.confirm-delete.account-%s", selShort), currentAC.contextName, actShort, ""),
							view:     "Dashboard",
							mode:     "confirm_delete",
							active:   currentAct,
							selected: currentSel,
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDashboard, tuiConfirmDelete, currentAct, currentSel)
								s.confirmEmail = currentSel
								return s
							},
						},
						scenario{
							suffix:   suffixFor(fmt.Sprintf("quota.confirm-delete.account-%s", selShort), currentAC.contextName, actShort, ""),
							view:     "Quota",
							mode:     "confirm_delete",
							active:   currentAct,
							selected: currentSel,
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewQuota, tuiConfirmDelete, currentAct, currentSel)
								s.confirmEmail = currentSel
								return s
							},
						},
					)
				}
			}

			// Tags forms & edit operations
			if len(currentAC.remain) > 0 {
				scenarios = append(scenarios,
					scenario{
						suffix:   suffixFor("dashboard.form.tags.field-0", currentAC.contextName, actShort, ""),
						view:     "Dashboard",
						mode:     "form",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDashboard, tuiBrowse, currentAct, firstRemain)
							a.beginTUIForm(s, "tags")
							if s.form != nil {

								s.form.Index = 0

							}
							s.active = currentAct
							return s
						},
					},
					scenario{
						suffix:   suffixFor("quota.form.tags.field-0", currentAC.contextName, actShort, ""),
						view:     "Quota",
						mode:     "form",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewQuota, tuiBrowse, currentAct, firstRemain)
							a.beginTUIForm(s, "tags")
							if s.form != nil {

								s.form.Index = 0

							}
							s.active = currentAct
							return s
						},
					},
				)
				for _, ed := range editOps {
					currentEd := ed
					scenarios = append(scenarios,
						scenario{
							suffix:   suffixFor(fmt.Sprintf("dashboard.form.tags.field-0.edit-%s", currentEd.opName), currentAC.contextName, actShort, ""),
							view:     "Dashboard",
							mode:     "form",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDashboard, tuiBrowse, currentAct, firstRemain)
								a.beginTUIForm(s, "tags")
								if s.form != nil {

									s.form.Index = 0

								}
								if s.form != nil {

									s.formKey(currentEd.keyName)

								}
								s.active = currentAct
								return s
							},
						},
						scenario{
							suffix:   suffixFor(fmt.Sprintf("quota.form.tags.field-0.edit-%s", currentEd.opName), currentAC.contextName, actShort, ""),
							view:     "Quota",
							mode:     "form",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewQuota, tuiBrowse, currentAct, firstRemain)
								a.beginTUIForm(s, "tags")
								if s.form != nil {

									s.form.Index = 0

								}
								if s.form != nil {

									s.formKey(currentEd.keyName)

								}
								s.active = currentAct
								return s
							},
						},
					)
				}
			}

			// 3. Profiles
			scenarios = append(scenarios,
				scenario{
					suffix:   suffixFor("profiles.ready.profile-0", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.profileIndex = 0
						return s
					},
				},
				scenario{
					suffix:   suffixFor("profiles.ready.profile-1", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.profileIndex = 1
						return s
					},
				},
				scenario{
					suffix:   suffixFor("profiles.confirm-remove", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "confirm_action",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiConfirmAction, currentAct, "")
						s.confirmTitle = "Remove selected profile"
						s.confirmAction = "profile-remove"
						return s
					},
				},
				scenario{
					suffix:   suffixFor("profiles.post-remove.work", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.settings = settingsPostRemoveWork
						s.profileNames = []string{"personal"}
						s.profileIndex = 0
						s.message = "Removed profile work"
						s.messageType = "success"
						return s
					},
				},
				scenario{
					suffix:   suffixFor("profiles.post-remove.personal", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.settings = settingsPostRemovePersonal
						s.profileNames = []string{"work"}
						s.profileIndex = 0
						s.message = "Removed profile personal"
						s.messageType = "success"
						return s
					},
				},
				scenario{
					suffix:   suffixFor("profiles.post-remove", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.settings = settingsPostRemoveWork
						s.profileNames = []string{"personal"}
						s.profileIndex = 0
						s.message = "Removed profile work"
						s.messageType = "success"
						return s
					},
				},
				scenario{
					suffix:   suffixFor("profiles.post-remove.personal.profiles-empty", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.settings = settingsPostRemoveAll
						s.profileNames = []string{}
						s.profileIndex = 0
						s.message = "Removed profile personal"
						s.messageType = "success"
						return s
					},
				},
				scenario{
					suffix:   suffixFor("profiles.post-remove.work.profiles-empty", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.settings = settingsPostRemoveAll
						s.profileNames = []string{}
						s.profileIndex = 0
						s.message = "Removed profile work"
						s.messageType = "success"
						return s
					},
				},
				scenario{
					suffix:   suffixFor("profiles.ready.profiles-no-work", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.settings = settingsPostRemoveWork
						s.profileNames = []string{"personal"}
						s.profileIndex = 0
						return s
					},
				},
				scenario{

					suffix:   suffixFor("profiles.ready.profiles-empty", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.settings.Profiles = map[string]Profile{}
						s.profileNames = []string{}
						s.profileIndex = 0
						return s
					},
				},
				scenario{
					suffix:   suffixFor("profiles.ready.profiles-no-personal", currentAC.contextName, actShort, ""),
					view:     "Profiles",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
						s.settings = settingsPostRemovePersonal
						s.profileNames = []string{"work"}
						s.profileIndex = 0
						return s
					},
				},
			)

			// Profile edit for profile 0 (personal) and profile 1 (work)
			for fIdx := 0; fIdx < 4; fIdx++ {
				field := fIdx
				scenarios = append(scenarios,
					scenario{
						suffix:   suffixFor(fmt.Sprintf("profiles.form.profile-create.field-%d", field), currentAC.contextName, actShort, ""),
						view:     "Profiles",
						mode:     "form",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
							a.beginTUIForm(s, "profile-create")
							if s.form != nil {

								s.form.Index = field

							}
							s.active = currentAct
							return s
						},
					},
					scenario{
						suffix:   suffixFor(fmt.Sprintf("profiles.form.profile-edit.profile-0.field-%d", field), currentAC.contextName, actShort, ""),
						view:     "Profiles",
						mode:     "form",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
							s.profileIndex = 0
							a.beginTUIForm(s, "profile-edit")
							if s.form != nil {

								s.form.Index = field

							}
							s.active = currentAct
							return s
						},
					},
					scenario{
						suffix:   suffixFor(fmt.Sprintf("profiles.form.profile-edit.profile-1.field-%d", field), currentAC.contextName, actShort, ""),
						view:     "Profiles",
						mode:     "form",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
							s.profileIndex = 1
							a.beginTUIForm(s, "profile-edit")
							if s.form != nil {

								s.form.Index = field

							}
							s.active = currentAct
							return s
						},
					},
					scenario{
						suffix:   suffixFor(fmt.Sprintf("profiles.form.profile-edit.field-%d", field), currentAC.contextName, actShort, ""),
						view:     "Profiles",
						mode:     "form",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
							s.profileIndex = 1
							a.beginTUIForm(s, "profile-edit")
							if s.form != nil {

								s.form.Index = field

							}
							s.active = currentAct
							return s
						},
					},
				)

				// Form edit operations on text fields (field 0, 1, 3)
				if field != 2 {
					for _, ed := range editOps {
						currentEd := ed
						scenarios = append(scenarios,
							scenario{
								suffix:   suffixFor(fmt.Sprintf("profiles.form.profile-edit.profile-0.field-%d.edit-%s", field, currentEd.opName), currentAC.contextName, actShort, ""),
								view:     "Profiles",
								mode:     "form",
								active:   currentAct,
								selected: "",
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
									s.profileIndex = 0
									a.beginTUIForm(s, "profile-edit")
									if s.form != nil {

										s.form.Index = field

									}
									if s.form != nil {

										s.formKey(currentEd.keyName)

									}
									s.active = currentAct
									return s
								},
							},
							scenario{
								suffix:   suffixFor(fmt.Sprintf("profiles.form.profile-edit.profile-1.field-%d.edit-%s", field, currentEd.opName), currentAC.contextName, actShort, ""),
								view:     "Profiles",
								mode:     "form",
								active:   currentAct,
								selected: "",
								setup: func(d struct {
									layout                string
									innerCols, rows, cols int
								}) *tuiState {
									s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewProfiles, tuiBrowse, currentAct, "")
									s.profileIndex = 1
									a.beginTUIForm(s, "profile-edit")
									if s.form != nil {

										s.form.Index = field

									}
									if s.form != nil {

										s.formKey(currentEd.keyName)

									}
									s.active = currentAct
									return s
								},
							},
						)
					}
				}
			}

			// 4. History
			for hIdx := 0; hIdx < 3; hIdx++ {
				hIndex := hIdx
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor(fmt.Sprintf("history.ready.index-%d", hIndex), currentAC.contextName, actShort, ""),
					view:     "History",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewHistory, tuiBrowse, currentAct, "")
						s.historyIndex = hIndex
						return s
					},
				})
			}
			scenarios = append(scenarios,
				scenario{
					suffix:   suffixFor("history.post-clear", currentAC.contextName, actShort, ""),
					view:     "History",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewHistory, tuiBrowse, currentAct, "")
						s.history = nil
						s.historyIndex = 0
						s.message = "History cleared"
						s.messageType = "success"
						return s
					},
				},
			)
			scenarios = append(scenarios,
				scenario{
					suffix:   suffixFor("history.confirm-action", currentAC.contextName, actShort, ""),
					view:     "History",
					mode:     "confirm_action",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewHistory, tuiConfirmAction, currentAct, "")
						s.confirmTitle = "Clear local history"
						s.confirmAction = "history-clear"
						return s
					},
				},
				scenario{
					suffix:   suffixFor("history.form.history-export.field-0", currentAC.contextName, actShort, ""),
					view:     "History",
					mode:     "form",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewHistory, tuiBrowse, currentAct, "")
						a.beginTUIForm(s, "history-export")
						if s.form != nil {

							s.form.Index = 0

						}
						s.active = currentAct
						return s
					},
				},
			)
			for _, ed := range editOps {
				currentEd := ed
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor(fmt.Sprintf("history.form.history-export.field-0.edit-%s", currentEd.opName), currentAC.contextName, actShort, ""),
					view:     "History",
					mode:     "form",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewHistory, tuiBrowse, currentAct, "")
						a.beginTUIForm(s, "history-export")
						if s.form != nil {

							s.form.Index = 0

						}
						if s.form != nil {

							s.formKey(currentEd.keyName)

						}
						s.active = currentAct
						return s
					},
				})
			}

			// 5. Settings
			scenarios = append(scenarios, scenario{
				suffix:   suffixFor("settings.ready", currentAC.contextName, actShort, ""),
				view:     "Settings",
				mode:     "ready",
				active:   currentAct,
				selected: "",
				setup: func(d struct {
					layout                string
					innerCols, rows, cols int
				}) *tuiState {
					return createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewSettings, tuiBrowse, currentAct, "")
				},
			})
			for fIdx := 0; fIdx < 13; fIdx++ {
				field := fIdx
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor(fmt.Sprintf("settings.form.settings.field-%d", field), currentAC.contextName, actShort, ""),
					view:     "Settings",
					mode:     "form",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewSettings, tuiBrowse, currentAct, "")
						a.beginTUIForm(s, "settings")
						if s.form != nil {

							s.form.Index = field

						}
						s.active = currentAct
						return s
					},
				})
				if field == 2 || field == 5 || field == 9 || field == 11 || field == 12 {
					for _, ed := range editOps {
						currentEd := ed
						scenarios = append(scenarios, scenario{
							suffix:   suffixFor(fmt.Sprintf("settings.form.settings.field-%d.edit-%s", field, currentEd.opName), currentAC.contextName, actShort, ""),
							view:     "Settings",
							mode:     "form",
							active:   currentAct,
							selected: "",
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewSettings, tuiBrowse, currentAct, "")
								a.beginTUIForm(s, "settings")
								if s.form != nil {

									s.form.Index = field

								}
								if s.form != nil {

									s.formKey(currentEd.keyName)

								}
								s.active = currentAct
								return s
							},
						})
					}
				}
			}
			for fIdx := 0; fIdx < 2; fIdx++ {
				field := fIdx
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor(fmt.Sprintf("settings.form.alias.field-%d", field), currentAC.contextName, actShort, ""),
					view:     "Settings",
					mode:     "form",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewSettings, tuiBrowse, currentAct, "")
						a.beginTUIForm(s, "alias")
						if s.form != nil {

							s.form.Index = field

						}
						s.active = currentAct
						return s
					},
				})
			}
			for fIdx := 0; fIdx < 3; fIdx++ {
				field := fIdx
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor(fmt.Sprintf("settings.form.binding.field-%d", field), currentAC.contextName, actShort, ""),
					view:     "Settings",
					mode:     "form",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewSettings, tuiBrowse, currentAct, "")
						a.beginTUIForm(s, "binding")
						if s.form != nil {

							s.form.Index = field

						}
						s.active = currentAct
						return s
					},
				})
			}
			for fIdx := 0; fIdx < 2; fIdx++ {
				field := fIdx
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor(fmt.Sprintf("settings.form.target.field-%d", field), currentAC.contextName, actShort, ""),
					view:     "Settings",
					mode:     "form",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewSettings, tuiBrowse, currentAct, "")
						a.beginTUIForm(s, "target")
						if s.form != nil {

							s.form.Index = field

						}
						s.active = currentAct
						return s
					},
				})
			}

			// 6. Doctor
			scenarios = append(scenarios,
				scenario{
					suffix:   suffixFor("doctor.ready", currentAC.contextName, actShort, ""),
					view:     "Doctor",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDoctor, tuiBrowse, currentAct, "")
						s.doctorChecks = doctorChecks
						return s
					},
				},
				scenario{
					suffix:   suffixFor("doctor.running", currentAC.contextName, actShort, ""),
					view:     "Doctor",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDoctor, tuiBrowse, currentAct, "")
						s.doctorChecks = doctorChecks[:2]
						s.job = &tuiJobState{Label: "Checking endpoints", Done: false}
						return s
					},
				},
				scenario{
					suffix:   suffixFor("doctor.completed", currentAC.contextName, actShort, ""),
					view:     "Doctor",
					mode:     "ready",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewDoctor, tuiBrowse, currentAct, "")
						s.doctorChecks = doctorChecks
						s.doctorHealthy = true
						s.message = "Health check completed: all checks healthy"
						s.messageType = "success"
						return s
					},
				},
			)

			// 7. Backup
			scenarios = append(scenarios, scenario{
				suffix:   suffixFor("backup.ready", currentAC.contextName, actShort, ""),
				view:     "Backup",
				mode:     "ready",
				active:   currentAct,
				selected: "",
				setup: func(d struct {
					layout                string
					innerCols, rows, cols int
				}) *tuiState {
					return createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewBackup, tuiBrowse, currentAct, "")
				},
			})
			for fIdx := 0; fIdx < 3; fIdx++ {
				field := fIdx
				scenarios = append(scenarios,
					scenario{
						suffix:   suffixFor(fmt.Sprintf("backup.form.backup-export.field-%d", field), currentAC.contextName, actShort, ""),
						view:     "Backup",
						mode:     "form",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewBackup, tuiBrowse, currentAct, "")
							a.beginTUIForm(s, "backup-export")
							if s.form != nil {

								s.form.Index = field

							}
							s.active = currentAct
							return s
						},
					},
					scenario{
						suffix:   suffixFor(fmt.Sprintf("backup.form.backup-import.field-%d", field), currentAC.contextName, actShort, ""),
						view:     "Backup",
						mode:     "form",
						active:   currentAct,
						selected: "",
						setup: func(d struct {
							layout                string
							innerCols, rows, cols int
						}) *tuiState {
							s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewBackup, tuiBrowse, currentAct, "")
							a.beginTUIForm(s, "backup-import")
							if s.form != nil {

								s.form.Index = field

							}
							s.active = currentAct
							return s
						},
					},
				)
			}
			for fIdx := 0; fIdx < 2; fIdx++ {
				field := fIdx
				scenarios = append(scenarios, scenario{
					suffix:   suffixFor(fmt.Sprintf("backup.form.backup-verify.field-%d", field), currentAC.contextName, actShort, ""),
					view:     "Backup",
					mode:     "form",
					active:   currentAct,
					selected: "",
					setup: func(d struct {
						layout                string
						innerCols, rows, cols int
					}) *tuiState {
						s := createBaseStateWithActiveAndAccounts(currentAC.accounts, tuiViewBackup, tuiBrowse, currentAct, "")
						a.beginTUIForm(s, "backup-verify")
						if s.form != nil {

							s.form.Index = field

						}
						s.active = currentAct
						return s
					},
				})
			}
		}
	}

	// 8. Post-Delete combinations (all 9 active × deleted combinations for 3-account store)
		// 8. Post-Delete combinations (dynamically for all contexts)
	seenDeleteSuffixes := make(map[string]bool)
	for _, currentAC := range accountContexts {
		if len(currentAC.remain) == 0 {
			continue
		}
		actives := append(currentAC.remain, "")
		for _, activeEmail := range actives {
			for _, deletedEmail := range currentAC.remain {
				resultingAccs := NewAccounts()
				for _, k := range currentAC.accounts.Order {
					if k != deletedEmail {
						resultingAccs.Set(k, currentAC.accounts.ByEmail[k])
					}
				}
				resultingAct := activeEmail
				if resultingAct == deletedEmail {
					resultingAct = ""
				}

				resultingSel := ""
				if resultingAccs.Len() > 0 {
					if len(resultingAccs.Order) > 0 {
						resultingSel = resultingAccs.Order[0]
					}
				}

				delShort := shortNames[deletedEmail]
				actShort := "none"
				if resultingAct != "" {
					actShort = shortNames[resultingAct]
				}

				for _, v := range []struct {
					view  tuiView
					vName string
					vCap  string
				}{
					{tuiViewDashboard, "dashboard", "Dashboard"},
					{tuiViewQuota, "quota", "Quota"},
				} {
					currentV := v
					suf := suffixFor(fmt.Sprintf("%s.post-delete.deleted-%s", currentV.vName, delShort), currentAC.contextName, actShort, "")
					if seenDeleteSuffixes[suf] {
						continue
					}
					seenDeleteSuffixes[suf] = true
					
					scenarios = append(scenarios, scenario{
						suffix:   suf,
						view:     currentV.vCap,
						mode:     "ready",
						active:   resultingAct,
						selected: resultingSel,
						setup: func(d struct{ layout string; innerCols, rows, cols int }) *tuiState {
							s := createBaseStateWithActiveAndAccounts(resultingAccs, currentV.view, tuiBrowse, resultingAct, resultingSel)
							s.message = fmt.Sprintf("Removed account %s", deletedEmail)
							s.messageType = "success"
							return s
						},
					})
				}
			}
		}
	}

	// Canonical post-delete aliases
	scenarios = append(scenarios,
		scenario{
			suffix:   "dashboard.post-delete.alpha",
			view:     "Dashboard",
			mode:     "ready",
			active:   "",
			selected: "beta@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accountsPostAlpha, tuiViewDashboard, tuiBrowse, "", "beta@example.invalid")
				s.message = "Removed account alpha@example.invalid"
				s.messageType = "success"
				return s
			},
		},
		scenario{
			suffix:   "dashboard.post-delete.beta",
			view:     "Dashboard",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accountsPostBeta, tuiViewDashboard, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
				s.message = "Removed account beta@example.invalid"
				s.messageType = "success"
				return s
			},
		},
		scenario{
			suffix:   "dashboard.post-delete.gamma",
			view:     "Dashboard",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accountsPostGamma, tuiViewDashboard, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
				s.message = "Removed account gamma@example.invalid"
				s.messageType = "success"
				return s
			},
		},
		scenario{
			suffix:   "quota.post-delete.alpha",
			view:     "Quota",
			mode:     "ready",
			active:   "",
			selected: "beta@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accountsPostAlpha, tuiViewQuota, tuiBrowse, "", "beta@example.invalid")
				s.message = "Removed account alpha@example.invalid"
				s.messageType = "success"
				return s
			},
		},
		scenario{
			suffix:   "quota.post-delete.beta",
			view:     "Quota",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accountsPostBeta, tuiViewQuota, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
				s.message = "Removed account beta@example.invalid"
				s.messageType = "success"
				return s
			},
		},
		scenario{
			suffix:   "quota.post-delete.gamma",
			view:     "Quota",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accountsPostGamma, tuiViewQuota, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
				s.message = "Removed account gamma@example.invalid"
				s.messageType = "success"
				return s
			},
		},
	)

	// 9. Canonical aliases for top-level defaults
	scenarios = append(scenarios,
		scenario{
			suffix:   "dashboard.ready",
			view:     "Dashboard",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				return createBaseStateWithActiveAndAccounts(accounts, tuiViewDashboard, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
			},
		},
		scenario{
			suffix:   "quota.ready",
			view:     "Quota",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				return createBaseStateWithActiveAndAccounts(accounts, tuiViewQuota, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
			},
		},
		scenario{
			suffix:   "dashboard.switched.account-beta",
			view:     "Dashboard",
			mode:     "ready",
			active:   "beta@example.invalid",
			selected: "beta@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewDashboard, tuiBrowse, "beta@example.invalid", "beta@example.invalid")
				s.message = "Switched to beta@example.invalid"
				s.messageType = "success"
				return s
			},
		},
		scenario{
			suffix:   "dashboard.switched.account-gamma",
			view:     "Dashboard",
			mode:     "ready",
			active:   "gamma@example.invalid",
			selected: "gamma@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewDashboard, tuiBrowse, "gamma@example.invalid", "gamma@example.invalid")
				s.message = "Switched to gamma@example.invalid"
				s.messageType = "success"
				return s
			},
		},
		scenario{
			suffix:   "profiles.ready",
			view:     "Profiles",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewProfiles, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
				s.profileIndex = 1
				return s
			},
		},
		scenario{
			suffix:   "history.ready",
			view:     "History",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewHistory, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
				s.historyIndex = 0
				return s
			},
		},
		scenario{
			suffix:   "settings.ready",
			view:     "Settings",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				return createBaseStateWithActiveAndAccounts(accounts, tuiViewSettings, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
			},
		},
		scenario{
			suffix:   "doctor.ready",
			view:     "Doctor",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewDoctor, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
				s.doctorChecks = doctorChecks
				return s
			},
		},
		scenario{
			suffix:   "backup.ready",
			view:     "Backup",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				return createBaseStateWithActiveAndAccounts(accounts, tuiViewBackup, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
			},
		},
		scenario{
			suffix:   "dashboard.confirm-delete",
			view:     "Dashboard",
			mode:     "confirm_delete",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewDashboard, tuiConfirmDelete, "alpha@example.invalid", "alpha@example.invalid")
				s.confirmEmail = "alpha@example.invalid"
				return s
			},
		},
		scenario{
			suffix:   "quota.confirm-delete",
			view:     "Quota",
			mode:     "confirm_delete",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewQuota, tuiConfirmDelete, "alpha@example.invalid", "alpha@example.invalid")
				s.confirmEmail = "alpha@example.invalid"
				return s
			},
		},
		scenario{
			suffix:   "profiles.confirm-remove",
			view:     "Profiles",
			mode:     "confirm_action",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewProfiles, tuiConfirmAction, "alpha@example.invalid", "alpha@example.invalid")
				s.confirmTitle = "Remove selected profile"
				s.confirmAction = "profile-remove"
				return s
			},
		},
		scenario{
			suffix:   "history.confirm-action",
			view:     "History",
			mode:     "confirm_action",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewHistory, tuiConfirmAction, "alpha@example.invalid", "alpha@example.invalid")
				s.confirmTitle = "Clear local history"
				s.confirmAction = "history-clear"
				return s
			},
		},
		scenario{
			suffix:   "history.post-clear",
			view:     "History",
			mode:     "ready",
			active:   "alpha@example.invalid",
			selected: "alpha@example.invalid",
			setup: func(d struct {
				layout                string
				innerCols, rows, cols int
			}) *tuiState {
				s := createBaseStateWithActiveAndAccounts(accounts, tuiViewHistory, tuiBrowse, "alpha@example.invalid", "alpha@example.invalid")
				s.history = nil
				s.historyIndex = 0
				s.message = "History cleared"
				s.messageType = "success"
				return s
			},
		},
	)

	// Expand scenarios with options
	var expandedScenarios []scenario
	for _, sc := range scenarios {
		expandedScenarios = append(expandedScenarios, sc)
		if sc.mode == "form" && !strings.Contains(sc.suffix, ".edit-") && !strings.Contains(sc.suffix, ".option-") {
			dummyD := struct {
				layout                string
				innerCols, rows, cols int
			}{layout: "wide", innerCols: 98, rows: 24, cols: 100}
			s := sc.setup(dummyD)
			if s.form != nil && s.form.Index >= 0 && s.form.Index < len(s.form.Fields) {
				field := s.form.Fields[s.form.Index]
				if len(field.Options) > 0 {
					for _, opt := range field.Options {
						optName := opt
						if optName == "" {
							optName = "empty"
						}
						optVal := opt
						baseSC := sc
						oldFieldPart := fmt.Sprintf(".field-%d", s.form.Index)
						newFieldPart := fmt.Sprintf(".field-%d.option-%s", s.form.Index, optName)
						newSuffix := strings.Replace(baseSC.suffix, oldFieldPart, newFieldPart, 1)

						expandedScenarios = append(expandedScenarios, scenario{
							suffix:   newSuffix,
							view:     baseSC.view,
							mode:     baseSC.mode,
							active:   baseSC.active,
							selected: baseSC.selected,
							setup: func(d struct {
								layout                string
								innerCols, rows, cols int
							}) *tuiState {
								s2 := baseSC.setup(d)
								s2.form.Fields[s2.form.Index].Value = optVal
								return s2
							},
						})
					}
				}
			}
		}
	}
	scenarios = expandedScenarios

	var fixtures []Fixture
	for _, sc := range scenarios {
		for _, d := range dims {
			id := fmt.Sprintf("%s.%s", d.layout, sc.suffix)
			s := sc.setup(d)

			now := time.Now()
			for _, acct := range s.accounts.ByEmail {
				var delta time.Duration
				switch acct["email"] {
				case "alpha@example.invalid":
					delta = 2*time.Hour + 30*time.Minute + 45*time.Second
				case "beta@example.invalid":
					delta = 45*time.Minute + 45*time.Second
				case "gamma@example.invalid":
					delta = 15*time.Minute + 45*time.Second
				default:
					continue
				}
				if qs, ok := acct["quota_snapshot"].(map[string]any); ok {
					if groups, ok := qs["groups"].([]any); ok {
						for _, g := range groups {
							if group, ok := g.(map[string]any); ok {
								if buckets, ok := group["buckets"].([]any); ok {
									for _, b := range buckets {
										if bucket, ok := b.(map[string]any); ok {
											bucket["reset_at"] = now.Add(delta).Format(time.RFC3339)
										}
									}
								}
							}
						}
					}
				}
			}

			lines := a.tuiLines(s, d.innerCols, d.rows)

			ansiLines := make([]string, len(lines))
			plainLines := make([]string, len(lines))
			for i, line := range lines {
				ansiLines[i] = line
				plainLines[i] = stripANSI(line)
			}

			fixtures = append(fixtures, Fixture{
				ID: id,
				Dimensions: Dimensions{
					Cols:      d.cols,
					Rows:      d.rows,
					InnerCols: d.innerCols,
				},
				Layout:   d.layout,
				View:     sc.view,
				Mode:     sc.mode,
				Active:   sc.active,
				Selected: sc.selected,
				Lines:    ansiLines,
				Plain:    plainLines,
			})
		}
	}

	return fixtures
}

func TestTUIWebFixtures(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("ACCESSIBILITY_REDUCED_MOTION", "")
	t.Setenv("TZ", "UTC")
	time.Local = time.UTC

	frozenTime := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	renderNow := time.Now().UTC()

	a := &Application{
		Version: "2.1.2",
		p:       makePalette(true),
		color:   true,
	}

	resetAlpha := renderNow.Add(2*time.Hour + 30*time.Minute + 45*time.Second)
	resetBeta := renderNow.Add(45*time.Minute + 45*time.Second)
	resetGamma := renderNow.Add(15*time.Minute + 45*time.Second)

	accounts := NewAccounts()
	accounts.Set("alpha@example.invalid", makeSanitizedAccount("alpha@example.invalid", "Alpha User", 1.0, 0.85, resetAlpha, frozenTime.Add(-5*time.Minute)))
	accounts.Set("beta@example.invalid", makeSanitizedAccount("beta@example.invalid", "Beta User", 0.4, 0.4, resetBeta, frozenTime.Add(-2*time.Minute)))
	accounts.Set("gamma@example.invalid", makeSanitizedAccount("gamma@example.invalid", "Gamma User", 0.0, 0.0, resetGamma, frozenTime.Add(-10*time.Minute)))

	settings := defaultSettings()
	settings.Aliases = map[string]string{"alpha": "alpha@example.invalid", "work": "beta@example.invalid"}
	settings.Tags = map[string][]string{"alpha@example.invalid": {"primary", "dev"}}
	settings.Profiles = map[string]Profile{
		"personal": {Account: "alpha@example.invalid", Family: "gemini", Policy: "sticky"},
		"work":     {Account: "beta@example.invalid", Family: "claude", Policy: "balanced", NotifyThreshold: 15},
	}

	doctorChecks := []doctorCheck{
		{Name: "Configuration directory permissions", Status: "ok", Message: "0700 ~/.config/agy-swap"},
		{Name: "Secure credential vault backend", Status: "ok", Message: "Apple Keychain available and responsive"},
		{Name: "Network connectivity to endpoints", Status: "ok", Message: "TLS handshakes completed in 42ms"},
		{Name: "Active profile integrity", Status: "ok", Message: "Profile 'work' valid with 3 active accounts"},
	}

	fingerprint := getFingerprint(t)

	// 1. Generate full fixtures
	fixturesFirst := buildFixtures(t, a, accounts, settings, doctorChecks, frozenTime, renderNow)
	outFirst := FixtureOutput{
		Schema:            1,
		Renderer:          "internal/app.(*Application).tuiLines",
		Version:           "2.1.2",
		SourceFingerprint: fingerprint,
		Fixtures:          fixturesFirst,
	}

	// 2. Generate second run for byte-identical check
	fixturesSecond := buildFixtures(t, a, accounts, settings, doctorChecks, frozenTime, renderNow)
	outSecond := FixtureOutput{
		Schema:            1,
		Renderer:          "internal/app.(*Application).tuiLines",
		Version:           "2.1.2",
		SourceFingerprint: fingerprint,
		Fixtures:          fixturesSecond,
	}

	firstBytes, err := json.MarshalIndent(outFirst, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal first fixtures run: %v", err)
	}
	secondBytes, err := json.MarshalIndent(outSecond, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal second fixtures run: %v", err)
	}

	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("fixture generation is nondeterministic between consecutive runs")
	}

	// Verify countdown strings
	foundAlphaCountdown := false
	foundBetaCountdown := false
	foundGammaCountdown := false
	for _, f := range outFirst.Fixtures {
		for _, line := range f.Plain {
			if strings.Contains(line, "2h 30m") {
				foundAlphaCountdown = true
			}
			if strings.Contains(line, "45m") {
				foundBetaCountdown = true
			}
			if strings.Contains(line, "15m") {
				foundGammaCountdown = true
			}
		}
	}
	if !foundAlphaCountdown || !foundBetaCountdown || !foundGammaCountdown {
		t.Fatalf("expected deterministic countdown strings (2h 30m, 45m, 15m) not found in fixtures")
	}

	// 3. Schema & metadata validations
	if outFirst.Schema != 1 {
		t.Fatalf("schema = %d, want 1", outFirst.Schema)
	}
	if outFirst.Renderer != "internal/app.(*Application).tuiLines" {
		t.Fatalf("renderer = %q, want internal/app.(*Application).tuiLines", outFirst.Renderer)
	}
	if outFirst.Version != "2.1.2" {
		t.Fatalf("version = %q, want 2.1.2", outFirst.Version)
	}
	if outFirst.SourceFingerprint == "" {
		t.Fatal("sourceFingerprint is empty")
	}
	if len(outFirst.Fixtures) < 100 {
		t.Fatalf("fixture count = %d, want at least 100", len(outFirst.Fixtures))
	}

	seenIDs := make(map[string]bool)
	var initialFixtures []Fixture

	for idx, f := range outFirst.Fixtures {
		if f.ID == "" {
			t.Fatalf("fixture at index %d has empty ID", idx)
		}
		if seenIDs[f.ID] {
			t.Fatalf("duplicate fixture ID: %s", f.ID)
		}
		seenIDs[f.ID] = true

		if f.Dimensions.Cols != f.Dimensions.InnerCols+2 {
			t.Fatalf("fixture %s dimensions.cols=%d want %d", f.ID, f.Dimensions.Cols, f.Dimensions.InnerCols+2)
		}
		if f.Dimensions.Rows != len(f.Lines) {
			t.Fatalf("fixture %s dimensions.rows=%d, got %d lines", f.ID, f.Dimensions.Rows, len(f.Lines))
		}
		if f.Dimensions.Rows != len(f.Plain) {
			t.Fatalf("fixture %s dimensions.rows=%d, got %d plain lines", f.ID, f.Dimensions.Rows, len(f.Plain))
		}

		var activePlainLine string
		for _, p := range f.Plain {
			if strings.Contains(p, "ACTIVE") {
				activePlainLine = p
				break
			}
		}
		if f.Active != "" {
			if activePlainLine != "" {
				emailPrefix := strings.Split(f.Active, "@")[0]
				if !strings.Contains(activePlainLine, "<"+f.Active+">") && !strings.Contains(activePlainLine, "<"+emailPrefix) {
					t.Fatalf("fixture %s active metadata=%q but active line is: %q", f.ID, f.Active, activePlainLine)
				}
			} else if f.Mode == "ready" || f.Mode == "search" || f.Mode == "refreshing" {
				t.Fatalf("fixture %s in mode %s has active=%q but no ACTIVE plain line found", f.ID, f.Mode, f.Active)
			}
		} else {
			if activePlainLine != "" {
				if !strings.Contains(activePlainLine, "no saved session") {
					t.Fatalf("fixture %s active metadata is empty (none) but active line is: %q", f.ID, activePlainLine)
				}
			}
		}

		for r, l := range f.Lines {
			if strings.ContainsAny(l, "\r\n") {
				t.Fatalf("fixture %s line %d contains CR/LF: %q", f.ID, r, l)
			}
			if strings.ContainsRune(l, '\uFFFD') {
				t.Fatalf("fixture %s line %d contains replacement char U+FFFD: %q", f.ID, r, l)
			}
			if strings.Contains(l, "token_data") || strings.Contains(l, "/Users/") || strings.Contains(l, "/home/") || strings.Contains(l, "/root/") {
				t.Fatalf("fixture %s line %d contains forbidden token/secret/path: %q", f.ID, r, l)
			}
			if width := visibleWidth(l); width != f.Dimensions.Cols {
				t.Fatalf("fixture %s line %d visible width=%d, want=%d: %q", f.ID, r, width, f.Dimensions.Cols, l)
			}
		}

		for r, p := range f.Plain {
			if strings.ContainsAny(p, "\r\n") {
				t.Fatalf("fixture %s plain line %d contains CR/LF: %q", f.ID, r, p)
			}
			if strings.ContainsRune(p, '\uFFFD') {
				t.Fatalf("fixture %s plain line %d contains replacement char U+FFFD: %q", f.ID, r, p)
			}
			if strings.Contains(p, "token_data") || strings.Contains(p, "/Users/") || strings.Contains(p, "/home/") || strings.Contains(p, "/root/") {
				t.Fatalf("fixture %s plain line %d contains forbidden token/secret/path: %q", f.ID, r, p)
			}
			if width := visibleWidth(p); width != f.Dimensions.Cols {
				t.Fatalf("fixture %s plain line %d visible width=%d, want=%d: %q", f.ID, r, width, f.Dimensions.Cols, p)
			}
		}

		if f.ID == "wide.dashboard.ready.active-alpha.selected-alpha" ||
			f.ID == "stacked.dashboard.ready.active-alpha.selected-alpha" ||
			f.ID == "compact.dashboard.ready.active-alpha.selected-alpha" {
			initialFixtures = append(initialFixtures, f)
		}
	}

	if len(initialFixtures) != 3 {
		t.Fatalf("initialFixtures count = %d, want 3", len(initialFixtures))
	}

	initialOutput := FixtureOutput{
		Schema:            1,
		Renderer:          "internal/app.(*Application).tuiLines",
		Version:           "2.1.2",
		SourceFingerprint: fingerprint,
		Fixtures:          initialFixtures,
	}
	initialBytes, err := json.MarshalIndent(initialOutput, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal initial fixtures: %v", err)
	}

	t.Logf("Generated %d fixtures across %d unique IDs.", len(outFirst.Fixtures), len(seenIDs))

	destFull := filepath.Join("..", "..", "site", "src", "generated", "tui-fixtures.json")
	destInitial := filepath.Join("..", "..", "site", "src", "generated", "tui-initial-fixtures.json")
	shardsDir := filepath.Join("..", "..", "site", "src", "generated", "shards")

	if os.Getenv("UPDATE_TUI_WEB_FIXTURES") == "1" {
		if err := os.MkdirAll(filepath.Dir(destFull), 0755); err != nil {
			t.Fatalf("failed to create directory for fixtures: %v", err)
		}
		if err := os.MkdirAll(shardsDir, 0755); err != nil {
			t.Fatalf("failed to create shards directory: %v", err)
		}
		if err := os.WriteFile(destFull, firstBytes, 0644); err != nil {
			t.Fatalf("failed to write fixtures file: %v", err)
		}
		if err := os.WriteFile(destInitial, initialBytes, 0644); err != nil {
			t.Fatalf("failed to write initial fixtures file: %v", err)
		}

		// Partition fixtures into 21 layout + view shards
		layouts := []string{"wide", "stacked", "compact"}
		views := []string{"dashboard", "quota", "profiles", "history", "settings", "doctor", "backup"}

		for _, l := range layouts {
			for _, v := range views {
				var shardFixtures []Fixture
				for _, f := range outFirst.Fixtures {
					if f.Layout == l && strings.EqualFold(f.View, v) {
						shardFixtures = append(shardFixtures, f)
					}
				}
				shardOutput := FixtureOutput{
					Schema:            1,
					Renderer:          "internal/app.(*Application).tuiLines",
					Version:           "2.1.2",
					SourceFingerprint: fingerprint,
					Fixtures:          shardFixtures,
				}
				sBytes, err := json.MarshalIndent(shardOutput, "", "  ")
				if err != nil {
					t.Fatalf("failed to marshal shard %s.%s: %v", l, v, err)
				}
				shardPath := filepath.Join(shardsDir, fmt.Sprintf("%s.%s.json", l, v))
				if err := os.WriteFile(shardPath, sBytes, 0644); err != nil {
					t.Fatalf("failed to write shard file %s: %v", shardPath, err)
				}
			}
		}

		t.Logf("Wrote %d fixtures to %s, %d initial to %s, and 21 shards to %s", len(outFirst.Fixtures), destFull, len(initialFixtures), destInitial, shardsDir)
	}

	diskFullBytes, err := os.ReadFile(destFull)
	if err != nil {
		t.Fatalf("fixture file missing at %s, run with UPDATE_TUI_WEB_FIXTURES=1: %v", destFull, err)
	}
	if !bytes.Equal(firstBytes, diskFullBytes) {
		t.Fatalf("fixture file at %s is out of date or nondeterministic. Re-run UPDATE_TUI_WEB_FIXTURES=1", destFull)
	}

	diskInitialBytes, err := os.ReadFile(destInitial)
	if err != nil {
		t.Fatalf("initial fixture file missing at %s, run with UPDATE_TUI_WEB_FIXTURES=1: %v", destInitial, err)
	}
	if !bytes.Equal(initialBytes, diskInitialBytes) {
		t.Fatalf("initial fixture file at %s is out of date or nondeterministic. Re-run UPDATE_TUI_WEB_FIXTURES=1", destInitial)
	}
}
