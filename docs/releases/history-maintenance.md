# Source history maintenance

Repository source history was prepared on 2026-10-11 and republished on 2026-10-12. Current release records and source links identify the resulting source commits. All 47 version names and recorded tag dates are preserved.

Published binary assets, checksums, release dates and original build identifiers retain their existing values. Product source and behavior did not change. A commit reference in current documentation identifies the current source tree, rather than a newly built binary.

Existing clones need a fresh checkout of the current remote history before publishing further changes; merging old history can restore removed working material. Historical installation and deployment observations remain dated evidence.

## Inventory reconciliation — 2026-10-28

The generated release index still contained 57 source commit IDs from before the completed history migration. Each transition was checked against the existing migration commit map and the current remote tag commit before updating the metadata. No tag, Release body/date, binary asset or checksum was changed by this reconciliation.

The normal history helper then regenerated the index from complete current API observations: 70 version tags, 69 published Releases, latest stable v2.18.4. All previously recorded evidence classifications and document links were preserved, and v1.5.0 remains tag-only without an invented release date or assets. The already-published v2.18.4 delivery summary is linked from its new entry.

The helper's immutable-tag and missing-history guards remain unchanged. Future refreshes must still reject an unexplained commit change. The existing helper regression suite and repeated write/check runs passed; old inventory files and the reconciliation proof were retained in private maintenance evidence.

Refs [#94](https://github.com/aklkbqx/agy-swap/issues/94). This metadata-only update does not rebuild or replace v2.18.4 binaries or the deployed Site image.
