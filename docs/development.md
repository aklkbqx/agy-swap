# Developer Guide & Testing Workflows

This document guides contributors through setting up, testing, and verifying changes across `agy-swap`.

---

## 1. Prerequisites

- **Go**: Version 1.26 or newer (`go version`).
- **Make**: Standard POSIX make.
- **Xcode Command Line Tools** (macOS only): Required for compiling CGO bindings with Apple's `Security.framework`.
- **Bun** or **Node.js** (Optional): For developing the React site and native browser terminal in `site/`.

---

## 2. Common Development Commands

All standard workflows are codified in the [`Makefile`](../Makefile):

```bash
# Compile local binary to ./agy-swap
make build

# Install binary to ~/.local/bin/agy-swap and verify version
make install

# Run unit tests across all packages
make test

# Run tests with the Go race detector enabled
make race

# Run Go static analysis
make vet

# Run memory allocation benchmarks
make benchmark

# Run terminal UI smoke test across multiple viewport dimensions
make tui-smoke

# Complete QA verification (formatting, test, race, vet, tui-smoke)
make qa
```

---

## 3. Testing Strategies

### 3.1 Unit & Regression Tests
Unit tests are co-located alongside the code they verify. All persistent operations use temporary directories (`t.TempDir()`) or isolated test environments.

### 3.2 Race Detection
Always verify concurrent operations with the Go race detector:
```bash
make race
```
The race detector ensures that background quota refreshes and terminal rendering routines do not experience data races on shared state.

### 3.3 TUI Smoke Testing
The `./scripts/tui-smoke.sh` script verifies that the terminal interface initializes and renders correctly across common terminal dimensions:
- `28x12` (Ultra-compact / mobile)
- `40x20` (Compact)
- `80x24` (Standard ANSI terminal)
- `120x30` (Wide widescreen view)

### 3.4 Website Story Frames and Local AGY Live

The public site displays committed still frames produced by the Go TUI renderer. It is a static Vite build served by one Nginx container; production does not run a PTY gateway and `/demo/ws` is unavailable.

After changing the renderer or demo data, regenerate and verify the fixtures:

```bash
AGY_SWAP_UPDATE_STORY_FRAMES=1 go test ./internal/app -run '^TestSiteStoryFrames$' -count=1
go test ./internal/app -run 'TestSiteStoryFrames' -count=1
```

The fixture test pins its terminal color environment; the production TUI still selects colors from the actual terminal capabilities.

AGY Live is an opt-in local development tool. The owner starts it with `make live`, which builds the demo binary, runs the PTY gateway on loopback, and enables the browser terminal with `VITE_AGY_LIVE=1`. It finds free gateway and Vite ports; open the **Local URL printed by Vite**, then `#demo`. Each demo uses an isolated temporary home. Neither the gateway nor the live section ships in the production image.

Use `go test ./internal/demoserver ./internal/app` for the native demo path, and `bun test --cwd site` plus `cd site && bun run build` for the browser UI. Agents must ask the owner to start review servers, following `AGENTS.md`.

---

## 4. Cross-Platform Builds

Release binaries are compiled for 6 target architectures using `./scripts/build-release.sh`:

| Platform | Architecture | CGO Enabled | Keystore with AGY_SWAP_VAULT=keychain |
| :--- | :--- | :--- | :--- |
| **macOS** | `arm64` / `amd64` | Yes (`CGO_ENABLED=1`) | Apple `Security.framework` |
| **Linux** | `arm64` / `amd64` | No (`CGO_ENABLED=0`) | Secret Service |
| **Windows**| `arm64` / `amd64` | No (`CGO_ENABLED=0`) | Windows Credential Manager |

By default every platform keeps saved tokens in `vault.json`; the keystore column applies only when `AGY_SWAP_VAULT=keychain` is set.

To test cross-compilation locally:
```bash
# Linux arm64
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o /tmp/agy-swap-linux ./cmd/agy-swap

# Windows amd64
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o /tmp/agy-swap-win.exe ./cmd/agy-swap
```

## 5. Release Assets and Homebrew Publication

`make release-assets VERSION=X.Y.Z BUILD_ID=release-vX.Y.Z` cross-compiles the six binaries, verifies metadata and checksums, and generates `dist/release/agy-swap.rb`. It does not discover, edit, commit, or push a Homebrew tap.

After the local review gate and owner approval, publish the matching GitHub release and assets before updating Homebrew. Then run `make publish-homebrew-tap VERSION=X.Y.Z` explicitly. It uses `HOMEBREW_TAP_DIR` when set, or the installed `aklkbqx/agy-swap` tap; the tap must be on `main` with a clean working tree and index. Publication fetches the current remote tip, refuses behind/diverged branches and unrelated unpublished commits, and permits retrying a formula-only pending push. It commits only `Formula/agy-swap.rb` and pushes `origin main`. The helper's default invocation without `--publish` only generates the local formula.
