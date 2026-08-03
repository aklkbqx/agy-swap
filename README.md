# agy-swap

[![Homebrew Formula](https://img.shields.io/badge/homebrew-agy--swap-brightgreen?style=flat-square&logo=homebrew)](https://github.com/aklkbqx/homebrew-tap)
[![GitHub Stars](https://img.shields.io/github/stars/aklkbqx/agy-swap?style=flat-square&logo=github&color=orange)](https://github.com/aklkbqx/agy-swap/stargazers)
[![GitHub License](https://img.shields.io/github/license/aklkbqx/agy-swap?style=flat-square&color=blue)](https://github.com/aklkbqx/agy-swap/blob/main/LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-informational?style=flat-square)](https://github.com/aklkbqx/agy-swap)

A small, dependency-free Python account switcher and quota monitor for Google Antigravity CLI (`agy`).

## Features

- **Seamless Switching:** Switch saved Google Antigravity OAuth sessions instantly without repeated browser logins.
- **Quota Monitoring:** Real-time Google Code Assist quota tracking with remaining percentages, reset timers, and tier badges.
- **Smart Cooldown Detection:** Scans local Antigravity logs for rate-limit cooldowns when Google API usage is unavailable.
- **Dual Interface:** Keyboard-driven Terminal TUI and scriptable CLI subcommands.
- **Secure Credentials:** Owner-only file permissions (`0700`/`0600`) mirrored to native OS keychains (macOS Keychain, Windows Credential Manager, Linux Secret Service).
- **Zero External Dependencies:** Built entirely with Python's standard library.

## Installation

### Homebrew (macOS)

```bash
brew tap aklkbqx/tap
brew install agy-swap
```

### One-line Installer (macOS / Linux)

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash
```

### PyPI / Git

```bash
pip install git+https://github.com/aklkbqx/agy-swap.git
```

### From Source

```bash
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap
./install.sh
```

Windows users can copy `agy-swap` to a directory in `PATH`; Python 3 is required on every platform.

## Interactive TUI

Run `agy-swap` to open the full-screen terminal interface.

| Key | Action |
| :--- | :--- |
| `↑` / `↓` | Move selection |
| `Enter` | Switch to selected account |
| `1`–`9` | Quick select account by number |
| `a` | Add or log in a new account |
| `r` | Force refresh usage quota |
| `t` | Toggle manual tier label fallback |
| `d` / `Backspace` | Delete selected account |
| `q` / `Esc` | Quit |

## CLI Commands

```bash
# Account Management
agy-swap add
printf '%s' "$TOKEN" | agy-swap add --token -
agy-swap list
agy-swap remove user@gmail.com
agy-swap status
agy-swap logout

# Switching & Auto-Rotation
agy-swap switch user@gmail.com
agy-swap switch 2
agy-swap next
agy-swap next --family claude

# Quota & Limits
agy-swap limits
agy-swap limits --refresh --verbose
agy-swap limit set 1 6d --group claude
agy-swap limit set user@gmail.com reset --group claude

# Self-Update
agy-swap update
```

## Security

- OAuth tokens are stored locally in owner-only files (`0700` directory, `0600` file) and synced to native OS keychains.
- Refresh tokens are sent exclusively to Google's OAuth endpoints.
- Concurrent operations use cross-process file locks to prevent state corruption.
- Never commit or share `~/.gemini/agy-swap/accounts.json`.

## License

MIT — see [LICENSE](LICENSE).
