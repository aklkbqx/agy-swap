# agy-swap

Small account switcher for Google Antigravity CLI (`agy`) on macOS.

## Install

```bash
install -m 755 agy-swap ~/.local/bin/agy-swap
```

## Usage

```bash
agy-swap --list
agy-swap add
agy-swap switch 1
agy-swap status
```

Saved accounts live in `~/.gemini/agy-swap/accounts.json`. Active Antigravity
session data is read from and written to macOS Keychain.
