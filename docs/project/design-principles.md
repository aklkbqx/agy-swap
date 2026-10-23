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
Solid bars and cards carry the structure: no outer frame and no full-width rules, only one quiet divider between the list and the gauges. ASCII-safe labels
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

- An account is judged by what it can be used for now, not by one summary number. Every account has the shared model pools (Gemini, Claude/GPT), each with a weekly and an optional 5-hour limit, and some Pro accounts add a weekly Argon pool. The row shows a status word and one meter per shared pool; the selected account gets one gauge panel per pool, two to a row when they sit side by side; the quota overview adds an Argon column when any listed account has one. Status words judge the shared pools only.
- The status words are `READY`, `LOW`, `GEMINI ONLY`, `CLAUDE/GPT ONLY`, `LIMITED`, `NO TOKEN`. Color never stands alone: every state has a word, and filled chips become `[ WORD ]` without color.
- Thresholds are shared by the list, the gauges and the CLI: more than 30% is green, 30% or less is yellow, 10% or less is red.
- A used-up pool says when it is back. A 5-hour limit that waits for the weekly limit reads "paused until weekly resets" and never draws a full bar.
- The frame is at most 160 columns wide and centred on a wider terminal, like a page container; inside it a panel gets at most one row of air between its sections. With one gauge column (92 to 119 columns) the digits sit beside the bars.
- Detail shrinks with the space: big digits (5 rows, then 3 rows), then plain limit lines, then a tight form for very short windows. The terminal size picks the arrangement; the free rows pick the detail.
- The account list shows names only (no initials) and a dot only for the account in use. The selected account carries an orange bar `┃` down the left edge of every line of its block, on a gray tint with a row of padding above and below and a column on the right, and a blank row of margin outside the tint; the website legend shows the same mark.
- Green, yellow and red belong to quota state only. The active account is blue: its `●` and the word `ACTIVE` beside its name (yellow when it needs attention); on a narrow row the word gives way before the name. Orange marks the selection.
- Every question and the help screen use one dialog: title, a short body, then a row of buttons chosen with the arrows and pressed with Enter. The chosen button is `[ Label ]` on the selection tint; destructive questions start on Cancel. A modal (dialog, palette, form) sits on a backdrop: everything behind it but the status line and key hints is redrawn in one dim gray.
- Spacing is one unit everywhere: one row of padding above and below and two columns either side of the content in the bars and the cards, two columns at the screen edges and either side of the divider. Side-by-side cards are the same width and end flush with the column; an odd column goes to the gap between them.
- Colour of surfaces: the body is black (`#0a0c10`, the website's background) and every other surface is one ramp of cool grays above it: bars and cards `#1c2128`, the selection tint `#2d333b`, key chips and quiet lines `#373e47`, text behind a modal `#545d68`. 256-colour terminals use 232, 234, 236 and 238.
- The screen has no line frame. The header (logo, active account) and the footer (status line, key hints) are solid bars in one cool gray that run edge to edge, closed into a solid frame by a one-column rail down each side, outside the content's two columns of air, so bar text and body text line up; keys are chips on a lighter gray, the same width as their `[Key]` form, which is what a terminal without colour shows. Colour inside a bar never breaks it.
- A quota pool is a solid card on the same gray with its state colour as a bar `▌` down the left edge; stacked cards are parted by a blank row. Without colour a card keeps an outline, the only way to group it.
- Motion shows a change: 120–250 ms, eased out, and a new change starts from where the old one is. Two things loop: the light that sweeps the screen while a refresh the user asked for runs, and the active dot's slow breath at rest (truecolor only, ten frames a second). Frames run only while something moves, and only changed rows are repainted. Every animation is a function of the render clock, so tests sample it; `AGY_SWAP_REDUCED_MOTION=1` draws every end state at once.
- Glyphs have ASCII stand-ins of the same width, so a frame keeps its shape on terminals that cannot draw blocks.
