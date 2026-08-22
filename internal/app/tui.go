package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

type tuiKeyEvent struct{ key string }
type tuiAccountsEvent struct {
	accounts    *Accounts
	quotaErrors map[string]string
}
type tuiActiveEvent struct {
	token string
	email string
}

func readTerminalKey(reader io.Reader) string {
	var first [1]byte
	if _, err := io.ReadFull(reader, first[:]); err != nil {
		return ""
	}
	switch first[0] {
	case 0, 0xe0:
		var second [1]byte
		if _, err := io.ReadFull(reader, second[:]); err != nil {
			return ""
		}
		switch second[0] {
		case 'H':
			return "up"
		case 'P':
			return "down"
		case 'S':
			return "delete"
		}
		return ""
	case 0x1b:
		if file, ok := reader.(*os.File); ok {
			_ = file.SetReadDeadline(time.Now().Add(75 * time.Millisecond))
			defer file.SetReadDeadline(time.Time{})
		}
		var second [1]byte
		if _, err := reader.Read(second[:]); err != nil {
			return "esc"
		}
		if second[0] != '[' && second[0] != 'O' {
			return ""
		}
		var third [1]byte
		if _, err := reader.Read(third[:]); err != nil {
			return ""
		}
		switch third[0] {
		case 'A':
			return "up"
		case 'B':
			return "down"
		case '3':
			var fourth [1]byte
			if _, err := reader.Read(fourth[:]); err == nil && fourth[0] == '~' {
				return "delete"
			}
		}
		return ""
	case '\r', '\n':
		return "enter"
	case 0x7f, 0x08:
		return "backspace"
	case 0x03:
		return "ctrl-c"
	case 0x04:
		return "ctrl-d"
	default:
		return string(first[:])
	}
}

