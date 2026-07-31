# agy-swap

[![Homebrew Formula](https://img.shields.io/badge/homebrew-agy--swap-brightgreen?style=flat-square&logo=homebrew)](https://github.com/aklkbqx/homebrew-tap)
[[![Live Showcase](https://img.shields.io/badge/Live_Showcase-aklkbqx.github.io%2Fagy--swap-orange?style=flat-square&logo=googlechrome)](https://aklkbqx.github.io/agy-swap)
[![GitHub Stars](https://img.shields.io/github/stars/aklkbqx/agy-swap?style=flat-square&logo=github&color=orange)](https://github.com/aklkbqx/agy-swap/stargazers)
[![GitHub License](https://img.shields.io/github/license/aklkbqx/agy-swap?style=flat-square&color=blue)](https://github.com/aklkbqx/agy-swap/blob/main/LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-informational?style=flat-square)](https://github.com/aklkbqx/agy-swap)

> **A minimal, lightning-fast multi-account switcher & quota tracker (TUI) for Google Antigravity CLI (`agy`) on macOS, Linux, and Windows.**

*Created & maintained by [@aklkbqx](https://github.com/aklkbqx)*

---

## 🌟 Key Features

- ⚡ **Instant Account Switching**: Switch active Google Antigravity (`agy`) profiles in seconds without re-authenticating in your browser.
- 🎨 **Minimalist TUI**: Ultra-clean terminal interface with zero clutter, clean hotkeys, and smooth micro-animations.
- ⏱️ **Rate Limit & Quota Tracker**: Automatically parses local Antigravity logs to track 5-hour rolling limits, weekly quotas, and exact cooldown resets.
- 🔐 **Secure Keychain Integration**: Works directly with macOS Keychain, Linux Secret Service, and Windows Credential Manager.
- 🚀 **Zero Dependencies**: Pure Python standard library implementation. No `pip install` required.

---

## 📦 Installation

### Option 1: Via Homebrew (Recommended for macOS)
```bash
brew tap aklkbqx/tap
brew install agy-swap
```

### Option 2: Curl One-liner (macOS & Linux)
```bash
curl -fsSL https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash
```

### Option 3: Manual Clone (macOS, Linux & Windows)
```bash
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap
# On macOS / Linux:
./install.sh
# On Windows:
# Copy 'agy-swap' to any folder in your PATH (e.g., C:\Windows\System32 or custom bin)
```

---

## 🚀 Quick Start

Launch the interactive dashboard:
```bash
agy-swap
```

### ⌨️ Interactive TUI Navigation & Hotkeys:

| Key | Action |
| :--- | :--- |
| `↑` / `↓` | Navigate accounts and actions |
| `Enter` | Switch active account immediately |
| `[1-9]` | Jump directly to account number |
| `a` | Add / Login new Google account |
| `s` | Cycle sort order (*Active First, Cooldown, A-Z*) |
| `d` / `Backspace` | Delete highlighted account |
| `l` | View Quota Limit Resets & Expiry details |
| `t` | Set custom rate limit cooldown timer |
| `i` | Show current active session details |
| `u` | Auto-update `agy-swap` to latest version |
| `x` | Logout active session |
| `q` / `Esc` | Quit dashboard |

---

## 🛠️ CLI Command Reference

`agy-swap` can also be used directly from non-interactive terminal scripts or automated workflows:

```bash
# Switch to account by index number or email substring
agy-swap switch 2
agy-swap switch user@gmail.com

# List all managed accounts
agy-swap list

# Display quota limits and token expiry
agy-swap limits

# Set manual rate limit cooldown (e.g. 5h, 4h30m, reset)
agy-swap limit set 1 5h

# Show active session details
agy-swap status

# Update switcher script to latest version
agy-swap update
```

---

## 🔄 Updating agy-swap

Update to the latest version anytime:
```bash
agy-swap update
```
*(Or press **`u`** inside the TUI dashboard)*

---

## 📄 License

Distributed under the [MIT License](LICENSE).
