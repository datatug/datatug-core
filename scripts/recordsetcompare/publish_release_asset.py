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
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlsplit
from urllib.request import HTTPRedirectHandler, Request, build_opener

from verify_package import verify_archive

RELEASES_PER_PAGE = 100
MAX_RELEASE_LIST_PAGES = 10


class RejectRedirects(HTTPRedirectHandler):
    def redirect_request(self, request, response, code, message, headers, new_url):
        return None


def open_upload(request: Request):
    # The bearer token is scoped to uploads.github.com; never forward it through
    # an unexpected redirect to another host or endpoint.
    return build_opener(RejectRedirects()).open(request, timeout=60)


def run(args: list[str], *, capture: bool = True) -> str:
    result = subprocess.run(args, check=True, text=True, capture_output=capture)
    return result.stdout.strip() if capture else ""


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
    if not isinstance(release.get("id"), int) or isinstance(release.get("id"), bool) or release["id"] <= 0:
        raise RuntimeError("GitHub Release response omitted its numeric release ID")
    if not isinstance(release.get("draft"), bool) or not isinstance(release.get("assets"), list):
        raise RuntimeError("GitHub Release response omitted its draft state or asset list")
    if any(not isinstance(asset, dict) for asset in release["assets"]):
        raise RuntimeError("GitHub Release response contained an invalid asset entry")
    return release


def read_release_by_id(repository: str, release_id: int, tag: str) -> dict:
    if not isinstance(release_id, int) or isinstance(release_id, bool) or release_id <= 0:
        raise RuntimeError("GitHub Release ID is invalid")
    try:
        response = json.loads(run(["gh", "api", f"repos/{repository}/releases/{release_id}"]))
    except json.JSONDecodeError as error:
        raise RuntimeError("GitHub Release response was not valid JSON") from error
    release = validate_release(response, tag)
    if release["id"] != release_id:
        raise RuntimeError("GitHub Release response did not match the requested numeric ID")
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


def ensure_release(repository: str, tag: str, source_commit: str) -> dict:
    release = read_release(repository, tag)
    if release is None:
        # Keep the create response. A workflow token can create a draft that it
        # cannot rediscover through the by-tag or list endpoints.
        try:
            response = json.loads(run([
                "gh", "api", "--method", "POST", f"repos/{repository}/releases",
                "--field", f"tag_name={tag}", "--field", "draft=true",
                "--field", f"name={tag}", "--field", "body=Source-bound package release.",
                "--field", f"target_commitish={source_commit}",
            ]))
        except (subprocess.CalledProcessError, json.JSONDecodeError) as error:
            # A duplicate/concurrent creation is not proof that the resulting
            # draft is visible or belongs to this source. Stop for operator
            # recovery instead of guessing from a failed lookup.
            detail = error.stderr.strip() if isinstance(error, subprocess.CalledProcessError) and error.stderr else ""
            raise RuntimeError(detail or "could not create a source-bound draft release") from error
        release = validate_release(response, tag)
        if not release["draft"]:
            raise RuntimeError("new GitHub Release was not created as a draft")
    if not release or release.get("tag_name") != tag:
        raise SystemExit("GitHub Release does not belong to the exact verified source tag")
    return release


def publish_release(repository: str, release_id: int, tag: str, source_commit: str, expected_asset_id: int, expected_asset_sha256: str, asset_name: str) -> None:
    if remote_tag_commit(repository, tag) != source_commit:
        raise SystemExit("remote source tag changed before release publication")
    release = read_release_by_id(repository, release_id, tag)
    if release.get("draft"):
        run(["gh", "api", "--method", "PATCH", f"repos/{repository}/releases/{release_id}", "--field", "draft=false"])
    if remote_tag_commit(repository, tag) != source_commit:
        raise SystemExit("remote source tag changed during release publication")
    release = read_release_by_id(repository, release_id, tag)
    if release.get("draft"):
        raise SystemExit("published release tag changed during asset upload")
    published = read_release(repository, tag)
    if not published or published.get("draft") or published["id"] != release_id:
        raise SystemExit("published release could not be verified by its exact source tag")
    matching = [asset for asset in published.get("assets", []) if asset.get("name") == asset_name]
    if len(matching) != 1 or matching[0].get("id") != expected_asset_id:
        raise SystemExit("published release is missing its exact immutable package asset")
    verify_download(repository, release_id, matching[0], asset_name, expected_asset_sha256)