func (a *Application) cmdInteractive(ctx context.Context) int {
	inFile, inOK := a.In.(*os.File)
	outFile, outOK := a.Out.(*os.File)
	if !inOK || !outOK || !term.IsTerminal(int(inFile.Fd())) || !term.IsTerminal(int(outFile.Fd())) {
		return a.cmdList(ctx, cliArgs{})
	}
	accounts, err := a.store.Load(false)
	if err != nil {
		return a.storeError(err)
	}
	current := a.credentials.Current(ctx)
	oldState, err := term.MakeRaw(int(inFile.Fd()))
	if err != nil {
		return a.storeError(err)
	}
	raw := true
	enterScreen := func() { fmt.Fprint(a.Out, "\x1b[?1049h\x1b[?25l\x1b[H") }
	leaveScreen := func() { fmt.Fprint(a.Out, "\x1b[?1049l\x1b[?25h") }
	enterScreen()
	defer func() {
		if raw {
			_ = term.Restore(int(inFile.Fd()), oldState)
		}
		leaveScreen()
	}()
	events := make(chan any, 8)
	ack := make(chan struct{})
	done := make(chan struct{})
	resolvedActive := localActiveEmail(accounts, current)
	resolvingToken := ""
	startActiveResolve := func() {
		local := localActiveEmail(accounts, current)
		if local != "" || current == "" {
			resolvedActive = local
			return
		}
		if resolvingToken == current {
			return
		}
		token := current
		snapshot := accounts
		resolvingToken = token
		go func() {
			email := a.activeEmail(ctx, snapshot, token)
			select {
			case events <- tuiActiveEvent{token: token, email: email}:
			case <-ctx.Done():
			}
		}()
	}
	go func() {
		for {
			key := readTerminalKey(inFile)
			select {
			case events <- tuiKeyEvent{key: key}:
			case <-done:
				return
			}
			select {
			case <-ack:
			case <-done:
				return
			}
		}
	}()
	refreshing := false
	startRefresh := func(force bool) {
		if refreshing {
			return
		}
		refreshing = true
		go func() {
			fresh, loadErr := a.store.Load(true)
			errs := map[string]string{}
			if loadErr == nil && fresh.Len() > 0 {
				errs = a.quota.Refresh(ctx, fresh, force, nil)
			}
			if loadErr != nil {
				errs["store"] = loadErr.Error()
			}
			select {
			case events <- tuiAccountsEvent{fresh, errs}:
			case <-ctx.Done():
			}
		}()
	}
	startRefresh(false)
	startActiveResolve()
	credentialTicker := time.NewTicker(1500 * time.Millisecond)
	defer credentialTicker.Stop()
	quotaTicker := time.NewTicker(tuiAutoRefresh)
	defer quotaTicker.Stop()
	selected, lastAccount := 0, 0
	message, messageType := "", "info"
	confirmDelete := ""
	quotaErrors := map[string]string{}
	render := func() {
		width, _, _ := term.GetSize(int(outFile.Fd()))
		width = max(28, min(110, width-2))
		active := resolvedActive
		items := accounts.Len() + 3
		if selected >= items {
			selected = items - 1
		}
		if selected < 0 {
			selected = 0
		}
		var lines []string
		sep := a.p.Gray + strings.Repeat("─", min(width, 80)) + a.p.Reset
		lines = append(lines, a.p.Bold+a.p.Orange+"AGY SWAP"+a.p.Reset+" "+a.p.Gray+"v"+a.Version+" · Google Antigravity Session Manager"+a.p.Reset, sep)
		if active != "" {
			name := "Google User"
			saved := false
			if account, ok := accounts.Get(active); ok {
				name = getString(account, "name")
				saved = true
			}
			tag := a.p.Yellow + "(Unsaved, press 'a' to save)" + a.p.Reset
			if saved {
				tag = a.p.Green + "(Saved)" + a.p.Reset
			}
			lines = append(lines, " "+a.p.Bold+"Active:"+a.p.Reset+" "+a.p.Green+"●"+a.p.Reset+" "+avatar(name, active, a.color)+" "+a.p.Bold+name+a.p.Reset+" "+a.p.Gray+"<"+active+">"+a.p.Reset+" "+tag)
		} else {
			lines = append(lines, " "+a.p.Gray+"Active: Not logged in"+a.p.Reset)
		}
		lines = append(lines, "", " "+a.p.Bold+"ACCOUNTS"+a.p.Reset)
		now := time.Now()
		for i, email := range accounts.Order {
			account := accounts.ByEmail[email]
			cursor := " "
			if i == selected {
				cursor = a.p.Orange + "❯" + a.p.Reset
			}
			marker := a.p.Gray + "○" + a.p.Reset
			if strings.EqualFold(email, active) {
				marker = a.p.Green + "●" + a.p.Reset
			}
			line := fmt.Sprintf(" %s %s [%d] %s %s%s%s %s<%s>%s", cursor, marker, i+1, avatar(getString(account, "name"), email, a.color), a.p.Bold, getString(account, "name"), a.p.Reset, a.p.Gray, email, a.p.Reset)
			lines = append(lines, line)
			if i == selected {
				lines = append(lines, "   "+accountStatus(account, a.p, now))
				for _, rawGroup := range quotaGroups(account) {
					group := getMap(rawGroup)
					lines = append(lines, "   "+getString(group, "name"))
					for _, rawBucket := range getSlice(group["buckets"]) {
						bucket := getMap(rawBucket)
						lines = append(lines, "     "+getString(bucket, "name")+"  "+formatQuotaBar(bucket, a.p, now, 10))
					}
				}
				if reset, ok := tokenResetInfo(getString(account, "token_data")); ok {
					lines = append(lines, "   Session Token  "+a.p.Cyan+reset+a.p.Reset)
				}
				if reason := quotaErrors[email]; reason != "" && len(quotaGroups(account)) == 0 {
					lines = append(lines, "   "+a.p.Yellow+"Usage unavailable"+a.p.Reset+" · "+cleanText(reason))
				}
			}
		}
		lines = append(lines, "", " "+a.p.Bold+"ACTIONS"+a.p.Reset)
		actions := []string{"[a] Add Account", "[d] Delete Account", "[q] Quit"}
		for i, label := range actions {
			index := accounts.Len() + i
			cursor := " "
			style := a.p.Gray
			if selected == index {
				cursor = a.p.Orange + "❯" + a.p.Reset
				style = a.p.Bold + a.p.White
			}
			lines = append(lines, " "+cursor+" "+style+label+a.p.Reset)
		}
		lines = append(lines, "", sep)
		if confirmDelete != "" {
			lines = append(lines, " "+a.p.Red+"Confirm delete "+confirmDelete+"? Press [y] to confirm, any key to cancel."+a.p.Reset)
		} else if message != "" {
			prefix, color := "› ", a.p.Cyan
			if messageType == "success" {
				prefix, color = "✓ ", a.p.Green
			} else if messageType == "error" {
				prefix, color = "✕ ", a.p.Red
			}
			lines = append(lines, " "+color+prefix+message+a.p.Reset)
		} else {
			lines = append(lines, " "+a.p.Gray+"Navigate: [↑/↓] │ Select: [Enter] │ Shortcuts: [a] Add  [r] Refresh  [t] Tier  [d] Delete  [q] Quit"+a.p.Reset)
		}
		fmt.Fprint(a.Out, "\x1b[H")
		for _, line := range lines {
			fmt.Fprint(a.Out, truncateVisible(line, width, a.p), "\x1b[K\r\n")
		}
		fmt.Fprint(a.Out, "\x1b[J")
	}
	suspend := func(action func() int) {
		_ = term.Restore(int(inFile.Fd()), oldState)
		raw = false
		leaveScreen()
		code := action()
		if code == 0 {
			message, messageType = "Completed", "success"
		} else {
			message, messageType = "Action failed", "error"
		}
		newState, stateErr := term.MakeRaw(int(inFile.Fd()))
		if stateErr == nil {
			oldState = newState
			raw = true
		}
		enterScreen()
		fresh, loadErr := a.store.Load(false)
		if loadErr == nil {
			accounts = fresh
		}
		current = a.credentials.Current(ctx)
		startActiveResolve()
	}
	render()
	for {
		select {
		case <-ctx.Done():
			close(done)
			return 0
		case <-credentialTicker.C:
			newToken := a.credentials.Current(ctx)
			if newToken != current {
				current = newToken
				resolvedActive = localActiveEmail(accounts, current)
				startActiveResolve()
				render()
			}
		case <-quotaTicker.C:
			startRefresh(false)
		case event := <-events:
			switch value := event.(type) {
			case tuiAccountsEvent:
				refreshing = false
				if value.accounts != nil {
					accounts = value.accounts
				}
				if local := localActiveEmail(accounts, current); local != "" {
					resolvedActive = local
				} else {
					startActiveResolve()
				}
				quotaErrors = value.quotaErrors
				message = "Usage refreshed"
				messageType = "success"
				render()
			case tuiActiveEvent:
				if value.token == current {
					resolvedActive = value.email
					resolvingToken = ""
					render()
				}
			case tuiKeyEvent:
				key := strings.ToLower(value.key)
				if confirmDelete != "" {
					email := confirmDelete
					confirmDelete = ""
					if key == "y" {
						fresh, loadErr := a.store.Load(false)
						if loadErr == nil {
							fresh.Delete(email)
							if saveErr := a.store.Save(fresh); saveErr == nil {
								accounts = fresh
								message = "Removed account " + email
								messageType = "success"
							} else {
								message = saveErr.Error()
								messageType = "error"
							}
						}
					} else {
						message = "Canceled"
						messageType = "info"
					}
					ack <- struct{}{}
					render()
					continue
				}
				message = ""
				items := accounts.Len() + 3
				switch key {
				case "up":
					selected--
					if selected < 0 {
						selected = items - 1
					}
				case "down":
					selected++
					if selected >= items {
						selected = 0
					}
				case "q", "esc", "ctrl-c", "ctrl-d":
					close(done)
					return 0
				case "r":
					suspend(func() int {
						fresh, loadErr := a.store.Load(true)
						if loadErr != nil {
							return a.storeError(loadErr)
						}
						failures := a.quota.Refresh(ctx, fresh, true, nil)
						if len(failures) > 0 {
							return 1
						}
						return 0
					})
					ack <- struct{}{}
					render()
					continue
				case "a":
					suspend(func() int { return a.addLoginFlow(ctx) })
					ack <- struct{}{}
					render()
					continue
				case "t":
					if selected < accounts.Len() {
						email := accounts.Order[selected]
						fresh, loadErr := a.store.Load(false)
						if loadErr == nil {
							account := fresh.ByEmail[email]
							if getMap(account["quota_snapshot"]) != nil {
								message = "Tier for " + email + " is synced from Google"
								messageType = "info"
							} else {
								plan := "Pro"
								if getString(account, "tier_source") == "manual" && getString(account, "plan") == "Pro" {
									plan = "Free"
								} else if getString(account, "tier_source") == "manual" && getString(account, "plan") == "Free" {
									plan = ""
								}
								if plan == "" {
									delete(account, "plan")
									delete(account, "tier")
									delete(account, "tier_source")
									delete(account, "is_pro")
								} else {
									account["plan"] = plan
									account["tier"] = plan
									account["tier_source"] = "manual"
									account["is_pro"] = plan == "Pro"
								}
								if saveErr := a.store.Save(fresh); saveErr == nil {
									accounts = fresh
									message = "Set tier label for " + email + " to " + firstString(plan, "Unknown")
									messageType = "success"
								}
							}
						}
					}
				case "d", "delete", "backspace":
					if selected < accounts.Len() {
						confirmDelete = accounts.Order[selected]
					}
				case "enter":
					if selected < accounts.Len() {
						email := accounts.Order[selected]
						account := accounts.ByEmail[email]
						suspend(func() int {
							fmt.Fprintf(a.Out, "Switching account to %s%s%s %s<%s>%s...\n", a.p.Bold, getString(account, "name"), a.p.Reset, a.p.Gray, email, a.p.Reset)
							if a.credentials.Apply(ctx, getString(account, "token_data"), email) {
								return 0
							}
							return 1
						})
						ack <- struct{}{}
						render()
						continue
					} else {
						action := selected - accounts.Len()
						if action == 0 {
							suspend(func() int { return a.addLoginFlow(ctx) })
							ack <- struct{}{}
							render()
							continue
						}
						if action == 1 && accounts.Len() > 0 {
							confirmDelete = accounts.Order[min(lastAccount, accounts.Len()-1)]
						}
						if action == 2 {
							close(done)
							return 0
						}
					}
				default:
					if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
						index := int(key[0] - '1')
						if index < accounts.Len() {
							selected = index
						}
					}
				}
				if selected < accounts.Len() {
					lastAccount = selected
				}
				ack <- struct{}{}
				render()
			}
		}
	}
}
