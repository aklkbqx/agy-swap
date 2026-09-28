# AGY Live, install page, brand, and section motion plan

> Planning artifact only. Implement with `executing-plans` after the user approves this direction. The repository's ship contract applies to a later implementation task; this plan does not authorize a commit, release, or deployment.

## Goal and decisions

Make the real Go TUI demo visibly ready without a Start button; make `/install.html` look and behave like the main site; replace the plain text nav mark with a deliberate typographic `agy-swap` wordmark; and give every homepage section a distinct but continuous 2D scroll depth. Keep the earlier decision to remove the 3D model. The `3d-web-experience` skill informs depth, framing, performance, and fallbacks; GSAP ScrollTrigger supplies scroll progress. The art supports the product story and does not imitate working controls.

“Every session” is interpreted as every homepage **section**: hero, first switch, live demo, features, install, and the footer transition. The install notes page gets a restrained static or slow decorative treatment so reading commands remains the priority.

## Evidence and problems in the current build

- `site/src/components/AgyLiveTerminal.jsx` gates the terminal behind `started=false`, renders a Start panel, and calls `terminal.focus()` on creation. A visitor who reaches the demo still has to make an extra click. Automatic focus would steal keyboard input from the page when this gate is removed.
- `site/src/App.jsx` registers GSAP layers only in the hero, getting-started, and features sections. The demo and install areas have no parallax scene, so the visual arc stops halfway through the page.
- The nav brand in `App.jsx` is plain `agy-swap` text. The existing account folder art is product-related, but its layered file-folder treatment reads as a separate illustration rather than a consistent visual language carried through the page.
- The main install section uses a radio fieldset; `site/public/install.html` is a standalone HTML file with inline styles, a native select, copied command strings, and separate copy behavior. This is why “Full install notes” looks unrelated to the main site.
- The dark hero's orange primary button is clear, but orange also appears in links, status, paths, and selection. A warm-white primary CTA with dark text should create cleaner hierarchy. Apply this to primary actions only; orange remains the TUI's selection signal. On light mode, use a dark primary surface for contrast instead of a white button on white.
- Existing `--gradient-agy` introduces purple/cyan that do not explain account switching. Audit visible gradients, generic glows, repeated claims, and floating objects against the actual TUI and `.impeccable.md` before adding art.

## Visitor experience and acceptance criteria

1. AGY Live always renders its terminal frame, status, instruction, and mobile key controls when the page loads. No Start button or HTML account selector appears. As the demo approaches the viewport, the actual same-origin Go demo session connects automatically. Entering the section shows native output as soon as the connection is ready. The frame says “Connecting…” during a slow connection and offers Retry after failure. No initial focus or scroll jump occurs; clicking/tapping/focusing the terminal enables keyboard input.
2. Auto-connect happens once per visit to the demo section, with a bounded prefetch distance rather than opening a WebSocket for every homepage visitor. Leaving the page closes the session. Restart creates a fresh isolated synthetic session. Gateway capacity/overload returns an honest error/retry state. The server-side demo policy and isolation remain the source of truth.
3. `/install.html` is a Vite-built page at the same URL. It shares the site's design tokens, wordmark, copy-command presentation, and a **reusable platform dropdown component** with the homepage. Both pages offer the same macOS/Linux and Windows choices and command values. The dropdown has a visible label, keyboard support, focus state, and native select semantics; copying reports success or failure accessibly. If the current radio treatment proves easier on desktop in visual QA, the shared component may render radio options above a width breakpoint, but its mobile dropdown and selection logic must stay identical on both pages.
4. A new typographic logo spells `agy-swap` correctly at nav size, works on dark and light backgrounds, and has a legible favicon. Use image generation for 2–3 visual concepts, then reconstruct the chosen letterforms in a clean SVG or licensed type treatment. Do not ship AI-generated pseudo-letters as the functional wordmark. Keep accessible text in the link and update favicon, manifest, and social artwork where appropriate.
5. Each homepage section has a coherent scroll-linked 2D composition with at least two depth planes where space permits. At 25%, 50%, and 75% scroll positions, the planes visibly move at different rates. Content, buttons, code, and the xterm DOM remain stable and clickable. Reduced-motion mode is a complete static composition. Mobile uses smaller travel and fewer layers, with no horizontal overflow.
6. Primary CTA is warm white on the dark theme and dark on the light theme, with high-contrast hover/focus/disabled states. Copy, retry, restart, selectors, and mobile terminal keys retain quieter control styling. Orange stays for selected account, route, and emphasis, not every clickable surface.

