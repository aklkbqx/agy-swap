# Changelog

## 2.17.2

### Changes

- TUI: motion runs at 60 FPS (`16ms` per frame while moving, `50ms` while idle) instead of 30 FPS (`33ms`), doubling the frame count of selection slides, meter sweeps, modal openings, toasts, and view transitions while stopping the timer immediately once settled (#82).
- TUI: truecolor sub-cell anti-aliasing (`lerpColor`) blends fractional edge rows of the selection band (`tuiSelectionBand` / `tuiBandRowAlpha`), fractional boundary columns of sliding dialog buttons (`tuiSlidingButtons`), the leading half-cell of quota meters (`renderMeter`), and the continuous cosine light wave during manual refreshes (`tuiRefreshSweep`) so motion glides smoothly across the character grid instead of snapping by whole rows or columns (#82).
- Site: the pinned story terminal exports multi-frame ANSI diff transition sequences (`transitions` in `tui-story-frames.json`) and plays them in `TuiStoryFrame` when a beat becomes active, while honouring `prefers-reduced-motion` (#82).

### Fixes

- TUI: hovering an unselected account now tints a full 4-row card (1 top padding row, 2 content rows, 1 bottom padding row) with 2 columns of right padding (`tuiHoverRow`), and body rows replace `\x1b[49m` with `BodyBg` so the hover highlight never leaks the terminal default background into the divider gap (#82).

## 2.17.1

### Fixes

- macOS & Linux: switching accounts, clearing the active session, and logging in a new account no longer call `/usr/bin/security` or prompt for the login keychain password (#79). `agy-swap` maintains `~/.gemini/antigravity-cli/cache/antigravity-keyring-unavailable` and configures the `~/.local/bin/agy` launcher so Antigravity's `CompositeTokenStorage` (`shouldBypassKeyring`) and `agy-swap` both use the `0600` file-backed session store (`antigravity-oauth-token`) directly instead of reading stale Keychain items.
- Multi-workspace sessions: a background `agy` process refreshing its OAuth token in another folder no longer switches newly opened workspaces away from the active account chosen in `agy-swap` (#79). `agy-swap --sync-session` (and automatic startup reconciliation) harvests refreshed tokens into the account vault by matching `refresh_token` and restores the active account selected in `google_accounts.json`.
- Launcher guard: when `~/.local/bin/agy` is present, `agy-swap` automatically wraps or upgrades the launcher to run `agy-swap --sync-session` before and after `agy` with zero manual configuration (#79).

## 2.17.0

### Changes

- TUI: the mouse can do what the keys do. Hover shows what a click would do (an account gets a faint tint, the divider or rule lights, a footer key lights) without moving the selection. Click a key hint in the footer to press it, a dialog button to choose it (the choice follows the pointer), a palette row to run it, or beside a dialog or the palette to close it. Under a dialog only the dialog and the key hints answer. kitty, foot, Ghostty and WezTerm show a hand over targets and a resize pointer over the divider and rule. `AGY_SWAP_MOUSE=0` turns mouse reporting off.
- TUI: quota stays close to live. Instead of every account once a minute, the account Antigravity is using is read every 20 seconds and the others every 2 minutes, an account is read again right after one of its pools resets (an account whose shared pools are used up waits for that), and the account in use is read within about 10 seconds of Antigravity writing to its logs (only the logs' sizes and times are checked, at most one read every 10 seconds). Countdowns tick every minute. Another agy-swap process changing the accounts reloads the screen.
- Quota reads remember each account's Code Assist project and tier for 15 minutes, so most reads are one request instead of two; any failed read forgets them. An account Google refuses (HTTP 429 or 5xx) backs off, doubling from 20 seconds to 10 minutes with jitter. Reset times are moved onto this machine's clock when it differs from Google's (from the replies' `Date` header) by 5 seconds or more.
- TUI: a list or gauge panel taller than its room scrolls instead of hiding accounts behind "▲ 3 more": a bar at its right edge shows where you are, the mouse wheel over it scrolls it, and moving the selection brings the selected account back into view. Where everything fits, the wheel still moves the selection.
- TUI: the rule between the list and the gauges on a narrow window moves: drag it with the mouse, or press `[` / `]` to shorten or lengthen the list, and double-click it or press `=` to reset. It moves only as far as it changes what shows, so it never opens empty space; its place is remembered like the side-by-side divider's.
- TUI: on a narrow window a click on an account selects it and a second click switches to it, as in the side-by-side layout.
- TUI: the quota overview page is gone; the dashboard shows the same accounts, statuses and meters. `v` now only verifies a backup on the Backup page, and Argon shows in its gauge panel.

### Fixes

- Quota shown in the TUI could be about two minutes old: the minute timer fired as snapshots turned a minute old and skipped them for a whole round. Snapshots a few seconds short of a minute now count as due.
- TUI: the profiles, history, settings, health check and backup pages keep a blank row above and below their content, like the dashboard, so their title no longer sits against the header and long content stops short of the status line (#74).
- TUI: on a narrow window, where the list sits above the gauges, the list title and the rule between list and gauges have a blank row on each side instead of touching the rows next to them (#74).
- TUI: the selection band no longer slides onto the wrong account when the list reflows (while the rule is dragged, or the list scrolls); it slides only when the selection moves.

## 2.16.0

### Features

- TUI: a new look. The screen has no line frame: the header (logo, active account) and the footer (status line, key hints) are solid bars that run edge to edge and close into a solid frame with a one-column rail down each side (outside the content's two columns of air), quota pools are solid cards with their state colour as a bar down the left edge, and one quiet divider separates the list from the gauges. Keys are chips (` Enter `) instead of `[Enter]`, at the same width. Without colour, cards keep their outline and keys stay `[Enter]`.
- TUI: the body is painted black (`#0a0c10`, the website's background) instead of showing the terminal's own background, and every gray is one ramp of cool grays above it: bars and cards, then the selection tint, then key chips and quiet lines.
- TUI: even spacing everywhere: one row above and below and two columns either side of the content in the bars, the cards and the body (inside the side rails), and two columns either side of the divider; bar text lines up with the body. Side-by-side cards are the same width and end flush with the column. From 32 rows the header has a blank row between the logo and the active account line.
- TUI: on a terminal wider than 160 columns the dashboard keeps 160 columns and stays centred; mouse clicks and divider drags follow it. From 92 to 119 columns each pool card puts its percentage beside the bars (about seven rows a pool instead of sixteen).
- TUI: Enter (and a double click or the palette's switch action) asks before switching, naming the account; Enter on the account already in use just says so.
- TUI: every question (switch, delete, remove profile, clear history, reset settings, update) and the help screen use one dialog. ←/→, ↑/↓ or Tab choose a button, Enter presses it, `y`/`n` still work and Esc cancels. The chosen button is drawn as `[ Label ]`; questions that delete or reset something start on Cancel. Dialogs, the command palette and forms sit on a dimmed backdrop; the status line and key hints stay lit.
- TUI: a calmer account list: no initials before each name (the header keeps them) and no dot before accounts that are not in use. The selected account carries an orange bar `┃` down every line of its block on a lighter tint, with a row of padding above and below and a column on the right inside the tint, and a blank row of margin outside it. The account Antigravity is using has the word `ACTIVE` beside its name and a blue `●` (in the list, the quota overview and the header); green, yellow and red now only say how much quota is left. The website legend uses the same marks.
- TUI: motion where it helps the eye follow a change, mostly 120–250 ms. The selection band slides to the next account; meters, percentages and big digits ease to new values, and the detail eases from the last account's numbers; a dialog, the palette or a form opens from its centre over a dimming backdrop; a toast rises into place and its bottom edge counts down; after a switch the active dot pulses and `ACTIVE` sweeps in; the chip of a pressed key lights; the divider lights while dragged; the search caret blinks; a status word that changes flashes once; start-up and a change of view wipe in from the top; a choice in a dialog slides from one button to the next; while a refresh you started with `r` runs, a soft diagonal light sweeps across the whole screen; and while nothing else moves the active account's dot breathes slowly (truecolor terminals, ten frames a second, only its row repainted). Redraws now rewrite only the rows that changed. `AGY_SWAP_REDUCED_MOTION=1` turns all motion off.
- TUI: background quota refreshes (on start, every minute, after adding an account) run silently; only a failed refresh says so. A refresh started with `r` still shows its progress and result. The account detail drops the session token countdown and "updated just now", and says "updated … ago" in yellow only when the numbers are more than five minutes old.
- Site: the legend and walkthrough describe the new screen in every language: the blue dot and `ACTIVE` for the live account, a switch confirmed with Switch, quota refreshed in the background, and the colour roles; the story frames show the new look.

### Fixes

- macOS: switching accounts no longer asks for the keychain password every time. agy-swap reset the access list of Antigravity's `gemini` keychain item on each switch, and macOS asks for the password whenever an access list changes. It now updates the item and keeps its access list, and opens a new item to Antigravity only when it creates one (#68).
- TUI: switching no longer blanks the screen for a moment. The switch runs inside the dashboard with a "Switching to …" status line instead of leaving the full-screen view while the session is written (#70).
- TUI: the right-most column of every row was wiped after drawing (an erase-to-end-of-line sent while the cursor waited on the last column), so the bars and cards stopped one column short and the right margin looked narrower than the left. The same happened to the bottom-right corner after the last row. Full-width rows are no longer followed by an erase.
- TUI: the logo and the active account line no longer touch, and on a tall window a pool card gets at most one row of air between its sections.

## 2.15.0

### Features

- Gemini 4 Argon: Google gives some Pro accounts a weekly Argon pool of their own. agy-swap now reads it, keeps it apart from the Gemini pool, and shows it in `list`, `limits`, its own TUI gauge panel and a new Argon column in the quota overview. `--family argon` (in `next`, `recommend`, `limit --group`, profiles and the policy setting) picks accounts by Argon quota; on an account without one (Ultra counts Argon against the Gemini pool), it reads the Gemini pool.
- A used-up Argon pool does not bench an account: the status word, plain `next` and `recommend` still judge the shared Gemini and Claude/GPT pools.
- Quota pools and limit windows that Google adds later (for example a daily or monthly limit) are kept and listed instead of being dropped.
- `AGY_SWAP_USER_AGENT` overrides the client string sent to Google, for the case where a future Antigravity release changes which pools Google reports.

### Fixes

- The Gemini weekly limit was missing for many accounts (#50): Google only reports the Gemini and Argon pools to a recent Antigravity client, and agy-swap now identifies as one.
- TUI: with three pools side by side, the third panel was not drawn; panels now sit two to a row.
- The Gemini quota read from the model list (when Google's summary leaves Gemini out) no longer takes the Argon models' quota.

## 2.14.0

### Features

- An account that Google wants verified before Antigravity can use it now shows `VERIFY`, in the TUI and in `list`, instead of staying pending. The state is saved with the account and clears after a successful refresh.
- `w` in the TUI and the new `agy-swap verify <account>` command open Google's verification page in the system browser, so there is no long link to copy. The CLI also prints the link on a line of its own. Only `https` pages on google.com are opened.
- `add` checks the new account once after saving it. When Google wants it verified, it says so, prints the link and offers to open it.

### Fixes

- TUI: after adding an account, the quota refreshes at once.
- TUI: an account that needs verification no longer says "Needs re-auth" or suggests `agy-swap add`, which did not help.
- `AGY_SWAP_QUOTA_DEBUG=1` describes Google's account check (field names, reason codes and link hosts only).

### Security

- Builds require Go 1.27.3, which fixes GO-2026-6809 (crypto/x509 now distrusts compromised intermediate certificates). `go.mod` declares `go 1.27.3`, so `go install` uses it too.

## 2.13.1

### Fixes

- TUI: each limit bar in the big gauge panels is one row again. Two rows read as two separate bars in terminals that leave a gap between lines.
- TUI: the selected account row is tinted the same gray in a truecolor terminal as in a 256-color one and on the website, instead of a brown that almost disappeared into the background.

## 2.13.0

### Features

- TUI: the dashboard is rebuilt around the two quota pools. Each account shows a Gemini and a Claude/GPT meter with a status word (READY, LOW, GEMINI ONLY, CLAUDE, GPT ONLY, LIMITED, NO TOKEN, ATTENTION), and the detail side draws one gauge per pool with big digits, a bar and the reset time. Accounts that need attention are grouped under their own divider, and a NO TOKEN row names the fix (`agy-swap add`).
- TUI: the dashboard uses the whole window. Digits grow up to twice their size, bars get thicker, the account list gets wider with longer bars, and on small windows the gauges degrade step by step to plain limit lines (down to 28 by 12).
- TUI: resize the accounts list against the detail panel by dragging the divider with the mouse or with `[` and `]` (`=` resets). Clicking an account selects it.
- TUI: `AGY_SWAP_ASCII=1` draws the dashboard with ASCII characters only. It is chosen automatically on a Linux console, a dumb terminal or a non-UTF-8 locale; `AGY_SWAP_ASCII=0` turns it off.
- TUI: every screen shows its own key hints, the welcome and help screens open with the logo and credit, and warnings are shown in yellow instead of as errors.

### Fixes

- TUI: the account list is no longer repeated under the gauges, and the divider runs down to the status line on tall windows.

## 2.12.2

### Fixes

- Quota: Gemini quota shows again. Google's quota summary can leave the Gemini group out while the model list still carries the Gemini quota, so agy-swap now reads it from the model list when the summary has none. Gemini limits there do not say whether they are weekly or 5-hour, so a reset within five hours is shown as the 5-hour limit and a later reset as weekly. Refs #50.
- Quota: a limit that Google marks disabled (for example the 5-hour limit while the weekly limit is used up) is no longer drawn as 100% remaining. Refs #55.

### Development

- `AGY_SWAP_QUOTA_DEBUG=1` prints, per account, which quota buckets were kept or skipped and why, the field names of Google's quota summary, and the quota of each model in the model list (percentages and reset times only). It is off by default.

## 2.12.1

### Site

- The pinned terminal on the home page now uses a 63-column frame on wide screens, so it fills its stage instead of leaving empty bands on both sides. It still lists every account. Phones keep the narrower frames.
- The command-line tool is unchanged; the version moves in step with the site.

## 2.12.0

### Site

- The website is redesigned as a flat operator deck. The headline now says what agy-swap does (switch Google Antigravity accounts from the terminal), a pinned terminal follows three scroll steps (accounts, quota, switch), and the page adds a before/after comparison, a legend for the terminal marks, every command grouped by job, direct answers on where tokens are stored, and an install section with all three install paths. English, Thai, Japanese and Chinese stay in step, and `install.html` shares the same components.
- Removed the WebGL scene, smooth-scroll library and decorative layers (gradient text, orbs, dot grid, tilted cards). The main script is about 76 kB.
- The command-line tool is unchanged in this release; the version moves in step with the site.

## 2.11.11

### Fixes

- Credentials: synchronize `~/.gemini/jetski-standalone-oauth-token` alongside `~/.gemini/antigravity-cli/antigravity-oauth-token`, `~/.gemini/oauth_creds.json` and `~/.gemini/google_accounts.json`. Session detection falls back to the standalone token when the primary CLI token is absent or unreadable, and account switches, logouts and login rollbacks update or restore both token files atomically. Refs #48 (reported in #41).

### Development

- `make release-check` now runs `make install` so the local `-dev` binary is installed to `~/.local/bin/agy-swap` before the review gate, and `releasetool bump` updates the release `BuildID` test fixture alongside `Version`.

## 2.11.10

### Fixes

- List: quota pools use compact `Weekly` and `5-Hour` labels with aligned percentages. Reset countdowns now include the local day, date, time and UTC offset; expired snapshots retain their timestamp beside `refresh due`. Saved account order, switch indices, quota colors and the TUI are unchanged. Refs #45.
- Version: CLI and TUI show the stable version without a parenthesized build label; local development builds use the existing `-dev` suffix.

### Development

- Release checks run the Site's configured test script, matching its Node test runner and file isolation.

## 2.11.9

### Fixes

- Login: `agy-swap add` and `a` in the TUI no longer freeze while `agy` asks for input or changes terminal settings. On macOS and Linux, `agy` now owns the terminal while it runs, and agy-swap takes the terminal back as soon as it exits, so the TUI resumes normally after a login. Without a controlling terminal, and on Windows, nothing changes. Reported in #41.

## 2.11.8

### Fixes

- Next: `agy-swap next` (and `n` in the TUI) no longer "rotates" to the account that is already active under the `balanced` policy. It now leaves the active account whenever another account is eligible, and `balanced` picks the eligible account with the most quota. With a single eligible account the behavior is unchanged.

## 2.11.7

### Fixes

- Limits: `limit set` and cooldown parsing reject durations above the 7-day ceiling in every syntax. Oversized values previously overflowed and were accepted as short or negative cooldowns (for example `9223372036854775807` became -1 minute and `213503982336d` became 33 hours).
- Update: for Homebrew installations, `agy-swap update` now confirms that the linked binary reports the new version. When the tap has not refreshed yet, it runs `brew update` once and retries; it no longer reports a successful update that did not happen.

### Site

- The saved or system theme is applied before first paint, so light-theme visitors no longer see a dark flash; without a saved choice the page follows OS theme changes.
- Unknown addresses return a real 404 page instead of the homepage, and `/install` opens the install page.

## 2.11.6

### Fixes

- Install: `go.mod` now declares `go 1.27.2`. `go install github.com/aklkbqx/agy-swap/v2/cmd/agy-swap@latest` applies the go directive (not the toolchain line), so builds from source now use a toolchain with the v2.11.5 standard-library security fixes; with the default `GOTOOLCHAIN=auto` an older go command downloads it. Source builds require Go 1.27.2 or later.
- Release tooling reads the release floor from the newer of the go directive and an optional toolchain line.

### Tests

- End-to-end tests for `switch` (index, email, alias, unknown and ambiguous targets, failed writes keeping the previous session), `remove` (confirmation and vault cleanup), `status`, `list`, `logout` and `add --token -` (verified identity, mismatch and empty input). These commands previously had no coverage.

### Site

- The install page states the Go 1.27.2 requirement in every locale, guarded by a test against `go.mod`. The lazy desktop-only 3D scene chunk has an explicit, tested size limit instead of a recurring build warning.

## 2.11.5

### Security

- Build with Go 1.27.2. The module now declares `toolchain go1.27.2`, which fixes nine standard-library vulnerabilities reachable from the HTTPS client (GO-2026-6603, GO-2026-6605, GO-2026-6607, GO-2026-6608, GO-2026-6610, GO-2026-6611, GO-2026-6612, GO-2026-6613, GO-2026-6617). v2.11.4 executables were built with Go 1.27.0.
- Update `golang.org/x/sys` to v0.49.0 and `golang.org/x/term` to v0.47.0.

### Fixes

- Install: `go install` uses the v2 module path, `go install github.com/aklkbqx/agy-swap/v2/cmd/agy-swap@latest`. The previous unsuffixed path can only resolve v1 versions and installed v1.8.2.
- Release: assets are built only from a clean, annotated `vX.Y.Z` tag with a toolchain at or above the module floor, after govulncheck passes. Every asset's Go build information must record the v2 module path, the exact tag and a patched Go version.
- Development: `make qa` runs golangci-lint (installed, or a pinned version through `go run`); `make vuln` runs govulncheck.

## 2.11.4

### Fixes

- Windows: use ASCII installer output so saved UTF-8 scripts run directly under Windows PowerShell 5.1 without an encoding workaround. Require exact version tokens and a successful binary self-test before replacement. Preserve checksum verification, backup and cleanup behavior; add native installation and rejection regressions.
- Docs: reconcile Issue #4 closure, native v2.11.3 CLI/TUI verification and the active Windows upgrade through current checkpoints and dated historical follow-ups.

## 2.11.3

### Maintenance

- Publish GitHub contributor guidance and product documentation, with Issues/Milestones, resumable checkpoints, reviewed plans and delivery evidence.
- Curate source-linked project knowledge, decisions and historical proposals, including the real logo and v2.11.2 workstream records.
- Add a local read-only release-history helper covering all observed tags/Releases, with pagination, immutable-history validation, private-data exclusion and regression tests.
- Record every historical version without inventing missing Releases or extending security support beyond latest stable.

## 2.11.2

- Windows: prepare compact active-session JSON before switching, preserve refreshed identity and provider fields, reject oversized Credential Manager payloads before writes, and restore exact previous credentials after OAuth-file failures.
- Release: asset builds generate a local Homebrew formula; tap commits and pushes require explicit `make publish-homebrew-tap`. Publication checks the actual remote tip and refuses dirty/diverged taps, unrelated unpublished commits, and invalid or duplicate checksums.
- Tests: make website TUI story snapshots independent of inherited terminal color capabilities while keeping production truecolor support.
- Docs and site: correct active-session versus saved-vault behavior, Doctor's storage/recovery effects in all four locales, local-only AGY Live instructions, and latest-stable-only security support with accurate network use.
- Deployment: publish downloadable release assets before promoting the default installers, and tag both repositories before building the site image.

## 2.11.1

- Installers & updater: detect Homebrew-managed installations, delegate to `brew upgrade`, and clean up shadowing binaries at `~/.local/bin/agy-swap`.
- Installers: default standalone updates to the existing installed binary location instead of forcing `~/.local/bin`.
- Site: redirect `/install.sh` and `/install.ps1` to canonical GitHub sources and update website commands to use domain routes.
- Release: automate Homebrew tap formula synchronization during asset build.

## 2.11.0

- TUI: partitioned accounts into `READY` and `ATTENTION REQUIRED` sections with warning marker (`⚠`), token health status override, and repair guidance in account detail.
- TUI: eliminated stacked duplicate rule under table header when account groups are active, opening sections directly with the `── READY ──` divider.
- TUI: interactive split resizing: drag the vertical divider (`│`) with mouse in real time, double-click divider to reset, and navigate accounts via mouse wheel.
- TUI: discoverable split controls with `+`, `-`, `[`, `]`, `alt-left`, `alt-right`, and footer keycaps indicator `[[]/[]] Resize` in wide layout.
- TUI: 0ms vault token check on startup to partition missing vault credentials without waiting for background network quota refresh.
- TUI: wider default left accounts table proportion (46%, min 48 columns) preventing quota meter truncation on standard viewports.

## 2.10.0

- TUI: unified operator deck redesign with TrueColor palette engine, rounded container borders (`╭─╮`, `╰─╯`), brand pill header, active capsule, and pill keycaps footer.
- TUI: precision Unicode fractional quota meters (`█ ▉ ▊ ▋ ▌ ▍ ▎ ▏`) with clear track and reset countdown.
- TUI: modern chevron cursor (`❯`) for account selection across stacked and wide views.
- TUI: header displays dev build indicator (`v2.10.0-dev`) when running from local dev installation.

## 2.9.1

- Docs and site: saved account tokens are described as they are stored, in `vault.json` (`0600`, not encrypted), with `AGY_SWAP_VAULT=keychain` for the OS credential store. SECURITY.md no longer claims an encrypted fallback.
- CLI: `doctor` reports where saved tokens are stored; messages, the palette, the help screen, and the first-run note say "the vault" instead of "the OS vault".
- Internal: `internal/app` uses `internal/config` for paths and version checks; unused release-client code is removed.
- Site: the live arc draws as a line with a little sparkle over a faint guide, and the session ring appears only on the switch beat.
- AGY Live (local): the browser terminal shrinks to the TUI's 28x12 minimum.
- Backups: encrypted export reports an encoding error instead of writing empty output.
- Internal: golangci-lint now checks the whole codebase with no baseline; dead helpers are removed.

## 2.9.0

- Site: production runs as one Watchtower-managed container. AGY Live runs only locally on loopback with `make live`, and the site image is built, previewed, and published from the local machine.
- Site: the story, commands, and install band follow one scroll clock, with an arc drawn down the margin on every device and a particle arc in 3D on capable desktops. Wheel scrolling is smoothed while touch and reduced motion stay native.
- Site: one floating glass capsule serves as the navbar on both pages. Cards, panels, dropdowns, and buttons share one radius scale and one set of glass tokens. Buttons use a soft orange gradient with white text, and a quiet backdrop fills the empty background. The AU/BU/GU account coins are gone.
- Site: the dev server picks a free port instead of pinning 5173.
- TUI: narrow quota views keep the reset window (`43% · 2d 4h`) and show account health before the email.
- Site: the browser tab title follows the selected language.
- Docs: a shorter README, plus CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, architecture, and development guides.
- Internal: atomic writes, file locks, and the release-check client moved into `internal/store`, `internal/client`, and `internal/config`. Behavior is unchanged.
- Keep local working instructions, planning documents, worktrees, and environment files out of Git while allowing environment examples.
- Stop tracking the previously committed internal implementation plan.
- Site: story terminals size to the drawn screen, so the hero has no empty strip and the pinned terminal keeps its right border.

## 2.8.8

- TUI: toasts and dialogs keep the frame's side borders instead of blanking whole rows; the success toast sits above the status rule.
- TUI: the quota view labels the selected account once instead of repeating its email above the detail block.
- Site: "How it works" pins one terminal while its three steps scroll past, with a progress rule on the current step. Small or short screens keep each step's frame inline.
- Site: removed decorative filler: the gradient route line, hero glow, gradient headline text, and uppercase eyebrow labels. The independence note moved into the hero footnote.
- Site: the feature list now covers the credential vault, rollback-protected switching, `doctor`, and backups instead of repeating the story. Install uses the same heading-and-content layout, with first-run commands shown as code.
- Site: the AGY Live status dot follows the connection state; terminal key labels, footer copy, and install link text are translated in every language; Thai, Japanese, and Chinese quota and account terms are consistent.
- Site: fixed the skip link peeking out at the top edge, the wordmark using an unloaded font, the install page's smaller wordmark, and low-contrast selected dropdown text in light theme.

## 2.8.7

- Use one accessible dropdown component for the navbar language menu and the system picker on both install surfaces.

## 2.8.6

- Render the hero and three account-flow scenes in the same xterm emulator as AGY Live, using frames produced by the Go TUI instead of HTML/CSS terminal facsimiles.
- Give desktop scenes the native wide two-pane layout, retain native tablet and mobile layouts, and keep the terminal frame fully visible at each size.

## 2.8.5

- Align the full install notes header and page sections with the landing page's 1536px container and responsive gutters. Keep install command content at the same readable width on both pages.
- Refresh the native TUI story frames and social preview for this release version.

## 2.8.4

- Replace the handmade hero preview and three story illustrations with responsive snapshots from the actual Go TUI renderer: account list, quota view, and a completed account switch.
- Keep the snapshots in sync with the release version and native demo data through a deterministic Go fixture test. Add a narrower native layout for 320px screens and refresh the social preview.

## 2.8.3

- Replace the decorative account-card image in the homepage hero with a terminal-style account and quota preview using the demo's sample account values.
- Keep the interactive Go TUI in AGY Live, remove the unused hero image, and refresh the social preview to show the terminal.

## 2.8.2

- Self-host Manrope for Latin text, Noto Sans Thai for Thai text, and JetBrains Mono for technical labels and commands; retain system CJK fallbacks.
- Refine the hero and install heading rhythm for Thai and Japanese, keep the narrowest mobile header within the viewport, and update the social preview to match the new type.

## 2.8.1

- Give the account artwork more vertical space on narrow screens so the privacy note remains clear and unobstructed.

## 2.8.0

- Give Google Antigravity a distinct place in the homepage headline while keeping agy-swap's orange action color and independent-product attribution.
- Connect the hero, account, quota-refresh, switching, live demo, and install sections with a scroll-drawn signal path. Keep the route static and subdued when reduced motion is preferred.
- Move the hero explanation and actions beside the headline, add localized account-to-session chapters, and refresh the social preview for the new layout.

## 2.7.0

- Open AGY Live automatically when its section approaches the viewport. Keep the real, isolated Go TUI visible from the first render, with a retry action if the demo gateway is unavailable.
- Replace the standalone install notes markup with a Vite page that shares the site's language, theme, platform picker, and copyable install commands.
- Add a custom agy-swap wordmark, favicon, and account-switch artwork. Give each homepage scene its own restrained two-depth scroll motion, with a static reduced-motion experience.
- Use a warm white primary action in the dark theme and refine the homepage copy and visual hierarchy around the actual account-switch workflow.

## 2.6.1

- Replace the outdated social preview that showed the removed account picker with the current panorama headline and account artwork. Update the homepage and install page image metadata and dimensions.

## 2.6.0

- Run the actual Go TUI in the browser against isolated, disposable demo accounts. Native keys and mobile key buttons switch the sample session; blocked system actions stay inside the demo.
- Replace the browser-side account picker and generated terminal fixtures with a private WebSocket/PTy gateway and lazy terminal renderer.
- Add layered 2D scroll artwork, retain the typing headline, and update the responsive copy and deployment runbook.

## 2.5.1

Use Vite's default localhost binding for site development so another server bound to localhost cannot take precedence over agy-swap on the same port.

## 2.5.0

Make the website demo a direct, clickable 2D account switcher. Visitors can select a sample account, inspect Accounts or Quota, and switch the simulated local session; the terminal remains available for keyboard exploration. Remove the 3D model, textures, and repeated eyebrow labels. Add a reduced-motion-aware typing title, restrained parallax with generated account/session artwork, and an updated social preview. Review the page against product-specific anti-slop criteria and verify desktop, mobile, and Thai layouts.

## 2.4.1

Fix the guided website demo so the terminal visibly updates to the selected
account after the sample switch. The demo now loads the full dashboard fixture
before applying the switch, including when visitors jump directly to step 3.

## 2.4.0

This release makes the website easier to understand and use on a first visit.

- Explain the account-health and local-session workflow in the hero, search metadata, README, and install notes.
- Add a three-step first-run path from installation to the first account switch.
- Replace the long scroll-driven showcase with three named demo steps and a readable mobile preview. Keep the 3D terminal as an optional desktop view.
- Show one install command at a time for the selected operating system and move advanced methods into troubleshooting.
- Align English, Thai, Japanese, and Chinese interface copy with quota snapshots and explicit refresh behavior.

## 2.3.3

This patch keeps the agy CLI from asking for the macOS login password on every launch.

- Publish the shared `gemini` / `antigravity` session item with `/usr/bin/security add-generic-password -U -A`, the same code identity the agy CLI uses to read it.
- Update that item in place. A delete followed by a create drops Always Allow on the next launch.
- Leave the agy-swap account vault on the in-process Keychain path. That vault is a different item and stays with the agy-swap binary.

## 2.3.2

This patch keeps saved account tokens when the vault file is shared by two processes, when it is corrupt, or when a quota refresh fails.

- Serialize `vault.json` with a cross-process file lock and replace it through the existing atomic write path, including on Windows.
- Refuse to rewrite a vault file that cannot be parsed, so one save cannot erase the other accounts.
- Delete a replaced vault entry only after `accounts.json` has been saved. A failed copy or a failed save leaves the previous secret in place.
- Keep the last quota snapshot when `agy-swap next` cannot refresh an account, so a fresh cache can still be selected.
- Store the refreshed token hash and a non-secret access expiry with the account. The detail pane shows that expiry without reading the vault on every frame.
- Reject a refreshed token whose email does not match the account before the account record is changed.
- Pause the TUI key reader before a suspended login command takes stdin, and leave unread input in the terminal buffer.

## 2.3.1

This patch release permanently eliminates repeated macOS Keychain authorization prompts, cascades of permission dialogs on launch, and orphaned keychain items.

- **Deterministic Secret Refs**: Switched account secret references from randomized nonce strings (`account:<email>:<nonce>`) to stable deterministic refs (`account:<email>`), preserving access authorization across token refreshes and account updates.
- **In-Memory Token Hashing**: Added SHA-256 token hashing (`token_hash`) for active account identification in memory, completely eliminating the startup loop over all managed accounts.
- **Secure File Vault (`0600`) & Hybrid Vault**: Added `fileAccountVault` (`~/.gemini/agy-swap/vault.json` with strict `0600` permissions) and hybrid vault fallback for a 100% zero-prompt experience on macOS matching Antigravity and GitHub CLI standards.
- **Keychain Orphan Cleaner**: Added automatic detection and cleanup of obsolete orphaned keychain items left behind by previous versions in macOS `login.keychain-db`.


## 2.3.0

This minor release introduces interactive split resizing for wide TUI displays, persistent layout preferences, and dynamic account table column expansion.

- Interactive split resizing via keyboard (`[` / `]` or `<` / `>` for fine step, `{` / `}` for large step, and `=` to reset).
- Command Palette actions for widening, narrowing, and resetting the pane split.
- Layout split preference persisted in `settings.json` under `ui.split_offset` and configurable via CLI (`agy-swap config set ui.split_offset <val>`).
- Dynamic account column widths with increased health column cap (up to 32 characters) to prevent text truncation on wider displays.

## 2.2.1

This patch release fixes macOS Keychain secret retrieval and deletion by performing all operations in-process through Security.framework instead of external CLI calls, eliminating repeated OS authorization prompts, and adds automated single-command version bumping.

- Read and delete macOS Keychain secrets directly via Security.framework in-process to align code identity and partition lists with writes.
- Prevent recurring system permission prompts on Darwin caused by `/usr/bin/security` partition list mismatches.
- Add opt-in Keychain probe test for real keychain round-trip verification.
- Add automated single-command version bumping (`make bump` and `scripts/bump-version.sh`) across all code, documentation, and fixture surfaces.
- Add `make install` to compile and install directly into active local CLI path.

## 2.2.0

This minor release adds working account policies, reserve accounts, and directory
binding actions while fixing account storage, backup recovery, quota selection,
and browser demo reliability.

- Detect concurrent account creation and stale account/settings writes. Serialize
  history changes and recover interrupted account/settings imports together.
- Share backup validation across CLI and TUI. Reject malformed encrypted data and
  invalid settings before import; preserve credentials during metadata merges.
  New encrypted backups use PBKDF2-SHA256 and AES-GCM; legacy backups remain readable.
- Combine manual, log, and API cooldowns. Require fresh, known available quota for
  automatic selection. Implement sticky, balanced, and round-robin policies,
  profile reserves, notification thresholds, and prompt/recommend/auto bindings.
- Verify new account identity through Google userinfo, persist refreshed OAuth
  credentials even when quota retrieval fails, repair session files on switching,
  and record successful switches. Restore prior sessions when login fails.
- Write macOS credentials through Security.framework without secret process
  arguments. Report plaintext fallback when the OS vault is unavailable.
- Correct family quota metrics, unknown statusline values, refresh behavior,
  child exit codes, and concurrent log attribution. JSON errors exit nonzero.
- Preserve UTF-8 input and letter case in forms; fix Tab and Shift-Tab navigation.
- Verify release asset contents against checksums, prevent unintended downgrades,
  test downloaded binaries before installation, and roll back failed replacement.
- Preserve manual demo navigation during scrolling and ignore stale async loads.
  Pack 51,114 renderer fixtures into shared rows/frames and fit the 3D terminal on
  mobile screens. Update all four site translations and storage claims.
- Consolidate site packaging around Docker/Nginx and local release tooling.

Existing account files and legacy encrypted backups remain supported. Automatic
selection now fails when no account has verified fresh capacity; use an explicit
`switch ACCOUNT` when intentionally overriding that safeguard. OS credential vault
failure can leave tokens in local plaintext files with restricted permissions;
Antigravity's own session files also contain credentials. Backups with secrets
must be encrypted. Older vault entries are retained for backup recovery.
