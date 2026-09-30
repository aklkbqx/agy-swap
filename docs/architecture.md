# Architecture

`agy-swap` is one Go binary. Almost all behavior lives in `internal/app`; a few pieces that have no UI or account logic live in small packages beside it.

## Layout

| Path | Responsibility |
|---|---|
| `cmd/agy-swap` | Entry point. Sets `version` and `buildID`, then runs `internal/app`. |
| `cmd/agy-swap-demo`, `cmd/agy-swap-demo-server`, `internal/demoserver` | Sample-account build of the TUI and the loopback WebSocket bridge used by `make live`. Not part of the public site. |
| `cmd/releasetool` | Version bumps, checksums, and release asset and metadata checks. |
| `internal/app` | CLI commands (`cli.go`, `extended.go`), TUI (`tui*.go`), account store (`store.go`, `model.go`), credentials and OS vaults (`credentials.go`, `vault*.go`, `keychain_darwin.go`, `credential_*.go`), OAuth (`oauth.go`), quota (`quota.go`, `display.go`), history and logs, backups, doctor, statusline, metrics, targets, and self-update (`updater.go`). |
| `internal/store` | Private directories, atomic write-then-rename, and per-OS file locks (`flock` on Unix, `LockFileEx` on Windows). |
| `internal/config` | The private directory mode the store uses, plus path and version helpers that `internal/app` does not call yet. |
| `internal/client` | Release tag normalization and checksum lookup used by the updater. |
| `site/` | Marketing and install site (React + Vite), served by one Nginx container. |

## Data and credentials

- Account metadata and settings are JSON files under the agy-swap config directory, written atomically under a file lock.
- Saved account tokens go to `vault.json` in the config directory (`0600`, not encrypted). `AGY_SWAP_VAULT=keychain` uses the OS credential store instead (macOS Keychain through Security.framework, Windows Credential Manager, Linux Secret Service through `secret-tool`); by default that store is read only for older tokens. `agy-swap doctor` reports the vault in use, accounts that still hold legacy plaintext tokens, and secrets it cannot read.
- Switching snapshots the shared local Antigravity session files, writes the new ones, and restores the snapshot if a step fails.
- History is local JSONL trimmed by size and age; backups are JSON, and `--include-secrets` exports are encrypted with a passphrase.

## Network

The CLI talks to Google for sign-in, account, and quota data, and to GitHub for release checks and updates. It sends no telemetry.

## TUI

The TUI draws with ANSI escape sequences in raw terminal mode, without a UI framework. Layout switches between wide, stacked, and compact modes by terminal size (down to 28×12, checked by `make tui-smoke`). The website's terminal frames are rendered by the same code: `TestSiteStoryFrames` regenerates `site/src/generated/tui-story-frames.json`.
