#!/bin/sh
# AGY Live on this Mac only: the Go demo gateway on loopback plus a Vite dev
# server with the section enabled. Both take the first free port, like Vite
# does by default. Production never ships AGY Live.
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
gateway_port=${AGY_LIVE_GATEWAY_PORT:-$(cd "$root/site" && node --input-type=module -e "import('./scripts/free-port.mjs').then(m => m.firstFreePort(8787)).then(port => console.log(port))")}
gateway="http://127.0.0.1:$gateway_port"
tmp=$(mktemp -d)
pid=
cleanup() {
  [ -n "$pid" ] && kill "$pid" 2>/dev/null || true
  rm -rf "$tmp"
}
trap cleanup EXIT INT TERM
(cd "$root" && go build -o "$tmp/agy-swap-demo" ./cmd/agy-swap-demo)
# Vite's proxy presents the gateway's own origin, so the page port never matters.
(cd "$root" && AGY_DEMO_ADDR="127.0.0.1:$gateway_port" AGY_DEMO_ORIGIN="$gateway" \
  AGY_DEMO_BINARY="$tmp/agy-swap-demo" AGY_DEMO_TMP="$tmp" \
  go run ./cmd/agy-swap-demo-server) &
pid=$!
echo "AGY Live gateway: $gateway. Open the Local URL Vite prints below, then #demo."
cd "$root/site"
VITE_AGY_LIVE=1 AGY_DEMO_GATEWAY="$gateway" bun run dev
