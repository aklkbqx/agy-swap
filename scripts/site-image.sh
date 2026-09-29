#!/bin/sh
# The production site image: one static Nginx container, no demo gateway.
#   preview                 build this checkout, serve it on 127.0.0.1:$AGY_SITE_PORT
#   release VERSION [--push] build a clean checkout of tag vVERSION, smoke-test it,
#                           and with --push publish :vVERSION-COMMIT and :latest.
# Watchtower on the server pulls :latest. Nothing here connects to the server.
set -eu
repo=ghcr.io/aklkbqx/agy-swap-site
port=${AGY_SITE_PORT:-4796}
root=$(cd "$(dirname "$0")/.." && pwd)
container=agy-swap-site-local

smoke() { # IMAGE VERSION
  docker rm -f "$container" >/dev/null 2>&1 || true
  docker run -d --rm --name "$container" --platform linux/amd64 -p "127.0.0.1:$port:80" "$1" >/dev/null
  base="http://127.0.0.1:$port"
  tries=0
  until curl -fsS "$base/healthz" >/dev/null 2>&1; do
    tries=$((tries + 1)); [ "$tries" -lt 30 ] || { echo "site never became healthy" >&2; return 1; }
    sleep 1
  done
  for path in / /install.html /og-panorama.png /sitemap.xml /robots.txt; do
    code=$(curl -s -o /dev/null -w '%{http_code}' "$base$path"); [ "$code" = 200 ] || { echo "$path returned $code" >&2; return 1; }
  done
  code=$(curl -s -o /dev/null -w '%{http_code}' "$base/demo/ws"); [ "$code" = 404 ] || { echo "/demo/ws returned $code, want 404" >&2; return 1; }
  curl -fsS "$base/" | grep -q "\"softwareVersion\": \"v$2\"" || { echo "page does not report v$2" >&2; return 1; }
}

case "${1:-}" in
  preview)
    version=$(node -p "require('$root/site/package.json').version")
    (cd "$root/site" && bun install --frozen-lockfile && bun run build)
    docker buildx build --platform linux/amd64 --load -t agy-swap-site:preview "$root/site"
    smoke agy-swap-site:preview "$version"
    echo "Production-equivalent preview: http://127.0.0.1:$port  (stop: docker rm -f $container)"
    ;;
  release)
    version=${2:?usage: site-image.sh release VERSION [--push]}
    push=${3:-}
    commit=$(git -C "$root" rev-parse --short "v$version^{commit}")
    work=$(mktemp -d)
    trap 'docker rm -f "$container" >/dev/null 2>&1 || true; git -C "$root" worktree remove --force "$work/src" >/dev/null 2>&1 || true; rm -rf "$work"' EXIT
    git -C "$root" worktree add --detach "$work/src" "v$version" >/dev/null
    (cd "$work/src/site" && bun install --frozen-lockfile && bun run build)
    tag="$repo:v$version-$commit"
    docker buildx build --platform linux/amd64 --load -t "$tag" -t "$repo:latest" "$work/src/site"
    smoke "$tag" "$version"
    if [ "$push" = --push ]; then
      docker push "$tag"
      docker push "$repo:latest"
      docker buildx imagetools inspect "$repo:latest" --format '{{json .Manifest.Digest}}'
    else
      echo "Built and smoke-tested $tag. Re-run with --push to publish it and :latest."
    fi
    ;;
  *)
    echo "usage: site-image.sh preview | release VERSION [--push]" >&2
    exit 2
    ;;
esac
