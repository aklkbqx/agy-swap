#!/bin/sh
set -eu
target=${1:-patch}
root=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
go run "$root/cmd/releasetool" bump "$target" "$root"
