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
  local columns="$1"
  local rows="$2"
  HOME="$smoke_home" AGY_SWAP_REDUCED_MOTION=1 \
    AGY_SWAP_TUI_COLS="$columns" AGY_SWAP_TUI_ROWS="$rows" \
    AGY_SWAP_TUI_BINARY="$binary" \
    expect <<'EXPECT_SCRIPT'
set timeout 10
log_user 0
set stty_init "rows $env(AGY_SWAP_TUI_ROWS) columns $env(AGY_SWAP_TUI_COLS)"
spawn -noecho $env(AGY_SWAP_TUI_BINARY)
after 250
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
  printf 'tui-smoke: %sx%s passed\n' "$columns" "$rows"
}

run_shape 28 12
run_shape 40 20
run_shape 80 24
run_shape 120 30
