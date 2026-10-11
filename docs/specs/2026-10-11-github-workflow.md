# GitHub project workspace design

Date: 2026-10-11. State: accepted for execution by the owner; the existing review and verification contract applies to publication.

## Goal and boundaries

Make GitHub the complete project workspace for agy-swap: knowledge, plans, notes, tasks, decisions, review, delivery and every version's history. A contributor or agent must be able to clone the affected repository, find the governing rules, discover current work and resume it without access to a previous chat.

This changes project governance and documentation. It does not change account switching behavior, production hosting, the security support window, token scopes, the prohibition on GitHub Actions, or authorization for external workers. Source publication follows the existing owner review gate.

## Verified starting point

| Item | Observation on 2026-10-11 |
| --- | --- |
| Tool | Public `aklkbqx/agy-swap`, main, 45 version tags, 44 published Releases |
| Site | Private `aklkbqx/agy-swap-site`, main, 21 tags |
| Latest stable | `v2.11.2`; publication and live verification recorded separately |
| Historical gap | `v1.5.0` is tag-only; many old Tool versions have no matching Site tag |
| Existing bug | Tool Issue #4 remains open for Windows reporter-environment confirmation |
| Current documentation | Tracked architecture/development docs; some local plans are ignored |
| Rules | Root AGENTS/aliases ignored; Site AGENTS points outside its repository |
| CLI | gh 2.98.0; Issues/Milestones accessible; Projects read fails for missing `read:project` scope |
| Local helper runtime | Node 24.11.0; no new dependency required |
| Discussions | Disabled on Tool; Issues are sufficient for current decisions/questions |

The snapshot is a dated observation, not a permanent count. Refresh before execution/publication. A missing Site tag does not prove no historical Site deployment occurred. A published Release does not prove every historical binary or deployment was retested.

## Chosen structure

1. **One contract:** track root AGENTS.md and its existing alias symlinks. Site uses a tracked bootstrap, not a duplicated policy. A standalone Site checkout retrieves the public root contract through gh and reads it before editing. Paired checkouts use the local root contract so local owner amendments apply.
2. **One knowledge hierarchy:** docs index -> existing architecture/development -> specs/plans/decisions -> releases. Curated project notes become source-linked docs, with private Site material in Site docs. The source remains authoritative for implementation claims.
3. **One work record:** Issues hold acceptance, dependencies, state and checkpoints. PRs hold code review. Milestones coordinate real planned versions. No second TODO database or board-only tasks.
4. **One historical inventory:** all Tool version tags, public Release metadata and Site tag-presence evidence are recorded. Current release evidence is deeper than old recorded history, and that distinction is explicit. Future refreshes are deterministic and idempotent.
5. **One handoff protocol:** a new session reads the contract, Issue, canonical plan, newest checkpoint and current Git state before claiming work. Checkpoints keep failed checks and changed decisions visible.

## Alternatives and tradeoffs

- A board-first workspace would require unavailable Projects scopes and hide notes outside the repository. Issues/Milestones/docs work with current access; a board can be added later as a private view.
- A Wiki-first knowledge base would split review/versioning and does not exist on the Site repo. Tracked docs retain review, history and clone availability.
- Copying all rules into Site would drift. A bootstrap adds a retrieval step to standalone clones, but preserves one policy. If retrieval fails, source edits stop until the owner provides the contract; do not guess the rules.
- Creating Releases and Milestones for every old tag would invent delivery history and clutter work tracking. Record old history as observed; create Milestones only where real work is planned or accepted historical task mapping is needed.
- Using version-specific labels or a board option per version has an eventual maintenance ceiling. Labels stay version-neutral; Milestones and version text/history records cover future versions.

## Task state and coordination

The lifecycle in [github-workflow.md](../github-workflow.md) is authoritative. Exactly one status label is allowed on open Issues. Dependencies use native sub-issues/blocking relationships supported by the installed gh CLI, with readable links in the body as well. No provider/model-specific workflow is required.

