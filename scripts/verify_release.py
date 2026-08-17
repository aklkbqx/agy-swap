#!/usr/bin/env python3
"""Validate release metadata, checksums, and version consistency across all repository manifests."""

import hashlib
from pathlib import Path
import re
import sys


def verify():
    root = Path(__file__).resolve().parents[1]

    # 1. agy-swap bundle
    core_path = root / "agy-swap"
    if not core_path.exists():
        raise SystemExit("Error: agy-swap bundle not found. Run scripts/build.py first.")
    core = core_path.read_bytes()
    core_text = core.decode("utf-8").replace("\r\n", "\n")
    try:
        compile(core_text, str(core_path), "exec")
    except SyntaxError as exc:
        raise SystemExit(f"Error: agy-swap bundle has syntax error: {exc}")

    core_ver_match = re.search(r'^VERSION = "([^"]+)"$', core_text, re.MULTILINE)
    if not core_ver_match:
        raise SystemExit("Error: VERSION not found in agy-swap bundle")
    core_version = core_ver_match.group(1)
    actual_sha = hashlib.sha256(core).hexdigest()

    # 2. src/agy_swap/__init__.py
    init_path = root / "src" / "agy_swap" / "__init__.py"
    init_text = init_path.read_text(encoding="utf-8")
    init_ver_match = re.search(r'^VERSION = "([^"]+)"$', init_text, re.MULTILINE)
    if not init_ver_match:
        raise SystemExit("Error: VERSION not found in src/agy_swap/__init__.py")
    init_version = init_ver_match.group(1)

    # 3. install.sh
    install_path = root / "install.sh"
    install_text = install_path.read_text(encoding="utf-8")
    install_ver_match = re.search(r'^VERSION="([^"]+)"$', install_text, re.MULTILINE)
    if not install_ver_match:
        raise SystemExit("Error: VERSION not found in install.sh")
    install_version = install_ver_match.group(1)

    sha_match = re.search(r'^EXPECTED_SHA256="([a-f0-9]+)"$', install_text, re.MULTILINE)
    if not sha_match:
        raise SystemExit("Error: EXPECTED_SHA256 not found in install.sh")
    installer_sha = sha_match.group(1)

    if re.search(r'curl\s+[^\n]*\s-(?:k\b|-insecure\b)', install_text):
        raise SystemExit("Error: install.sh contains insecure curl -k/--insecure flag")

    # 4. Formula/agy-swap.rb
    formula_path = root / "Formula" / "agy-swap.rb"
    formula_text = formula_path.read_text(encoding="utf-8")
    formula_ver_match = re.search(r'^\s*version "([^"]+)"$', formula_text, re.MULTILINE)
    if not formula_ver_match:
        raise SystemExit("Error: version not found in Formula/agy-swap.rb")
    formula_version = formula_ver_match.group(1)

    formula_sha_match = re.search(r'^\s*sha256 "([a-f0-9]+)"$', formula_text, re.MULTILINE)
    if not formula_sha_match:
        raise SystemExit("Error: sha256 not found in Formula/agy-swap.rb")
    formula_sha = formula_sha_match.group(1)

    formula_url_match = re.search(r'^\s*url "([^"]+)"$', formula_text, re.MULTILINE)
    if not formula_url_match or f"/v{formula_version}/" not in formula_url_match.group(1):
        raise SystemExit(f"Error: Formula URL is not release-pinned to v{formula_version}")

    if 'assert_match "Usage:",' in formula_text:
        raise SystemExit("Error: Formula/agy-swap.rb contains case-sensitive 'Usage:' assertion (expected 'usage: agy-swap')")

    # 5. pyproject.toml
    pyproject_path = root / "pyproject.toml"
    pyproject_text = pyproject_path.read_text(encoding="utf-8")
    pyproject_ver_match = re.search(r'^version = "([^"]+)"$', pyproject_text, re.MULTILINE)
    if not pyproject_ver_match:
        raise SystemExit("Error: version not found in pyproject.toml")
    pyproject_version = pyproject_ver_match.group(1)

    # Cross-validation
    versions = {
        "agy-swap": core_version,
        "src/agy_swap/__init__.py": init_version,
        "install.sh": install_version,
        "Formula/agy-swap.rb": formula_version,
        "pyproject.toml": pyproject_version,
    }
    distinct_versions = set(versions.values())
    if len(distinct_versions) != 1:
        details = ", ".join(f"{k}={v}" for k, v in versions.items())
        raise SystemExit(f"Error: Version mismatch across manifests: {details}")

    if installer_sha != actual_sha:
        raise SystemExit(f"Error: Installer checksum mismatch: expected={installer_sha}, actual={actual_sha}")

    if formula_sha != actual_sha:
        raise SystemExit(f"Error: Formula checksum mismatch: expected={formula_sha}, actual={actual_sha}")

    if len(sys.argv) > 1:
        tag_version = sys.argv[1].removeprefix("v")
        if tag_version != core_version:
            raise SystemExit(f"Error: Tag {sys.argv[1]} does not match version v{core_version}")

    print(f"✓ All release metadata verified successfully:")
    print(f"  Version:   v{core_version}")
    print(f"  SHA-256:   {actual_sha}")
    print(f"  Manifests: agy-swap, src/agy_swap, install.sh, Formula/agy-swap.rb, pyproject.toml")


if __name__ == "__main__":
    verify()
