# agy-swap

[![Homebrew Formula](https://img.shields.io/badge/homebrew-agy--swap-brightgreen?style=flat-square&logo=homebrew)](https://github.com/aklkbqx/homebrew-tap)
[![GitHub Stars](https://img.shields.io/github/stars/aklkbqx/agy-swap?style=flat-square&logo=github&color=orange)](https://github.com/aklkbqx/agy-swap/stargazers)
[![GitHub License](https://img.shields.io/github/license/aklkbqx/agy-swap?style=flat-square&color=blue)](https://github.com/aklkbqx/agy-swap/blob/main/LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-informational?style=flat-square)](https://github.com/aklkbqx/agy-swap)

A small, dependency-free Python account switcher and quota monitor for Google Antigravity CLI (`agy`).

## What it does

- Switches saved Google Antigravity OAuth sessions without repeating browser login.
- Provides a keyboard-driven terminal interface and scriptable subcommands.
- Fetches the same Google Code Assist quota summary used by `agy`, including real remaining percentages, reset times and subscription tier.
- Keeps model-specific cooldowns from recent local Antigravity logs as a fallback when Google usage cannot be refreshed.
- Stores saved profiles in an owner-only local file (`0700` directory, `0600` file) and mirrors the active session to the native OS credential store when available.
- Uses only the Python standard library; Linux native credential integration additionally requires `secret-tool`.

Google quota is grouped into Gemini and Claude/GPT model families. Paid accounts currently expose weekly and 5-hour buckets; Free accounts expose weekly buckets only. `agy-swap` renders only buckets returned by Google, so it never invents a 5-hour Free limit or a synthetic `100% Ready` state.

Quota snapshots are cached for five minutes. If refresh fails, the previous snapshot is preserved and local cooldown evidence remains available as a clearly labelled fallback.

## Installation

### Homebrew (recommended on macOS)

```bash
brew tap aklkbqx/tap
brew install agy-swap
```

### Verified installer (macOS and Linux)

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash
```

The installer downloads the version-pinned script, verifies its SHA-256 checksum and installs it atomically to `~/.local/bin/agy-swap`. Re-run the same command to update to the version published by the latest release.

### Clone

```bash
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap
./install.sh
```

Windows users can copy `agy-swap` to a directory in `PATH`; Python 3 is required on every platform.

## Interactive controls

Run `agy-swap` to open the TUI.

| Key | Action |
| :--- | :--- |
| `↑` / `↓` | Move between accounts and actions |
| `Enter` | Switch to the selected account |
| `1`–`9` | Select an account by number |
| `a` | Add or log in to an account |
| `r` | Refresh quota for every saved account with progress |
| `t` | Cycle the fallback manual tier label when Google usage is unavailable |
| `d` / `Backspace` | Delete the selected account after confirmation |
| `q` / `Esc` | Quit |

## CLI commands

```bash
agy-swap list
agy-swap switch 2
agy-swap switch user@gmail.com
agy-swap next
agy-swap next --family claude
agy-swap limits
agy-swap limits --refresh
agy-swap limits --verbose
agy-swap limit set 1 6d --group claude
agy-swap limit set user@gmail.com 4h30m --group gemini
agy-swap limit set user@gmail.com 2h --group gpt
agy-swap limit set 1 reset --group claude
agy-swap status
agy-swap logout
```

Importing a token is stdin-only so it does not appear in shell history or process arguments:

```bash
printf '%s' "$TOKEN" | agy-swap add --token -
```

Failed credential operations return a non-zero exit status, and ambiguous email substrings are rejected.

The first v1.6 load preserves legacy v1.5 quota fields under `legacy_quota` but ignores them because those records did not identify their source or exact model. Subscription tier labels are synced from Google with paid tier taking precedence over the accompanying Free tier; a manual label is used only when no Google snapshot exists.

`next` uses Google quota first and falls back to observed cooldowns. Use `--family claude`, `--family gemini`, or `--family gpt` to route against the matching Google quota group. `limits --verbose` shows quota sync age and sanitized fallback-log provenance without exposing full local paths.

Log evidence is reconciled by account and canonical model ID. A newer confirmed model response removes an older cooldown, while failed streams remain limited even though Antigravity logs both success and failure as `Stream completed`.

## Security notes

- Saved account data contains reusable OAuth credentials. `agy-swap` protects the directory and files with owner-only permissions and writes them atomically with a protected backup.
- Quota refresh sends the saved refresh token only to Google's OAuth endpoint, verifies the returned account identity, then requests quota from Google's Code Assist API.
- `logout` removes the active OS credential and all active OAuth files, but intentionally keeps saved profiles so they can be selected later.
- Session-changing commands use a cross-process lock so simultaneous `switch`, `next`, `logout`, and login operations cannot overwrite one another.
- Do not share `~/.gemini/agy-swap/accounts.json` or its `.bak` file.

## License

MIT — see [LICENSE](LICENSE).