Issue comments are an append-only execution journal. Claims are advisory rather than locks: a session must re-read current comments and coordinate overlap. A PR uses `Refs #N` and does not automatically close a delivery Issue before installation/deployment checks. Imported historical tasks preserve source dates in prose but keep GitHub's actual creation timestamps.

## Knowledge and migration policy

Inventory existing tracked docs, ignored plans and available local evidence. For each subject choose: retain/link the existing canonical document; rewrite an outdated note from current source; archive a historical proposal with its date/status; or keep sensitive material private. Publish one map of these decisions and open Issues for real unresolved work. Do not publish scratch directories wholesale.

Use source-first checks for account/auth storage, CLI/TUI behavior, local demo/PTY boundaries, installation, security/network behavior and Site design. Preserve actual approved decisions and exclusions. Remove obsolete links and private identifiers before any public write. Use existing reviewed logo assets where a knowledge page needs branding; do not create a new image solely for the migration.

## All-version data contract

`docs/releases/index.json` has `schemaVersion: 1`, `latestStableTag`, public repository identity and version records. Each record includes tag name, public Tool commit, tag URL, release state, nullable published date/release URL, actual assets, Site tag presence and evidence classification. Missing fields remain null/unknown. Public records never contain private Site commits or infrastructure details.

A local read-only helper will consume paginated GitHub snapshots. It must validate inputs before touching output, reject conflicting duplicate tags or a stable release missing its tag, preserve tag-only versions and old asset shapes, and avoid timestamp churn. Its `--check` mode performs no writes. Human evidence records and Release bodies are not overwritten.

Future stable versions use the same schema and pipeline. Draft/prerelease observations remain distinct from published stable Releases. Schema changes require explicit versioned migration and tests; never silently reinterpret historical fields. Historical support documentation covers all versions while security fixes cover latest stable only.

## Historical tasks and current release

Reconcile the v2.11.2 workstreams against source/release evidence before importing completed tasks. Windows implementation work is shipped, but the existing reporter Issue #4 stays open; do not confuse those states. Other historical releases link original changelog/Release notes instead of manufacturing task lists. The latest release record distinguishes Mac installed version, Windows temporary-candidate testing, unchanged Windows installation and untested IDE/chat generation.

## Rollout and rollback

The [implementation plan](../plans/2026-10-11-github-workflow.md) separates contract, GitHub organization, curated knowledge, history refresh tooling, historical task mapping and publication validation. Planning Issues can exist before source publication, explicitly marked pending. No existing tag or Release is rewritten. A new version is selected only when actual implementation scope is known; documentation/governance-only scope suggests a patch, subject to owner review.

Before publication, rollback is a reviewed local diff; do not revert unrelated files. For GitHub organization mistakes, correct labels/links with a checkpoint and close duplicates with reason “not planned”; preserve history. After publication, use normal corrective commits and the documented self-hosted image rollback if the Site is affected. Never force-push or retag to erase mistakes.

## Acceptance

- Every agent can discover the same contract from Tool and standalone Site checkouts.
- Current actionable work is discoverable as Issues with acceptance, state and checkpoints.
- Durable notes/plans/decisions are Git-versioned and linked once; sensitive Site material stays private.
- All 45 observed Tool tags have records; 44 published Releases and the one tag-only version remain distinguishable.
- New versions can be added without changing labels or rebuilding a fixed board version list.
- The new session can resume an Issue using only GitHub and current source.
- Source publication and any release/deployment happen only after required owner review and verification.
- No `.github/` configuration, token expansion, hosting change, delegated implementation or private-data disclosure is introduced.

## Windows follow-up — 2026-10-11

Issue #4 subsequently closed after owner-authorized native v2.11.3 verification, and the active Windows installation was updated to v2.11.3. See the canonical [verification and installation follow-up](../releases/v2.11.3.md#windows-follow-up--2026-10-11). Original observations above remain historical; no new reporter reply or IDE/chat test is asserted.
