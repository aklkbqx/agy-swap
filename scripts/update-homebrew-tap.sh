#!/bin/sh
set -eu

version=${1:?usage: update-homebrew-tap.sh VERSION [DIST_DIR] [--publish]}
version=${version#v}
dist=${2:-dist/release}
publish=${3:-}
if [ "$#" -gt 3 ] || { [ -n "$publish" ] && [ "$publish" != --publish ]; }; then
  echo 'Expected VERSION [DIST_DIR] [--publish]' >&2
  exit 2
fi
if ! printf '%s\n' "$version" | LC_ALL=C grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
  echo 'Expected a stable X.Y.Z version' >&2
  exit 2
fi
checksums="$dist/checksums.txt"

if [ ! -f "$checksums" ]; then
  echo "Missing $checksums" >&2
  exit 1
fi

platform_checksum() {
  awk -v asset="agy-swap_v${version}_$1" '
    $2 == asset { count++; hash=$1 }
    END {
      if (count != 1 || length(hash) != 64 || hash ~ /[^a-fA-F0-9]/) exit 1
      print hash
    }' "$checksums" || {
      echo "Missing, duplicate or invalid checksum for $1 in $checksums" >&2
      return 1
    }
}
darwin_arm64=$(platform_checksum darwin_arm64)
darwin_amd64=$(platform_checksum darwin_amd64)
linux_arm64=$(platform_checksum linux_arm64)
linux_amd64=$(platform_checksum linux_amd64)

# Local generation is independent of Homebrew and never mutates another repo.
formula="$dist/agy-swap.rb"
cat > "$formula" <<EOF
class AgySwap < Formula
  desc "Fast account switcher and quota monitor for Google Antigravity CLI"
  homepage "https://github.com/aklkbqx/agy-swap"
  version "$version"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v$version/agy-swap_v${version}_darwin_arm64"
      sha256 "$darwin_arm64"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v$version/agy-swap_v${version}_darwin_amd64"
      sha256 "$darwin_amd64"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/aklkbqx/agy-swap/releases/download/v$version/agy-swap_v${version}_linux_arm64"
      sha256 "$linux_arm64"
    else
      url "https://github.com/aklkbqx/agy-swap/releases/download/v$version/agy-swap_v${version}_linux_amd64"
      sha256 "$linux_amd64"
    end
  end

  def install
    binary = Dir["agy-swap_v#{version}_*"].first
    bin.install binary => "agy-swap"
  end

  test do
    assert_match "agy-swap v#{version}", shell_output("#{bin}/agy-swap --version")
  end
end
EOF

echo "Generated $formula (local only)"
[ "$publish" = --publish ] || exit 0

tap_dir="${HOMEBREW_TAP_DIR:-}"
if [ -z "$tap_dir" ] && command -v brew >/dev/null 2>&1; then
  tap_dir="$(brew --repository aklkbqx/agy-swap 2>/dev/null || true)"
fi
if [ -z "$tap_dir" ] || [ ! -d "$tap_dir" ] || ! git -C "$tap_dir" rev-parse --git-dir >/dev/null 2>&1; then
  echo 'Homebrew tap Git directory not found; publication aborted' >&2
  exit 1
fi
if [ "$(git -C "$tap_dir" branch --show-current)" != main ] || [ -n "$(git -C "$tap_dir" status --porcelain)" ]; then
  echo 'Homebrew tap must be on main with a clean working tree and index; publication aborted' >&2
  exit 1
fi
# Inspect the actual remote tip before creating a local formula commit. A clean
# index alone does not prevent pushing unrelated commits already on local main.
git -C "$tap_dir" fetch --no-tags origin main
remote_tip=$(git -C "$tap_dir" rev-parse FETCH_HEAD)
if ! git -C "$tap_dir" merge-base --is-ancestor "$remote_tip" HEAD; then
  echo 'Homebrew tap is behind or diverged from origin main; publication aborted' >&2
  exit 1
fi
pending_files=$(git -C "$tap_dir" log --format= --name-only -m "$remote_tip..HEAD")
if printf '%s\n' "$pending_files" | LC_ALL=C grep -Ev '^$|^Formula/agy-swap\.rb$' >/dev/null; then
  echo 'Homebrew tap has unrelated unpublished commits; publication aborted' >&2
  exit 1
fi
mkdir -p "$tap_dir/Formula"
cp "$formula" "$tap_dir/Formula/agy-swap.rb"
git -C "$tap_dir" add -- Formula/agy-swap.rb
if ! git -C "$tap_dir" diff --cached --quiet; then
  git -C "$tap_dir" commit -m "chore: update formula for v$version" -- Formula/agy-swap.rb
fi
# Retry a previous failed push even when the formula is already current.
git -C "$tap_dir" push origin main
echo "✓ Homebrew tap published for v$version"
