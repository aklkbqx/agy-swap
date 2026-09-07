# Changelog

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
