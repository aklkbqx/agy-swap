package app

import (
	"os"
	"strings"
	"time"
)

type tuiMode uint8

const (
	tuiBrowse tuiMode = iota
	tuiSearch
	tuiHelp
	tuiConfirmDelete
)

type tuiAnimation struct {
	kind   string
	start  time.Time
	until  time.Time
	phase  int
	active bool
}

type tuiState struct {
	accounts       *Accounts
	current        string
	active         string
	selectedEmail  string
	selectedBefore string
	search         string
	searchPrevious string
	mode           tuiMode
	confirmEmail   string
	message        string
	messageType    string
	quotaErrors    map[string]string
	refreshing     bool
	resolvingToken string
	width          int
	height         int
	motionEnabled  bool
	animation      tuiAnimation
}

func newTUIState(accounts *Accounts, current string) *tuiState {
	state := &tuiState{
		accounts:      accounts,
		current:       current,
		quotaErrors:   map[string]string{},
		messageType:   "info",
		motionEnabled: tuiMotionEnabled(),
	}
	state.active = localActiveEmail(accounts, current)
	state.clampSelection()
	return state
}

func (s *tuiState) visibleEmails() []string {
	if s.accounts == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(s.search))
	result := make([]string, 0, s.accounts.Len())
	for _, email := range s.accounts.Order {
		if query == "" {
			result = append(result, email)
			continue
		}
		account := s.accounts.ByEmail[email]
		if strings.Contains(strings.ToLower(email), query) || strings.Contains(strings.ToLower(getString(account, "name")), query) {
			result = append(result, email)
		}
	}
	return result
}

func (s *tuiState) selectedAccount() (string, Account, bool) {
	emails := s.visibleEmails()
	if len(emails) == 0 {
		return "", nil, false
	}
	if s.selectedEmail != "" {
		for _, email := range emails {
			if strings.EqualFold(email, s.selectedEmail) {
				return email, s.accounts.ByEmail[email], true
			}
		}
	}
	s.selectedEmail = emails[0]
	return emails[0], s.accounts.ByEmail[emails[0]], true
}

func (s *tuiState) clampSelection() {
	if s.accounts == nil {
		s.selectedEmail = ""
		return
	}
	emails := s.visibleEmails()
	if len(emails) == 0 {
		s.selectedEmail = ""
		return
	}
	for _, email := range emails {
		if strings.EqualFold(email, s.selectedEmail) {
			s.selectedEmail = email
			return
		}
	}
	s.selectedEmail = emails[0]
}

func (s *tuiState) setAccounts(accounts *Accounts) {
	if accounts != nil {
		s.accounts = accounts
	}
	s.clampSelection()
	if local := localActiveEmail(s.accounts, s.current); local != "" {
		s.active = local
	}
}

func (s *tuiState) move(delta int) {
	emails := s.visibleEmails()
	if len(emails) == 0 {
		return
	}
	index := 0
	for i, email := range emails {
		if strings.EqualFold(email, s.selectedEmail) {
			index = i
			break
		}
	}
	index = (index + delta) % len(emails)
	if index < 0 {
		index += len(emails)
	}
	s.selectedEmail = emails[index]
	s.beginAnimation("focus", 140*time.Millisecond)
}

func (s *tuiState) moveToBoundary(last bool) {
	emails := s.visibleEmails()
	if len(emails) == 0 {
		return
	}
	if last {
		s.selectedEmail = emails[len(emails)-1]
	} else {
		s.selectedEmail = emails[0]
	}
	s.beginAnimation("focus", 140*time.Millisecond)
}

func (s *tuiState) beginSearch() {
	s.searchPrevious = s.search
	s.selectedBefore = s.selectedEmail
	s.mode = tuiSearch
}

func (s *tuiState) cancelSearch() {
	s.search = s.searchPrevious
	if s.selectedBefore != "" {
		s.selectedEmail = s.selectedBefore
	}
	s.selectedBefore = ""
	s.mode = tuiBrowse
	s.clampSelection()
}

func (s *tuiState) beginAnimation(kind string, duration time.Duration) {
	if !s.motionEnabled {
		s.animation = tuiAnimation{}
		return
	}
	now := time.Now()
	s.animation = tuiAnimation{kind: kind, start: now, active: true}
	if duration > 0 {
		s.animation.until = now.Add(duration)
	}
}

func tuiMotionEnabled() bool {
	reduced := strings.ToLower(strings.TrimSpace(os.Getenv("AGY_SWAP_REDUCED_MOTION")))
	return reduced != "1" && reduced != "true" && reduced != "yes" && reduced != "on"
}

func (s *tuiState) advanceAnimation(now time.Time) bool {
	if !s.animation.active {
		return false
	}
	s.animation.phase++
	if !s.animation.until.IsZero() && !now.Before(s.animation.until) {
		s.animation.active = false
		return false
	}
	return true
}

func (s *tuiState) animationPhase() int {
	return s.animation.phase % 4
}

func (s *tuiState) selectedIndex() int {
	for i, email := range s.visibleEmails() {
		if strings.EqualFold(email, s.selectedEmail) {
			return i
		}
	}
	return 0
}