## Visual and motion direction

Use the TUI's graphite background, warm white text, orange selection line, small green healthy signal, and monospaced data as the visual grammar. The wordmark can modify the hyphen into a short switching path between `agy` and `swap`; letterforms should remain readable at 120–150 CSS px total width. Generated imagery should be transparent 2D fragments tied to real concepts: saved account identities, a route merging into one active session, quota markers, and a terminal cursor. Reject generic orbs, glass cards, pseudo-terminal text, fabricated controls, and 3D hardware.

| Section | Story and scroll layers | Interaction boundary |
|---|---|---|
| Hero | Sparse account marks in the far plane; one orange route and active-session signal in the near plane. Strongest depth, no text distortion. | Wordmark, heading, and CTA remain fixed in document flow. |
| First switch | Route bends across the install → add account → switch sequence; account marks move slower than the route. | Step links and copy stay still. |
| AGY Live | The route arrives behind the terminal and settles. Subtle side rails/markers move around the shell. | Never transform the terminal, key strip, or status text. |
| Features | Quota ticks and one selected-account line continue the route at lower amplitude. | Feature headings and explanations remain stable. |
| Install | The route resolves into a command prompt/cursor behind the command block, with the smallest travel. | Platform dropdown and copy button stay still. |
| Footer transition | One quiet continuation/fade of the route closes the panorama. | Navigation and legal/readable text stay still. |

Use `ScrollTrigger` with section-based `start`/`end` and `scrub`, transform-only x/y/opacity on decorative elements, and `gsap.matchMedia()` for desktop, mobile, and `prefers-reduced-motion`. Give each section a named scene wrapper and an explicit foreground/background layer so motion is deliberate and testable. Keep scroll native: no pinning, snapping, or scroll hijack. Dispose triggers on unmount and refresh measurements after image/font load or responsive layout changes. Cap movement within safe gutters at 390 px, 768 px, and 1440 px; use static images when reduced motion is requested.

## Architecture and file map

| Work | Main files | Interface/behavior |
|---|---|---|
| Terminal auto-start | `site/src/components/AgyLiveTerminal.jsx`, `.module.css`, `terminalClient.js`, `site/tests/live-terminal.test.mjs` | Keep shell mounted; `IntersectionObserver` initiates one session when near viewport; status/retry/restart state machine; no automatic focus. |
| Shared platform control | New `site/src/components/PlatformPicker.jsx` and CSS; `site/src/App.jsx`; new `site/src/InstallPage.jsx` | Controlled `value`/`onChange`, labelled native select for dropdown behavior, shared options from one install-command module. |
| Shared install commands | New `site/src/installCommands.js`, plus both page components and tests | One source for shell, Windows, Go, and proxy commands; the same copy utility/status behavior on both pages. |
| Multi-page build | Move `site/public/install.html` to `site/install.html`; add install entry module; `site/vite.config.mjs`; `site/tests/app.test.mjs`, deployment tests | Vite builds `/` and `/install.html`; production Docker/Nginx still serve the exact existing URL and metadata. |
| Brand and tokens | `site/src/styles.css`, `App.module.css`, shared logo component, `site/public/` assets, `site/index.html`, `site/install.html`, `site/public/site.webmanifest` | Generated concept → corrected vector wordmark; dark/light variants, favicon, metadata/social art; white/dark primary action tokens. |
| Section panorama | `site/src/App.jsx`, `App.module.css`, optionally one `useSectionParallax` hook or scene component; optimized images under `site/public/` | One section scene for each homepage content section, ScrollTrigger setup/cleanup, responsive/reduced-motion variants. |
| Copy and release surfaces | `site/src/i18n/translations.js`, `README.md`, install/docs references, screenshots/SEO as affected | Accurate labels for automatic real demo and common install controls in every supported language; no obsolete Start instruction. |

## Implementation sequence

### Task 1: Establish baseline and visual rules

