# Knowledge map and migration reconciliation

Reconciled 2026-10-11 against source, existing docs and published v2.11.2 evidence. This is an index of canonical knowledge, not a parallel task list. Migration [Epic #5](https://github.com/aklkbqx/agy-swap/issues/5) is complete. Current follow-up work is in [installer #19](https://github.com/aklkbqx/agy-swap/issues/19), [documentation #20](https://github.com/aklkbqx/agy-swap/issues/20), and their [plan](../plans/2026-10-11-windows-installer-followup.md).

| Subject | Canonical location | Supporting source / evidence | Disposition |
| --- | --- | --- | --- |
| Product and actual logo | [Overview](overview.md), [README](../../README.md) | Existing public Site brand image copied without edits | Published project knowledge |
| Architecture and repository boundaries | [Architecture](../architecture.md), [AGENTS](../../AGENTS.md) | `cmd/`, `internal/`, separate private Site repository | Retain source-linked docs |
| Saved-account storage and active sessions | [Security](../../SECURITY.md), [architecture](../architecture.md) | `internal/app/credentials.go`, `vault*.go`, `credential_*.go`, `windows_session.go` | Retain corrected vault/session distinction |
| CLI commands and selection | [README](../../README.md#commands) | `internal/app/cli.go`, `extended.go`, selection/quota code | Retain canonical command examples |
| TUI and color/layout contracts | [TUI design](tui-design.md), [development](../development.md) | `display.go`, `tui_*.go`, renderer tests and four smoke sizes | Curate public design record |
| Installation, update and uninstall | [README](../../README.md#install), [public install page](https://agy-swap.aklkbqx.com/install.html) | Reviewed shell/PowerShell installers and `internal/app/updater.go` | Retain existing public instructions |
| Security support and network behavior | [Security](../../SECURITY.md) | Google operational requests, GitHub update requests; no telemetry | Latest stable only; all older history archived |
| QA and reproducible story frames | [Development](../development.md) | `story_frames_test.go`, Go QA, Site tests | Retain and correct sequential build/test ordering |
| Site design and four translations | Private Site docs and source | Site design tokens, locale source/tests | Keep private implementation detail in Site |
| Local demo/PTY vs production | [Development](../development.md#34-website-story-frames-and-local-agy-live) | `cmd/agy-swap-demo-server`, `internal/demoserver`; static production routes | Retain explicit local-only boundary |
| Release and deployment | [Development](../development.md#5-release-assets-and-homebrew-publication), private Site runbook | Release tool/assets, explicit tap publication, self-hosted image flow | Public tooling docs; private operations |
| Every historical version | [History](../releases/README.md), [JSON](../releases/index.json), [changelog](../../CHANGELOG.md) | Paginated GitHub tags/Releases and Site tag-presence evidence | All known versions; no invented delivery evidence |
| v2.11.2 delivered findings | [Delivery evidence](../releases/v2.11.2.md), [workstream map](../releases/v2.11.2-workstreams.md) | Published Release, source and sanitized test results | Shipped in v2.11.2; Issue #4 later closed after independent native v2.11.3 verification |
| Windows closure and active installation | [v2.11.3 evidence](../releases/v2.11.3.md) | [Native acceptance](https://github.com/aklkbqx/agy-swap/issues/4#issuecomment-6107713281), [installed binary](https://github.com/aklkbqx/agy-swap/issues/4#issuecomment-6107821873) | Issue #4 closed; installed v2.11.3; no reporter reply or IDE/chat claim |
| Durable decisions | [Decisions](../decisions/README.md) | Current contract and approved migration design | Accepted rationale with explicit consequences |
| Earlier design/work proposals | [Archive](../plans/archive/README.md) | Dated local proposals compared with current source | Curated summaries; original scratch retained locally |
| Current and future actionable work | [Issues](https://github.com/aklkbqx/agy-swap/issues), private Site Issues | Acceptance, version-neutral labels, checkpoints and Milestones | No duplicated TODO database |

Private raw logs and account/infrastructure identifiers are not published. Existing scratch files remain local working material. Archived proposals are not authorization to restart old features or modify global agent tooling; any newly accepted implementation needs its own Issue and current scope.
