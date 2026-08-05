#!/usr/bin/env python3
"""Bundle src/agy_swap/ package modules into a single executable binary 'agy-swap'."""

import ast
import os
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
SRC_DIR = ROOT / "src" / "agy_swap"
OUTPUT_FILE = ROOT / "agy-swap"

MODULE_ORDER = [
    "__init__",
    "storage",
    "display",
    "tty",
    "network",
    "oauth",
    "logs",
    "store",
    "credentials",
    "quota",
    "commands",
    "tui",
    "updater",
    "__main__",
]


def extract_imports_and_body(source: str):
    """Extract top-level standard library imports and stripped body code from a module."""
    tree = ast.parse(source)
    lines = source.splitlines()

    import_lines = []
    remove_linenos = set()

    for node in tree.body:
        if isinstance(node, ast.Import):
            for alias in node.names:
                if not alias.name.startswith("agy_swap"):
                    for lno in range(node.lineno, node.end_lineno + 1):
                        import_lines.append(lines[lno - 1])
                        remove_linenos.add(lno)
                else:
                    for lno in range(node.lineno, node.end_lineno + 1):
                        remove_linenos.add(lno)
        elif isinstance(node, ast.ImportFrom):
            if node.module and node.module.startswith("agy_swap"):
                for lno in range(node.lineno, node.end_lineno + 1):
                    remove_linenos.add(lno)
            elif node.module:
                for lno in range(node.lineno, node.end_lineno + 1):
                    import_lines.append(lines[lno - 1])
                    remove_linenos.add(lno)

    # Walk full AST to strip any nested internal agy_swap imports inside functions
    for node in ast.walk(tree):
        if isinstance(node, ast.ImportFrom) and node.module and node.module.startswith("agy_swap"):
            for lno in range(node.lineno, node.end_lineno + 1):
                remove_linenos.add(lno)
        elif isinstance(node, ast.Import):
            for alias in node.names:
                if alias.name.startswith("agy_swap"):
                    for lno in range(node.lineno, node.end_lineno + 1):
                        remove_linenos.add(lno)

    body_lines = []
    for lno, line in enumerate(lines, 1):
        if lno not in remove_linenos:
            body_lines.append(line)

    body_text = "\n".join(body_lines).strip()
    return import_lines, body_text


def build():
    collected_imports = []
    module_bodies = []

    for mod_name in MODULE_ORDER:
        mod_path = SRC_DIR / f"{mod_name}.py"
        if not mod_path.exists():
            raise FileNotFoundError(f"Missing required module: {mod_path}")
        source = mod_path.read_text(encoding="utf-8")
        imports, body = extract_imports_and_body(source)
        for imp in imports:
            if imp not in collected_imports and not imp.startswith('"""') and not imp.startswith("'''"):
                collected_imports.append(imp)

        # Remove docstring at top of module if present
        if body.startswith('"""') or body.startswith("'''"):
            quote = body[:3]
            end_idx = body.find(quote, 3)
            if end_idx != -1:
                body = body[end_idx + 3:].strip()

        if mod_name == "__main__":
            # Remove if __name__ == "__main__": block since main() is called at end
            body = re_sub_main_guard(body)

        module_bodies.append((mod_name, body))

    # Construct final single-file bundle
    header = [
        "#!/usr/bin/env python3",
        "# Created by @aklkbqx (https://github.com/aklkbqx)",
        "# AUTO-GENERATED from src/agy_swap/ by scripts/build.py — DO NOT EDIT DIRECTLY",
        "",
    ]

    imports_section = deduplicate_imports(collected_imports)
    parts = header + imports_section + [""]

    for mod_name, body in module_bodies:
        if not body:
            continue
        banner = f"# ── Module: {mod_name} ──────────────────────────────────────────────────"
        parts.append(banner)
        parts.append(body)
        parts.append("")

    parts.append('if __name__ == "__main__":')
    parts.append("    main()")

    final_content = "\n".join(parts) + "\n"

    # Syntax validation check
    try:
        compile(final_content, str(OUTPUT_FILE), "exec")
    except SyntaxError as exc:
        print(f"Error: Generated bundle has syntax error at line {exc.lineno}: {exc.msg}", file=sys.stderr)
        sys.exit(1)

    OUTPUT_FILE.write_text(final_content, encoding="utf-8")
    os.chmod(str(OUTPUT_FILE), 0o755)
    print(f"✓ Successfully built {OUTPUT_FILE} ({len(final_content)} bytes)")


def deduplicate_imports(import_lines):
    seen = set()
    result = []
    for line in import_lines:
        line_str = line.strip()
        if line_str and line_str not in seen:
            seen.add(line_str)
            result.append(line_str)
    return sorted(result)


def re_sub_main_guard(body):
    lines = body.splitlines()
    filtered = []
    skip = False
    for line in lines:
        if line.strip() == 'if __name__ == "__main__":':
            skip = True
            continue
        if skip and line.startswith("    "):
            continue
        skip = False
        filtered.append(line)
    return "\n".join(filtered).strip()


if __name__ == "__main__":
    build()
