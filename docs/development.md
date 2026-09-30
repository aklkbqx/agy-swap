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

### 3.4 Web Simulator Fixtures
AGY Live on [agy-swap.aklkbqx.com](https://agy-swap.aklkbqx.com) runs the native Go TUI under a PTY in an isolated demo container. Nginx forwards `/demo/ws` to the private gateway, while xterm.js renders the actual ANSI output. Use `go test ./internal/demoserver ./internal/app` to verify the demo path, and `cd site && bun run test && bun run build` for the browser UI.

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
