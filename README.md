<div align="center">

# agy-swap

### See which Google Antigravity account is ready, then switch from your terminal

[![Release](https://img.shields.io/github/v/release/aklkbqx/agy-swap?color=FF891A&label=Release&style=flat-square)](https://github.com/aklkbqx/agy-swap/releases)
[![Website](https://img.shields.io/badge/Website-agy--swap.aklkbqx.com-38BDF8?style=flat-square)](https://agy-swap.aklkbqx.com)
[![License: MIT](https://img.shields.io/badge/License-MIT-34C759.svg?style=flat-square)](LICENSE)
[![Platform Support](https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-blue?style=flat-square&logo=apple&logoColor=white)](https://github.com/aklkbqx/agy-swap/releases)
[![Arch](https://img.shields.io/badge/Arch-arm64%20%7C%20x86__64-orange?style=flat-square)](https://github.com/aklkbqx/agy-swap/releases)

</div>

agy-swap keeps several Google Antigravity accounts on one machine. It shows each account's remaining quota and when it resets, then switches the shared local Antigravity session to the account you pick. Everything runs locally; saved tokens stay in a private `vault.json` readable only by your user, or in your OS credential store with `AGY_SWAP_VAULT=keychain`.

## Install

Shell and PowerShell installers verify downloaded release binaries against SHA-256 checksums. Go builds use the Go module toolchain.

macOS and Linux:

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.ps1 | iex
```

With Go 1.26 or later:

```bash
go install github.com/aklkbqx/agy-swap/cmd/agy-swap@latest
```

Pre-built binaries for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, `windows/amd64`, and `windows/arm64`, with checksums, are on [GitHub Releases](https://github.com/aklkbqx/agy-swap/releases). The [install page](https://agy-swap.aklkbqx.com/install.html) shows one command at a time for your operating system.

### Build from source

Requirements: Go 1.26 or later. macOS source builds require Xcode Command Line Tools and `CGO_ENABLED=1` for Keychain writes.

```bash
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap
go build -trimpath -ldflags "-s -w -X main.version=2.11.1 -X main.buildID=local" -o agy-swap ./cmd/agy-swap
./agy-swap version
```

### Behind a TLS-inspecting proxy

If an enterprise firewall, VPN, or proxy (for example Zscaler, Fortinet, or Netskope) inspects TLS, `curl` may report `SSL certificate problem: self signed certificate`. The installer can skip certificate checks for the download while still verifying the SHA-256 checksum:

```bash
curl -k -fsSL https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | AGY_SWAP_INSECURE=1 bash
```

You can also use `go install` or build from source as shown above.

## First run

1. Run `agy-swap add` and complete the browser sign-in to save an account.
2. Run `agy-swap` to open the terminal view. Check the active account and quota snapshot before switching.
3. Select an account and press Enter to update the shared local Antigravity session. Processes already running may need to reload credentials.

## Using the terminal UI

Run `agy-swap` with no arguments to open the terminal UI:

```bash
agy-swap
```

The layout follows your terminal size:

- **Wide (at least 92 columns and 18 rows):** the account list on the left, health details on the right.
- **Stacked (at least 64 columns and 16 rows, when wide does not fit):** the panels stacked vertically.
- **Compact (fewer than 64 columns or 16 rows):** one column for split panes and SSH from a phone.

| Key | Action |
| :--- | :--- |
| `↑` / `↓` or `j` / `k` | Move the highlight up or down |
| `[` / `]` or `<` / `>` | Narrow or widen the account pane in wide layout |
| `=` | Reset the pane split |
| `Enter` | Switch to the highlighted account |
| `1` – `9` | Select an account by number; press Enter to switch |
| `n` | Refresh and move to the next eligible account |
| `/` | Filter accounts by name or email |
| `Ctrl-K` or `:` | Open the command palette |
| `r` | Refresh quota in the background |
| `p` / `h` / `s` | Open Profiles, History, or Settings |
| `o` / `b` / `v` | Open Doctor, Backup, or the quota overview |
| `a` | Add a Google account through the browser sign-in |
| `d` / `Delete` | Remove the account from the local store, after confirmation |
| `e` | Edit tags, aliases, or the selected item |
| `m` | Move legacy tokens from accounts.json into the vault |
| `u` | Update agy-swap to the latest release, checking its checksum |
| `?` | Show or hide the shortcut list |
| `q` / `Esc` | Close an overlay or quit |

## Commands

Every TUI action is also a command, so you can script it.

| Command | What it does |
| :--- | :--- |
| `add` | Sign in with Google and save the account |
| `list` | List saved accounts with status and quota health |
| `switch` | Switch the local session to an account by email, alias, or number |
| `next` | Refresh, then switch to the next account with quota left |
| `status` | Show the active account |
| `limits` | Show quota for every account |
| `profile` / `bind` | Define profiles and bind directories to them |
| `recommend` | Explain which account is safest to use now |
| `run now` | Launch the configured CLI with the right account |
| `doctor` | Check storage, credentials, vault, and platform readiness |
| `statusline` | Render or install a statusline for your prompt |
| `backup` | Export or import accounts |
| `update` | Update agy-swap to the latest release |

### Accounts and switching

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

### Quota and cooldowns

```bash
# Check quota usage across all accounts
agy-swap limits

# Force live endpoint refresh with verbose breakdown
agy-swap limits --refresh --verbose

# Manually record or reset model cooldowns
agy-swap limit set 1 6h --group claude
agy-swap limit set dev@company.com reset --group claude
```

### Profiles, aliases, and project bindings

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

### Diagnostics, statusline, and metrics

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

### Backups and credential migration

```bash
# Move legacy plaintext tokens from accounts.json into the vault
agy-swap account migrate --force

# Export portable metadata backup
agy-swap backup export --output agy-swap-backup.json

# Export encrypted full backup including secrets with passphrase
printf '%s' "$BACKUP_PASSPHRASE" | agy-swap backup export --include-secrets --passphrase-stdin --output agy-secrets.json

# Restore from backup file
agy-swap backup import agy-swap-backup.json --merge
```

## How account selection works

`recommend` ranks eligible accounts first. Eligibility requires a quota snapshot no older than two minutes, positive remaining capacity at or above the policy reserve, and no matching manual or log cooldown. Without `--family`, the most restrictive model group and window govern readiness. `recommend --apply`, `next`, and bound runs refresh before selecting; failed refreshes cannot authorize a switch. Use `switch ACCOUNT` for an explicit override.

`sticky` prefers the active account (or profile primary), `balanced` favors remaining capacity, and `round-robin` advances through saved order. Profile reserves are fallback candidates when the primary is unavailable. `next` advances rotation even with sticky policy. `watch --account` limits polling to that account; `watch --profile` uses its primary and notification threshold.

Bindings take effect in `run now`, not when a shell merely changes directory. `--account` overrides a binding. Recommend mode prints a suggestion, prompt mode asks in a terminal, and auto mode requires `policy.allow_apply=true`. These profiles update one shared local Antigravity session; they do not isolate simultaneous processes. Targets launch executables and do not translate Google credentials into credentials for other providers.

## Security and privacy

- **Saved account tokens:** stored in `vault.json` in the agy-swap config directory, readable only by your user (`0600`). The file is not encrypted. Set `AGY_SWAP_VAULT=keychain` to keep them in the OS credential store instead ([macOS Keychain](https://support.apple.com/guide/security/keychain-data-protection-secb0694df1a/web), [Windows Credential Manager](https://learn.microsoft.com/en-us/windows/win32/secauthn/credentials-management), or [Linux Secret Service](https://specifications.freedesktop.org/secret-service/)); `AGY_SWAP_VAULT=file` uses the file only. `agy-swap doctor` shows which one is in use.
- **Active session:** switching writes the Antigravity session where Antigravity reads it, in its OS credential store entry and OAuth files under `~/.gemini`, protected by local permissions. In keychain mode, macOS releases write through Security.framework, not through process arguments. Old vault references may remain to support local backup recovery.
- **Identity:** Adding a credential requires a verified email returned by Google userinfo. Decoded JWT claims are only local identity hints, not signature verification.
- **Portable backups:** Metadata exports omit secrets and machine-local vault references. Merge keeps an existing credential when the backup has none. Secret exports use AES-GCM with PBKDF2-HMAC-SHA256 (600,000 iterations); legacy encrypted backups remain readable. Verify and import use the same validation. Imports use a recovery journal; do not delete it after an interrupted restore.
- **History:** Switch and quota events are local JSONL records with locking and retention. This is not an immutable audit ledger.
- **Atomic writes:** Configuration writes use advisory file locks (`flock` on Unix, `LockFileEx` on Windows) and write-to-temp-then-rename.
- **Token handling:** OAuth tokens are scrubbed from CLI logs and terminal output. Tokens can be passed on stdin.
- **No telemetry:** The CLI contacts Google for account and quota data and GitHub for releases and updates.
- **TLS verification:** Verification is enabled by default. Prefer a trusted corporate CA bundle; the explicit insecure option disables certificate authentication.

To report a vulnerability, see [SECURITY.md](SECURITY.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), [docs/development.md](docs/development.md) for build and test commands, and [docs/architecture.md](docs/architecture.md) for how the code is organized.

## License

Distributed under the MIT License. See [LICENSE](LICENSE).
