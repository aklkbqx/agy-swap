"""Self-updater downloading release assets and verifying SHA256 checksum."""

import hashlib
import json
import os
import re
import shutil
import sys
import tempfile
import urllib.error
import urllib.request

from agy_swap import VERSION, GITHUB_REPO, GITHUB_API_RELEASE, GITHUB_RAW, GREEN, RED, GRAY, RESET
from agy_swap.network import safe_urlopen
from agy_swap.tty import Spinner


def _is_valid_script_or_manifest(data):
    if not data or not isinstance(data, bytes):
        return False
    if b"<!DOCTYPE html>" in data or b"<html" in data or b"Web Page Blocked" in data:
        return False
    return True


def _fetch_first_available(urls, headers=None, timeout=15):
    last_exc = None
    headers = headers or {"User-Agent": "Mozilla/5.0"}
    for url in urls:
        try:
            req = urllib.request.Request(url, headers=headers)
            with safe_urlopen(req, timeout=timeout) as resp:
                data = resp.read()
                if _is_valid_script_or_manifest(data):
                    return data
        except Exception as exc:
            last_exc = exc
    if last_exc:
        raise last_exc
    raise RuntimeError("Failed to fetch valid content from candidate URLs")


def cmd_update(args):
    force = getattr(args, "force", False)

    release = None
    with Spinner("Checking for updates..."):
        try:
            req = urllib.request.Request(GITHUB_API_RELEASE, headers={"Accept": "application/vnd.github+json", "User-Agent": "Mozilla/5.0"})
            with safe_urlopen(req, timeout=10) as resp:
                data = resp.read()
                if _is_valid_script_or_manifest(data):
                    release = json.loads(data.decode("utf-8"))
        except Exception:
            release = None

    latest_tag = release.get("tag_name", "") if isinstance(release, dict) else ""
    latest_version = latest_tag.removeprefix("v")

    install_urls = []
    script_urls = []

    if latest_tag:
        install_urls.extend([
            f"https://cdn.jsdelivr.net/gh/{GITHUB_REPO}@{latest_tag}/install.sh",
            f"https://fastly.jsdelivr.net/gh/{GITHUB_REPO}@{latest_tag}/install.sh",
            f"https://gcore.jsdelivr.net/gh/{GITHUB_REPO}@{latest_tag}/install.sh",
            f"{GITHUB_RAW}/{latest_tag}/install.sh",
            f"https://github.com/{GITHUB_REPO}/raw/{latest_tag}/install.sh",
        ])
        script_urls.extend([
            f"https://cdn.jsdelivr.net/gh/{GITHUB_REPO}@{latest_tag}/agy-swap",
            f"https://fastly.jsdelivr.net/gh/{GITHUB_REPO}@{latest_tag}/agy-swap",
            f"https://gcore.jsdelivr.net/gh/{GITHUB_REPO}@{latest_tag}/agy-swap",
            f"{GITHUB_RAW}/{latest_tag}/agy-swap",
            f"https://github.com/{GITHUB_REPO}/raw/{latest_tag}/agy-swap",
        ])

    # Always add @main fallbacks
    install_urls.extend([
        f"https://cdn.jsdelivr.net/gh/{GITHUB_REPO}@main/install.sh",
        f"https://fastly.jsdelivr.net/gh/{GITHUB_REPO}@main/install.sh",
        f"https://gcore.jsdelivr.net/gh/{GITHUB_REPO}@main/install.sh",
        f"{GITHUB_RAW}/main/install.sh",
        f"https://github.com/{GITHUB_REPO}/raw/main/install.sh",
    ])
    script_urls.extend([
        f"https://cdn.jsdelivr.net/gh/{GITHUB_REPO}@main/agy-swap",
        f"https://fastly.jsdelivr.net/gh/{GITHUB_REPO}@main/agy-swap",
        f"https://gcore.jsdelivr.net/gh/{GITHUB_REPO}@main/agy-swap",
        f"{GITHUB_RAW}/main/agy-swap",
        f"https://github.com/{GITHUB_REPO}/raw/main/agy-swap",
    ])

    with Spinner("Fetching release metadata..."):
        try:
            install_bytes = _fetch_first_available(install_urls, timeout=15)
            install_text = install_bytes.decode("utf-8", errors="ignore")
        except Exception as exc:
            print(f"{RED}✕ Could not fetch update manifests: {exc}{RESET}", file=sys.stderr)
            sys.exit(1)

    if not latest_version:
        ver_match = re.search(r'^VERSION="([^"]+)"$', install_text, re.MULTILINE)
        if ver_match:
            latest_version = ver_match.group(1)

    if not latest_version:
        print(f"{RED}✕ Could not determine latest version.{RESET}", file=sys.stderr)
        sys.exit(1)

    if latest_version == VERSION and not force:
        print(f"{GREEN}✓ Already up to date (v{VERSION}).{RESET}")
        return

    print(f"  Current: {GRAY}v{VERSION}{RESET}")
    print(f"  Latest:  {GREEN}v{latest_version}{RESET}")

    sha_match = re.search(r'^EXPECTED_SHA256="([a-f0-9]+)"$', install_text, re.MULTILINE)
    expected_sha = sha_match.group(1) if sha_match else None

    with Spinner(f"Downloading v{latest_version}..."):
        try:
            new_script = _fetch_first_available(script_urls, timeout=30)
        except Exception as exc:
            print(f"{RED}✕ Download failed: {exc}{RESET}", file=sys.stderr)
            sys.exit(1)

    if expected_sha:
        actual_sha = hashlib.sha256(new_script).hexdigest()
        if actual_sha != expected_sha:
            print(f"{RED}✕ Checksum verification failed!{RESET}", file=sys.stderr)
            print(f"  Expected: {expected_sha}", file=sys.stderr)
            print(f"  Actual:   {actual_sha}", file=sys.stderr)
            sys.exit(1)

    try:
        compile(new_script.decode("utf-8"), "agy-swap", "exec")
    except SyntaxError as exc:
        print(f"{RED}✕ Downloaded script has syntax errors: {exc}{RESET}", file=sys.stderr)
        sys.exit(1)

    current_path = os.path.realpath(sys.argv[0]) if os.path.isfile(sys.argv[0]) else shutil.which("agy-swap")
    if not current_path or not os.path.isfile(current_path):
        current_path = os.path.expanduser("~/.local/bin/agy-swap")

    try:
        old_stat = os.stat(current_path) if os.path.exists(current_path) else None
        target_dir = os.path.dirname(current_path)
        os.makedirs(target_dir, exist_ok=True)

        fd, tmp_path = tempfile.mkstemp(prefix=".agy-swap.", dir=target_dir)
        try:
            with os.fdopen(fd, "wb") as f:
                f.write(new_script)
                f.flush()
                os.fsync(f.fileno())
            os.chmod(tmp_path, old_stat.st_mode if old_stat else 0o755)
            os.replace(tmp_path, current_path)
        except Exception:
            try:
                os.unlink(tmp_path)
            except OSError:
                pass
            raise
    except OSError as exc:
        print(f"{RED}✕ Failed to write update: {exc}{RESET}", file=sys.stderr)
        sys.exit(1)

    print(f"{GREEN}✓ Updated agy-swap v{VERSION} → v{latest_version}{RESET}")
    notes_url = release.get("html_url", "") if isinstance(release, dict) else ""
    if notes_url:
        print(f"  {GRAY}Release notes: {notes_url}{RESET}")
