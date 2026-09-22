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
  <b>Manage Google Antigravity accounts, inspect provider quota snapshots, and switch the shared local session from a native terminal interface.</b>
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
  - [macOS / Linux Automated Installer](#macos--linux-automated-installer)
  - [Go Install (`go install`)](#go-install-go-install)
  - [Windows PowerShell](#windows-powershell)
  - [Build from Source](#build-from-source)
  - [Corporate Networks & SSL Proxy Troubleshooting](#corporate-networks--ssl-proxy-troubleshooting)
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

- **Interactive TUI Preview**: Explore sample accounts through over 51,000 Go-rendered fixtures. The browser does not connect to your credentials or reproduce every native operation.
- **Interactive 3D Perspective Mode**: Real-time 3D hardware terminal visualization with mouse parallax.
- **Responsive Layout Engine**: Live preview adapting across mobile (320px), tablet stacked (640px), and desktop wide (1440px) split-pane modes.

---

## ✨ Key Features

- ⚡ **Native Go CLI**: No application server. macOS uses system frameworks; Linux vault support needs `secret-tool` and an unlocked Secret Service.
- 🔄 **Session Switching**: Locks and rollback protect session updates. Backup imports also journal their recovery state for interrupted restores.
- 📊 **Real-Time Quota & Cooldown Tracking**: Proactively tracks Gemini and third-party model limits, reset countdowns, and rate limits.
- 🔐 **Native OS Vault Integration**: Securely integrates with macOS Keychain, Windows Credential Manager, and Linux Secret Service (`libsecret` / DBus).
- 🎨 **Adaptive Terminal Interface**: Fluid responsive terminal layout with 256-color support, search filter, and Command Palette (`Ctrl-K` / `:`).
- 🩺 **Built-In System Doctor**: `agy-swap doctor` verifies permissions, OAuth tokens, endpoint health, and config integrity in one command.
- 📁 **Project Bindings**: `run now` resolves the current directory to a profile. Choose recommend, prompt, or explicitly enabled auto mode.
- 🛡️ **100% Private & Telemetry-Free**: Completely local operation. Zero data collection, analytics, or remote telemetry.

---

## 🚀 Quick Start & Installation

Shell and PowerShell installers verify downloaded release binaries against SHA-256 checksums. Go builds use the Go module toolchain.

### macOS / Linux Automated Installer

Single command installer with intelligent System CA certificate auto-detection:

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash
```

### Windows PowerShell

Run in PowerShell (enforces TLS 1.2+ and verifies SHA-256 integrity before installation):

```powershell
irm https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.ps1 | iex
```

### Go Install (`go install`)

If you already have Go installed (Go 1.26+):

```bash
go install github.com/aklkbqx/agy-swap/cmd/agy-swap@latest
```

*Pre-built standalone binaries for all architectures (`darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`, `windows/arm64`) with checksums are also available directly on [GitHub Releases](https://github.com/aklkbqx/agy-swap/releases).*

### Build from Source

Requirements: Go 1.26 or later. macOS source builds require Xcode Command Line Tools and `CGO_ENABLED=1` for Keychain writes.

```bash
# Clone repository
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap

# Compile native binary with build provenance
go build -trimpath -ldflags "-s -w -X main.version=2.3.3 -X main.buildID=local" -o agy-swap ./cmd/agy-swap

# Verify installation
./agy-swap version
```

### Corporate Networks & SSL Proxy Troubleshooting

If you are behind an enterprise firewall, VPN, or corporate proxy (e.g. Zscaler, Fortinet, Netskope) performing TLS inspection, `curl` may report `SSL certificate problem: self signed certificate`:

- **Automated Installer with Proxy Tolerance** (SHA-256 cryptographic verification remains strictly active):
  ```bash
  curl -k -fsSL https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | AGY_SWAP_INSECURE=1 bash
  ```
- **Alternative (Offline / Direct Go Build)**:
  Install directly via `go install` or build from source using the instructions above.

---

## 🖥️ Interactive Terminal UI (TUI)

Launch the full visual manager by running `agy-swap` with no arguments:

```bash
agy-swap
```

The TUI intelligently detects your terminal dimensions:
- **Wide Mode (≥92 terminal columns and ≥18 rows)**: Displays a split-pane layout with the active account list on the left and comprehensive health metrics on the right.
- **Stacked Mode (≥64 columns and ≥16 rows when wide mode does not fit)**: Vertically arranged panels optimized for mid-sized terminals.
- **Compact Mode (<64 columns or <16 rows)**: Minimalist stream-lined interface ideal for split terminal panes and mobile SSH.

### Keyboard Shortcuts Cheat Sheet

| Key | Action | Description |
| :--- | :--- | :--- |
| `↑` / `↓` or `j` / `k` | **Navigate** | Move highlighted cursor up or down |
| `[` / `]` or `<` / `>` | **Resize Split** | Narrow or widen the accounts pane split in Wide Mode |
| `=` | **Reset Split** | Reset layout pane split to default width |
| `Enter` | **Switch Account** | Activate highlighted account session immediately |
| `1` – `9` | **Quick Jump** | Select an account by index; press Enter to switch |
| `n` | **Cycle Next** | Refresh and rotate to an eligible account |
| `/` | **Search** | Filter accounts by name or email |
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

# Set primary/reserve accounts and a ranking policy
agy-swap profile set work-profile work --family gemini --policy sticky --reserve personal --threshold 15

# Bind a directory to an existing profile
agy-swap bind set /path/to/my-repo work-profile --mode recommend

# Recommend for that profile, or resolve the working directory when running
agy-swap recommend --profile work-profile --refresh
agy-swap run now
```

### Diagnostics, Statusline & Metrics

```bash
# Run comprehensive diagnostic health check
agy-swap doctor

# Starship / Tmux / Zsh statusline prompt integration
agy-swap statusline install
agy-swap statusline render < statusline.json

# Print a local Prometheus-compatible metrics snapshot (no HTTP server)
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
printf '%s' "$BACKUP_PASSPHRASE" | agy-swap backup export --include-secrets --passphrase-stdin --output agy-secrets.json

# Restore from backup file
agy-swap backup import agy-swap-backup.json --merge
```

---

## Selection and session behavior

`recommend` ranks eligible accounts first. Eligibility requires a quota snapshot no older than two minutes, positive remaining capacity at or above the policy reserve, and no matching manual or log cooldown. Without `--family`, the most restrictive model group and window govern readiness. `recommend --apply`, `next`, and bound runs refresh before selecting; failed refreshes cannot authorize a switch. Use `switch ACCOUNT` for an explicit override.

`sticky` prefers the active account (or profile primary), `balanced` favors remaining capacity, and `round-robin` advances through saved order. Profile reserves are fallback candidates when the primary is unavailable. `next` advances rotation even with sticky policy. `watch --account` limits polling to that account; `watch --profile` uses its primary and notification threshold.

Bindings take effect in `run now`, not when a shell merely changes directory. `--account` overrides a binding. Recommend mode prints a suggestion, prompt mode asks in a terminal, and auto mode requires `policy.allow_apply=true`. These profiles update one shared local Antigravity session; they do not isolate simultaneous processes. Targets launch executables and do not translate Google credentials into credentials for other providers.

## 🔒 Security & Architecture

`agy-swap` is engineered from the ground up with a security-first posture:

- **OS Keyring Integration**: Bearer tokens are stored in the host OS credential manager ([macOS Keychain](https://support.apple.com/guide/security/keychain-data-protection-secb0694df1a/web), [Windows Credential Manager](https://learn.microsoft.com/en-us/windows/win32/secauthn/credentials-management), or [Linux FreeDesktop Secret Service](https://specifications.freedesktop.org/secret-service/)).
- **Storage boundaries**: Legacy or fallback account tokens and active Antigravity OAuth files can be plaintext protected by local permissions. Vault fallback is reported. macOS releases write secrets directly to Security.framework without passing secrets in process arguments. Old vault references may remain to support local backup recovery.
- **Identity**: Adding a credential requires a verified email returned by Google userinfo. Decoded JWT claims are only local identity hints, not signature verification.
- **Portable backups**: Metadata exports omit secrets and machine-local vault references. Merge keeps an existing credential when the backup has none. Secret exports use AES-GCM with PBKDF2-HMAC-SHA256 (600,000 iterations); legacy encrypted backups remain readable. Verify and import use the same validation. Imports use a recovery journal; do not delete it after an interrupted restore.
- **History**: Switch and quota events are local JSONL records with locking and retention. This is not an immutable audit ledger.
- **Atomic File Transactions**: Configuration writes use advisory filesystem locks (`flock` on Unix, `LockFileEx` on Windows) and write-to-temp-then-rename semantics to prevent race conditions.
- **Memory Safety & Token Sanitization**: OAuth tokens are scrubbed from CLI logs and terminal outputs. Tokens are accepted securely via stdin streams.
- **No CLI Telemetry**: The CLI contacts Google for account and quota data and GitHub for releases and updates.
- **TLS Verification**: Verification is enabled by default. Prefer a trusted corporate CA bundle; the explicit insecure option disables certificate authentication.

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
npm ci
bun run build
bun run test
```

---

## 📄 License

Distributed under the **MIT License**. See [LICENSE](LICENSE) for full details.

---

<div align="center">
  <sub>Crafted with precision by <b><a href="https://github.com/aklkbqx">@aklkbqx</a></b> • Designed for the Google Antigravity developer ecosystem.</sub>
</div>
