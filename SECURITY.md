# Security Policy

`agy-swap` manages credentials, OAuth tokens, and session switches for Google Antigravity. We take the security and privacy of user authentication data extremely seriously.

---

## Supported Versions

Security fixes are released only for the latest stable version. When a new stable version is published, earlier versions are no longer supported; upgrade before reporting a problem against an older version.

| Version | Supported          |
| ------- | ------------------ |
| Latest stable release | :white_check_mark: |
| All earlier releases | :x: |

---

## Architecture & Credential Protection

`agy-swap` stores account and session data locally and does not collect telemetry:

1. **Where tokens live**:
   - **Saved account tokens** go to `vault.json` in the agy-swap config directory (`~/.gemini/agy-swap/`), written atomically with `0600` permissions. The file is **not encrypted**: anything that can read files as your user can read it.
   - **`AGY_SWAP_VAULT=keychain`** keeps them in the OS credential store instead: macOS Keychain through `Security.framework`, Windows Credential Manager, or the Linux Secret Service through `secret-tool`. `AGY_SWAP_VAULT=file` uses the file only. By default the OS store is read only for tokens saved by older versions.
   - **The active Antigravity session** is written where Antigravity reads it: its OS credential store entry and the OAuth files under `~/.gemini`. On macOS this uses `/usr/bin/security`, with the session credential in command arguments, for Antigravity compatibility. Saved vault writes in keychain mode use `Security.framework`; the vault setting does not select the active-session backend.
   - If the vault cannot be written (for example `secret-tool` fails in keychain mode), the token stays as plaintext `token_data` in `accounts.json` (`0600`) and `agy-swap doctor` warns with `vault_migration`; run `agy-swap account migrate --force` once the vault works.
   - `agy-swap doctor` shows which vault is in use.

2. **Atomic Session Swapping**:
   - Session files are swapped using atomic file replacement operations (`os.Rename` on POSIX systems, `MoveFileEx` with replace flags on Windows) guarded by system file locks (`flock` / `LockFileEx`) to prevent partial writes, corruption, or race conditions.

3. **No Telemetry & Network Use**:
   - `agy-swap` does not collect telemetry, analytics, or crash reports.
   - Google requests support OAuth login, token refresh, account identity verification, and quota checks. Login receives the browser callback through a local loopback listener.
   - Quota polling and token refresh can run automatically while a user-invoked TUI, watch, recommendation, or bound-run session is active.
   - GitHub requests support release checks, download verification, and self-updates. These requests are operational network traffic, not telemetry.

---

## Reporting a Vulnerability

If you discover a potential security vulnerability in `agy-swap`, please report it responsibly:

> [!IMPORTANT]
> **Do not open a public GitHub issue for security vulnerabilities.**

Report it privately through GitHub: open the repository's **Security** tab and choose **Report a vulnerability** (https://github.com/aklkbqx/agy-swap/security/advisories/new). Please do not open a public issue.

### What to Include in Your Report
To help us triage and resolve the issue quickly, please include:
- A clear description of the vulnerability.
- Steps to reproduce the issue or a minimal proof-of-concept.
- Affected platforms (macOS, Linux, Windows) and `agy-swap --version`.
- Any potential mitigation or remediation steps if known.

### Response Timeline
- **Initial Response**: Within 48 hours acknowledging receipt of your report.
- **Assessment & Triage**: Within 5 business days with status and remediation plan.
- **Fix & Disclosure**: We will coordinate with you on releasing a patch and crediting your contribution responsibly.
