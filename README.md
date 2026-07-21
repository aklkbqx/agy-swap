# agy-swap

A beautiful interactive account switcher (TUI) for Google Antigravity CLI (`agy`) on macOS.

![TUI Demo Screenshot](https://raw.githubusercontent.com/aklkbqx/agy-swap/main/assets/demo_placeholder.png) <!-- Link to screenshot if added -->

## Features

- **Interactive TUI Dashboard**: A full-screen Terminal User Interface (similar to `vim` or `htop`) with smooth navigation and a beautiful color theme.
- **Flicker-Free & Safe Redraws**: Uses the macOS Alternate Screen Buffer to eliminate layout corruption and flicker from wrapped terminal text.
- **Single-Key Actions**: 
  - `Arrow Keys (↑/↓)` to scroll through accounts.
  - `Enter` to switch to the selected account immediately.
  - `d` / `x` / `Backspace` / `Delete` to remove the highlighted account with inline confirmation.
  - `a` to auto-add the current Antigravity session from Keychain (fetches user profile via Google API in the background).
- **Backwards Compatible**: Still supports standard command-line flags and subcommands for scripting and automation.

## Quick Installation

Simply clone the repository and run the installation script:

```bash
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap
./install.sh
```

The script copies the executable to `~/.local/bin/agy-swap` and guides you on adding it to your shell `PATH`.

## CLI Usage (Scripting)

If you prefer scripting or non-interactive usage, all commands remain available:

```bash
# Add current active session
agy-swap add

# List all saved accounts
agy-swap list

# Switch to account by index or email substring
agy-swap switch <index_or_email>

# Show active session details
agy-swap status

# Logout of active session
agy-swap logout
```

## Session Data

Saved accounts live in `~/.gemini/agy-swap/accounts.json`. Active Antigravity session data is safely updated inside your macOS Keychain.
