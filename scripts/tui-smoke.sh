#!/usr/bin/env bash
set -euo pipefail

binary="${1:-./agy-swap}"
if [[ ! -x "$binary" ]]; then
  printf 'tui-smoke: binary is not executable: %s\n' "$binary" >&2
  exit 1
fi
if ! command -v expect >/dev/null 2>&1; then
  printf 'tui-smoke: expect not installed; skipped\n'
  exit 0
fi

smoke_home="$(mktemp -d "${TMPDIR:-/tmp}/agy-swap-tui.XXXXXX")"
trap 'rm -rf "$smoke_home"' EXIT
binary="$(cd "$(dirname "$binary")" && pwd)/$(basename "$binary")"

run_shape() {
  local home_dir="$1"
  local columns="$2"
  local rows="$3"
  local label="$4"
  HOME="$home_dir" AGY_SWAP_REDUCED_MOTION=1 AGY_SWAP_NO_UPDATE_CHECK=1 AGY_SWAP_SKIP_SYNC=1 \
    AGY_SWAP_TUI_COLS="$columns" AGY_SWAP_TUI_ROWS="$rows" \
    AGY_SWAP_TUI_BINARY="$binary" \
    expect <<'EXPECT_SCRIPT'
set timeout 10
log_user 0
set stty_init "rows $env(AGY_SWAP_TUI_ROWS) columns $env(AGY_SWAP_TUI_COLS)"
spawn -noecho $env(AGY_SWAP_TUI_BINARY)
after 250
send "j"
after 80
send "k"
after 80
send "?"
after 120
send "x"
after 120
send "/"
after 120
send "demo"
after 120
send "\033"
after 200
send "q"
expect eof
EXPECT_SCRIPT
  printf 'tui-smoke: %s %sx%s passed\n' "$label" "$columns" "$rows"
}

run_shape "$smoke_home" 28 12 empty
run_shape "$smoke_home" 40 20 empty
run_shape "$smoke_home" 80 24 empty
run_shape "$smoke_home" 63 24 empty
run_shape "$smoke_home" 96 28 empty
run_shape "$smoke_home" 120 18 empty
run_shape "$smoke_home" 100 30 empty
run_shape "$smoke_home" 120 30 empty
run_shape "$smoke_home" 200 60 empty

# Seed realistic multi-account state so wide/stacked gauges, lists, and detail scroll paths are verified in PTY.
seeded_home="$(mktemp -d "${TMPDIR:-/tmp}/agy-swap-tui-seeded.XXXXXX")"
trap 'rm -rf "$smoke_home" "$seeded_home"' EXIT
mkdir -p "$seeded_home/.gemini/agy-swap"
cat > "$seeded_home/.gemini/agy-swap/accounts.json" <<'JSON'
{
  "work@example.invalid": {
    "email": "work@example.invalid",
    "name": "Work Account",
    "quota_schema": 2,
    "quota_snapshot": {
      "observed_at": "2030-01-01T00:00:00Z",
      "tier": {"id": "pro", "name": "Google AI Pro"},
      "groups": [
        {"id": "gemini", "name": "Gemini models", "buckets": [{"id": "g-w", "name": "Weekly", "window": "weekly", "remaining_fraction": 0.72, "reset_at": "2030-01-05T00:00:00Z"}, {"id": "g-5h", "name": "5-Hour", "window": "5h", "remaining_fraction": 0.45, "reset_at": "2030-01-01T04:00:00Z"}]},
        {"id": "third_party", "name": "Claude and GPT models", "buckets": [{"id": "3p-w", "name": "Weekly", "window": "weekly", "remaining_fraction": 0.31, "reset_at": "2030-01-04T00:00:00Z"}]},
        {"id": "argon", "name": "Gemini 4 Argon", "buckets": [{"id": "a-w", "name": "Weekly", "window": "weekly", "remaining_fraction": 0.88, "reset_at": "2030-01-06T00:00:00Z"}]}
      ]
    }
  },
  "personal@example.invalid": {
    "email": "personal@example.invalid",
    "name": "Personal Account",
    "quota_schema": 2,
    "quota_snapshot": {
      "observed_at": "2030-01-01T00:00:00Z",
      "tier": {"id": "free-tier", "name": "Free"},
      "groups": [
        {"id": "gemini", "name": "Gemini models", "buckets": [{"id": "g-w", "name": "Weekly", "window": "weekly", "remaining_fraction": 0.12, "reset_at": "2030-01-02T00:00:00Z"}]}
      ]
    }
  }
}
JSON
chmod 0600 "$seeded_home/.gemini/agy-swap/accounts.json"

run_shape "$seeded_home" 40 20 seeded
run_shape "$seeded_home" 80 24 seeded
run_shape "$seeded_home" 96 28 seeded
run_shape "$seeded_home" 120 18 seeded
run_shape "$seeded_home" 120 30 seeded
