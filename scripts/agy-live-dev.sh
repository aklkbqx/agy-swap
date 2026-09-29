#!/bin/sh
# AGY Live on this Mac only: the Go demo gateway on loopback plus a Vite dev
# server with the section enabled. Production never ships AGY Live.
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
gateway_port=${AGY_LIVE_GATEWAY_PORT:-8787}
site_port=${AGY_LIVE_SITE_PORT:-5174}
tmp=$(mktemp -d)
gateway=
cleanup() {
  [ -n "$gateway" ] && kill "$gateway" 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT INT TERM
(cd "$root" && go build -o "$tmp/agy-swap-demo" ./cmd/agy-swap-demo)
(cd "$root" && AGY_DEMO_ADDR="127.0.0.1:$gateway_port" \
  AGY_DEMO_ORIGIN="http://localhost:$site_port" \
  AGY_DEMO_BINARY="$tmp/agy-swap-demo" AGY_DEMO_TMP="$tmp" \
  go run ./cmd/agy-swap-demo-server) &
gateway=$!
echo "AGY Live: http://localhost:$site_port/#demo"
cd "$root/site"
VITE_AGY_LIVE=1 AGY_DEMO_GATEWAY="http://127.0.0.1:$gateway_port" bun run dev --port "$site_port" --strictPort
