#!/usr/bin/env bash
# Install script for agy-swap by @aklkbqx (https://github.com/aklkbqx)
set -euo pipefail

VERSION="1.6.5"
EXPECTED_SHA256="48661e202193d67d75f4b2b57bd620816bf98aa86abd905b5834c2162da0f949"
TARGET_DIR="${AGY_SWAP_TARGET_DIR:-${HOME}/.local/bin}"
TARGET_FILE="${TARGET_DIR}/agy-swap"

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'
if [[ ! -t 1 || -n "${NO_COLOR:-}" ]]; then
    GREEN=''
    BLUE=''
    YELLOW=''
    RED=''
    NC=''
fi

if ! command -v python3 >/dev/null 2>&1; then
    printf "%bError: python3 is not installed.%b\n" "$RED" "$NC" >&2
    exit 1
fi

mkdir -p "$TARGET_DIR"
tmp_file="$(mktemp "${TARGET_DIR}/.agy-swap.XXXXXX")"
trap 'rm -f "$tmp_file"' EXIT

source_file=""
script_path="${BASH_SOURCE[0]:-}"
if [[ -n "$script_path" && -f "$script_path" ]]; then
    script_dir="$(cd -- "$(dirname -- "$script_path")" && pwd)"
    if [[ -f "${script_dir}/agy-swap" ]]; then
        source_file="${script_dir}/agy-swap"
    fi
fi

printf "%bInstalling agy-swap v%s...%b\n" "$BLUE" "$VERSION" "$NC"
if [[ -n "$source_file" ]]; then
    cp "$source_file" "$tmp_file"
else
    if ! command -v curl >/dev/null 2>&1; then
        printf "%bError: curl is required for remote installation.%b\n" "$RED" "$NC" >&2
        exit 1
    fi
    curl -fsSL --proto '=https' --tlsv1.2 \
        "https://raw.githubusercontent.com/aklkbqx/agy-swap/v${VERSION}/agy-swap" \
        -o "$tmp_file"
fi

actual_sha="$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$tmp_file")"
if [[ "$actual_sha" != "$EXPECTED_SHA256" ]]; then
    printf "%bError: checksum verification failed; installation aborted.%b\n" "$RED" "$NC" >&2
    exit 1
fi

python3 -c 'import sys; compile(open(sys.argv[1], encoding="utf-8").read(), sys.argv[1], "exec")' "$tmp_file"
chmod 0755 "$tmp_file"
mv -f "$tmp_file" "$TARGET_FILE"
trap - EXIT

printf "%bInstalled agy-swap to %s%b\n" "$GREEN" "$TARGET_FILE" "$NC"
if [[ ":${PATH}:" != *":${TARGET_DIR}:"* ]]; then
    printf "%b%s is not in PATH. Add this line to your shell profile:%b\n" "$YELLOW" "$TARGET_DIR" "$NC"
    printf '  export PATH="%s:$PATH"\n' "$TARGET_DIR"
fi
