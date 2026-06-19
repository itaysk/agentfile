#!/usr/bin/env python3
"""Check or sync fenced code blocks in docs Markdown from `source=` annotations.

For each fenced block like ```yaml source=examples/foo.yaml ... ```,
the body is compared with the current contents of that source file. With
`--write`, discrepant blocks are replaced. Source paths are resolved from the
repository root, so `source=/docs/examples/foo.yaml` and
`source=docs/examples/foo.yaml` both reference the same file.
"""

import argparse
import re
import sys
from pathlib import Path

DOCS_DIR = Path(__file__).resolve().parent
REPO_ROOT = DOCS_DIR.parent

FENCE_OPEN_RE = re.compile(r"^(?P<indent>[ \t]*)(?P<fence>`{3,}|~{3,})(?P<info>[^\n]*)$")
SOURCE_RE = re.compile(r"(?:^|\s)source=(?P<quote>['\"]?)(?P<src>[^\s'\"]+)(?P=quote)(?=\s|$)")


def source_path(src: str) -> Path:
    """Resolve a source annotation from the repository root."""
    normalized = src[1:] if src.startswith("/") else src
    return REPO_ROOT / normalized


def is_fence_close(line: str, fence_char: str, fence_len: int) -> bool:
    stripped = line.strip(" \t\r\n")
    return stripped.startswith(fence_char * fence_len) and set(stripped) <= {fence_char}


def sync(text: str) -> tuple[str, int, list[tuple[int, str]], list[str]]:
    updated = 0
    updated_sources: list[tuple[int, str]] = []
    missing: list[str] = []
    lines = text.splitlines(keepends=True)
    output: list[str] = []
    index = 0
    in_fence: tuple[str, int] | None = None

    while index < len(lines):
        line = lines[index]
        open_match = FENCE_OPEN_RE.match(line)

        if in_fence:
            output.append(line)
            fence_char, fence_len = in_fence
            if is_fence_close(line, fence_char, fence_len):
                in_fence = None
            index += 1
            continue

        if not open_match:
            output.append(line)
            index += 1
            continue

        fence = open_match.group("fence")
        info = open_match.group("info")
        source_match = SOURCE_RE.search(info)

        if fence.startswith("~") or not source_match:
            output.append(line)
            in_fence = (fence[0], len(fence))
            index += 1
            continue

        src = source_match.group("src")
        src_path = source_path(src)
        if not src_path.is_file():
            missing.append(src)
            output.append(line)
            in_fence = (fence[0], len(fence))
            index += 1
            continue

        block_start = index
        index += 1
        while index < len(lines) and not is_fence_close(lines[index], fence[0], len(fence)):
            index += 1

        if index >= len(lines):
            missing.append(f"{src} (unclosed fence)")
            output.extend(lines[block_start:])
            break

        close_line = lines[index]
        old_block = "".join(lines[block_start : index + 1])
        body = src_path.read_text().rstrip("\n") + "\n"
        new_block = f"{line}{body}{close_line}"
        if new_block != old_block:
            updated += 1
            updated_sources.append((block_start + 1, src))
        output.append(new_block)
        index += 1

    return "".join(output), updated, updated_sources, missing


def markdown_paths(args: list[str]) -> list[Path]:
    if args:
        return [Path(arg) if Path(arg).is_absolute() else REPO_ROOT / arg for arg in args]
    return sorted(DOCS_DIR.rglob("*.md"))


def display_path(path: Path) -> Path:
    try:
        return path.relative_to(REPO_ROOT)
    except ValueError:
        return path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Check Markdown code blocks annotated with source= against their source files.",
    )
    parser.add_argument(
        "--write",
        action="store_true",
        help="update discrepant Markdown code blocks instead of only reporting them",
    )
    parser.add_argument(
        "paths",
        nargs="*",
        help="Markdown files to check; defaults to every Markdown file under docs/",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    all_missing: list[tuple[Path, str]] = []
    all_updated: list[tuple[Path, list[tuple[int, str]]]] = []

    for md_path in markdown_paths(args.paths):
        original = md_path.read_text()
        new_text, updated, updated_sources, missing = sync(original)

        for src in missing:
            all_missing.append((md_path, src))

        shown_path = display_path(md_path)
        if new_text != original:
            all_updated.append((md_path, updated_sources))
            if args.write:
                md_path.write_text(new_text)
                print(f"{shown_path}: synced {updated} block(s)")
            else:
                print(f"{shown_path}: {updated} block(s) out of sync")
            for line_number, src in updated_sources:
                print(f"  line {line_number}: source: {src}")
        elif args.write:
            print(f"{shown_path}: already in sync")

    for md_path, src in all_missing:
        shown_path = display_path(md_path)
        print(f"warning: {shown_path}: source not found: {src}", file=sys.stderr)

    if all_missing:
        return 1
    if all_updated and not args.write:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
