# 2026-10-11 audit remediation

Historical source: the local audit-remediation plan approved for execution on 2026-10-11. Delivered in [v2.11.2](../../releases/v2.11.2.md). This public summary omits private operational context and obsolete links.

The plan preserved the existing renderer and credential stores while correcting deterministic fixtures, credential/security/network documentation, Homebrew publication guards, four-locale Doctor copy, local PTY boundaries, and Windows credential preparation/repeat/rollback behavior.

The [workstream map](../../releases/v2.11.2-workstreams.md) records the source and delivery evidence for each finding. Tool QA, Site tests/build and native Windows validation were recorded. The Windows installation was intentionally retained at its old version while a temporary release candidate was tested; IDE/chat generation was not tested.

Status: implementation shipped, with the existing [Windows reporter Issue #4](https://github.com/aklkbqx/agy-swap/issues/4) still awaiting reporter-environment confirmation. Historical task imports must preserve that distinction.
