#!/usr/bin/env bash
# Install the native agy-swap binary. No Python runtime is required.
set -euo pipefail

VERSION="2.0.0"
TARGET_DIR="${AGY_SWAP_TARGET_DIR:-${HOME}/.local/bin}"
TARGET_FILE="${TARGET_DIR}/agy-swap"
RELEASE_BASE="https://github.com/aklkbqx/agy-swap/releases/download/v${VERSION}"

GREEN='\033[0;32m'; BLUE='\033[0;34m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
if [[ ! -t 1 || -n "${NO_COLOR:-}" ]]; then GREEN=''; BLUE=''; YELLOW=''; RED=''; NC=''; fi

if ! command -v curl >/dev/null 2>&1; then
  printf "%bError: curl is required.%b\n" "$RED" "$NC" >&2
  exit 1
fi

case "$(uname -s)" in
  Darwin) os_name="darwin" ;;
  Linux) os_name="linux" ;;
  *) printf "%bError: this installer supports macOS and Linux; use the Windows release asset directly.%b\n" "$RED" "$NC" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch_name="amd64" ;;
  arm64|aarch64) arch_name="arm64" ;;
  *) printf "%bError: unsupported architecture: %s%b\n" "$RED" "$(uname -m)" "$NC" >&2; exit 1 ;;
esac

asset_name="agy-swap_v${VERSION}_${os_name}_${arch_name}"
mkdir -p "$TARGET_DIR"
tmp_dir="$(mktemp -d "${TARGET_DIR}/.agy-swap-install.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT

printf "%bInstalling agy-swap v%s for %s/%s...%b\n" "$BLUE" "$VERSION" "$os_name" "$arch_name" "$NC"
curl -fsSL --proto '=https' --tlsv1.2 "${RELEASE_BASE}/${asset_name}" -o "${tmp_dir}/${asset_name}"
curl -fsSL --proto '=https' --tlsv1.2 "${RELEASE_BASE}/checksums.txt" -o "${tmp_dir}/checksums.txt"

expected_sha="$(awk -v name="$asset_name" '$2 == name || $2 == "*" name { print $1; exit }' "${tmp_dir}/checksums.txt")"
if [[ ! "$expected_sha" =~ ^[a-fA-F0-9]{64}$ ]]; then
  printf "%bError: release checksum is missing or invalid.%b\n" "$RED" "$NC" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual_sha="$(sha256sum "${tmp_dir}/${asset_name}" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual_sha="$(shasum -a 256 "${tmp_dir}/${asset_name}" | awk '{print $1}')"
else
  actual_sha="$(openssl dgst -sha256 "${tmp_dir}/${asset_name}" | awk '{print $NF}')"
fi
if [[ "$actual_sha" != "$expected_sha" ]]; then
  printf "%bError: checksum verification failed; installation aborted.%b\n" "$RED" "$NC" >&2
  exit 1
fi

chmod 0755 "${tmp_dir}/${asset_name}"
"${tmp_dir}/${asset_name}" --version >/dev/null
if [[ -f "$TARGET_FILE" ]]; then cp -p "$TARGET_FILE" "${TARGET_FILE}.bak"; fi
if ! mv -f "${tmp_dir}/${asset_name}" "$TARGET_FILE"; then
  [[ -f "${TARGET_FILE}.bak" ]] && mv -f "${TARGET_FILE}.bak" "$TARGET_FILE"
  printf "%bError: failed to install the binary; the previous version was restored.%b\n" "$RED" "$NC" >&2
  exit 1
fi

trap - EXIT
rm -rf "$tmp_dir"
printf "%bInstalled agy-swap to %s%b\n" "$GREEN" "$TARGET_FILE" "$NC"
if [[ ":${PATH}:" != *":${TARGET_DIR}:"* ]]; then
  printf "%b%s is not in PATH. Add this line to your shell profile:%b\n" "$YELLOW" "$TARGET_DIR" "$NC"
  printf '  export PATH="%s:$PATH"\n' "$TARGET_DIR"
fi
