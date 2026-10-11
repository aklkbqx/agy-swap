# GitHub workflow implementation plan

Date: 2026-10-11. State: required source/release/live delivery verified; final GitHub acceptance closure is pending. GitHub Issue checkpoints record execution and publication evidence.

> Execute directly with the `executing-plans` skill, one task at a time. Checkboxes describe acceptance, not assumed completion. Do not delegate implementation unless the owner explicitly asks. GitHub planning metadata is authorized; source publication retains the AGENTS.md local review gate.

**Goal:** Put project knowledge, notes, actionable work, review and complete version history on GitHub, with a workflow every agent can discover and resume.

**Architecture:** Canonical root AGENTS.md -> tracked workflow/docs -> Issues with resumable checkpoints -> PR review -> Milestones/tags/Releases and version evidence. Site has a private bootstrap and task/docs surface. Projects is an optional view, not a prerequisite.

**Tech stack:** Existing Go Tool and Vite/React Site; installed gh 2.98.0; Node 24.11.0 built-ins for a future local history helper. No new packages or cloud CI.

**Spec:** [GitHub workspace design](../specs/2026-10-11-github-workflow.md).

## Verified execution checkpoint

On 2026-10-11, v2.11.3 was published and verified: [delivery evidence](../releases/v2.11.3.md). Tasks 1–5 acceptance is satisfied; Task 6 delivery is verified and final metadata closure is pending. Optional Projects remains intentionally disabled with no token scope expansion. Its optional planning checkbox records evaluation, not board creation.

## Global constraints

- Keep public Tool and private Site Git operations separate. Inspect both statuses before editing and before publication. Do not stage unrelated files.
- GitHub Actions and all `.github/` configuration remain prohibited. Templates belong in `docs/templates/` and automation runs locally.
- Production stays self-hosted Docker/Nginx/Watchtower using `:latest`; no production SSH deployment.
- No token scope expansion, secrets, private identifiers, external implementation workers, invented evidence, existing-tag movement or release-note overwrites.
- Preserve latest-stable-only security support while documenting every historical version.
- Do not start/restart/kill user-owned servers. Use the already running local server for applicable visual review, or have the owner start the required process.
- Approve the concrete source candidate once at the existing local review gate before commit/tag/push/publish. New material scope must be reviewed again; do not ask for the same approval twice.
- Current latest is v2.11.2. Do not choose or bump the next version during planning. A governance-only patch can target v2.11.3 after impact review; additional features may require a different SemVer.

## Review focus

1. Are rules available in a clean standalone checkout, including Windows alias behavior?
2. Can another session resume without chat, private scratch files or a Project token scope?
3. Does every historical version exist without invented release/deployment/task evidence?
4. Are public/private boundaries preserved in every doc, Issue and payload?
5. Are task completion, publication and live delivery distinct and independently evidenced?
6. Can repeated metadata/history operations resume without duplicates or overwriting concurrent changes?

## Baseline and execution ledger

The source/API audit on 2026-10-11 observed 45 Tool tags, 44 published Releases, 21 Site tags, latest v2.11.2 and existing open Tool Issue #4. v1.5.0 is tag-only. API access works for Issues/Milestones; Projects requires an unavailable scope. Counts are refreshed before execution and are not hard-coded into future policy.

Local candidates prepared during planning: the root rule, Site bootstrap, workflow, templates, docs index, design, this plan and a dated release inventory/summary. They are not yet published source. GitHub planning Issue links and metadata are recorded after creation; implementation acceptance stays open until verified. Do not label the entire migration complete merely because this plan exists.

### GitHub execution records

