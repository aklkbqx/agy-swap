# Changelog

## 2.3.1

This patch release permanently eliminates repeated macOS Keychain authorization prompts, cascades of permission dialogs on launch, and orphaned keychain items.

- **Deterministic Secret Refs**: Switched account secret references from randomized nonce strings (`account:<email>:<nonce>`) to stable deterministic refs (`account:<email>`), preserving access authorization across token refreshes and account updates.
- **In-Memory Token Hashing**: Added SHA-256 token hashing (`token_hash`) for active account identification in memory, completely eliminating the startup loop over all managed accounts.
- **Secure File Vault (`0600`) & Hybrid Vault**: Added `fileAccountVault` (`~/.gemini/agy-swap/vault.json` with strict `0600` permissions) and hybrid vault fallback for a 100% zero-prompt experience on macOS matching Antigravity and GitHub CLI standards.
- **Keychain Orphan Cleaner**: Added automatic detection and cleanup of obsolete orphaned keychain items left behind by previous versions in macOS `login.keychain-db`.


## 2.3.0

This minor release introduces interactive split resizing for wide TUI displays, persistent layout preferences, and dynamic account table column expansion.

- Interactive split resizing via keyboard (`[` / `]` or `<` / `>` for fine step, `{` / `}` for large step, and `=` to reset).
- Command Palette actions for widening, narrowing, and resetting the pane split.
- Layout split preference persisted in `settings.json` under `ui.split_offset` and configurable via CLI (`agy-swap config set ui.split_offset <val>`).
- Dynamic account column widths with increased health column cap (up to 32 characters) to prevent text truncation on wider displays.

## 2.2.1

This patch release fixes macOS Keychain secret retrieval and deletion by performing all operations in-process through Security.framework instead of external CLI calls, eliminating repeated OS authorization prompts, and adds automated single-command version bumping.

- Read and delete macOS Keychain secrets directly via Security.framework in-process to align code identity and partition lists with writes.
- Prevent recurring system permission prompts on Darwin caused by `/usr/bin/security` partition list mismatches.
- Add opt-in Keychain probe test for real keychain round-trip verification.
- Add automated single-command version bumping (`make bump` and `scripts/bump-version.sh`) across all code, documentation, and fixture surfaces.
- Add `make install` to compile and install directly into active local CLI path.

## 2.2.0

This minor release adds working account policies, reserve accounts, and directory
binding actions while fixing account storage, backup recovery, quota selection,
and browser demo reliability.

- Detect concurrent account creation and stale account/settings writes. Serialize
  history changes and recover interrupted account/settings imports together.
- Share backup validation across CLI and TUI. Reject malformed encrypted data and
  invalid settings before import; preserve credentials during metadata merges.
  New encrypted backups use PBKDF2-SHA256 and AES-GCM; legacy backups remain readable.
- Combine manual, log, and API cooldowns. Require fresh, known available quota for
  automatic selection. Implement sticky, balanced, and round-robin policies,
  profile reserves, notification thresholds, and prompt/recommend/auto bindings.
- Verify new account identity through Google userinfo, persist refreshed OAuth
  credentials even when quota retrieval fails, repair session files on switching,
  and record successful switches. Restore prior sessions when login fails.
- Write macOS credentials through Security.framework without secret process
  arguments. Report plaintext fallback when the OS vault is unavailable.
- Correct family quota metrics, unknown statusline values, refresh behavior,
  child exit codes, and concurrent log attribution. JSON errors exit nonzero.
- Preserve UTF-8 input and letter case in forms; fix Tab and Shift-Tab navigation.
- Verify release asset contents against checksums, prevent unintended downgrades,
  test downloaded binaries before installation, and roll back failed replacement.
- Preserve manual demo navigation during scrolling and ignore stale async loads.
  Pack 51,114 renderer fixtures into shared rows/frames and fit the 3D terminal on
  mobile screens. Update all four site translations and storage claims.
- Consolidate site packaging around Docker/Nginx and local release tooling.

Existing account files and legacy encrypted backups remain supported. Automatic
selection now fails when no account has verified fresh capacity; use an explicit
`switch ACCOUNT` when intentionally overriding that safeguard. OS credential vault
failure can leave tokens in local plaintext files with restricted permissions;
Antigravity's own session files also contain credentials. Backups with secrets
must be encrypted. Older vault entries are retained for backup recovery.
