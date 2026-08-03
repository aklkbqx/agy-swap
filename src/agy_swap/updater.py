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

from agy_swap import VERSION, GITHUB_API_RELEASE, GITHUB_RAW, GREEN, RED, GRAY, RESET
from agy_swap.tty import Spinner


def cmd_update(args):
    force = getattr(args, "force", False)

    with Spinner("Checking for updates..."):
        try:
            req = urllib.request.Request(GITHUB_API_RELEASE, headers={"Accept": "application/vnd.github+json"})
            with urllib.request.urlopen(req, timeout=10) as resp:
                release = json.loads(resp.read().decode("utf-8"))
        except (OSError, ValueError, urllib.error.HTTPError) as exc:
            print(f"{RED}✕ Could not check for updates: {exc}{RESET}", file=sys.stderr)
            sys.exit(1)

    latest_tag = release.get("tag_name", "")
    latest_version = latest_tag.removeprefix("v")

    if not latest_version:
        print(f"{RED}✕ Could not determine latest version.{RESET}", file=sys.stderr)
        sys.exit(1)

    if latest_version == VERSION and not force:
        print(f"{GREEN}✓ Already up to date (v{VERSION}).{RESET}")
        return

    print(f"  Current: {GRAY}v{VERSION}{RESET}")
    print(f"  Latest:  {GREEN}v{latest_version}{RESET}")

    install_url = f"{GITHUB_RAW}/{latest_tag}/install.sh"
    script_url = f"{GITHUB_RAW}/{latest_tag}/agy-swap"

    with Spinner(f"Downloading v{latest_version}..."):
        try:
            with urllib.request.urlopen(install_url, timeout=15) as resp:
                install_text = resp.read().decode("utf-8")
            sha_match = re.search(r'^EXPECTED_SHA256="([a-f0-9]+)"$', install_text, re.MULTILINE)
            if not sha_match:
                print(f"{RED}✕ Could not extract checksum from release.{RESET}", file=sys.stderr)
                sys.exit(1)
            expected_sha = sha_match.group(1)

            with urllib.request.urlopen(script_url, timeout=30) as resp:
                new_script = resp.read()
        except (OSError, ValueError, urllib.error.HTTPError) as exc:
            print(f"{RED}✕ Download failed: {exc}{RESET}", file=sys.stderr)
            sys.exit(1)

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
        print(f"{RED}✕ Cannot locate agy-swap binary to update.{RESET}", file=sys.stderr)
        sys.exit(1)

    try:
        old_stat = os.stat(current_path)
        fd, tmp_path = tempfile.mkstemp(prefix=".agy-swap.", dir=os.path.dirname(current_path))
        try:
            with os.fdopen(fd, "wb") as f:
                f.write(new_script)
                f.flush()
                os.fsync(f.fileno())
            os.chmod(tmp_path, old_stat.st_mode)
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
    notes_url = release.get("html_url", "")
    if notes_url:
        print(f"  {GRAY}Release notes: {notes_url}{RESET}")
