#!/usr/bin/env bash
# Install the native agy-swap binary. No Python runtime is required.
set -euo pipefail

VERSION="${AGY_SWAP_VERSION:-2.1.3}"
VERSION="${VERSION#v}"
TARGET_DIR="${AGY_SWAP_TARGET_DIR:-${HOME}/.local/bin}"
TARGET_FILE="${TARGET_DIR}/agy-swap"
RELEASE_BASE="https://github.com/aklkbqx/agy-swap/releases/download/v${VERSION}"

GREEN='\033[0;32m'; BLUE='\033[0;34m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; BOLD='\033[1m'; NC='\033[0m'
if [[ ! -t 1 || -n "${NO_COLOR:-}" ]]; then GREEN=''; BLUE=''; YELLOW=''; RED=''; BOLD=''; NC=''; fi

if [[ ! "$VERSION" =~ ^[0-9]+(\.[0-9]+){2}([.-][0-9A-Za-z.-]+)?$ ]]; then
  printf "%bError: invalid release version: %s%b\n" "$RED" "$VERSION" "$NC" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  printf "%bError: curl is required.%b\n" "$RED" "$NC" >&2
  exit 1
fi

case "$(uname -s)" in
  Darwin) os_name="darwin" ;;
  Linux) os_name="linux" ;;
  *) printf "%bError: this installer supports macOS and Linux. On Windows run install.ps1.%b\n" "$RED" "$NC" >&2; exit 1 ;;
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

find_ca_bundle() {
  local candidates=(
    "/etc/ssl/cert.pem"
    "/etc/ssl/certs/ca-certificates.crt"
    "/etc/pki/tls/certs/ca-bundle.crt"
    "/etc/ssl/ca-bundle.pem"
    "/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem"
  )
  for ca in "${candidates[@]}"; do
    if [[ -f "$ca" ]]; then
      echo "$ca"
      return 0
    fi
  done
  return 1
}

download_file() {
  local url="$1"
  local dest="$2"
  local opts=("-fsSL" "--proto" "=https" "--tlsv1.2")

  if [[ "${AGY_SWAP_INSECURE:-0}" == "1" ]]; then
    opts+=("-k")
  fi
  if [[ -n "${AGY_SWAP_CURL_FLAGS:-}" ]]; then
    # shellcheck disable=SC2206
    opts+=(${AGY_SWAP_CURL_FLAGS})
  fi

  # Attempt 1: Standard TLS verification
  if curl "${opts[@]}" "$url" -o "$dest" 2>/dev/null; then
    return 0
  fi

  # Attempt 2: Auto-detect System Root CA Bundle if standard failed
  local sys_ca
  if sys_ca="$(find_ca_bundle 2>/dev/null)"; then
    if curl "${opts[@]}" --cacert "$sys_ca" "$url" -o "$dest" 2>/dev/null; then
      return 0
    fi
  fi

  # Attempt 3: Verbose attempt to surface error
  curl "${opts[@]}" "$url" -o "$dest"
}

printf "%b●%b Installing %bagy-swap v%s%b for %s/%s...\n" "$BLUE" "$NC" "$BOLD" "$VERSION" "$NC" "$os_name" "$arch_name"
printf "  %b↓%b Downloading release binary from GitHub...\n" "$BLUE" "$NC"

if ! download_file "${RELEASE_BASE}/${asset_name}" "${tmp_dir}/${asset_name}"; then
  printf "\n%bError: Failed to download release asset.%b\n" "$RED" "$NC" >&2
  printf "%bIf you are behind a corporate proxy, VPN, or firewall with custom SSL inspection:%b\n" "$YELLOW" "$NC" >&2
  printf "  1) Run with proxy tolerance (%bSHA-256 integrity remains strictly verified%b):\n" "$BOLD" "$NC" >&2
  printf "     %bcurl -k -fsSL https://raw.githubusercontent.com/aklkbqx/agy-swap/main/install.sh | AGY_SWAP_INSECURE=1 bash%b\n" "$BOLD" "$NC" >&2
  printf "  2) Or compile directly with Go (bypasses GitHub releases):\n" >&2
  printf "     %bgo install github.com/aklkbqx/agy-swap/cmd/agy-swap@latest%b\n\n" "$BOLD" "$NC" >&2
  exit 1
fi

if ! download_file "${RELEASE_BASE}/checksums.txt" "${tmp_dir}/checksums.txt"; then
  printf "\n%bError: Failed to download release checksums.%b\n" "$RED" "$NC" >&2
  exit 1
fi

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

actual_sha="$(printf '%s' "$actual_sha" | tr '[:upper:]' '[:lower:]')"
expected_sha="$(printf '%s' "$expected_sha" | tr '[:upper:]' '[:lower:]')"

if [[ "$actual_sha" != "$expected_sha" ]]; then
  printf "%bError: SHA-256 checksum verification failed; installation aborted.%b\n" "$RED" "$NC" >&2
  exit 1
fi
printf "  %b✓%b Cryptographic SHA-256 integrity verified: %b%s...%b\n" "$GREEN" "$NC" "$BOLD" "${expected_sha:0:16}" "$NC"

chmod 0755 "${tmp_dir}/${asset_name}"
reported_version="$("${tmp_dir}/${asset_name}" --version 2>/dev/null || true)"
if [[ "$reported_version" != "agy-swap v${VERSION}"* ]]; then
  printf "%bError: downloaded binary reported an unexpected version: %s%b\n" "$RED" "$reported_version" "$NC" >&2
  exit 1
fi
printf "  %b✓%b Binary self-test passed: %b%s%b\n" "$GREEN" "$NC" "$BOLD" "$reported_version" "$NC"

if [[ -f "$TARGET_FILE" ]]; then cp -p "$TARGET_FILE" "${TARGET_FILE}.bak"; fi
if ! mv -f "${tmp_dir}/${asset_name}" "$TARGET_FILE"; then
  [[ -f "${TARGET_FILE}.bak" ]] && mv -f "${TARGET_FILE}.bak" "$TARGET_FILE"
  printf "%bError: failed to install the binary; the previous version was restored.%b\n" "$RED" "$NC" >&2
  exit 1
fi

trap - EXIT
rm -rf "$tmp_dir"

printf "  %b✓%b Installed binary to %b%s%b\n" "$GREEN" "$NC" "$BOLD" "$TARGET_FILE" "$NC"
if [[ ":${PATH}:" != *":${TARGET_DIR}:"* ]]; then
  printf "\n%bNote: %s is not in your current PATH. Add this line to your shell profile (~/.zshrc or ~/.bashrc):%b\n" "$YELLOW" "$TARGET_DIR" "$NC"
  printf '  export PATH="%s:$PATH"\n' "$TARGET_DIR"
fi

printf "\n%b🚀 Installation complete! Run '%bagy-swap%b' to launch the interactive manager.%b\n\n" "$GREEN" "$BOLD" "$GREEN" "$NC"