Coordination: [Tool Epic #5](https://github.com/aklkbqx/agy-swap/issues/5). Milestone: [GitHub workflow migration](https://github.com/aklkbqx/agy-swap/milestone/1), with no deadline or product version assigned.

| Task | Execution record | Planning-time state |
| --- | --- | --- |
| 1: Canonical contract | [#6](https://github.com/aklkbqx/agy-swap/issues/6) | Local candidate; publication pending |
| 2: GitHub organization | [#7](https://github.com/aklkbqx/agy-swap/issues/7) | Metadata created; source publication pending |
| 3: Curated knowledge | [#8](https://github.com/aklkbqx/agy-swap/issues/8) | Backlog; source reconciliation required |
| 4: History helper | [#9](https://github.com/aklkbqx/agy-swap/issues/9) | Backlog; dated inventory exists, helper not implemented |
| 5: Historical tasks | [#10](https://github.com/aklkbqx/agy-swap/issues/10) | Backlog; #4 remains open |
| 6: Verify and publish | [#11](https://github.com/aklkbqx/agy-swap/issues/11) | Backlog; required implementation/review remains |

The private Site counterpart and its milestone are recorded in the private repository; public coordination does not expose private task URLs. These dated entries are the initial ledger. The latest Issue checkpoint and labels govern current execution state.

## Task 1 — Publish one discoverable agent contract

**Files:** modify root `AGENTS.md`, `.gitignore`, `CONTRIBUTING.md`; retain root `CLAUDE.md`, `GROK.md`, `GEMINI.md` symlinks. Modify Site `.gitignore`; replace only Site `AGENTS.md` external symlink with a small regular-file bootstrap; retain Site alias symlinks. Add `docs/README.md`, `docs/github-workflow.md`, `docs/templates/{README,task,checkpoint,pull-request,decision,release}.md`, this spec and plan.

**Interface:** all agent entry points lead to root AGENTS.md; the bootstrap reads local `../AGENTS.md` when paired, otherwise obtains the root contract and workflow with gh raw-content requests. Pin both requests to the same observed root commit for reproducibility. Retrieval failure blocks source edits rather than guessing rules. No personal global-rule copy or decoding dependency is created.

- [x] Confirm both repositories are clean apart from these candidates; inspect existing symlink targets with `git ls-files -s` after staging is authorized.
- [x] Remove only the four agent-rule ignore entries per repository. Keep secret, `.github/`, `/site/`, scratch and generated-output exclusions intact.
- [x] Review the concise root rule and detailed workflow. Check repository routing, latest-stable support, checkpoint protocol, owner gate and data privacy.
- [x] Review the standalone Site bootstrap and verify it does not duplicate policy. On Windows `core.symlinks=false`, explicitly load regular `AGENTS.md` when an alias is a text pointer.
- [x] Check local Markdown links and ignore behavior. Suggested commands: `git check-ignore --no-index .env .github/workflows/sample.yml site/package.json dist/sample.json` in Tool; check `.env` is never displayed. `git check-ignore --no-index AGENTS.md CLAUDE.md GROK.md GEMINI.md` should report no matching rule in each affected repo.
- [x] After owner approval and publication, verify clean Tool and standalone Site clones expose the intended regular file/symlinks and can retrieve the canonical rule. Record exact commits in the Issue checkpoint.

**Acceptance:** the public contract and private bootstrap are committed to the correct repos, aliases are intact, no sensitive ignore rule changed, and a standalone clone can read the governing rule.

## Task 2 — Organize GitHub work without depending on Projects

**Files/data:** `docs/github-workflow.md`; GitHub label/milestone/epic/task records in each affected repository. Store operational payloads and returned IDs under ignored `dist/github-workflow/` until curated evidence is published.

**Interface:** exactly one open-Issue `status:*`; independent scope/type/priority labels; one governance milestone `GitHub workflow migration` with no due date. Six open implementation tasks link this plan and acceptance. Site contract work has a private child record, coordinated with Task 1 through public-safe summaries.

- [x] Read all Issues with pagination and search exact proposed titles before creating objects. Preserve Issue #4 and existing labels/reporter text.
- [x] Read labels/milestones, create only missing taxonomy entries, and record IDs/URLs. Reuse a matching existing milestone without changing its deadline or description unilaterally.
- [x] Create a coordination epic with the full reviewable plan candidate or its published canonical link, current capability limits, acceptance and child-task links. Create the six task Issues using reviewed `--body-file` payloads and native `--parent` relationships.
- [x] Use `status:review` for a prepared candidate, `status:backlog` for unstarted implementation, and blocking links only where work truly depends on another task. No arbitrary assignees, external workers, deadlines or version support promises.
- [x] Read back every created record and compare body, labels, parent, milestone and state against the payload. On retry, reuse recorded IDs; do not create duplicates.
- [x] Keep Project activation as an optional enhancement: the owner can authorize scopes or use the selected browser later. If available, use a private board with Status, Surface, Priority, native Milestone and Target version text. Keep Issue status authoritative and verify private items are not exposed by views.

**Commands:** `gh issue create --help`, `gh issue edit --help`, `gh api repos/aklkbqx/agy-swap/issues --paginate --slurp`, `gh api repos/aklkbqx/agy-swap/labels --paginate --slurp`, `gh api repos/aklkbqx/agy-swap/milestones?state=all --paginate --slurp`. Use explicit repo and JSON/body files for mutations; never echo auth credentials.

**Acceptance:** a session can find the epic, all six tasks, correct state/dependencies and the private Site counterpart with current access; the workflow remains usable with no Project board.

## Task 3 — Curate durable knowledge and prior plans

**Files:** retain/link `README.md`, `SECURITY.md`, `docs/architecture.md`, `docs/development.md`. Add `docs/project/overview.md`, `docs/project/knowledge-map.md`, accepted `docs/decisions/` records, and sanitized historical plans under `docs/plans/archive/`. Add private Site `docs/README.md`, `docs/workflow.md`, `docs/operations/release.md` when its evidence is ready.

**Interface:** the knowledge map has subject, canonical document, supporting source/release, historical status, privacy classification and unresolved Issue. No duplicated architecture or parallel TODO list.

- [x] Inventory tracked docs, available notes/evidence and ignored plans. Use graft first, then targeted Serena/source lookup to verify each implementation claim. Do not publish local scratch files wholesale.
- [x] Reconcile these subjects: product overview/logo, account/auth storage, CLI/TUI, installation/update/uninstall, security/network behavior, Site design/translations, local demo/PTY, QA, release/deployment, decisions and unresolved work.
- [x] For each subject, retain an accurate canonical doc, rewrite an outdated note from source, archive a dated historical proposal with explicit implemented/partial/superseded/unknown status, or retain private evidence privately. Link original public Releases/changelog instead of reproducing all release notes.
- [x] Curate the ignored October 1 TUI/operator-deck and context-tooling plans and the October 11 audit plan. Verify which steps landed from commits/source. Do not treat checked boxes or proposal language as proof.
- [x] Link the existing real logo asset using a stable repository path after confirming its license/location. Verify rendering; do not invent an upload or replace the logo without a design request.
- [x] Redact credentials/account identifiers/private infrastructure, remove obsolete URLs, and check links. Keep raw sensitive evidence local; write sanitized conclusions with limits.
- [x] Convert only actual remaining action into Issues with acceptance. Link the knowledge map from `docs/README.md`; checkpoint the exact remaining unknowns.

**Acceptance:** each required subject has one source-linked canonical location; archived proposals retain dates/status; no personal/private data or stale actionable TODO list remains in published project docs.

## Task 4 — Cover all history and automate future read-only refreshes

**Files:** add `scripts/github-history.mjs`, `scripts/github-history.test.mjs`; maintain `docs/releases/index.json`, `docs/releases/README.md`, `docs/releases/v2.11.2.md` and future version-specific evidence records. Fixtures are minimal sanitized API objects in the test file; no new package/lockfile dependency.

**Interfaces:** export `buildInventory({tags, releases, siteTags, latestStable})`, `renderIndex(inventory)` and `readPages(text)`; CLI `node scripts/github-history.mjs --check` and `node scripts/github-history.mjs --write`. Defaults fetch fresh read-only gh API data with `--paginate --slurp`; an explicit fixture input mode is for offline tests. A successful write uses a validated temporary file and atomic rename. CLI has no GitHub mutation or source publication behavior.

- [x] Write failing Node tests before adding the helper. Cover multi-page input; tag-only v1.5.0; historical Releases with zero/custom assets; missing Site tags; duplicate/conflicting Tool tags; latest stable without a tag; draft/prerelease separation; unsupported schema; deterministic repeat output; and `--check` leaving files unchanged.

```js
import test from 'node:test';
import assert from 'node:assert/strict';
import { buildInventory } from './github-history.mjs';

test('tag-only history is retained without invented release evidence', () => {
  const inventory = buildInventory({
    tags: [{ name: 'v1.5.0', commit: { sha: 'a'.repeat(40) } },
           { name: 'v2.11.2', commit: { sha: 'b'.repeat(40) } }],
    releases: [{ tag_name: 'v2.11.2', draft: false, prerelease: false,
      published_at: '2026-10-11T08:06:11Z', assets: [] }],
    siteTags: [], latestStable: { tag_name: 'v2.11.2' }
  });
  const old = inventory.versions.find(v => v.tag === 'v1.5.0');
  assert.equal(old.releaseState, 'tag-only');
  assert.equal(old.publishedAt, null);
  assert.deepEqual(old.assets, []);
  assert.equal(old.siteTagPresent, false);
});
```

- [x] Run `node --test scripts/github-history.test.mjs` and confirm the expected missing-module/behavior failure, then implement the smallest built-in-only helper that passes the contract. Use `spawnSync` argument arrays for gh, not shell-interpolated payloads.
- [x] Flatten all paginated arrays and validate records. Preserve nullable fields and exact public asset metadata; do not derive fake dates from tags or assume six binaries on old Python releases. Record non-version tags separately rather than silently losing them. Confirm immutable Tool tag commits through GitHub/Git before publication.
- [x] Compare the refreshed inventory with dated baseline: currently 45 Tool tags, 44 published Releases, 21 Site tags and one tag-only Tool version. Counts are evidence for this run, not permanent assertions in the helper.
- [x] Write human-readable all-version links and current v2.11.2 evidence. Keep human per-version records untouched by generated refreshes. Public output includes no private Site commit IDs or deployment internals.
- [x] Run `--write`, then `--check` twice and verify no content churn. Simulate gh failure, malformed API input and conflicting records; assert previous output remains intact. Missing private Site read access fails explicitly or produces an explicitly partial diagnostic, never silently claims no Site tag.
- [x] Document the refresh command in `docs/github-workflow.md` and the release runbook. A future schema upgrade includes migration tests and preserves old provenance.

**Acceptance:** every observed historical version is represented truthfully, repeat refreshes are stable, failed reads do not corrupt output, and future versions can be added without changing taxonomy or manual board options.

## Task 5 — Reconcile shipped work and genuine remaining tasks

**Files/data:** `docs/project/knowledge-map.md`, `docs/releases/v2.11.2.md`, historical task Issues, existing Tool #4, private Site history where relevant. Use a reviewed mapping table with workstream ID, title, release evidence, Issue ID and actual state.

- [x] Verify v2.11.2 workstreams from source/release evidence: deterministic snapshots; credential documentation; package/update guards; Doctor copy in four locales; latest-stable security/network docs; local PTY docs; terminal color rules; Windows blob bounds/repeat/rollback.
- [x] Search existing open/closed Issues before import. Where a completed workstream needs an Issue, create it with original work dates in prose, link the shipped release and checks, then close only after acceptance is evidenced. Its GitHub creation date stays the actual import date.
- [x] Keep Windows implementation evidence separate from original reporter confirmation. Reuse #4 for the outstanding confirmation; do not create another identical Windows bug, close it automatically, or imply IDE/chat generation was tested.
- [x] For older versions, retain the original Release/changelog links. Create historical task records only when real task evidence exists; never invent one per version to make a board look complete.
- [x] Create future tasks only for actual discovered gaps, each with scope/acceptance/priority if assessed and no invented due date. Map Site-private details to private tasks.
- [x] Read back all mappings and verify there is no duplicate bug, incorrect shipped status, lost historical date or public leak. Record imported IDs so a retry reuses the same objects.

**Acceptance:** shipped work, reporter follow-up, missing evidence and future implementation are distinguishable; Issue #4 remains truthful and discoverable.

## Task 6 — Verify fresh-session use, review and publish

**Files/data:** all reviewed docs/rules/helper changes; release/version sources only if publication impact requires a new release; Issue checkpoints and release evidence.

- [x] Run local docs-link, privacy, ignore and symlink checks. Inspect diffs in both repos and validate that no `.github/`/hosting/secret/config drift occurred. Check all new GitHub links/body payloads against read-back data.
- [x] Execute a fresh-context rehearsal without delegating: use only GitHub docs/Issues and Git status to identify the next step of one Tool task and one standalone Site task. Record missing context as an Issue and fix the handoff.
- [x] If Windows alias behavior requires native verification, use the previously authorized WOL/SSH Windows route for a clean rules checkout in temporary space; do not expose host details or alter the installed app/accounts. Verify regular AGENTS and symlink-pointer handling. Clean only resources created by this test.
- [x] Run helper tests and repository-required checks sequentially: `make qa`; then `bun run build` in `site/`; then `bun test` in `site/`. Site tests read built assets, so do not run build and tests concurrently. Record existing build warnings separately from failures. Do not add prose-mirroring tests or cloud CI.
- [x] Review applicable visible changes using the selected browser and already running Vite URL/approved preview. If only governance/docs changed, review Markdown/rule rendering and state that product UI is unchanged; do not fabricate a new UI screenshot requirement or claim a new browser check.
- [x] Present the concrete candidate, file list, links, evidence, proposed SemVer if needed and limits. Wait for owner approval before source commit/tag/push/release. No commit is buried inside an earlier task.
- [x] After approval, stage explicit Tool paths and explicit Site paths separately. Commit with human Conventional Commit messages and no AI trailers. Push the reviewed docs/rules; when the full project release contract applies, use the existing cross-surface version tool, matching new annotated tags, build/install/assets and Site release command. Never reuse v2.11.2 or overwrite existing release assets.
- [x] Verify published docs from GitHub at exact commits and test standalone retrieval. If a release/Site deployment was affected, verify installed version, downloadable checksums, public site version/assets/health and image digest using the documented flow; production uses `:latest` and no SSH deployment.
- [ ] Update the release/Issue evidence, close only accepted tasks, leave real unresolved tasks open, and close the migration milestone/epic only when all required acceptance is met. Add the final checkpoint with exact next-release workflow.

**Acceptance:** GitHub alone contains the published contract, curated knowledge, work queue and truthful version history; all required delivery evidence is durable and another agent can resume without private chat context.

## Dependency and publication order

Task 1 and Task 2 establish discoverability and work records. Task 3 supplies knowledge; Task 4 supplies history refreshes; Task 5 reconciles history and real work. Task 6 depends on their required acceptance and owner approval. Independent audit/read work may proceed while approval or an optional board is pending. A board is never a blocker for core migration.

Do not close an implementation Issue because a local draft exists. During planning, summarize partial progress in checkpoints. Upon approval, execute this plan with `executing-plans`, revising the ledger only when evidence warrants it.

## Final handoff fields

Report Tool/Site commits, published rule/docs URLs, epic/task/milestone links, history coverage and tag-only gaps, helper tests and repository checks, accepted decisions, installed/released/deployed versions when affected, registry digest/live checks when affected, unchanged surfaces and all intentionally open work. Never state “migration complete” while canonical source is unpublished or required acceptance is pending.
