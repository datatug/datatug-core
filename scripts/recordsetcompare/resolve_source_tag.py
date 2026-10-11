#!/usr/bin/env python3
"""Resolve the one existing Core semver tag attached to a source commit."""

from __future__ import annotations

import argparse
import os
import re
import subprocess


TAG_PATTERN = re.compile(r"v[0-9]+\.[0-9]+\.[0-9]+$")


def run(args: list[str]) -> str:
    result = subprocess.run(args, check=True, text=True, capture_output=True)
    return result.stdout.strip()


def resolve_tag(tags: list[str], source_commit: str, head_commit: str, peel) -> str:
    candidates = sorted(tag for tag in tags if TAG_PATTERN.fullmatch(tag))
    matches = [(tag, peel(tag)) for tag in candidates]
    matches = [(tag, commit) for tag, commit in matches if commit == source_commit]
    if len(matches) != 1:
        raise SystemExit(f"expected exactly one Core semver tag for {source_commit}; found {len(matches)}: {[tag for tag, _ in matches]}")
    tag, tag_commit = matches[0]
    if head_commit != source_commit:
        raise SystemExit(f"tag/source/checkout identity mismatch: tag={tag_commit} source={source_commit} head={head_commit}")
    return tag


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", default=os.environ.get("GITHUB_SHA", ""))
    args = parser.parse_args()
    if not args.source:
        raise SystemExit("GITHUB_SHA or --source is required")
    tags = run(["git", "tag", "--list"]).splitlines()
    head = run(["git", "rev-parse", "HEAD"])
    tag = resolve_tag(tags, args.source, head, lambda value: run(["git", "rev-parse", f"{value}^{{commit}}"]))
    print(tag)


if __name__ == "__main__":
    main()
