#!/usr/bin/env python3
"""Verify a recordsetcompare release archive without extracting paths."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import tarfile
from pathlib import Path

ABI = "recordsetcompare/1"
REQUIRED = {
    "recordsetcompare.wasm",
    "wasm_exec.js",
    "index.mjs",
    "recordsetcompare-worker.js",
    "node-loader.mjs",
    "recordsetcompare-node-worker.mjs",
    "index.d.ts",
    "DATATUG-LICENSE.txt",
    "GO-LICENSE.txt",
    "THIRD-PARTY-NOTICES.md",
    "manifest.json",
}


def verify_archive(path: Path, source: str, tag: str | None, *, allow_missing_licenses: bool = False) -> dict:
    if tag and not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
        raise SystemExit("tag is not an exact Core semver tag")
    with tarfile.open(path, "r:gz") as archive:
        members = archive.getmembers()
        names = [member.name for member in members]
        if len(names) != len(set(names)) or set(names) != REQUIRED:
            raise SystemExit(f"archive file set differs from required set: {sorted(set(names) ^ REQUIRED)}")
        if any(not member.isfile() or member.name != Path(member.name).name for member in members):
            raise SystemExit("archive contains a non-file or path traversal entry")
        files = {member.name: archive.extractfile(member).read() for member in members}
    manifest = json.loads(files["manifest.json"])
    if manifest.get("abi") != ABI or manifest.get("sourceCommit") != source or manifest.get("sourceTag") != tag:
        raise SystemExit("manifest ABI/source/tag does not match requested release identity")
    if manifest.get("goVersion") != "1.27.3":
        raise SystemExit("manifest Go compiler pin does not match the supported release toolchain")
    expected_files = set(manifest.get("files", {}))
    if expected_files != REQUIRED - {"manifest.json"}:
        raise SystemExit("manifest file list does not match the required package contents")
    for name, facts in manifest["files"].items():
        data = files[name]
        if len(data) != facts.get("bytes") or hashlib.sha256(data).hexdigest() != facts.get("sha256"):
            raise SystemExit(f"manifest hash/size mismatch for {name}")
    notices = files["THIRD-PARTY-NOTICES.md"].decode("utf-8")
    if "without bundled license metadata" in notices and not allow_missing_licenses:
        raise SystemExit("a linked Go module has no bundled license/notice; refusing source-bound publication")
    return manifest


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("archive", type=Path)
    parser.add_argument("--source", required=True)
    parser.add_argument("--tag")
    parser.add_argument(
        "--allow-missing-licenses",
        action="store_true",
        help="allow validation of a non-published archive while third-party license metadata is unresolved",
    )
    args = parser.parse_args()
    manifest = verify_archive(args.archive, args.source, args.tag, allow_missing_licenses=args.allow_missing_licenses)
    print(json.dumps({"archive": str(args.archive), "source": args.source, "tag": args.tag, "files": len(manifest["files"]) + 1, "verified": True}, sort_keys=True))


if __name__ == "__main__":
    main()
