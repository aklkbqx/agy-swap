#!/usr/bin/env bash
# Record the showcase clips in scripts/tapes/ with the demo binary and
# fictional @example.invalid accounts. No real credentials are read.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

for tool in vhs ttyd ffmpeg go; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "error: '$tool' is not installed (brew install vhs installs ttyd and ffmpeg)." >&2
    exit 1
  fi
done

VERSION="${VERSION:-$(sed -n 's/^VERSION ?= //p' Makefile)}"
if [ -z "$VERSION" ]; then
  echo "error: VERSION is not set and could not be read from Makefile." >&2
  exit 1
fi

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/agy-media-XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT
BIN_DIR="$WORK_DIR/bin"
LOG="$WORK_DIR/invocations.log"
mkdir -p "$BIN_DIR" "$WORK_DIR/home" docs/assets site/public/media

echo "==> Building demo binary (v${VERSION})..."
go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o "$BIN_DIR/agy-swap-demo" ./cmd/agy-swap-demo

# NewDemo seeds accounts.json and refuses to overwrite an existing one, so
# every call gets a fresh demo home. The log proves the tapes ran this
# wrapper and not an agy-swap installed elsewhere on PATH.
cat >"$BIN_DIR/agy-swap" <<EOF
#!/usr/bin/env bash
demo_home="\$(mktemp -d "$WORK_DIR/home/run-XXXXXX")"
status=0
HOME="\$demo_home" "$BIN_DIR/agy-swap-demo" "\$@" || status=\$?
echo "\$status \$*" >>"$LOG"
exit "\$status"
EOF
chmod +x "$BIN_DIR/agy-swap"

export PATH="$BIN_DIR:$PATH"
export HOME="$WORK_DIR/home"
export TERM="xterm-256color"
# Reset times in the clips should not reveal the recording machine's time zone.
export TZ="UTC"

# Poster frame (seconds into the MP4) for each clip; empty means the last frame.
poster_at() {
  case "$1" in
    switch) echo 2 ;;
    dashboard) echo 4.5 ;;
    *) echo "" ;;
  esac
}

for tape in scripts/tapes/*.tape; do
  name="$(basename "$tape" .tape)"
  echo "==> Recording $name..."
  : >"$LOG"
  vhs "$tape"
  if [ ! -s "$LOG" ]; then
    echo "error: $name did not run the demo wrapper; refusing to publish the clip." >&2
    rm -f "docs/assets/$name.gif" "docs/assets/$name.mp4"
    exit 1
  fi
  if grep -qv '^0 ' "$LOG"; then
    echo "error: $name had a failing demo command:" >&2
    cat "$LOG" >&2
    exit 1
  fi

  at="$(poster_at "$name")"
  seek=(-sseof -0.2)
  [ -z "$at" ] || seek=(-ss "$at")
  # A 256-colour palette keeps terminal text sharp at a third of the size.
  ffmpeg -v error -y "${seek[@]}" -i "docs/assets/$name.mp4" -frames:v 1 \
    -vf "split[a][b];[a]palettegen[p];[b][p]paletteuse=dither=none" "site/public/media/$name.png"
  cp -f "docs/assets/$name.mp4" "site/public/media/$name.mp4"
done

# Local launch kit (ignored by Git) reuses the switch clip.
if [ -d promo ]; then
  cp -f docs/assets/switch.gif promo/agy-swap-demo.gif
  cp -f docs/assets/switch.mp4 promo/agy-swap-demo.mp4
fi

echo "==> Recorded:"
ls -lh docs/assets/*.gif docs/assets/*.mp4 site/public/media/*
