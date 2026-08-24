<div align="center">

# agy-swap

### High-Performance Account Switcher & Quota Monitor for Google Antigravity (`agy`)

[![Release](https://img.shields.io/github/v/release/aklkbqx/agy-swap?color=FF891A&label=Release&style=flat-square)](https://github.com/aklkbqx/agy-swap/releases)
[![Live Demo](https://img.shields.io/badge/Live%20Demo-agy--swap.aklkbqx.com-38BDF8?style=flat-square&logo=google-chrome&logoColor=white)](https://agy-swap.aklkbqx.com)
[![Go Report Card](https://goreportcard.com/badge/github.com/aklkbqx/agy-swap)](https://goreportcard.com/report/github.com/aklkbqx/agy-swap)
[![License: MIT](https://img.shields.io/badge/License-MIT-34C759.svg?style=flat-square)](LICENSE)
[![Platform Support](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-blue?style=flat-square&logo=apple&logoColor=white)](https://github.com/aklkbqx/agy-swap/releases)
[![Arch](https://img.shields.io/badge/Arch-arm64%20%7C%20x86__64-orange?style=flat-square)](https://github.com/aklkbqx/agy-swap/releases)

<p align="center">
  <b>Seamlessly switch accounts, monitor real-time AI quotas, and manage Antigravity developer sessions with instant sub-millisecond native Go performance.</b>
</p>

<p align="center">
  <a href="https://agy-swap.aklkbqx.com"><strong>🌐 Explore Live Interactive Web Demo & 3D Terminal Simulator »</strong></a>
</p>

---

</div>

## 📑 Table of Contents

- [🌐 Live Web Demo](#-live-web-demo)
- [✨ Key Features](#-key-features)
- [🚀 Quick Start & Installation](#-quick-start--installation)
  - [Homebrew (macOS)](#homebrew-macos)
  - [macOS / Linux Automated Installer](#macos--linux-automated-installer)
  - [Windows PowerShell](#windows-powershell)
  - [Build from Source](#build-from-source)
- [🖥️ Interactive Terminal UI (TUI)](#️-interactive-terminal-ui-tui)
  - [Keyboard Shortcuts Cheat Sheet](#keyboard-shortcuts-cheat-sheet)
- [⌨️ CLI Command Reference](#️-cli-command-reference)
  - [Account Management & Fast Switching](#account-management--fast-switching)
  - [Quota Tracking & Cooldowns](#quota-tracking--cooldowns)
  - [Profiles, Aliases & Project Bindings](#profiles-aliases--project-bindings)
  - [Diagnostics, Statusline & Metrics](#diagnostics-statusline--metrics)
  - [Backups & OS Keyring Migration](#backups--os-keyring-migration)
- [🔒 Security & Architecture](#-security--architecture)
- [🛠️ Development & Testing](#️-development--testing)
- [📄 License](#-license)

---

## 🌐 Live Web Demo

Experience `agy-swap` directly in your browser without installing anything:

👉 **[https://agy-swap.aklkbqx.com](https://agy-swap.aklkbqx.com)**

- **Full Live TUI Emulation**: Experience exact terminal behavior driven by over 51,000 Go state engine fixtures.
- **Interactive 3D Perspective Mode**: Real-time 3D hardware terminal visualization with mouse parallax.
- **Responsive Layout Engine**: Live preview adapting across mobile (320px), tablet stacked (640px), and desktop wide (1440px) split-pane modes.

---

## ✨ Key Features

- ⚡ **Sub-Millisecond Native Binary**: Pure Go rewrite replacing legacy Python runtime. Instant startup, 0ms CLI lag, and zero runtime dependencies.
- 🔄 **Transactional Session Switching**: Atomic snapshots with automatic rollback safeguards. Never corrupt your Antigravity tokens or workspace state.
- 📊 **Real-Time Quota & Cooldown Tracking**: Proactively tracks Gemini and third-party model limits, reset countdowns, and rate limits.
- 🔐 **Native OS Vault Integration**: Securely integrates with macOS Keychain, Windows Credential Manager, and Linux Secret Service (`libsecret` / DBus).
- 🎨 **Adaptive Terminal Interface**: Fluid responsive terminal layout with 256-color support, search filter, and Command Palette (`Ctrl-K` / `:`).
- 🩺 **Built-In System Doctor**: `agy-swap doctor` verifies permissions, OAuth tokens, endpoint health, and config integrity in one command.
- 📁 **Project & Directory Auto-Binding**: Automatically switch or recommend the right account when entering specific Git repositories or directories.
- 🛡️ **100% Private & Telemetry-Free**: Completely local operation. Zero data collection, analytics, or remote telemetry.

---

## 🚀 Quick Start & Installation

### Homebrew (macOS)

```bash
brew install aklkbqx/agy-swap/agy-swap
```

### macOS / Linux Automated Installer

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash
```

### Windows PowerShell

Run in PowerShell (verifies SHA-256 integrity before installing):

```powershell
irm https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.ps1 | iex
```

*Pre-built standalone binaries for all architectures (`darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`, `windows/arm64`) are also available directly on [GitHub Releases](https://github.com/aklkbqx/agy-swap/releases).*

### Build from Source

Requirements: Go 1.26 or later.

```bash
# Clone repository
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap

# Compile native binary with build provenance
go build -trimpath -ldflags "-s -w -X main.version=2.1.2 -X main.buildID=local" -o agy-swap ./cmd/agy-swap

# Verify installation
./agy-swap version
```

---

## 🖥️ Interactive Terminal UI (TUI)

Launch the full visual manager by running `agy-swap` with no arguments:

```bash
agy-swap
```

The TUI intelligently detects your terminal dimensions:
- **Wide Mode (≥96 cols)**: Displays a split-pane layout with the active account list on the left and comprehensive health metrics on the right.
- **Stacked Mode (64–95 cols)**: Vertically arranged panels optimized for mid-sized terminals.
- **Compact Mode (<64 cols)**: Minimalist stream-lined interface ideal for split terminal panes and mobile SSH.

### Keyboard Shortcuts Cheat Sheet

| Key | Action | Description |
| :--- | :--- | :--- |
| `↑` / `↓` or `j` / `k` | **Navigate** | Move highlighted cursor up or down |
| `Enter` | **Switch Account** | Activate highlighted account session immediately |
| `1` – `9` | **Quick Jump** | Directly jump and switch to account by index |
| `n` | **Cycle Next** | Instantly rotate to next available account |
| `/` | **Search** | Fuzzy search accounts by name or email |
| `Ctrl-K` or `:` | **Command Palette** | Access all operations, views, and commands |
| `r` | **Refresh Quota** | Pull live quota data from endpoints in background |
| `p` / `h` / `s` | **Switch View** | Jump to Profiles (`p`), History (`h`), Settings (`s`) |
| `o` / `b` / `v` | **Tool Views** | Doctor Health Check (`o`), Backup (`b`), Quota Overview (`v`) |
| `a` | **Add Account** | Connect a new Google account via OAuth browser flow |
| `d` / `Delete` | **Delete** | Remove account from local store (with confirmation) |
| `e` | **Edit** | Edit tags, aliases, or active view items |
| `m` | **Migrate Vault** | Migrate plain tokens into OS Keychain/Vault |
| `u` | **Update** | Self-update to latest release with checksum verification |
| `?` | **Help** | Toggle in-app keyboard shortcut cheat sheet |
| `q` / `Esc` | **Quit / Back** | Close overlay or exit application |

---

## ⌨️ CLI Command Reference

`agy-swap` can be automated seamlessly in shell scripts, CI pipelines, and terminal prompts.

### Account Management & Fast Switching

```bash
# Add a new account (interactive OAuth browser login)
agy-swap add

# Add via stdin token for automated environments
printf '%s' "$TOKEN" | agy-swap add --token -

# List all configured accounts with status & quota health
agy-swap list
agy-swap list --verbose

# Switch active account by email or numeric index
agy-swap switch dev@company.com
agy-swap switch 2

# Rotate to next account with available quota
agy-swap next

# Rotate specifically within a model family (e.g. claude / gemini)
agy-swap next --family claude

# Display current active session status
agy-swap status

# Log out current active session
agy-swap logout
```

### Quota Tracking & Cooldowns

```bash
# Check quota usage across all accounts
agy-swap limits

# Force live endpoint refresh with verbose breakdown
agy-swap limits --refresh --verbose

# Manually record or reset model cooldowns
agy-swap limit set 1 6h --group claude
agy-swap limit set dev@company.com reset --group claude
```

### Profiles, Aliases & Project Bindings

```bash
# Create custom account aliases
agy-swap alias set work dev@company.com
agy-swap alias set personal user@gmail.com

# Create custom profiles
agy-swap profile set work-profile work --family gemini

# Bind current project directory to a specific profile or account
agy-swap bind set /path/to/my-repo work --mode recommend

# Get smart recommendation for current directory
agy-swap recommend
```

### Diagnostics, Statusline & Metrics

```bash
# Run comprehensive diagnostic health check
agy-swap doctor

# Starship / Tmux / Zsh statusline prompt integration
agy-swap statusline install
agy-swap statusline render < statusline.json

# Local Prometheus-compatible metrics endpoint
agy-swap metrics prometheus

# Run Antigravity CLI immediately after verifying session
agy-swap run now
agy-swap run now --account dev@company.com -- -p "Audit codebase"
```

### Backups & OS Keyring Migration

```bash
# Securely migrate plaintext tokens to macOS Keychain / Linux Secret Service / Windows Vault
agy-swap account migrate --force

# Export portable metadata backup
agy-swap backup export --output agy-swap-backup.json

# Export encrypted full backup including secrets with passphrase
printf '%s' 'YourStrongPassphrase' | agy-swap backup export --include-secrets --passphrase-stdin --output agy-secrets.json

# Restore from backup file
agy-swap backup import agy-swap-backup.json --merge
```

---

## 🔒 Security & Architecture

`agy-swap` is engineered from the ground up with a security-first posture:

- **OS Keyring Integration**: Bearer tokens are stored in the host OS credential manager ([macOS Keychain](https://support.apple.com/guide/security/keychain-data-protection-secb0694df1a/web), [Windows Credential Manager](https://learn.microsoft.com/en-us/windows/win32/secauthn/credentials-management), or [Linux FreeDesktop Secret Service](https://specifications.freedesktop.org/secret-service/)).
- **Atomic File Transactions**: Configuration writes use advisory filesystem locks (`flock` on Unix, `LockFileEx` on Windows) and write-to-temp-then-rename semantics to prevent race conditions.
- **Memory Safety & Token Sanitization**: OAuth tokens are scrubbed from CLI logs and terminal outputs. Tokens are accepted securely via stdin streams.
- **Zero Inbound/Outbound Telemetry**: No tracking, phone-home beacons, or external telemetry servers. Network calls are strictly limited to Google OAuth and quota endpoints.
- **Strict TLS Verification**: All API interactions enforce strict TLS certificate validation.

---

## 🛠️ Development & Testing

```bash
# Clone the repository
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap

# Run complete Go test suite with race detector
go test -race ./...

# Run Go linter & static analysis
go vet ./...

# Run performance benchmarks
go test -bench . ./internal/app

# Build and test web application
cd site
npm install
npm test
npm run build
```

---

## 📄 License

Distributed under the **MIT License**. See [LICENSE](LICENSE) for full details.

---

<div align="center">
  <sub>Crafted with precision by <b><a href="https://github.com/aklkbqx">@aklkbqx</a></b> • Designed for the Google Antigravity developer ecosystem.</sub>
</div>
