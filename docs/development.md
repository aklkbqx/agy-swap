# Developer Guide & Testing Workflows

This document guides contributors through setting up, testing, and verifying changes across `agy-swap`.

---

## 1. Prerequisites

- **Go**: Version 1.27.2 or newer (`go version`). `go.mod` declares `go 1.27.2`, the security floor; with the default `GOTOOLCHAIN=auto` an older go command downloads that toolchain, both in this checkout and for `go install …@latest` (`go env GOVERSION` shows the one in use).
- **Make**: Standard POSIX make.
- **Xcode Command Line Tools** (macOS only): Required for compiling CGO bindings with Apple's `Security.framework`.
- **Bun** or **Node.js** (Optional): For developing the React site and native browser terminal in `site/`.

---

## 2. Common Development Commands

All standard workflows are codified in the [`Makefile`](../Makefile):

```bash
# Read-only overview: repository sync, VERSION/tag/Release, live site,
# installed binary, open Issues/PRs in both repositories, and any drift
make status

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

# Run golangci-lint (installed binary, or the pinned version through go run)
make lint

# Check reachable code against the Go vulnerability database (needs network)
make vuln

# Run memory allocation benchmarks
make benchmark

# Run terminal UI smoke test across multiple viewport dimensions
make tui-smoke

# Complete QA verification (formatting, test, race, vet, lint, tui-smoke)
make qa
```

CLI and TUI share the same version format: release builds show `vX.Y.Z`, while local builds from `make build` show `vX.Y.Z-dev`. The release build ID remains embedded in the binary for build inspection, without appearing in the displayed version.

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

Use `go test ./internal/demoserver ./internal/app` for the native demo path. In `site/`, run `bun run build` followed by `bun run test` sequentially: the browser suite reads built assets and must not race the build that replaces them. The test script uses Node's test runner; `bun test` invokes a different runner. Review servers require owner authorization.

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

Release flow, in order:

1. `make bump VERSION=X.Y.Z`, write the CHANGELOG section, regenerate story fixtures if needed.
2. `make release-check` — every pre-gate check (`make qa`, `make vuln`, Site install/build/tests, production-equivalent preview). Then the owner review gate.
3. Commit, annotated tag, `make release-assets` (below), push branch and tag.
4. `make release-notes VERSION=X.Y.Z > notes.md` and `gh release create vX.Y.Z --verify-tag --notes-file notes.md dist/release/*` before merging, so the promoted installers find published assets.
5. Merge, `make publish-homebrew-tap`, Site tag and `make site-release VERSION=X.Y.Z PUSH=1`, then `make site-verify-live VERSION=X.Y.Z`.
6. `make release-evidence VERSION=X.Y.Z` — read-only checks of Release digests against `checksums.txt`, asset build information, the Homebrew formula and installed binary, `go install …@latest` through the public proxy, the live version and the Site image digests. It prints the delivery-record Markdown (the Site image lines belong only in the private Site record) and exits non-zero on any mismatch.

Release assets are built from the release tag, not from a working tree. After the local review gate and owner approval: commit the release, create the annotated tag (`git tag -a vX.Y.Z -m vX.Y.Z`), then run `make release-assets VERSION=X.Y.Z BUILD_ID=release-vX.Y.Z`. In order, it:

1. verifies version metadata;
2. runs `releasetool verify-source`, which refuses uncommitted or untracked files, a HEAD that is not the annotated `vX.Y.Z` tag, and a compiler older than the `go.mod` toolchain floor;
3. runs govulncheck (pinned by `GOVULNCHECK_VERSION`) and stops on any reachable vulnerability;
4. cross-compiles the six binaries and writes checksums;
5. runs `releasetool verify-builds`, which reads each asset's Go build information and requires the `github.com/aklkbqx/agy-swap/v2` module, the exact `vX.Y.Z` version (no `+dirty` or pseudo-version) and a Go version at or above the floor;
6. generates `dist/release/agy-swap.rb`. It does not discover, edit, commit, or push a Homebrew tap.

Push the commit and tag, then publish the matching GitHub release and assets before updating Homebrew. Then run `make publish-homebrew-tap VERSION=X.Y.Z` explicitly. It uses `HOMEBREW_TAP_DIR` when set, or the installed `aklkbqx/agy-swap` tap; the tap must be on `main` with a clean working tree and index. Publication fetches the current remote tip, refuses behind/diverged branches and unrelated unpublished commits, and permits retrying a formula-only pending push. It commits only `Formula/agy-swap.rb` and pushes `origin main`. The helper's default invocation without `--publish` only generates the local formula.

## 6. Live Site Verification

After `make site-release VERSION=X.Y.Z PUSH=1`, run `make site-verify-live VERSION=X.Y.Z`. It builds Site tag `vX.Y.Z` in a temporary worktree, waits up to ten minutes (`AGY_SITE_WAIT`) for production to report that version, then runs `releasetool verify-live`: health and page status codes, `softwareVersion`, both installer redirects and the version of the installers they promote, the production demo endpoint returning 404, and byte equality of every built file. `sitemap.xml` carries its build date and is skipped; the Cloudflare Web Analytics beacon that the edge injects into HTML is removed before comparison, and any other difference fails. `AGY_SITE_URL` points it at another deployment.

## Native Windows installer regression

On Windows with Go and Windows PowerShell 5.1 installed, run:

```powershell
go test ./cmd/releasetool -run '^TestWindowsInstaller(SavedFile|DecodedText)$' -v
```

These tests execute the actual saved installer and UTF-8-decoded invocation in isolated temporary targets containing spaces. Only release downloads use controlled fixtures; checksum/version rejection, installation, previous-binary backup and cleanup run in the real script. They do not change the active installation, user PATH, accounts or credentials. The tests are Windows-only; a Mac `make qa` does not replace this native check.
