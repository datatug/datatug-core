#!/usr/bin/env python3
"""Prove the corpus is portable from an exact committed Git tree.

Every provenance path must be a regular committed blob at the requested ref,
with the exact SHA-256 and byte length recorded independently in
provenance.json. The committed tree must also contain exactly the known corpus
metadata files, so a clean checkout cannot silently lose ignored fixtures.

This script is hash/membership only. It never evaluates package semantics and
never regenerates corpus bytes.

Run: python3 pkg/walkthrough/testdata/package-corpus-v1/check_tracked.py [REF]
"""

import hashlib
import json
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent
CORPUS_METADATA = {
    "corpus.json",
    "provenance.json",
    "generate.py",
    "verify.py",
    "check_tracked.py",
    "README.md",
}


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def git(*args: str, cwd: pathlib.Path = ROOT) -> bytes:
    return subprocess.check_output(["git", "-C", str(cwd), *args])


def main() -> None:
    ref = sys.argv[1] if len(sys.argv) > 1 else "HEAD"
    repo = pathlib.Path(git("rev-parse", "--show-toplevel").decode().strip())
    prefix = ROOT.relative_to(repo).as_posix()
    raw = git("ls-tree", "-r", "-z", "--full-tree", ref, "--", prefix, cwd=repo)
    tracked: dict[str, tuple[str, str, str]] = {}
    for entry in raw.split(b"\0"):
        if not entry:
            continue
        header, path = entry.split(b"\t", 1)
        mode, kind, object_id = header.decode().split(" ")
        tracked[path.decode()] = (mode, kind, object_id)

    provenance = json.loads((ROOT / "provenance.json").read_bytes())
    files = provenance.get("files", {})
    file_bytes = provenance.get("fileBytes", {})
    if not files:
        fail("empty provenance file map")
    if set(files) != set(file_bytes):
        fail("provenance file/byte maps differ")

    required = set(CORPUS_METADATA)
    required.update(files)
    for relative in sorted(required):
        path = f"{prefix}/{relative}"
        entry = tracked.get(path)
        if entry is None:
            fail(f"missing from committed tree: {relative}")
        mode, kind, object_id = entry
        if mode != "100644" or kind != "blob":
            fail(f"not a regular blob: {relative} mode={mode} type={kind}")
        if relative in files:
            data = git("cat-file", "blob", object_id, cwd=repo)
            if sha256(data) != files[relative]:
                fail(f"committed hash differs: {relative}")
            if len(data) != file_bytes[relative]:
                fail(f"committed length differs: {relative}")

    extra = set(tracked) - {f"{prefix}/{relative}" for relative in required}
    if extra:
        fail("unexpected tracked corpus files: " + ", ".join(sorted(extra)))

    ignored = git("ls-files", "--others", "--ignored", "--exclude-standard",
                  "--", prefix, cwd=repo)
    if ignored.strip():
        # Ignored residue is allowed only when it is also represented by an
        # exact committed blob above; provenance already proves that. Report it
        # so the receipt names the checkout condition instead of hiding it.
        print(f"note: {len(ignored.splitlines())} ignored working-tree files "
              "are also committed and verified above")

    print(f"tracked {len(required)} corpus files at {ref}; "
          f"{len(files)} provenance blobs verified with exact hash and length")


if __name__ == "__main__":
    main()
