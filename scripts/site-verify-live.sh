#!/bin/sh
# Verify production serves exactly the Site build of tag vVERSION: wait for the
# deployed version, then compare endpoints and every built file byte for byte.
set -eu
version=${1:?usage: site-verify-live.sh VERSION}
base=${AGY_SITE_URL:-https://agy-swap.aklkbqx.com}
wait_seconds=${AGY_SITE_WAIT:-600}
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'git -C "$root/site" worktree remove --force "$work/src" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT
git -C "$root/site" worktree add --detach "$work/src" "v$version" >/dev/null
(cd "$work/src" && bun install --frozen-lockfile >/dev/null && bun run build >/dev/null)
waited=0
until curl -fsS --max-time 10 "$base/" 2>/dev/null | grep -q "\"softwareVersion\": \"v$version\""; do
  if [ "$waited" -ge "$wait_seconds" ]; then
    echo "production never reported v$version within ${wait_seconds}s" >&2
    exit 1
  fi
  sleep 10
  waited=$((waited + 10))
done
cd "$root"
go run ./cmd/releasetool verify-live "$version" "$work/src/dist/client" "$base"
echo "Live site matches the v$version build ($base)"
