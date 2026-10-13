#!/bin/sh
# Read-only post-publication evidence for release vVERSION. Prints Markdown for
# the delivery records and exits non-zero when any check fails.
set -u
version=${1:?usage: release-evidence.sh VERSION}
tag=v$version
root=${AGY_EVIDENCE_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}
repo=aklkbqx/agy-swap
module=github.com/aklkbqx/agy-swap/v2
site_url=${AGY_SITE_URL:-https://agy-swap.aklkbqx.com}
image=${AGY_SITE_IMAGE:-ghcr.io/aklkbqx/agy-swap-site}
dist=${AGY_DIST:-$root/dist/release}
tmp=$(mktemp -d)
trap 'chmod -R u+w "$tmp" 2>/dev/null; rm -rf "$tmp"' EXIT
failures=0
ok() { printf -- '- %s\n' "$1"; }
fail() { printf -- '- **FAIL:** %s\n' "$1"; failures=$((failures + 1)); }
at_least() { # VERSION FLOOR
  [ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -1)" = "$2" ]
}
floor=$(sed -n 's/^go \([0-9.]*\)$/go\1/p' "$root/go.mod" | head -1)
host="$(go env GOHOSTOS)_$(go env GOHOSTARCH)"

printf '## Evidence for %s\n\n' "$tag"

# Release and asset digests against the published checksums.
set -- $(gh release view "$tag" -R "$repo" --json isDraft,isPrerelease,publishedAt,assets --jq '"\(.isDraft) \(.isPrerelease) \(.publishedAt) \(.assets | length)"' 2>/dev/null) "" "" "" ""
draft=$1 prerelease=$2 published=$3 assets=$4
gh release view "$tag" -R "$repo" --json assets --jq '.assets[] | "\(.name) \(.digest)"' > "$tmp/digests" 2>/dev/null
gh release download "$tag" -R "$repo" -p checksums.txt -D "$tmp" >/dev/null 2>&1
if [ "$draft" != false ] || [ "$prerelease" != false ] || [ ! -s "$tmp/checksums.txt" ]; then
  fail "Release $tag is missing, a draft or a prerelease, or has no checksums.txt"
else
  mismatched=""
  while read -r sha name; do
    grep -qx "$name sha256:$sha" "$tmp/digests" || mismatched="$mismatched $name"
  done < "$tmp/checksums.txt"
  if [ -z "$mismatched" ]; then
    ok "Release $tag published $published with $assets assets; GitHub asset digests match \`checksums.txt\`."
  else
    fail "GitHub asset digest differs from \`checksums.txt\`:$mismatched"
  fi
fi

# Build information of the local release assets.
if ls "$dist"/agy-swap_"$tag"_* >/dev/null 2>&1; then
  if (cd "$root" && go run ./cmd/releasetool verify-builds "$version" "$dist" .) >"$tmp/builds" 2>&1; then
    ok "Every asset's build information records \`$module\`, \`$tag\` and a Go version at or above $floor."
  else
    fail "Asset build information does not match the release: $(tr '\n' ' ' < "$tmp/builds")"
  fi
else
  ok "Asset build information not checked: no local assets in \`$dist\`."
fi

# Homebrew formula and the installed binary.
formula=$(brew info aklkbqx/agy-swap/agy-swap 2>/dev/null | head -1)
case "$formula" in
  *"stable $version"*) ok "Homebrew formula is $version." ;;
  *) fail "Homebrew formula is not $version: ${formula:-unavailable}" ;;
esac
installed_path=$(command -v agy-swap 2>/dev/null)
installed=$(agy-swap --version 2>/dev/null)
host_sha=$(awk -v name="agy-swap_${tag}_$host" '$2 == name {print $1}' "$tmp/checksums.txt" 2>/dev/null)
actual_sha=$(shasum -a 256 "$installed_path" 2>/dev/null | cut -d' ' -f1)
if [ "$installed" = "agy-swap $tag" ] && [ -n "$host_sha" ] && [ "$actual_sha" = "$host_sha" ]; then
  ok "Installed \`$installed_path\` reports \`$tag\`; SHA-256 equals the ${host} asset."
else
  fail "installed binary \`${installed_path:-none}\` reports \`${installed:-nothing}\`; SHA-256 ${actual_sha:-?} vs ${host} asset ${host_sha:-?}"
fi

# go install through the public module proxy, as a new user would.
mkdir -p "$tmp/gopath" "$tmp/bin"
latest=$(cd "$tmp" && GOPATH="$tmp/gopath" GOFLAGS= GOTOOLCHAIN=auto go list -m "$module@latest" 2>/dev/null | awk '{print $2}')
(cd "$tmp" && GOPATH="$tmp/gopath" GOBIN="$tmp/bin" GOFLAGS= GOTOOLCHAIN=auto go install "$module/cmd/agy-swap@latest" >/dev/null 2>&1)
built=$("$tmp/bin/agy-swap" --version 2>/dev/null)
built_go=$(go version -m "$tmp/bin/agy-swap" 2>/dev/null | head -1 | awk '{print $2}')
if [ "$latest" = "$tag" ] && [ "$built" = "agy-swap $tag-dev" ] && [ -n "$built_go" ] && at_least "$built_go" "$floor"; then
  ok "Public proxy: \`$module@latest\` resolves to $tag; \`go install\` built \`$built\` with $built_go."
else
  fail "go install: \`@latest\` resolves to ${latest:-nothing}, built \`${built:-nothing}\` with ${built_go:-unknown} (want $tag and at least $floor; the proxy can lag a few minutes after tagging)"
fi

# Live Site version.
live=$(curl -s --max-time 10 "$site_url/" | sed -n 's/.*"softwareVersion": *"\([^"]*\)".*/\1/p' | head -1)
if [ "$live" = "$tag" ]; then
  ok "Live site reports softwareVersion $live (run \`make site-verify-live VERSION=$version\` for the byte comparison)."
else
  fail "live site softwareVersion is ${live:-unknown}, want $tag"
fi

# Site image identifiers belong only in the private Site record.
printf '\n### Site (private record only)\n\n'
site_commit=$(git -C "$root/site" rev-parse --short "$tag^{commit}" 2>/dev/null)
versioned=$(docker buildx imagetools inspect "$image:$tag-$site_commit" --format '{{json .Manifest.Digest}}' 2>/dev/null | tr -d '"')
latest_digest=$(docker buildx imagetools inspect "$image:latest" --format '{{json .Manifest.Digest}}' 2>/dev/null | tr -d '"')
if [ -n "$versioned" ] && [ "$versioned" = "$latest_digest" ]; then
  ok "Image \`$image:$tag-$site_commit\` and \`:latest\` resolve to \`$versioned\`."
else
  fail "image \`$image:$tag-${site_commit:-?}\` is ${versioned:-missing} but \`:latest\` is ${latest_digest:-missing}"
fi

if [ "$failures" -gt 0 ]; then
  printf '\n%s check(s) failed.\n' "$failures"
  exit 1
fi
