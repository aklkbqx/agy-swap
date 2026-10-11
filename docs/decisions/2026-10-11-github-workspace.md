# GitHub as the canonical project workspace

Date: 2026-10-11. State: accepted by the owner. Coordination: [Epic #5](https://github.com/aklkbqx/agy-swap/issues/5).

## Context and decision

All contributors need versioned project guidance, findable tasks and handoffs that survive a chat/session change. Use tracked repository docs for knowledge/specs/plans/decisions, Issues for actionable work and checkpoints, PRs for review, and Milestones/tags/Releases for delivery.

Public project guidance lives in the documentation index and GitHub workflow. Private operational instructions remain in the private repository. Templates are ordinary documents outside `.github/`. Issues remain authoritative when a Project board is unavailable.

## Alternatives and consequences

A Wiki or board-first workspace would duplicate knowledge or require unavailable access. Repository docs and Issues work with current permissions and provide review/history. A future private Project can present the same work without becoming another task source of truth.

Public Tool material is sanitized; private Site source and operational details stay private. GitHub project management does not enable GitHub Actions, change product hosting, expand token scopes or authorize external implementation services. Owner review and self-hosted `:latest` delivery remain part of the contract.
