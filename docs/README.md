# Project documentation

GitHub is the project workspace. The Tool repository is public; the Site repository is private. Read the [GitHub workflow](github-workflow.md) before making changes.

| Need | Canonical location |
| --- | --- |
| How to contribute and resume work | [GitHub workflow](github-workflow.md) |
| Open work, bugs, acceptance, checkpoints | [Tool Issues](https://github.com/aklkbqx/agy-swap/issues), private Site Issues |
| Implementation architecture | [Architecture](architecture.md) |
| Product notes, real logo and knowledge map | [Overview](project/overview.md), [knowledge map](project/knowledge-map.md) |
| Decisions and earlier proposals | [Decisions](decisions/README.md), [historical plan archive](plans/archive/README.md) |
| Local development and verification | [Development](development.md), [contributing](../CONTRIBUTING.md) |
| GitHub workspace migration execution | [Epic #5](https://github.com/aklkbqx/agy-swap/issues/5), [Milestone](https://github.com/aklkbqx/agy-swap/milestone/1) |
| All historical versions | [Release index](releases/README.md), [machine-readable inventory](releases/index.json), [changelog](../CHANGELOG.md), [source history maintenance](releases/history-maintenance.md) |
| Latest recorded delivery | [v2.12.0 installation and live verification](releases/v2.12.0.md) (bug-hunt record: [v2.11.7](releases/v2.11.7.md)) |
| Latest patch plan | [v2.11.5 release hardening plan](plans/2026-10-12-v2.11.5-release-hardening.md), [v2.11.6 delivery](releases/v2.11.6.md) |
| New task, PR, checkpoint, decision, release | [Templates](templates/README.md) |

There is one canonical document per subject. Link existing architecture/development documentation rather than copying it into Issues. Store decisions in `docs/decisions/`, specifications in `docs/specs/`, and approved implementation plans in `docs/plans/`. An Issue records execution state and links the relevant document at its reviewed commit. Local scratch files are working material until reviewed and published.

Historical coverage is different from security support: all known versions are recorded; only the latest stable release receives security fixes. Missing historical Site tags and verification evidence remain explicitly unknown.
