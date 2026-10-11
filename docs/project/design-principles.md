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
