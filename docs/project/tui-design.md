# TUI design record

The canonical design context is [`.impeccable.md`](../../.impeccable.md), tracked so fresh clones can read the same rules. Preserve the source tokens in `internal/app/display.go` and existing rendering contracts. This note links implementation and verification rather than copying the design policy.

The implementation uses ANSI escape sequences and its own terminal/input code. It does not depend on Bubble Tea or Lip Gloss. `isTrueColorSupported` and `makePalette` choose truecolor or ANSI fallback and honor `NO_COLOR`; current source governs exact behavior. Small terminals down to 28×12 are checked by the smoke suite.

Keep health/error indicators actionable and do not present a failed refresh as fresh quota. Decorative glyphs, width measurement and overflow need regression evidence before changes. Site story frames come from the native renderer and must be regenerated when that representation changes.

Older [operator-deck/grouping proposals](../plans/archive/2026-10-01-tui-proposals.md) contain superseded technology assumptions. They are historical design context, not a replacement specification. The canonical test and regeneration commands are in the [development guide](../development.md).
