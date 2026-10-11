# Windows installer follow-up implementation plan


**Goal:** Fix saved-file Windows PowerShell 5.1 installation and reconcile the actual Windows closure/install evidence.

**Architecture:** Keep the existing installer flow, replace non-ASCII output with plain ASCII, and execute that real script in native PowerShell regression tests. Network downloads are the sole fixture boundary. Historical records receive a dated addendum; current checkpoints link observed GitHub evidence. Release candidate metadata is coordinated across the two existing repositories.

**Tech Stack:** Go 1.26.0, Windows PowerShell 5.1, existing Bun/Node Site tests, GitHub CLI and local Docker release tooling.

**Spec:** [Windows installer follow-up](../specs/2026-10-11-windows-installer-followup.md).

## Global constraints

- Go 1.26.0; existing standard-library test tooling only.
- Windows PowerShell 5.1 is the native saved-file regression target.
- GitHub is canonical: tracked docs, Issues, Milestones, immutable tags and Releases.
- Tool and Site remain separate repositories; no private deployment or account data in public records.
- No GitHub Actions, new hosting provider, external implementation service or production SSH deployment.
- Production image reference remains `:latest`; local Docker/Nginx/Watchtower flow.
- Local owner review gate applies before commit, tag, push, release or image publication.

## Review Focus

1. A saved no-BOM script on Windows PowerShell 5.1 must install successfully without a conversion wrapper.
2. Corrupt downloaded bytes must leave an existing target byte-identical.
3. A correctly hashed wrong-version executable must never replace the installed target.
4. A target directory containing spaces must preserve exact binary and backup paths.
5. Current closed-Issue evidence must not rewrite historical checks or claim an original reporter reply.

### Task 1: Fix native installer execution — Tool Issue #19

**Files:** Create `cmd/releasetool/install_windows_test.go`; modify `install.ps1`.

**Interfaces:** Consumes the existing installer `-Version` / `-TargetDir`; produces an ASCII-safe script with the same installation interface and native tests `TestWindowsInstallerSavedFile` / `TestWindowsInstallerDecodedText`.

- [x] Step 1: Add Windows-only Go tests executing the unchanged saved installer under `powershell.exe -NoProfile -NonInteractive`. A helper mode in the test executable reports fixture version `2.0.0`; PowerShell overrides only `Invoke-WebRequest` to copy exact fixture executable/checksum payloads for known release URLs. Use cases success, corrupt checksum, wrong reported version, prefix collision, prerelease suffix and failed self-test; a spaced target and isolated TEMP cover backup/cleanup in each case. A second test evaluates UTF-8-decoded script text with the same success fixture.

```go
if wantSuccess && runErr != nil { t.Fatalf("native installer failed: %v", runErr) }
if !wantSuccess && (runErr == nil || !strings.Contains(string(out), wantError)) {
    t.Fatalf("expected candidate rejection containing %q", wantError)
}
installed, err := os.ReadFile(target)
if err != nil { t.Fatal(err) }
want := previous
if wantSuccess {
    want = fixture
    backup, err := os.ReadFile(target + ".bak")
    if err != nil || !bytes.Equal(backup, previous) { t.Fatal("backup mismatch") }
}
if !bytes.Equal(installed, want) { t.Fatal("target bytes mismatch") }
remaining, err := os.ReadDir(downloads)
if err != nil || len(remaining) != 0 { t.Fatal("download cleanup failed") }
```

