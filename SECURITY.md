# Security Policy

`agy-swap` manages credentials, OAuth tokens, and session switches for Google Antigravity. We take the security and privacy of user authentication data extremely seriously.

---

## Supported Versions

Security fixes are actively released for the latest stable release line:

| Version | Supported          |
| ------- | ------------------ |
| 2.3.x   | :white_check_mark: |
| < 2.3.0 | :x:                |

---

## Architecture & Credential Protection

`agy-swap` is architected as a **100% local, privacy-first tool**:

1. **Where tokens live**:
   - **Saved account tokens** go to `vault.json` in the agy-swap config directory (`~/.gemini/agy-swap/`), written atomically with `0600` permissions. The file is **not encrypted**: anything that can read files as your user can read it.
   - **`AGY_SWAP_VAULT=keychain`** keeps them in the OS credential store instead: macOS Keychain through `Security.framework`, Windows Credential Manager, or the Linux Secret Service through `secret-tool`. `AGY_SWAP_VAULT=file` uses the file only. By default the OS store is read only for tokens saved by older versions.
   - **The active Antigravity session** is written where Antigravity reads it: its OS credential store entry and the OAuth files under `~/.gemini`.
   - `agy-swap doctor` shows which vault is in use.

2. **Atomic Session Swapping**:
   - Session files are swapped using atomic file replacement operations (`os.Rename` on POSIX systems, `MoveFileEx` with replace flags on Windows) guarded by system file locks (`flock` / `LockFileEx`) to prevent partial writes, corruption, or race conditions.

3. **Zero Telemetry & Zero Remote Calls**:
   - `agy-swap` does not send telemetry, analytics, crash reports, or user information to any external server.
   - All network traffic is strictly limited to user-initiated actions:
     - Google OAuth login via local loopback.
     - Direct quota checks against the Google Antigravity API endpoint.
     - Optional release checks against public GitHub release metadata.

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
