# agy-swap

A minimal interactive account switcher (TUI) for Google Antigravity CLI (`agy`) on macOS, Linux, and Windows.

[![Homebrew Formula](https://img.shields.io/badge/homebrew-agy--swap-brightgreen?style=flat-square&logo=homebrew)](https://github.com/aklkbqx/homebrew-tap)
[![GitHub Stars](https://img.shields.io/github/stars/aklkbqx/agy-swap?style=flat-square&logo=github&color=orange)](https://github.com/aklkbqx/agy-swap/stargazers)
[![GitHub License](https://img.shields.io/github/license/aklkbqx/agy-swap?style=flat-square&color=blue)](https://github.com/aklkbqx/agy-swap/blob/main/LICENSE)

*Created and maintained by [@aklkbqx](https://github.com/aklkbqx)*

## Install

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
# On macOS/Linux:
./install.sh
# On Windows:
# Just copy 'agy-swap' script to any directory in your PATH (as agy-swap or agy-swap.py)
```

## Update

To update to the latest version at any time, simply run:
```bash
agy-swap update
```
*(Or choose **`[🔄] Update Switcher Script`** inside the TUI dashboard)*

## Quick Start

Simply run the command to launch the interactive dashboard:
```bash
agy-swap
```

### Keyboard Shortcuts:
- `↑ / ↓` : Move selection
- `Enter` : Switch to selected account
- `d` / `x` / `Backspace` : Delete highlighted account
- `a` : Auto-add your current active `agy` session to the list
- `q` / `Esc` : Exit TUI

---

## Mini Tutorial

### Step 1: Add your accounts
First, log in using the official Antigravity CLI:
```bash
agy login
```
Then, open the switcher and press **`a`** (or select **`[+] Add Current Session`**). This saves your logged-in session to the list.

*Repeat this step for other accounts by logging out (`agy logout`) and logging in to different emails.*

### Step 2: Swap accounts instantly
Whenever you need to change accounts, simply run:
```bash
agy-swap
```
Scroll to your target email and press **`Enter`**. You are switched instantly without needing to re-authenticate in the browser!
