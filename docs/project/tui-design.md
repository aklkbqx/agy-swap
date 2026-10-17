# TUI design record

The canonical design context is [design principles](design-principles.md), tracked so fresh clones can read the same rules. Preserve the source tokens in `internal/app/display.go` and existing rendering contracts. This note links implementation and verification rather than copying the design policy.

The implementation uses ANSI escape sequences and its own terminal/input code. It does not depend on Bubble Tea or Lip Gloss. `isTrueColorSupported` and `makePalette` choose truecolor or ANSI fallback and honor `NO_COLOR`; current source governs exact behavior. Small terminals down to 28×12 are checked by the smoke suite.

Keep health/error indicators actionable and do not present a failed refresh as fresh quota. Decorative glyphs, width measurement and overflow need regression evidence before changes. Site story frames come from the native renderer and must be regenerated when that representation changes.

Older [operator-deck/grouping proposals](../plans/archive/2026-10-01-tui-proposals.md) contain superseded technology assumptions. They are historical design context, not a replacement specification. The canonical test and regeneration commands are in the [development guide](../development.md).

## Layouts and glyphs (v2.13)

`tuiLayoutPlanFor(width, height)` in `internal/app/tui_layout.go` decides the arrangement from the width (wide from 92 columns, stacked from 64, compact from 41, narrow below) and the digit mode from the rows. The body builder then keeps the richest digit mode whose detail fits the rows actually available, so a short window shows plain limit lines instead of a cut-off panel. The pool model (`tui_pools.go`) reads the saved quota snapshot only; rendering lives in `tui_rows.go`, `tui_detail.go`, `tui_panel.go` and `tui_body.go`.

`bigNumberScaled` repeats that bitmap at scale 1 to 6 (the dashboard stops at 2) (a pixel is `2*scale` columns by `scale` rows, so digits stay square) and `tuiFitPlan` picks the largest scale whose digits fit a panel and whose panels fit the rows, then thickens the bars (to 2 rows) and adds up to three blank rows of air. `bigNumber` draws 5-row digits from one 3 by 5 bitmap and packs the same bitmap into 3 rows with half blocks. `asciiLines` maps every glyph to a one-column ASCII character; it is used when `AGY_SWAP_ASCII=1`, on a Linux console or dumb terminal, or with a non-UTF-8 locale. Block elements count as one column in `terminalRuneWidth`; terminals that draw them double wide need the ASCII mode.

Verification: `TestEveryTerminalSizeProducesAnExactFrame` sweeps sizes from 28 by 12 upward with 0, 1, 3 and 12 accounts and checks the exact row and column count; the smoke script runs seven sizes; the story frames for the website come from the same renderer.
