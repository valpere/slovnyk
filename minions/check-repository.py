#!/usr/bin/env python3
"""Check active Markdown, local-data ignore rules and minion Python syntax."""

import argparse
import ast
import json
import re
import subprocess
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parent.parent


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("paths", nargs="*", help="Markdown files; default: active documents, excluding historical reports")
    args = parser.parse_args()
    paths = [ROOT / name for name in args.paths] if args.paths else [
        ROOT / "README.md", ROOT / "data/README.md", ROOT / "minions/README.md",
        *([ROOT / "testdata/README.md"] if (ROOT / "testdata/README.md").exists() else []),
        *sorted((ROOT / "docs").glob("*.md")),
    ]
    errors = []
    for path in paths:
        if not path.is_file():
            errors.append(f"Missing document: {path.relative_to(ROOT) if path.is_relative_to(ROOT) else path}")
            continue
        body = path.read_text(encoding="utf-8")
        label = str(path.relative_to(ROOT)) if path.is_relative_to(ROOT) else str(path)
        for number, line in enumerate(body.splitlines(), 1):
            if line.rstrip() != line:
                errors.append(f"{label}:{number}: trailing whitespace")
        for match in re.finditer(r"\]\((<[^>]+>|[^\s)]+)(?:\s+\"[^\"]*\")?\)", body):
            target = match.group(1).strip("<>")
            target = target.split("#", 1)[0].split("?", 1)[0]
            if not target or re.match(r"^[a-zA-Z][a-zA-Z0-9+.-]*:", target) or target.startswith("/"):
                continue
            if not (path.parent / unquote(target)).exists():
                errors.append(f"{label}: missing link target {target}")
    for path in sorted((ROOT / "minions").glob("*.py")):
        try:
            ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
        except SyntaxError as error:
            errors.append(f"{path.name}:{error.lineno}: invalid Python syntax")
    data = ROOT / "data/experiments"
    files = sorted(str(p.relative_to(ROOT)) for p in data.rglob("*") if p.is_file()) if data.exists() else []
    if files:
        result = subprocess.run(["git", "check-ignore", "--stdin"], cwd=ROOT,
                                input="\n".join(files) + "\n", text=True, capture_output=True)
        if result.returncode not in (0, 1):
            errors.append("git check-ignore failed")
        else:
            for path in sorted(set(files) - set(result.stdout.splitlines())):
                errors.append(f"Local data is not ignored: {path}")
    # These sources must remain eligible for Git, including newly added scripts.
    sources = [ROOT / "data/README.md", *sorted((ROOT / "minions").glob("*"))]
    sources.extend(sorted((ROOT / "testdata").rglob("*")))
    for path in sources:
        if not path.is_file():
            continue
        relative = str(path.relative_to(ROOT))
        result = subprocess.run(["git", "check-ignore", "-q", relative], cwd=ROOT)
        if result.returncode != 1:
            errors.append(f"Source ignored or ignore check failed: {relative}")
    report = {"passed": not errors, "markdown_files": len(paths), "ignored_data_files": len(files), "errors": errors}
    output = ROOT / "tmp/minions/repository-check.json"
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False))
    return int(bool(errors))


if __name__ == "__main__":
    raise SystemExit(main())
