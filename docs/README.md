# Project documentation

GitHub is the project workspace. The Tool repository is public; the Site repository is private. Read the [agent contract](../AGENTS.md) before making changes.

| Need | Canonical location |
| --- | --- |
| How to work and resume across agents | [GitHub workflow](github-workflow.md) |
| Open work, bugs, acceptance, checkpoints | [Tool Issues](https://github.com/aklkbqx/agy-swap/issues), private Site Issues |
| Implementation architecture | [Architecture](architecture.md) |
| Product notes, real logo and knowledge map | [Overview](project/overview.md), [knowledge map](project/knowledge-map.md) |
| Decisions and earlier proposals | [Decisions](decisions/README.md), [historical plan archive](plans/archive/README.md) |
| Local development and verification | [Development](development.md), [contributing](../CONTRIBUTING.md) |
| GitHub workflow design and rollout | [Design](specs/2026-10-11-github-workflow.md), [implementation plan](plans/2026-10-11-github-workflow.md) |
| GitHub workspace migration execution | [Epic #5](https://github.com/aklkbqx/agy-swap/issues/5), [Milestone](https://github.com/aklkbqx/agy-swap/milestone/1) |
| All historical versions | [Release index](releases/README.md), [machine-readable inventory](releases/index.json), [changelog](../CHANGELOG.md) |
| Latest recorded delivery | [v2.11.3 evidence and Windows follow-up](releases/v2.11.3.md) |
| Current patch preparation | [Windows installer plan](plans/2026-10-11-windows-installer-followup.md), [candidate v2.11.4](releases/v2.11.4.md) — unpublished |
| New task, PR, checkpoint, decision, release | [Templates](templates/README.md) |

There is one canonical document per subject. Link existing architecture/development documentation rather than copying it into Issues. Store decisions in `docs/decisions/`, specifications in `docs/specs/`, and approved implementation plans in `docs/plans/`. An Issue records execution state and links the relevant document at its reviewed commit. Local scratch files are working material until reviewed and published.

Historical coverage is different from security support: all known versions are recorded; only the latest stable release receives security fixes. Missing historical Site tags and verification evidence remain explicitly unknown.
