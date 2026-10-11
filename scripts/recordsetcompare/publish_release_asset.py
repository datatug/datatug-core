#!/usr/bin/env python3
"""Upload a verified release asset without replacing existing bytes."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile

from verify_package import verify_archive

RELEASES_PER_PAGE = 100
MAX_RELEASE_LIST_PAGES = 10


def run(args: list[str], *, capture: bool = True) -> str:
    result = subprocess.run(args, check=True, text=True, capture_output=capture)
    return result.stdout.strip() if capture else ""


def release_command(repository: str, operation: str, *args: str) -> list[str]:
    return ["gh", "release", operation, *args, "--repo", repository]


def remote_tag_commit(repository: str, tag: str) -> str:
    reference = json.loads(run(["gh", "api", f"repos/{repository}/git/ref/tags/{tag}"]))
    obj = reference.get("object", {})
    for _ in range(8):
        if obj.get("type") == "commit":
            commit = obj.get("sha", "")
            if re.fullmatch(r"[0-9a-f]{40,64}", commit):
                return commit
            break
        if obj.get("type") != "tag" or not obj.get("sha"):
            break
        annotated = json.loads(run(["gh", "api", f"repos/{repository}/git/tags/{obj['sha']}"]))
        obj = annotated.get("object", {})
    raise SystemExit("remote source tag does not resolve to a commit")


def validate_release(release: object, tag: str) -> dict:
    if not isinstance(release, dict) or release.get("tag_name") != tag:
        raise RuntimeError(f"GitHub Release response did not match exact tag {tag}")
    if not isinstance(release.get("draft"), bool) or not isinstance(release.get("assets"), list):
        raise RuntimeError("GitHub Release response omitted its draft state or asset list")
    if any(not isinstance(asset, dict) for asset in release["assets"]):
        raise RuntimeError("GitHub Release response contained an invalid asset entry")
    return release


def read_release(repository: str, tag: str) -> dict | None:
    result = subprocess.run(
        ["gh", "api", f"repos/{repository}/releases/tags/{tag}"],
        check=False,
        text=True,
        capture_output=True,
    )
    if result.returncode == 0:
        try:
            release = json.loads(result.stdout)
        except json.JSONDecodeError as error:
            raise RuntimeError("GitHub Release response was not valid JSON") from error
        return validate_release(release, tag)
    if "HTTP 404" not in result.stderr:
        raise RuntimeError(result.stderr.strip() or "could not read GitHub Release")
    return read_release_from_list(repository, tag)


def read_release_from_list(repository: str, tag: str) -> dict | None:
    matches: list[dict] = []
    for page in range(1, MAX_RELEASE_LIST_PAGES + 1):
        endpoint = f"repos/{repository}/releases?per_page={RELEASES_PER_PAGE}&page={page}"
        try:
            response = json.loads(run(["gh", "api", endpoint]))
        except subprocess.CalledProcessError as error:
            detail = error.stderr.strip() if error.stderr else ""
            raise RuntimeError(detail or "could not list GitHub Releases") from error
        except json.JSONDecodeError as error:
            raise RuntimeError("GitHub Releases response was not valid JSON") from error
        if not isinstance(response, list) or any(
            not isinstance(release, dict) or not isinstance(release.get("tag_name"), str)
            for release in response
        ):
            raise RuntimeError("GitHub Releases response did not contain a valid release list")
        matches.extend(release for release in response if release["tag_name"] == tag)
        if len(matches) > 1:
            raise RuntimeError(f"multiple GitHub Releases found for exact tag {tag}")
        if len(response) < RELEASES_PER_PAGE:
            return validate_release(matches[0], tag) if matches else None
    raise RuntimeError(f"GitHub Releases lookup exceeded {MAX_RELEASE_LIST_PAGES} pages")


def ensure_release(repository: str, tag: str) -> dict:
    release = read_release(repository, tag)
    if release is None:
        result = subprocess.run(
            release_command(repository, "create", tag, "--draft", "--verify-tag", "--title", tag, "--notes", "Source-bound package release."),
            check=False,
            text=True,
            capture_output=True,
        )
        if result.returncode != 0:
            # Another publisher may have created the release after our read.
            release = read_release(repository, tag)
            if release is None:
                raise RuntimeError(result.stderr.strip() or "could not create release for the existing tag")
        else:
            release = read_release(repository, tag)
    if not release or release.get("tag_name") != tag:
        raise SystemExit("GitHub Release does not belong to the exact verified source tag")
    return release


def publish_release(repository: str, tag: str, source_commit: str, expected_asset_sha256: str, asset_name: str) -> None:
    if remote_tag_commit(repository, tag) != source_commit:
        raise SystemExit("remote source tag changed before release publication")
    release = ensure_release(repository, tag)
    if release.get("draft"):
        result = subprocess.run(
            release_command(repository, "edit", tag, "--draft=false"),
            check=False,
            text=True,
            capture_output=True,
        )
        if result.returncode != 0:
            # A concurrent publisher may already have published the exact asset.
            if remote_tag_commit(repository, tag) != source_commit:
                raise SystemExit("remote source tag changed during release publication")
            release = ensure_release(repository, tag)
            matching = [asset for asset in release.get("assets", []) if asset.get("name") == asset_name]
            if release.get("draft") or len(matching) != 1:
                raise RuntimeError(result.stderr.strip() or "could not publish the source-bound release")
            verify_download(repository, tag, asset_name, expected_asset_sha256)
    elif release.get("tag_name") != tag:
        raise SystemExit("published release tag changed during asset upload")


def verify_download(repository: str, tag: str, asset_name: str, expected_sha256: str) -> None:
    with tempfile.TemporaryDirectory(prefix="recordsetcompare-release-asset-") as temporary:
        run(release_command(repository, "download", tag, "--pattern", asset_name, "--dir", temporary))
        current = Path(temporary) / asset_name
        if not current.is_file() or digest(current) != expected_sha256:
            raise SystemExit("release asset differs; refusing to overwrite or publish it")


def digest(path: Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def upload_asset(repository: str, tag: str, archive: Path) -> subprocess.CompletedProcess:
    return subprocess.run(release_command(repository, "upload", tag, str(archive)), check=False, text=True, capture_output=True)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("tag")
    parser.add_argument("archive", type=Path)
    args = parser.parse_args()
    repository = os.environ.get("GITHUB_REPOSITORY", "")
    if not repository:
        raise SystemExit("GITHUB_REPOSITORY is required for source-bound release publication")
    source_commit = os.environ.get("GITHUB_SHA", "")
    if not source_commit:
        raise SystemExit("GITHUB_SHA is required for source-bound release publication")
    if not args.archive.is_file():
        raise SystemExit("verified release archive is missing")
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", args.tag):
        raise SystemExit("release tag is not an exact Core semver tag")
    # This is deliberately strict and happens before any GitHub request or write.
    # The publisher has no validation-only escape hatch for missing licenses.
    manifest = verify_archive(args.archive, source_commit, args.tag)
    remote_commit = remote_tag_commit(repository, args.tag)
    if remote_commit != manifest["sourceCommit"]:
        raise SystemExit("remote tag no longer resolves to the archive's source commit")
    release = ensure_release(repository, args.tag)
    asset_name = args.archive.name
    existing = [asset for asset in release.get("assets", []) if asset.get("name") == asset_name]
    if len(existing) > 1:
        raise SystemExit("release has duplicate assets with the immutable package name")
    if existing:
        verify_download(repository, args.tag, asset_name, digest(args.archive))
        publish_release(repository, args.tag, manifest["sourceCommit"], digest(args.archive), asset_name)
        print(json.dumps({"asset": asset_name, "action": "existing-byte-identical", "sha256": digest(args.archive)}, sort_keys=True))
        return
    if not release.get("draft"):
        raise SystemExit("published release is missing its immutable package asset; refusing mutation")
    upload = upload_asset(repository, args.tag, args.archive)
    if upload.returncode != 0:
        # A concurrent upload is acceptable only if the resulting immutable bytes match.
        release = ensure_release(repository, args.tag)
        existing = [asset for asset in release.get("assets", []) if asset.get("name") == asset_name]
        if len(existing) != 1:
            raise RuntimeError(upload.stderr.strip() or "release asset upload failed")
        verify_download(repository, args.tag, asset_name, digest(args.archive))
        publish_release(repository, args.tag, manifest["sourceCommit"], digest(args.archive), asset_name)
        print(json.dumps({"asset": asset_name, "action": "concurrent-byte-identical", "sha256": digest(args.archive)}, sort_keys=True))
        return
    verify_download(repository, args.tag, asset_name, digest(args.archive))
    publish_release(repository, args.tag, manifest["sourceCommit"], digest(args.archive), asset_name)
    print(json.dumps({"asset": asset_name, "action": "uploaded", "sha256": digest(args.archive)}, sort_keys=True))


if __name__ == "__main__":
    main()
