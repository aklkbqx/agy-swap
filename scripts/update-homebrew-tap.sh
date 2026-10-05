#!/bin/sh
set -eu

version=${1:?usage: update-homebrew-tap.sh VERSION [DIST_DIR]}
version=${version#v}
dist=${2:-dist/release}
checksums="$dist/checksums.txt"

if [ ! -f "$checksums" ]; then
  echo "Missing $checksums" >&2
  exit 1
fi

tap_dir="${HOMEBREW_TAP_DIR:-}"
if [ -z "$tap_dir" ] && command -v brew >/dev/null 2>&1; then
  tap_dir="$(brew --repository aklkbqx/agy-swap 2>/dev/null || true)"
fi

if [ -z "$tap_dir" ] || [ ! -d "$tap_dir" ]; then
  echo "Homebrew tap directory not found, skipping tap update" >&2
  exit 0
fi

darwin_arm64=$(awk -v v="$version" '$2 ~ "agy-swap_v" v "_darwin_arm64" {print $1}' "$checksums")
darwin_amd64=$(awk -v v="$version" '$2 ~ "agy-swap_v" v "_darwin_amd64" {print $1}' "$checksums")
linux_arm64=$(awk -v v="$version" '$2 ~ "agy-swap_v" v "_linux_arm64" {print $1}' "$checksums")
linux_amd64=$(awk -v v="$version" '$2 ~ "agy-swap_v" v "_linux_amd64" {print $1}' "$checksums")

if [ -z "$darwin_arm64" ] || [ -z "$darwin_amd64" ] || [ -z "$linux_arm64" ] || [ -z "$linux_amd64" ]; then
  echo "Could not find all required platform checksums in $checksums" >&2
  exit 1
fi

mkdir -p "$tap_dir/Formula"
cat > "$tap_dir/Formula/agy-swap.rb" <<EOF
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

# Copy formula to dist directory for release assets
cp "$tap_dir/Formula/agy-swap.rb" "$dist/agy-swap.rb"

# Commit and push tap if git is present
if [ -d "$tap_dir/.git" ]; then
  git -C "$tap_dir" add Formula/agy-swap.rb
  if ! git -C "$tap_dir" diff --cached --quiet; then
    git -C "$tap_dir" commit -m "chore: update formula for v$version"
    git -C "$tap_dir" push origin main
    echo "✓ Homebrew tap updated and pushed for v$version"
  else
    echo "✓ Homebrew tap formula is already up to date"
  fi
fi
