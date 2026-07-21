# agy-swap

A minimal interactive account switcher (TUI) for Google Antigravity CLI (`agy`) on macOS.

## Install

### Option 1: Via Homebrew (Recommended)
```bash
brew tap aklkbqx/tap
brew install agy-swap
```

### Option 2: Curl One-liner (No Clone Required)
```bash
curl -fsSL https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash
```

### Option 3: Manual Clone
```bash
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap
./install.sh
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
