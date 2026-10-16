## Design Context

### Users

Operators who manage several Google Antigravity accounts from a terminal. The
primary job is to identify account health quickly, switch the active session,
and recover from quota or token problems without leaving the TUI.

### Brand Personality

Focused, technical, and dependable. The interface should feel like an
operator console: calm under pressure, information-dense, and explicit about
state.

### Aesthetic Direction

Dark terminal-first operator deck with a restrained orange/green/cyan palette.
Tables, aligned columns, and thin rules carry the structure. ASCII-safe labels
and fallback glyphs are preferred whenever a decorative Unicode character could
corrupt alignment or render as a replacement glyph. Motion is brief and
purposeful, with reduced-motion support.

### Design Principles

- Make account identity, selection, active state, and health readable in one scan.
- Use fixed table columns and measured terminal widths; never let content move a rule.
- Reserve color for state and action, not decoration.
- Prefer resilient text and glyph fallbacks over fragile visual flourishes.
- Keep destructive actions explicit and preserve keyboard-first navigation.

### Dashboard model (v2.13)

- An account is judged by what it can be used for now, not by one summary number. Every account has up to two model pools (Gemini, Claude/GPT), each with a weekly and an optional 5-hour limit; the row shows a status word and one meter per pool, and the selected account gets one gauge panel per pool.
- The status words are `READY`, `LOW`, `GEMINI ONLY`, `CLAUDE/GPT ONLY`, `LIMITED`, `NO TOKEN`. Color never stands alone: every state has a word, and filled chips become `[ WORD ]` without color.
- Thresholds are shared by the list, the gauges and the CLI: more than 30% is green, 30% or less is yellow, 10% or less is red.
- A used-up pool says when it is back. A 5-hour limit that waits for the weekly limit reads "paused until weekly resets" and never draws a full bar.
- Detail shrinks with the space: big digits (5 rows, then 3 rows), then plain limit lines, then a tight form for very short windows. The terminal size picks the arrangement; the free rows pick the detail.
- Glyphs have ASCII stand-ins of the same width, so a frame keeps its shape on terminals that cannot draw blocks.