- [ ] Recheck `git status --short`, manifests/lockfile, and current source before editing. Preserve all unrelated Go and docs changes already present.
- [ ] Capture current homepage and install page at 390, 768, and 1440 px, dark/light, using `agent-browser`. Record the current selected controls, button contrast, reading order, and section scroll positions.
- [ ] Define a tiny visual decision sheet: wordmark geometry, spacing, palette roles, CTA hierarchy, art motif, and motion limits. Run an anti-slop pass on every current decorative image/gradient and every section heading; remove elements that do not explain account selection, quota, switching, or installation.

### Task 2: Make the native demo immediately usable

- [ ] Add focused terminal tests for shell visible without Start, observer-triggered connection, one socket only, no focus theft, Retry/Restart, unmount cleanup, and mobile key senders.
- [ ] Remove `started` gate. Render the terminal host and status on first paint; load xterm and open `/demo/ws` when within a near-viewport `IntersectionObserver` margin. Maintain connecting/ready/error/closed states and an explicit Retry after connection failure.
- [ ] Check the gateway's existing max-session and idle limits against automatic entry. Verify two visitors remain isolated, page leave frees capacity, and overload does not loop reconnects.

### Task 3: Unify install pages and dropdown

- [ ] Add a common install-command module and a controlled `PlatformPicker` with a labelled `<select>`; use site tokens and the same responsive width, radius, icon/chevron, selected value, focus ring, and touch target on both pages. Prefer the native select for reliable keyboard/mobile behavior; do not build a fake listbox solely for appearance.
- [ ] Convert the public static page to Vite multi-page HTML and React `InstallPage`, keeping `/install.html`, canonical/OG/robots behavior, and useful HTML fallback/metadata. Move its inline CSS and script into shared styles/components.
- [ ] Reuse the command block/copy feedback treatment from the homepage. Test choosing both platforms and copying exactly the matching command on each route, including clipboard failure and keyboard use.

### Task 4: Produce brand assets and section scenes

- [ ] Generate 2–3 image concepts for a bespoke `agy-swap` typographic mark: terminal lettering, a readable switch-path hyphen, graphite/warm-white/orange palette, flat 2D. Review spelling and small-size legibility; manually vectorize/refine the chosen concept and create dark/light/favicon versions. Update social art if the old mark becomes inconsistent.
- [ ] Generate or draw only product-specific transparent 2D fragments needed for the six-scene route. Optimize file sizes, specify intrinsic dimensions, and avoid decorative text or fake controls in raster artwork.
- [ ] Extract/extend parallax scene setup for every homepage section. Keep separate decorative containers behind content, implement mobile/reduced-motion variants, then visually tune positions and scroll travel section by section.
- [ ] Switch dark primary CTA to warm white with charcoal text and light primary CTA to charcoal with white text. Audit hover/focus, disabled, and contrast across homepage, install page, and demo recovery actions.

### Task 5: Verify and ship in the implementation task

- [ ] Run focused site tests, `make qa`, `make test`, and `cd site && bun run build`. Update tests only where behavior changed; test the terminal connection lifecycle, exact install commands, and multi-page build output.
- [ ] Use `agent-browser` to inspect `/` and `/install.html` at 390/768/1440 px, dark/light and reduced motion. Capture top/middle/bottom scroll positions of each section; verify actual differential layer transforms, no overlaps or horizontal overflow, dropdown/CTA keyboard behavior, native TUI interaction, and accessibility checks. Close browser sessions.
- [ ] Review translations, metadata, sitemap, images, docs, download/install copy, release checks, stale Start text, secrets, and scoped diff. Regenerate affected screenshots. Choose SemVer from actual impact (likely a minor visual/behavior feature), then follow the repository's local/private-runner release and Docker/Nginx deployment process with live verification. Keep production `:latest` references while deploying the site image by pinned digest through the documented runner.

## Risk checks

- Opening a WebSocket near viewport increases demo sessions: use one-shot observation, gateway limits, proper teardown, and a calm capacity message.
- The static install page currently has no React runtime: moving it into Vite multi-page output must preserve URL, SEO head content, direct navigation, and Docker asset paths.
- Image generation is exploratory for lettering: final wordmark must be corrected vector artwork with selectable accessible text alongside the image.
- A white button on the light theme would disappear; primary action styling is theme-specific. Orange must remain meaningful in the TUI.
- Parallax can reduce clarity or performance: animate only decorative 2D transforms, never terminal/controls, and accept a fully static reduced-motion rendering.
