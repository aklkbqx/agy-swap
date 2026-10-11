# Version delivery record

- Release state and UTC date:
- Tool commit, immutable tag and public Release:
- Site commit and matching tag: keep private commit details in the private repository.
- Accepted Issues and PRs:

## Verification

Record Tool QA, Site checks/build, platform-specific checks, visual review when relevant, owner approval, and limitations. Distinguish historical recorded evidence from checks performed for this release.

## Installation and publication

Record installed binary path/version/build ID, asset names/checksums, download verification and package publication. State any platform whose existing installed version was intentionally retained.

## Site delivery

Record image tags and registry digest, public version/assets/health checks, and rollback. Production remains `:latest`. Put sensitive operational detail in the private repository. Mark an unaffected surface explicitly rather than inventing delivery evidence.

## Remaining work

Link unresolved Issues, reporter confirmations and deferred acceptance. Missing evidence remains unknown. Historical documentation covers all versions; security support covers only latest stable.
