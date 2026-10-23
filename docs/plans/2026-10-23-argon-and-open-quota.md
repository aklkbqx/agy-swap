# Argon pool and open quota schema plan (v2.15.0)

**Goal:** Read the Gemini 4 Argon weekly pool and the Gemini weekly limit that Google already reports, keep pools and windows Google adds later instead of dropping them, and let users pick accounts by Argon quota, without one used-up Argon pool benching an account.

**Architecture:** The Code Assist requests identify as a recent Antigravity hub client, so Google answers with every pool. The summary parser names each group from its bucket ids (`gemini-argon-` before `gemini-`, then `3p-`, then the bucket id itself), merges groups that share a name and keeps any well-formed window. The store validates ids and windows by shape instead of by allowlist. One small catalog (`internal/app/pools.go`) maps a model family to the pools it draws from per account. The TUI lays side-by-side panels out two to a row and the quota overview adds an Argon column when an account has one.

**Tech stack:** Go standard library only. No new dependencies.

**Issues:** Tool [#65](https://github.com/aklkbqx/agy-swap/issues/65), Site [aklkbqx/agy-swap-site#27](https://github.com/aklkbqx/agy-swap-site/issues/27); follows the Gemini weekly report in [#50](https://github.com/aklkbqx/agy-swap/issues/50).

## Findings (observed 2026-10-23 against v2.14.0)

| Finding | Evidence |
| --- | --- |
| Google chooses the quota groups from the client version in the `User-Agent` | `AGY_SWAP_QUOTA_DEBUG=1 agy-swap list --refresh` with `AGY_SWAP_USER_AGENT` set, four saved accounts |
| `antigravity` (sent until v2.14.0) and `antigravity/<1.x>` get only Claude/GPT | `3p-weekly`, `3p-5h` only; Gemini then came from the model-list fallback, which cannot tell the weekly limit from the 5-hour one |
| `antigravity/2.10`+ (with or without `hub/`) adds `gemini-weekly` and `gemini-5h` | `antigravity/hub/2.10.0`, `antigravity/2.23.0 darwin/arm64`, `antigravity/2.30.0 darwin/arm64` |
| Only `antigravity/hub/` from 2.20 adds `gemini-argon-wtu-weekly` | `antigravity/hub/2.20.0`, `hub/2.23.0`, `hub/9.0.0 linux/amd64`; not `hub/2.15.0`, not `antigravity/2.30.0 darwin/arm64`; platform does not matter |
| Probing the installed `agy --version` would not help | the CLI reports `1.4.6`, below every threshold above |
| Argon is weekly only; one of two Pro accounts had it, the Ultra accounts did not | Google's description: "For paid Pro plans, Argon quota is separate. Higher Argon quota (included as part of the larger Gemini quota bucket) is available on Ultra plans." |
| v2.14.0 would file Argon under `gemini` | `quotaGroupsFromSummary` matched `gemini-` first, so the Argon group was saved as a second `gemini` group |
| The model-list fallback reads every `gemini-*` model | `geminiGroupFromModels`; Argon models are named `gemini-4-argon-*` and would lend their quota to Gemini |
| The CLI flag is `--family`, accepted only `claude`, `gemini`, `gpt` | `parseCLI`, `parseExtended`, `setConfigValue`, `normalizeSettings`, TUI forms |
| Side-by-side panels drew two pools only | `tuiPoolPanels` (`panelsSide` branches) |

## Decisions

- Default `User-Agent`: `antigravity/hub/2.23.0 <GOOS>/<GOARCH>`; `AGY_SWAP_USER_AGENT` overrides it. No `agy --version` probe.
- `--family argon` reads the Argon pool when the account has one and the Gemini pool otherwise (Ultra). Unknown pools are listed but are not families.
- No family (plain `next`, `recommend`, the status word) judges the shared Gemini and Claude/GPT pools only, so a used-up Argon pool never benches an account. Log and manual limits apply when their family's pools overlap the request's.
- The account row keeps its two meters; Argon gets a gauge panel and a quota-overview column. A weekly-only pool is never drawn as a paused 5-hour limit.
- Out of scope for v2.15.0: G1 credits, reading quota through `agy -p /usage`.

## Tasks

- [x] Spike: compare `User-Agent` values against the live quota summary (findings above).
- [x] `cloudUserAgent` and `AGY_SWAP_USER_AGENT` (`internal/app/user_agent.go`, `user_agent_test.go`).
- [x] Pool catalog (`internal/app/pools.go`): pool names, id/window shape checks, `familyPools`, `familiesOverlap`.
- [x] Summary parser: Argon before Gemini, open ids and windows, merge same-id groups; model-list fallback skips Argon models (`quota.go`, `quota_argon_test.go`, `quota_diagnostics_test.go`).
- [x] Store: shape checks for group ids and windows; `argon` limit family (`store.go`).
- [x] Families: `argon` in `--family`, `limit --group`, `policy.prefer_family`, profiles and TUI forms; `modelFamily` maps Argon models to `argon`; `quotaWait`, `accountRemaining`, `accountCooldown` use the catalog.
- [x] TUI: status words ignore non-shared pools; panels two to a row; Argon overview column; weekly-only pools never paused (`tui_argon_test.go`).
- [x] Demo: Pro accounts carry Argon, the Ultra account does not; site frames regenerated.
- [x] Docs: README, CHANGELOG, design principles, CLI usage.
- [x] Site: legend copy in `en`, `th`, `ja`, `zh`; frames test (`site/tests/argon-pool.test.mjs`).
- [x] `make qa vuln install`, Site install/build/tests, live `agy-swap limits --refresh` with the candidate (Gemini weekly on all four accounts, Argon on the one Pro account that has the pool).
- [ ] `make site-preview` (owner starts Docker) and owner review gate.
- [ ] Release v2.15.0 in both repositories, Homebrew, site image, live verification, release evidence.
