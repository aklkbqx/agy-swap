# Complete history and latest-stable security support

Date: 2026-10-11. State: accepted by the owner. Coordination: [history task #9](https://github.com/aklkbqx/agy-swap/issues/9).

## Decision

Preserve every observed Tool version tag, original Release metadata and nullable evidence in the [release inventory](../releases/index.json). Do not manufacture missing Releases, old task lists or deployment checks. Historical documentation coverage does not imply security maintenance: only the latest stable release receives security fixes, as [SECURITY.md](../../SECURITY.md) defines.

The helper reads all API pages, rejects conflicting/removed immutable tag records, keeps draft/prerelease states separate, and preserves human delivery records. A failure leaves the reviewed history untouched. Version-neutral labels and native Milestones avoid a fixed list of future versions.

## Consequences

Older versions remain discoverable and tag-only versions remain explicit. Current release evidence is richer than unverified historical metadata. Future releases append records and checkpoints; they do not retag old commits or overwrite human notes. Post-delivery history/evidence commits document the already published release without creating an endless new-release cycle.
