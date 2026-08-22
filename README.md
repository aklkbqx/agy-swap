# agy-swap

A fast, native account switcher and quota monitor for Google Antigravity CLI (`agy`), rewritten in Go.

## Highlights

- Native binaries for macOS, Linux, and Windows on amd64 and arm64
- Instant cached startup with a non-blocking terminal UI
- Google Code Assist quota tracking and local cooldown detection
- Transactional session switching with native OS credential stores
- Backward-compatible with the v1 `accounts.json`, OAuth files, commands, and shortcuts
- No Python runtime and no telemetry

## Install

### Homebrew (macOS)

```bash
brew install aklkbqx/agy-swap/agy-swap
```

### macOS / Linux installer

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash
```

### Windows

Download the matching `.exe` from [GitHub Releases](https://github.com/aklkbqx/agy-swap/releases), verify it against `checksums.txt`, rename it to `agy-swap.exe`, and place it in `PATH`.

### Build from source

```bash
go build -ldflags "-X main.version=2.1.1" -o agy-swap ./cmd/agy-swap
```

Go 1.26 or later is required only when building from source.

## Interactive TUI

Run `agy-swap` in a terminal. The Operator Deck adapts automatically: wide
terminals show the account list beside the selected account's health details;
smaller terminals stack the same information or switch to a compact list.
Cached accounts are painted immediately, while quota refreshes run in the
background.

| Key | Action |
| :--- | :--- |
| `↑` / `↓` or `j` / `k` | Move selection |
| `Enter` | Switch to the selected account |
| `1`–`9` | Jump to an account |
| `/` | Search by name or email |
| `a` | Add or log in to an account |
| `r` | Force-refresh quota data |
| `n` | Choose the next available account |
| `t` | Toggle the manual tier fallback |
| `d` / `Backspace` / `Delete` | Delete the selected account |
| `l` | Log out the active session |
| `?` | Open the keyboard guide |
| `q` / `Esc` | Quit (or close the current overlay) |

Search and destructive actions use focused overlays so the current account is
never changed accidentally. Set `AGY_SWAP_REDUCED_MOTION=1` for a static TUI
when motion should be minimized.

## CLI

```bash
# Account management
agy-swap add
printf '%s' "$TOKEN" | agy-swap add --token -
agy-swap list
agy-swap list --verbose
agy-swap remove user@gmail.com
agy-swap status
agy-swap logout

# Switching and rotation
agy-swap switch user@gmail.com
agy-swap switch 2
agy-swap next
agy-swap next --family claude

# Quota and cooldowns
agy-swap limits
agy-swap limits --refresh --verbose
agy-swap limit set 1 6d --group claude
agy-swap limit set user@gmail.com reset --group claude

# Native binary update
agy-swap update
```

The legacy top-level flags (`--add`, `--list`, `--next`, `--switch-to`, `--remove`, `--status`, and `--logout`) remain supported.

## Data compatibility and security

- Existing data stays at `~/.gemini/agy-swap/accounts.json`; account order and unknown legacy fields are preserved.
- A disposable `log-cache-v1.json` makes repeated log scans fast. It contains cooldown evidence but never OAuth tokens.
- Files use owner-only permissions on Unix and atomic, locked writes on every supported OS.
- The active credential remains in macOS Keychain, Windows Credential Manager, or Linux Secret Service using the same identifiers as Antigravity.
- Tokens are accepted only on stdin and are never placed in process arguments.
- TLS verification is strict by default. `AGY_SWAP_INSECURE_TLS=1` explicitly enables the legacy insecure fallback and prints a warning.
- Switching snapshots all credential files and restores the previous session if any write fails.

Rolling back to v1.8.2 does not require a data migration because v2 continues to write quota schema 2.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
go test -bench . ./internal/app
```

## License

MIT — see [LICENSE](LICENSE).
