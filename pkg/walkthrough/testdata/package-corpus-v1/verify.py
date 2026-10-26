#!/usr/bin/env python3
"""Verify the checked-in training-package corpus without executing Go.

Recomputes every raw-file SHA-256, the corpus hash, each vector's index pin and
the trusted binding's package digest. This proves byte identity only; it never
evaluates package semantics, so it can never become a second validator.

Run: python3 pkg/walkthrough/testdata/package-corpus-v1/verify.py
"""

import hashlib
import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parent


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def fail(message: str) -> None:
    print(f"FAIL: {message}", file=sys.stderr)
    raise SystemExit(1)


def main() -> None:
    corpus_bytes = (ROOT / "corpus.json").read_bytes()
    provenance = json.loads((ROOT / "provenance.json").read_bytes())
    corpus = json.loads(corpus_bytes)
    if provenance.get("corpusSha256") != sha256(corpus_bytes):
        fail("corpus.json hash")
    if corpus.get("corpusVersion") != "training-package-corpus-v1":
        fail("corpus version")
    files = provenance.get("files", {})
    if not files:
        fail("empty provenance file map")
    for relative, expected in files.items():
        data = (ROOT / relative).read_bytes()
        if sha256(data) != expected:
            fail(f"file hash {relative}")
    for vector in corpus["vectors"]:
        case = ROOT / "cases" / vector["directory"]
        index = (case / vector["indexPath"]).read_bytes()
        reference = vector["reference"]
        if sha256(index) != reference["sha256"]:
            fail(f"index hash {vector['id']}")
        if str(len(index)) != reference["bytes"]:
            fail(f"index length {vector['id']}")
        if vector["binding"]["packageSha256"] != reference["sha256"] and \
                vector["id"] != "binding-package-digest":
            fail(f"binding digest {vector['id']}")
    print(f"verified {len(corpus['vectors'])} vectors and {len(files)} files")


if __name__ == "__main__":
    main()
