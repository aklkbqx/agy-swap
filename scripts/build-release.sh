#!/bin/sh
set -eu
version=${1:?usage: build-release.sh VERSION BUILD_ID [DIST_DIR]}
build_id=${2:?build ID is required}
dist=${3:-dist/release}
if ! printf '%s\n' "$version" | LC_ALL=C grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$'; then
  echo 'Expected a stable X.Y.Z version' >&2
  exit 2
fi
case "$build_id" in *[!a-zA-Z0-9._-]*|'') echo 'Invalid build ID' >&2; exit 2;; esac
if [ "$(go env GOHOSTOS)" != darwin ]; then
  echo 'Run on the macOS release builder: Darwin assets require Security.framework and cgo.' >&2
  exit 1
fi
go run ./cmd/releasetool verify-metadata "$version" .
mkdir -p "$dist"
for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  platform=${target%/*}
  arch=${target#*/}
  suffix=
  cgo=0
  if [ "$platform" = darwin ]; then cgo=1; fi
  if [ "$platform" = windows ]; then suffix=.exe; fi
  asset="$dist/agy-swap_v${version}_${platform}_${arch}${suffix}"
  CGO_ENABLED="$cgo" GOOS="$platform" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$version -X main.buildID=$build_id" -o "$asset" ./cmd/agy-swap
  printf 'Built %s\n' "$asset"
done
go run ./cmd/releasetool checksums "$dist"
go run ./cmd/releasetool verify-assets "$version" "$dist"
go run ./cmd/releasetool verify-version "$version" install.sh
go run ./cmd/releasetool verify-version "$version" install.ps1
"$dist/agy-swap_v${version}_darwin_$(go env GOHOSTARCH)" --version
