# Windows installer and documentation follow-up

Owner requested creating GitHub tasks and implementing both review findings on 2026-10-11. Public work: [installer #19](https://github.com/aklkbqx/agy-swap/issues/19), [documentation #20](https://github.com/aklkbqx/agy-swap/issues/20). Target: v2.11.4 patch; v2.11.3 remains published stable until delivery.

## Required behavior

A downloaded `install.ps1` must execute on Windows PowerShell 5.1 without a BOM/encoding workaround. Use ASCII status messages so both saved-file and UTF-8-decoded invocation work; preserve download URL selection, certificate verification, SHA-256 verification, the binary version verification contract, target directory and replacement/rollback semantics. Test real PowerShell file execution with controlled download fixtures, then an isolated native install using the actual published executable. Never change the user's active account or configuration for installer tests.

Compare the complete brand/version tokens and require a successful binary self-test before replacement; a prefix match is insufficient. This also fixes an existing validation defect discovered in candidate review.

Current documentation must state that Issue #4 closed after owner-authorized native verification and that the Windows installation now uses v2.11.3. Historical observations must retain their dates and original context, with dated follow-ups rather than invented reporter confirmation. Latest stable and a local candidate are distinct states.

## Global constraints

- Go 1.26.0; existing standard-library test tooling only.
- Windows PowerShell 5.1 is the native saved-file regression target.
- GitHub is canonical: tracked docs, Issues, Milestones, immutable tags and Releases.
- Tool and Site remain separate repositories; no private deployment or account data in public records.
- No GitHub Actions, new hosting provider, external implementation service or production SSH deployment.
- Production image reference remains `:latest`; local Docker/Nginx/Watchtower flow.
- Local owner review gate applies before commit, tag, push, release or image publication.

## Acceptance and limits

Native installer tests cover successful install and backup, checksum rejection with previous bytes unchanged, candidate version rejection (different version, prefix collision and prerelease suffix), failed self-test rejection, spaces in target paths, cleanup, and UTF-8-decoded execution. Tool `make qa`, Site test suite and production build pass. Matching patch metadata is prepared across both repositories. Native test fixtures are temporary and do not touch credential/account state. Approval precedes publication; installation and live verification precede task closure. This scope does not assert new IDE/chat-generation coverage.

Encoding reference: [Microsoft character encoding documentation](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_character_encoding?view=powershell-7.6).
