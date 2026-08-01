#!/usr/bin/env python3
import hashlib
import json
from pathlib import Path
import re
import sys


root = Path(__file__).resolve().parents[1]
core = (root / "agy-swap").read_bytes()
core_text = core.decode("utf-8")
install_text = (root / "install.sh").read_text(encoding="utf-8")
site_version = json.loads((root / "site/package.json").read_text(encoding="utf-8"))["version"]

core_version = re.search(r'^VERSION = "([^"]+)"$', core_text, re.MULTILINE).group(1)
install_version = re.search(r'^VERSION="([^"]+)"$', install_text, re.MULTILINE).group(1)
expected_sha = re.search(r'^EXPECTED_SHA256="([a-f0-9]+)"$', install_text, re.MULTILINE).group(1)
actual_sha = hashlib.sha256(core).hexdigest()

versions = {core_version, install_version, site_version}
if len(versions) != 1:
    raise SystemExit(f"Version mismatch: core={core_version}, installer={install_version}, site={site_version}")
if expected_sha != actual_sha:
    raise SystemExit(f"Installer checksum mismatch: expected={expected_sha}, actual={actual_sha}")
if len(sys.argv) > 1 and sys.argv[1].removeprefix("v") != core_version:
    raise SystemExit(f"Tag {sys.argv[1]} does not match version {core_version}")

print(f"release metadata OK: v{core_version} sha256={actual_sha}")