- [x] Step 2: Cross-compile the native test executable and run only these tests on the owner's Windows using SSH, copying the unmodified saved installer as bytes. Expected: saved success fails against v2.11.3 due to incorrect decoding; decoded-text control passes. Keep raw evidence local; remove the exact owned remote folder.

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o dist/release-evidence/v2.11.4/installer-tests.exe ./cmd/releasetool
```

- [x] Step 3: Replace output symbols with ASCII (`[info]`, `[ok]`, `Downloading`, `Installation complete`). Preserve integrity, TLS, target selection and rollback behavior. Review follow-up: compare complete brand/version tokens and require successful self-test exit status; native tests must first expose prefix/prerelease/failed-self-test acceptance.
- [x] Step 4: Re-run native tests. Expected: saved success and decoded success pass; all five rejection cases preserve old bytes; no download folders remain. Run the actual candidate installer against published v2.11.3 in an isolated target with no download stubs. Expected: installed checksum/version match published asset; active installation remains unchanged.
- [x] Step 5: Record native RED/GREEN and end-to-end results in ledger and GitHub checkpoint. Commit remains pending the owner gate.

### Task 2: Reconcile current and historical evidence — Tool Issue #20

**Files:** `docs/README.md`, `docs/project/overview.md`, `docs/project/knowledge-map.md`, `docs/releases/v2.11.2.md`, `docs/releases/v2.11.2-workstreams.md`, `docs/releases/v2.11.3.md`, `docs/development.md`, `docs/plans/archive/README.md`, `docs/plans/archive/2026-10-11-audit-remediation.md`.

**Interfaces:** Consumes Issue #4 native comment `6107713281` and installation comment `6107821873`; produces current-state links plus explicitly dated historical addenda. Does not change installer behavior.

- [x] Step 1: Read Issue #4 body/comments and exact current/historical references. Expected: Issue CLOSED/completed, independent native verification and v2.11.3 installation observed.
- [x] Step 2: Correct current overview/knowledge entry point/checkpoint; add a dated 2026-10-11 Windows follow-up to historical release/plan/spec records. Preserve baseline observations and immutable release metadata. Candidate v2.11.4 remains pending.
- [x] Step 3: Resolve relative Markdown links for changed documents and review diff. Expected: no missing link target or false current open-state claim outside explicitly dated baseline history. Human prose receives no mirror tests.
- [x] Step 4: Record reviewed document list and remaining publication step. Commit remains pending the owner gate.

### Task 3: Prepare and verify the v2.11.4 candidate — coordinated Site task

**Files:** Tool `Makefile`, entrypoint version constants, installers, README build version, `CHANGELOG.md`, existing version test fixture; Site `package.json`, `package-lock.json`, `index.html`, `src/generated/tui-story-frames.json`. Add `docs/releases/v2.11.4.md` with candidate checks clearly distinguished from delivery.

**Interfaces:** Consumes Tasks 1/2, produces matching candidate v2.11.4 metadata, local evidence and review-ready diff. Site UI behavior is unchanged.

- [x] Step 1: Use existing release tooling to bump the real checkout once; no tag replacement.

```sh
go run ./cmd/releasetool bump 2.11.4 .
go run ./cmd/releasetool verify-metadata 2.11.4 .
```

Expected: every version source agrees; lockfile preserved except package version.

- [x] Step 2: Regenerate the native story fixtures for the new version, then build the Site and run complete verification with logs in ignored release evidence.

```sh
AGY_SWAP_UPDATE_STORY_FRAMES=1 go test ./internal/app -run '^TestSiteStoryFrames$'
(cd site && bun run build)
make qa
bun run --cwd site test
node --test scripts/github-history.test.mjs
```

Expected: all tests, race detector, vet, TUI smoke, Site tests and production build pass. Do not substitute `bun test` for the Node test script if Bun reports no tests.

- [x] Step 3: Build a versioned local candidate without replacing the active stable install; review the diff independently. Verify visible Site metadata through the user-owned dev server and authorized local production preview. Expected: candidate version 2.11.4, release metadata consistent and visible pages intact. Do not start a server/container without owner authorization; request only missing runtime startup if required.
- [x] Step 4: Present changed files, checks, Windows evidence and remaining approval/delivery steps. Wait for the concrete local review gate before any commit, tag, push or publish.
- [x] Step 5: After approval, create focused Tool/Site Conventional Commits, immutable annotated v2.11.4 tags, build/verify/publish release assets, update the Homebrew tap and active Mac installation, and update Windows through the final published installer. Run `make site-release VERSION=2.11.4 PUSH=1`, verify public health/version/install and record private digest evidence. Refresh history inventory; mark Issues shipped/closed and close both milestones only after acceptance.

## Execution checkpoint

Session `work-20261011-02`; base Tool `9f19a0b`, Site `2534bc8`. Owner approved the reviewed candidate with “implement” on 2026-10-11. Native Windows RED/GREEN, Tool QA and Site tests/build/visual checks passed. Independent review's existing version-prefix weakness was resolved in one fix pass. Tool PR #21 merged; immutable v2.11.4 tags and seven release assets were published. Active Mac/Windows installations and live Site now use v2.11.4, with matching final checksums, preserved Windows backup/account/config state and verified public build bytes. [Delivery evidence](../releases/v2.11.4.md) records details. History and delivery evidence are published. Tool Issues #19/#20 and the private Site task are closed as shipped/completed with checked acceptance; both v2.11.4 milestones are closed with zero open issues. All plan tasks are complete.
