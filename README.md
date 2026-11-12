<div align="center">

# agy-swap

### Switch Google Antigravity accounts in the CLI and the IDE from one terminal

[![Release](https://img.shields.io/github/v/release/aklkbqx/agy-swap?color=FF891A&label=Release&style=flat-square)](https://github.com/aklkbqx/agy-swap/releases)
[![Website](https://img.shields.io/badge/Website-agy--swap.aklkbqx.com-38BDF8?style=flat-square)](https://agy-swap.aklkbqx.com)
[![Sponsor](https://img.shields.io/badge/Sponsor-GitHub%20Sponsors-EA4AAA?style=flat-square&logo=githubsponsors&logoColor=white)](https://github.com/sponsors/aklkbqx)
[![License: MIT](https://img.shields.io/badge/License-MIT-34C759.svg?style=flat-square)](LICENSE)
[![Platform Support](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-blue?style=flat-square&logo=apple&logoColor=white)](https://github.com/aklkbqx/agy-swap/releases)
[![Arch](https://img.shields.io/badge/Arch-arm64%20%7C%20x86__64-orange?style=flat-square)](https://github.com/aklkbqx/agy-swap/releases)

<br/>

<a href="https://agy-swap.aklkbqx.com/#showcase">
  <img src="docs/assets/switch.gif" alt="agy-swap switching between two demo accounts in the terminal" width="800" />
</a>

<p>
  <a href="#install">Install</a> ·
  <a href="#first-run">First run</a> ·
  <a href="#switch-the-cli-and-the-ide-together">IDE switching</a> ·
  <a href="#using-the-terminal-ui">Terminal UI</a> ·
  <a href="#commands">Commands</a> ·
  <a href="#walkthrough">Walkthrough</a> ·
  <a href="#security-and-privacy">Security</a> ·
  <a href="#support">Support</a>
</p>

</div>

agy-swap keeps several Google Antigravity accounts on one machine, shows each account's remaining quota and reset time, and switches both the `agy` CLI and the Antigravity IDE (on macOS and Linux) from one terminal. Everything runs locally on your machine. Saved tokens stay in `vault.json` (`0600`), which is not encrypted, or in your OS credential store when you set `AGY_SWAP_VAULT=keychain`.

## Install

Shell and PowerShell installers verify downloaded release binaries against SHA-256 checksums. The [install page](https://agy-swap.aklkbqx.com/install.html) shows the command for your operating system, and pre-built binaries for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`, and `windows/arm64` are on [GitHub Releases](https://github.com/aklkbqx/agy-swap/releases).

### macOS and Linux

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash
```

### Homebrew (macOS and Linux)

```bash
brew install aklkbqx/agy-swap/agy-swap
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.ps1 | iex
```

### Go install

```bash
go install github.com/aklkbqx/agy-swap/v2/cmd/agy-swap@latest
```

The module requires Go 1.27.4 or newer (with the default `GOTOOLCHAIN=auto`, an older `go` command downloads it first). Published binaries use the same toolchain floor.

### Build from source

Requirements: Go 1.27.4 or later. macOS source builds require Xcode Command Line Tools and `CGO_ENABLED=1` for Keychain writes.

```bash
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap
go build -trimpath -ldflags "-s -w -X main.version=2.23.5 -X main.buildID=local" -o agy-swap ./cmd/agy-swap
./agy-swap version
```

### On a restricted network

Corporate networks that inspect TLS (Zscaler, Fortinet, Netskope), block GitHub downloads, or redirect DNS can use these environment variables with `install.sh` and `agy-swap update` (the PowerShell installer supports `AGY_SWAP_RELEASE_MIRROR` and `AGY_SWAP_INSECURE`):

| Variable | What it does |
| :--- | :--- |
| `AGY_SWAP_CA_BUNDLE` | Trust a custom root certificate PEM file (also reads `CURL_CA_BUNDLE`). Safest option for TLS inspection |
| `AGY_SWAP_RELEASE_MIRROR` | Download release binaries and `checksums.txt` from an internal mirror URL |
| `AGY_SWAP_DOH_URL` | Override the DNS-over-HTTPS resolver used after a TLS error (defaults to Cloudflare `1.1.1.1`, then Google `8.8.8.8`, ignoring private IP answers) |
| `AGY_SWAP_GO_FALLBACK=1` | Build the release with `go install` after any download error (needs Go on your PATH; does not run on Windows) |
| `AGY_SWAP_NO_GO_FALLBACK=1` | Disable the automatic `go install` fallback on TLS certificate errors |
| `AGY_SWAP_INSECURE=1` | Skip TLS certificate checks in the installer as a last resort (still verifies SHA-256 checksums) |

Trust your company's root certificate:

```bash
curl -fsSL --cacert /path/to/company-ca.pem https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh \
  | AGY_SWAP_CA_BUNDLE=/path/to/company-ca.pem bash
```

Or install from an internal release mirror:

```bash
AGY_SWAP_RELEASE_MIRROR=https://mirror.example.com/agy-swap bash install.sh
```

The installers expect release files at the mirror's top level. `agy-swap update` tries GitHub first, then `vX.Y.Z/` on the mirror, then the mirror's top level. When a TLS certificate error blocks the download and Go is on your PATH, `agy-swap update` falls back to `go install` (this does not run on Windows; Go verifies the source module checksum, though the compiled binary is not compared against the release checksum).

## First run

1. Install the Google Antigravity CLI (`agy`) if needed: `agy-swap install-agy` (or let `agy-swap add` offer to install it).
2. Save an account via browser sign-in: `agy-swap add`.
3. Open the terminal UI: `agy-swap`.
4. Highlight an account, press `Enter`, and choose `Switch`. The `agy` CLI and, on macOS and Linux, the Antigravity IDE switch to that account immediately.

## Switch the CLI and the IDE together

One switch moves the `agy` CLI, the Antigravity IDE (on macOS and Linux), and Jetski to the same Google account without signing out of the IDE or opening a browser. Windows switches the CLI session only.

| Component | How agy-swap updates it |
| :--- | :--- |
| Session files | Writes the active OAuth session under `~/.gemini` (`0600`) where the CLI, the IDE, and Jetski read it |
| System keychain | Mirrors the active session into the system keychain (`/usr/bin/security` on macOS, `secret-tool` on Linux when installed) |
| IDE account label | Updates the signed-in email shown in the IDE's `state.vscdb` when `sqlite3` is installed |
| CLI & IDE wrapper | Wraps `~/.local/bin/agy` and `~/.gemini/bin/agy` (plus `AGY_SWAP_AGY_BIN` or `install-agy --dir`) so `agy-swap --sync-session` runs before and after `agy`. The original binary is kept at `~/.local/libexec/agy` or `.agy.real` beside it (symlinks keep their target) |
| IDE version metadata | Updates size and modification time in `agy.version.json` so the IDE accepts the wrapper while keeping the version and checksum intact |
| Background service | Writes `~/.gemini/antigravity-cli/cache/agy-swap-hub-reload` and stops the background `agy` service with `pkill` so the wrapper restarts it under the new account (active IDE agent work stops on switch) |

- When the CLI or the IDE refreshes a token, agy-swap saves the updated token back to the matching account on the next run.
- If the IDE or `agy` is signed in to an unsaved account, agy-swap leaves it alone and `agy-swap add` offers to save it. After `agy-swap logout`, agy-swap does not sign you back in until you pick an account.
- To remove the wrapper, uninstall agy-swap first, then move `.agy.real` (or `~/.local/libexec/agy`) back over `agy` or reinstall `agy`.

## Using the terminal UI

Run `agy-swap` with no arguments to open the interactive operator deck:

```bash
agy-swap
```

### Account status and quota pools

Each account row shows a status badge and two quota bars: `Gem` (Gemini) and `C/G` (Claude and GPT), each displaying the tighter of its weekly and 5-hour windows. Accounts on Pro plans with a separate weekly Gemini 4 Argon pool show an additional Argon panel in the detail pane.

| Status badge | Meaning |
| :--- | :--- |
| `READY` | Both Gemini and Claude/GPT pools have more than 30% remaining (green) |
| `LOW` | At least one pool is at 30% or less (yellow; 10% or less turns red) |
| `GEMINI ONLY` / `CLAUDE/GPT ONLY` | The other model pool is exhausted (`0%`) |
| `LIMITED` | Both main model pools are exhausted |
| `NO TOKEN` | Account has no saved credential token |
| `VERIFY` | Google requires account verification before use (press `w` to open the verification URL) |

### Responsive layout and live refresh

- Wide (`120+` cols): account list on the left, side-by-side pool panels with large digits on the right (up to 160 cols).
- Medium (`92–119` cols): account list on the left, stacked pool panels on the right.
- Stacked (`64–91` cols) and compact (`28–63` cols): single-column layout with selected account gauges below the list.
- Live quota refresh: polls the active account every 20 seconds, inactive accounts every 2 minutes, right after a pool reset time passes, and within ~10 seconds of Antigravity log activity. Code Assist project IDs are cached for 15 minutes.

### Environment variables

| Variable | Effect |
| :--- | :--- |
| `AGY_SWAP_REDUCED_MOTION=1` | Disable UI transitions and animations |
| `AGY_SWAP_ASCII=1` / `0` | Force ASCII borders on or off |
| `AGY_SWAP_MOUSE=0` | Disable mouse clicks, wheel scrolling, and drag resizing |
| `AGY_SWAP_QUOTA_DEBUG=1` | Print quota endpoint diagnostics with `agy-swap list --refresh` |
| `AGY_SWAP_USER_AGENT` | Override the HTTP User-Agent sent to Google quota endpoints |

### Keyboard and mouse shortcuts

| Key / Input | Action |
| :--- | :--- |
| `↑` / `↓` or `j` / `k` | Move selection up or down |
| `1` – `9` | Jump to account slot 1–9 |
| `Enter` | Open the switch confirmation dialog for the highlighted account |
| `n` | Refresh quotas and switch to the next eligible account |
| `r` | Refresh every account's quota now |
| `/` | Filter accounts by name or email |
| `Ctrl+K` or `:` | Open the command palette |
| `p` / `h` / `s` / `o` / `b` | Open Profiles, History, Settings, Doctor, or Backup (`Esc` or `b` returns to Dashboard) |
| `a` / `d` / `e` | Add an account (`a`), delete the selected account (`d`), or edit / export encrypted backup (`e`) |
| `w` | Open Google's verification page for an account marked `VERIFY` |
| `m` / `u` | Migrate legacy tokens to the vault (`m`) or update `agy-swap` (`u`) |
| `[` / `]` and `=` | Resize the split pane width/height, or press `=` to reset |
| `?` | Show or hide the shortcut guide (includes the GitHub Sponsors link) |
| `q` / `Esc` | Close an active dialog/sub-view, or press `q` on the Dashboard to quit |
| Mouse click & drag | Click an account to select (click again to switch), click footer keycaps or dialog buttons, drag the pane divider to resize (double-click to reset), or scroll with the wheel |

## Commands

Every TUI action is available as a CLI command for scripts and shell prompts.

| Command | What it does |
| :--- | :--- |
| `add` | Sign in with Google and save the account |
| `list` | List saved accounts with status and quota health |
| `switch <account>` | Switch the local session by email, alias, or slot number |
| `next` | Refresh quotas, then switch to the next eligible account |
| `status` | Show the active account |
| `limits` | Show quota and local reserve limits for every account (`--refresh` updates first) |
| `limit <set\|clear>` | Set or clear a manual cooldown or reserve limit for one account |
| `verify <account>` | Open Google's verification page for an account marked `VERIFY` |
| `profile` / `bind` | Define account profiles and bind project directories to them |
| `recommend` | Explain which account is safest to use now |
| `run now` | Launch the configured CLI with the recommended or bound account |
| `install-agy` | Download and install the Google Antigravity CLI (`agy`), verifying its SHA-512 checksum |
| `doctor` | Check local storage, credentials, vault, and platform readiness |
| `statusline` | Render or install a compact statusline for Starship, tmux, or zsh |
| `backup` | Export or import accounts (with optional AES-GCM secret encryption) |
| `update` | Update `agy-swap` to the latest release |
| `donate` | Print the GitHub Sponsors link, or open it in your browser with `--open` |

### Accounts and switching

```bash
# Add an account through browser sign-in, or pass a token on stdin
agy-swap add
printf '%s' "$TOKEN" | agy-swap add --token -

# List saved accounts with status and quota pools
agy-swap list
agy-swap list --verbose

# Switch by email, alias, or 1-based slot number
agy-swap switch dev@company.com
agy-swap switch 2

# Refresh and switch to the next eligible account (optionally for one model family)
agy-swap next
agy-swap next --family claude

# Show the active account or sign out
agy-swap status
agy-swap logout
```

`list` keeps accounts in saved order so numeric slots match `switch`. Reset times show both the countdown and local timestamp (`resets in 2d 19h 6m (Fri 16 Oct 14:06 +0700)`), or `refresh due` once the timestamp has passed.

### Quota and manual cooldowns

```bash
# Show cached quota for every account, or refresh from Google first
agy-swap limits
agy-swap limits --refresh --verbose

# Set or clear a manual cooldown for a specific model family
agy-swap limit set 1 6h --group claude
agy-swap limit clear dev@company.com --group claude
```

### Profiles, aliases, and project bindings

```bash
# Assign short aliases
agy-swap alias set work dev@company.com
agy-swap alias set personal user@example.com

# Create a profile with a primary account, reserve fallback, and policy
agy-swap profile set work-profile work --family gemini --policy sticky --reserve personal --threshold 15

# Bind a repository path to a profile and run
agy-swap bind set /path/to/my-repo work-profile --mode recommend
agy-swap recommend --profile work-profile --refresh
agy-swap run now
```

### Diagnostics, statusline, backups, and support

```bash
# Check storage, credentials, and platform readiness
agy-swap doctor

# Add agy-swap to Starship, tmux, or zsh prompts, or export Prometheus metrics
agy-swap statusline install
agy-swap metrics prometheus

# Export accounts (metadata only, or encrypted with a passphrase on stdin)
agy-swap backup export --output agy-swap-backup.json
printf '%s' "$BACKUP_PASSPHRASE" | agy-swap backup export --include-secrets --passphrase-stdin --output agy-secrets.json
agy-swap backup import agy-swap-backup.json --merge

# Print or open the GitHub Sponsors page
agy-swap donate
agy-swap donate --open
```

## Walkthrough

The clips below use fictional `@example.invalid` accounts and are recorded from the real binary with `make media` ([VHS](https://github.com/charmbracelet/vhs)).

### 1. Switch accounts
Run `agy-swap` to open the operator deck, move through accounts with `↑`/`↓`, inspect weekly and 5-hour pools, and press `Enter` twice to switch.

<p align="center">
  <img src="docs/assets/switch.gif" alt="Switching accounts in the operator deck" width="750" />
</p>

### 2. Keyboard guide and command palette
Press `?` to inspect all keyboard shortcuts and the support link, or press `Ctrl+K` (or `:`) to search actions in the command palette.

<p align="center">
  <img src="docs/assets/dashboard.gif" alt="Keyboard guide and action palette" width="750" />
</p>

### 3. CLI commands for scripts
Run the same workflow non-interactively in scripts:

```bash
agy-swap list
agy-swap status
agy-swap recommend
agy-swap switch beta@example.invalid
agy-swap donate
```

<p align="center">
  <img src="docs/assets/cli.gif" alt="list, status, recommend, switch, and donate in a terminal" width="750" />
</p>

## How account selection works

### Eligibility rules
`recommend` ranks eligible accounts first. An account is eligible when its quota snapshot is no older than 2 minutes, its remaining quota is above `0%` and at or above the policy reserve threshold, and no manual or log cooldown is active. Without `--family`, the tighter of the Gemini and Claude/GPT pools governs readiness; `--family argon` checks the Argon pool (falling back to the Gemini pool on Ultra plans). `recommend --apply`, `next`, and bound runs refresh quotas before selecting.

### Ranking policies

| Policy | Behavior |
| :--- | :--- |
| `sticky` | Keeps the active account (or profile primary) until it becomes ineligible; `next` still rotates away from the active account |
| `balanced` | Picks the eligible account with the highest remaining quota |
| `round-robin` | Advances through eligible accounts in saved order |

### Project bindings
Directory bindings apply when running `agy-swap run now` (overridden by `--account`). `recommend` mode prints a suggestion, `prompt` mode asks before switching, and `auto` mode switches automatically when `policy.allow_apply=true`. Profiles update the shared local Antigravity session and do not isolate concurrent processes.

## Security and privacy

### Token storage and active session files
- Saved account tokens live in `vault.json` (`0600`) inside the agy-swap config directory. The file is not encrypted. Set `AGY_SWAP_VAULT=keychain` to store saved tokens in [macOS Keychain](https://support.apple.com/guide/security/keychain-data-protection-secb0694df1a/web), [Windows Credential Manager](https://learn.microsoft.com/en-us/windows/win32/secauthn/credentials-management), or [Linux Secret Service](https://specifications.freedesktop.org/secret-service/), or `AGY_SWAP_VAULT=file` to force the file vault.
- If the selected vault cannot be written, the token stays as plaintext `token_data` in `accounts.json` (`0600`) and `agy-swap doctor` reports `vault_migration` until you run `agy-swap account migrate --force`.
- Switching writes the active session to `~/.gemini` (`0600`) and, on macOS and Linux, mirrors it to the system keychain (`/usr/bin/security` on macOS passes the token in process arguments while running; saved-account keychain mode uses native `Security.framework`).

### Backups, locking, and redaction
- Metadata exports omit secrets. Encrypted exports (`--include-secrets`) use AES-GCM with PBKDF2-HMAC-SHA256 (600,000 iterations) and keep a recovery journal during import.
- Config writes use advisory file locks (`flock` on Unix, `LockFileEx` on Windows) and atomic temp-file renames. OAuth tokens are redacted from CLI logs and terminal output.

### Network endpoints
agy-swap sends no telemetry. It contacts only the endpoints required for the command you run:

| Endpoint | When contacted |
| :--- | :--- |
| Google OAuth & Code Assist APIs | Sign-in, token refresh, verified email check, and quota polling |
| GitHub Releases | `agy-swap update` and `agy-swap doctor --refresh` |
| `agy-swap.aklkbqx.com` | Fallback version check during `agy-swap update` when GitHub is unreachable |
| DNS-over-HTTPS (`1.1.1.1`, `8.8.8.8`, or `AGY_SWAP_DOH_URL`) | Host lookup retry only after a TLS error |
| Custom mirror, Go proxy, or Google CLI download | Only when `AGY_SWAP_RELEASE_MIRROR`, the `go install` fallback, or `install-agy` is used |

TLS certificates are always verified unless you explicitly set `AGY_SWAP_INSECURE=1` (installer) or `AGY_SWAP_INSECURE_TLS=1` (binary); prefer `AGY_SWAP_CA_BUNDLE`. To report a security issue, see [SECURITY.md](SECURITY.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), [docs/development.md](docs/development.md) for build and test workflows, and [docs/architecture.md](docs/architecture.md) for package structure.

## Support

agy-swap is free and open-source software under the MIT License. If it saves you time across accounts, you can support ongoing development and release infrastructure via [GitHub Sponsors](https://github.com/sponsors/aklkbqx):

```bash
# Print the GitHub Sponsors URL
agy-swap donate

# Open GitHub Sponsors in your browser
agy-swap donate --open
```

You can also open the support link from the terminal UI via `?` (Keyboard guide) or `Ctrl+K` (`Sponsor & donate`).

## License

Distributed under the MIT License. See [LICENSE](LICENSE).