def verify_download(repository: str, release_id: int, asset: dict, asset_name: str, expected_sha256: str) -> None:
    asset_id = asset.get("id")
    if not isinstance(asset_id, int) or isinstance(asset_id, bool) or asset_id <= 0:
        raise RuntimeError("GitHub release asset omitted its numeric asset ID")
    try:
        release_assets = json.loads(run(["gh", "api", f"repos/{repository}/releases/{release_id}/assets?per_page=100"]))
    except json.JSONDecodeError as error:
        raise RuntimeError("GitHub release asset list was not valid JSON") from error
    if not isinstance(release_assets, list) or any(not isinstance(candidate, dict) for candidate in release_assets):
        raise RuntimeError("GitHub release asset list was invalid")
    exact_assets = [candidate for candidate in release_assets if candidate.get("name") == asset_name]
    if len(exact_assets) != 1 or exact_assets[0].get("id") != asset_id:
        raise SystemExit("release asset ID is not the unique exact-name asset on the verified release")
    with tempfile.TemporaryDirectory(prefix="recordsetcompare-release-asset-") as temporary:
        current = Path(temporary) / asset_name
        with current.open("wb") as output:
            result = subprocess.run(
                ["gh", "api", "--header", "Accept: application/octet-stream", f"repos/{repository}/releases/assets/{asset_id}"],
                check=False,
                stdout=output,
                stderr=subprocess.PIPE,
            )
        if result.returncode != 0:
            current.unlink(missing_ok=True)
            detail = result.stderr.decode("utf-8", errors="replace").strip()
            raise RuntimeError(detail or "could not download GitHub release asset by ID")
        if not current.is_file() or digest(current) != expected_sha256:
            raise SystemExit("release asset differs; refusing to overwrite or publish it")


def digest(path: Path) -> str:
    value = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            value.update(chunk)
    return value.hexdigest()


def upload_asset(repository: str, release: dict, archive: Path) -> dict:
    release_id = release["id"]
    upload_url = release.get("upload_url")
    parsed = urlsplit(upload_url if isinstance(upload_url, str) else "")
    expected_path = f"/repos/{repository}/releases/{release_id}/assets"
    template_path_ok = parsed.path == expected_path and parsed.query == ""
    template_split_ok = parsed.path == expected_path + "{" and parsed.query == "name,label}"
    if parsed.scheme != "https" or parsed.netloc != "uploads.github.com" or not (template_path_ok or template_split_ok) or parsed.fragment:
        raise RuntimeError("GitHub Release supplied an unsafe or mismatched asset upload URL")
    token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if not token:
        raise RuntimeError("GH_TOKEN or GITHUB_TOKEN is required to upload a GitHub release asset")
    endpoint = f"https://uploads.github.com{expected_path}?name={quote(archive.name, safe='')}"
    request = Request(
        endpoint,
        data=archive.read_bytes(),
        headers={
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {token}",
            "Content-Type": "application/octet-stream",
            "X-GitHub-Api-Version": "2022-11-28",
        },
        method="POST",
    )
    try:
        with open_upload(request) as response:
            result = json.loads(response.read())
    except (HTTPError, URLError, json.JSONDecodeError) as error:
        detail = error.read().decode("utf-8", errors="replace") if isinstance(error, HTTPError) else str(error)
        raise RuntimeError(detail or "GitHub release asset upload failed") from error
    if not isinstance(result, dict) or result.get("name") != archive.name:
        raise RuntimeError("GitHub release asset upload response did not match the package name")
    if not isinstance(result.get("id"), int) or isinstance(result.get("id"), bool) or result.get("id", 0) <= 0:
        raise RuntimeError("GitHub release asset upload response omitted its numeric asset ID")
    if result.get("size") != archive.stat().st_size:
        raise RuntimeError("GitHub release asset upload response size did not match the archive")
    return result


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
    release = ensure_release(repository, args.tag, manifest["sourceCommit"])
    asset_name = args.archive.name
    existing = [asset for asset in release.get("assets", []) if asset.get("name") == asset_name]
    if len(existing) > 1:
        raise SystemExit("release has duplicate assets with the immutable package name")
    if existing:
        verify_download(repository, release["id"], existing[0], asset_name, digest(args.archive))
        publish_release(repository, release["id"], args.tag, manifest["sourceCommit"], existing[0]["id"], digest(args.archive), asset_name)
        print(json.dumps({"asset": asset_name, "action": "existing-byte-identical", "sha256": digest(args.archive)}, sort_keys=True))
        return
    if not release.get("draft"):
        raise SystemExit("published release is missing its immutable package asset; refusing mutation")
    if remote_tag_commit(repository, args.tag) != manifest["sourceCommit"]:
        raise SystemExit("remote source tag changed before release asset upload")
    uploaded = upload_asset(repository, release, args.archive)
    release = read_release_by_id(repository, release["id"], args.tag)
    existing = [asset for asset in release.get("assets", []) if asset.get("name") == asset_name]
    if len(existing) != 1 or existing[0].get("id") != uploaded["id"]:
        raise SystemExit("uploaded asset could not be verified on its exact release")
    verify_download(repository, release["id"], existing[0], asset_name, digest(args.archive))
    publish_release(repository, release["id"], args.tag, manifest["sourceCommit"], uploaded["id"], digest(args.archive), asset_name)
    print(json.dumps({"asset": asset_name, "action": "uploaded", "sha256": digest(args.archive)}, sort_keys=True))


if __name__ == "__main__":
    main()
