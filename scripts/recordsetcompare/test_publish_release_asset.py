import importlib.util
import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch
from urllib.request import Request


SCRIPT = Path(__file__).with_name("publish_release_asset.py")
SPEC = importlib.util.spec_from_file_location("publish_release_asset", SCRIPT)
publisher = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(publisher)
verify_spec = importlib.util.spec_from_file_location("verify_package", Path(__file__).with_name("verify_package.py"))
verify_package = importlib.util.module_from_spec(verify_spec)
verify_spec.loader.exec_module(verify_package)


def make_release(*, draft: bool = True, assets: list[dict] | None = None, release_id: int = 42) -> dict:
    return {
        "id": release_id,
        "tag_name": "v1.2.3",
        "draft": draft,
        "assets": assets or [],
        "upload_url": f"https://uploads.github.com/repos/datatug/datatug-core/releases/{release_id}/assets{{?name,label}}",
    }


def make_archive(
    path: Path,
    *,
    missing_license: bool = False,
    corrupt_hash: bool = False,
    source_commit: str = "a" * 40,
    source_tag: str = "v1.2.3",
) -> None:
    files = {name: f"fixture:{name}".encode() for name in verify_package.REQUIRED - {"manifest.json"}}
    if missing_license:
        files["THIRD-PARTY-NOTICES.md"] = b"Linked modules without bundled license metadata."
    manifest_files = {
        name: {"bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()}
        for name, data in files.items()
    }
    if corrupt_hash:
        manifest_files["recordsetcompare.wasm"]["sha256"] = "0" * 64
    files["manifest.json"] = json.dumps({
        "abi": verify_package.ABI,
        "files": manifest_files,
        "goVersion": "1.27.2",
        "sourceCommit": source_commit,
        "sourceTag": source_tag,
    }, sort_keys=True).encode()
    with tarfile.open(path, "w:gz") as archive:
        for name, data in files.items():
            item = tarfile.TarInfo(name)
            item.size = len(data)
            archive.addfile(item, io.BytesIO(data))


class PublishReleaseAssetTests(unittest.TestCase):
    def test_remote_tag_commit_peels_annotated_tag(self):
        with patch.object(publisher, "run", side_effect=[
            '{"object":{"type":"tag","sha":"annotated"}}',
            '{"object":{"type":"commit","sha":"0123456789abcdef0123456789abcdef01234567"}}',
        ]) as run:
            commit = publisher.remote_tag_commit("datatug/datatug-core", "v1.2.3")
        self.assertEqual(commit, "0123456789abcdef0123456789abcdef01234567")
        self.assertEqual(run.call_count, 2)

    def test_missing_remote_tag_fails_before_release_creation(self):
        missing = subprocess.CalledProcessError(1, ["gh", "api"], stderr="HTTP 404: Not Found")
        with patch.object(publisher.subprocess, "run", side_effect=missing):
            with self.assertRaises(subprocess.CalledProcessError):
                publisher.remote_tag_commit("datatug/datatug-core", "v1.2.3")

    def test_missing_release_is_created_as_draft_for_existing_tag(self):
        absent = subprocess.CompletedProcess([], 1, "", "HTTP 404: Not Found")
        draft = make_release()
        with patch.object(publisher, "run", side_effect=[json.dumps([]), json.dumps(draft)]) as gh_api, patch.object(
            publisher.subprocess, "run", return_value=absent
        ) as gh_call:
            release = publisher.ensure_release("datatug/datatug-core", "v1.2.3", "a" * 40)
        self.assertTrue(release["draft"])
        self.assertEqual(release["id"], 42)
        create_args = gh_api.call_args_list[1].args[0]
        self.assertIn("--method", create_args)
        self.assertIn("POST", create_args)
        self.assertIn("repos/datatug/datatug-core/releases", create_args)
        self.assertIn("tag_name=v1.2.3", create_args)
        self.assertIn("draft=true", create_args)
        self.assertIn(f"target_commitish={'a' * 40}", create_args)
        self.assertEqual(gh_api.call_count, 2)
        self.assertEqual(gh_call.call_count, 1)
        self.assertIn("releases/tags/v1.2.3", gh_call.call_args.args[0][2])

    def test_non_not_found_release_lookup_fails_closed(self):
        failure = subprocess.CompletedProcess([], 1, "", "HTTP 403: Forbidden")
        with patch.object(publisher.subprocess, "run", return_value=failure) as run, patch.object(publisher, "run") as list_call:
            with self.assertRaisesRegex(RuntimeError, "403"):
                publisher.ensure_release("datatug/datatug-core", "v1.2.3", "a" * 40)
        self.assertEqual(run.call_count, 1)
        list_call.assert_not_called()

    def test_published_release_is_read_from_by_tag_endpoint(self):
        published = make_release(draft=False)
        with patch.object(
            publisher.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps(published), "")
        ) as gh_call, patch.object(publisher, "run") as list_call:
            release = publisher.read_release("datatug/datatug-core", "v1.2.3")
        self.assertFalse(release["draft"])
        self.assertEqual(gh_call.call_args.args[0], ["gh", "api", "repos/datatug/datatug-core/releases/tags/v1.2.3"])
        list_call.assert_not_called()

    def test_draft_release_is_found_after_by_tag_endpoint_returns_404(self):
        absent = subprocess.CompletedProcess([], 1, "", "HTTP 404: Not Found")
        draft = make_release()
        with patch.object(publisher.subprocess, "run", return_value=absent), patch.object(
            publisher, "run", return_value=json.dumps([draft])
        ) as list_call:
            release = publisher.read_release("datatug/datatug-core", "v1.2.3")
        self.assertEqual(release, draft)
        self.assertEqual(
            list_call.call_args.args[0],
            ["gh", "api", "repos/datatug/datatug-core/releases?per_page=100&page=1"],
        )

    def test_true_release_absence_is_returned_only_after_complete_short_page(self):
        absent = subprocess.CompletedProcess([], 1, "", "HTTP 404: Not Found")
        with patch.object(publisher.subprocess, "run", return_value=absent), patch.object(
            publisher, "run", return_value=json.dumps([])
        ) as list_call:
            self.assertIsNone(publisher.read_release("datatug/datatug-core", "v1.2.3"))
        list_call.assert_called_once_with(["gh", "api", "repos/datatug/datatug-core/releases?per_page=100&page=1"])

    def test_release_list_paginates_until_exact_tag_is_found(self):
        absent = subprocess.CompletedProcess([], 1, "", "HTTP 404: Not Found")
        older = [{"tag_name": f"v1.0.{index}", "draft": False, "assets": []} for index in range(publisher.RELEASES_PER_PAGE)]
        draft = make_release()
        with patch.object(publisher.subprocess, "run", return_value=absent), patch.object(
            publisher, "run", side_effect=[json.dumps(older), json.dumps([draft])]
        ) as list_call:
            release = publisher.read_release("datatug/datatug-core", "v1.2.3")
        self.assertEqual(release, draft)
        self.assertEqual(list_call.call_count, 2)
        self.assertIn("page=2", list_call.call_args_list[1].args[0][2])

    def test_release_lookup_rejects_duplicate_exact_tags(self):
        absent = subprocess.CompletedProcess([], 1, "", "HTTP 404: Not Found")
        duplicate = make_release()
        first_page = [{"tag_name": f"v1.0.{index}", "draft": False, "assets": []} for index in range(publisher.RELEASES_PER_PAGE - 1)]
        first_page.append(duplicate)
        with patch.object(publisher.subprocess, "run", return_value=absent), patch.object(
            publisher, "run", side_effect=[json.dumps(first_page), json.dumps([duplicate])]
        ) as list_call:
            with self.assertRaisesRegex(RuntimeError, "multiple GitHub Releases"):
                publisher.read_release("datatug/datatug-core", "v1.2.3")
        self.assertEqual(list_call.call_count, 2)

    def test_release_lookup_rejects_lookalike_tag_and_treats_list_lookalike_as_absent(self):
        lookalike = {**make_release(draft=False), "tag_name": "v1.2.30"}
        with patch.object(
            publisher.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps(lookalike), "")
        ), patch.object(publisher, "run") as list_call:
            with self.assertRaisesRegex(RuntimeError, "did not match exact tag"):
                publisher.read_release("datatug/datatug-core", "v1.2.3")
        list_call.assert_not_called()

        absent = subprocess.CompletedProcess([], 1, "", "HTTP 404: Not Found")
        with patch.object(publisher.subprocess, "run", return_value=absent), patch.object(
            publisher, "run", return_value=json.dumps([lookalike])
        ):
            self.assertIsNone(publisher.read_release("datatug/datatug-core", "v1.2.3"))

    def test_release_lookup_rejects_incomplete_release_metadata(self):
        incomplete = {**make_release(draft=False), "draft": "false"}
        with patch.object(
            publisher.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps(incomplete), "")
        ):
            with self.assertRaisesRegex(RuntimeError, "omitted its draft state or asset list"):
                publisher.read_release("datatug/datatug-core", "v1.2.3")

    def test_release_list_api_error_fails_closed(self):
        absent = subprocess.CompletedProcess([], 1, "", "HTTP 404: Not Found")
        failure = subprocess.CalledProcessError(1, ["gh", "api"], stderr="HTTP 403: Forbidden")
        with patch.object(publisher.subprocess, "run", return_value=absent), patch.object(
            publisher, "run", side_effect=failure
        ):
            with self.assertRaisesRegex(RuntimeError, "403"):
                publisher.read_release("datatug/datatug-core", "v1.2.3")

    def test_release_list_fails_closed_when_page_bound_is_exhausted(self):
        absent = subprocess.CompletedProcess([], 1, "", "HTTP 404: Not Found")
        full_page = json.dumps([{"tag_name": f"v1.0.{index}"} for index in range(2)])
        with patch.object(publisher, "MAX_RELEASE_LIST_PAGES", 2), patch.object(
            publisher, "RELEASES_PER_PAGE", 2
        ), patch.object(publisher.subprocess, "run", return_value=absent), patch.object(
            publisher, "run", side_effect=[full_page, full_page]
        ) as list_call:
            with self.assertRaisesRegex(RuntimeError, "exceeded 2 pages"):
                publisher.read_release("datatug/datatug-core", "v9.9.9")
        self.assertEqual(list_call.call_count, 2)

    def test_moved_tag_blocks_release_publication(self):
        with patch.object(publisher, "remote_tag_commit", return_value="f" * 40), patch.object(publisher, "ensure_release") as ensure:
            with self.assertRaisesRegex(SystemExit, "changed before"):
                publisher.publish_release("datatug/datatug-core", 42, "v1.2.3", "0" * 40, 77, "a" * 64, "package.tar.gz")
        ensure.assert_not_called()

    def test_downloaded_asset_mismatch_is_rejected(self):
        def api_response(args, **kwargs):
            if "stdout" in kwargs:
                kwargs["stdout"].write(b"different immutable bytes")
                return subprocess.CompletedProcess(args, 0, None, b"")
            return json.dumps([{"id": 77, "name": "package.tar.gz"}])

        with patch.object(publisher, "run", return_value=json.dumps([{"id": 77, "name": "package.tar.gz"}])) as api_run, patch.object(
            publisher.subprocess, "run", side_effect=api_response
        ) as run:
            with self.assertRaisesRegex(SystemExit, "differs"):
                publisher.verify_download("datatug/datatug-core", 42, {"id": 77}, "package.tar.gz", "0" * 64)
        self.assertIn("repos/datatug/datatug-core/releases/42/assets", api_run.call_args.args[0][2])
        self.assertIn("releases/assets/77", run.call_args.args[0][-1])

    def test_edit_and_upload_commands_are_repository_scoped(self):
        with patch.object(publisher, "remote_tag_commit", return_value="a" * 40), patch.object(
            publisher, "read_release_by_id", side_effect=[make_release(), make_release(draft=False, assets=[{"id": 77, "name": "package.tar.gz"}])]
        ), patch.object(publisher, "read_release", return_value=make_release(draft=False, assets=[{"id": 77, "name": "package.tar.gz"}])), patch.object(
            publisher, "verify_download"
        ) as verify, patch.object(publisher, "run", return_value="{}") as run, patch.object(
            publisher, "remote_tag_commit", side_effect=["a" * 40, "a" * 40]
        ):
            publisher.publish_release("datatug/datatug-core", 42, "v1.2.3", "a" * 40, 77, "b" * 64, "package.tar.gz")
        self.assertIn("repos/datatug/datatug-core/releases/42", run.call_args.args[0])
        self.assertIn("draft=false", run.call_args.args[0])
        verify.assert_called_once()

    def test_upload_uses_create_response_upload_url_and_never_tag_lookup(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "package.tar.gz"
            archive.write_bytes(b"verified bytes")
            response = unittest.mock.MagicMock()
            response.__enter__.return_value.read.return_value = json.dumps({"id": 77, "name": archive.name, "size": archive.stat().st_size}).encode()
            with patch.dict("os.environ", {"GH_TOKEN": "test-token"}), patch.object(publisher, "open_upload", return_value=response) as open_url:
                uploaded = publisher.upload_asset("datatug/datatug-core", make_release(), archive)
        request = open_url.call_args.args[0]
        self.assertEqual(uploaded["id"], 77)
        self.assertEqual(request.full_url, "https://uploads.github.com/repos/datatug/datatug-core/releases/42/assets?name=package.tar.gz")
        self.assertEqual(request.get_method(), "POST")
        self.assertEqual(request.data, b"verified bytes")
        self.assertEqual(request.get_header("Authorization"), "Bearer test-token")

    def test_upload_rejects_mismatched_release_upload_url_before_network(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "package.tar.gz"
            archive.write_bytes(b"bytes")
            other = {**make_release(), "upload_url": "https://uploads.github.com/repos/other/repo/releases/42/assets{?name,label}"}
            with patch.object(publisher, "open_upload") as open_url:
                with self.assertRaisesRegex(RuntimeError, "unsafe or mismatched"):
                    publisher.upload_asset("datatug/datatug-core", other, archive)
            open_url.assert_not_called()

    def test_upload_rejects_wrong_asset_identity_or_size(self):
        with tempfile.TemporaryDirectory() as directory:
            archive = Path(directory) / "package.tar.gz"
            archive.write_bytes(b"verified bytes")
            for result in (
                {"id": True, "name": archive.name, "size": archive.stat().st_size},
                {"id": 77, "name": archive.name, "size": 0},
            ):
                response = unittest.mock.MagicMock()
                response.__enter__.return_value.read.return_value = json.dumps(result).encode()
                with self.subTest(result=result), patch.dict("os.environ", {"GH_TOKEN": "test-token"}), patch.object(
                    publisher, "open_upload", return_value=response
                ):
                    with self.assertRaises(RuntimeError):
                        publisher.upload_asset("datatug/datatug-core", make_release(), archive)

    def test_redirects_are_rejected_before_upload_token_can_be_forwarded(self):
        handler = publisher.RejectRedirects()
        redirected = handler.redirect_request(
            Request("https://uploads.github.com/release", headers={"Authorization": "Bearer secret"}),
            None,
            302,
            "Found",
            {},
            "https://attacker.invalid/collect",
        )
        self.assertIsNone(redirected)

    def test_tag_moved_during_publish_fails_before_published_readback(self):
        with patch.object(publisher, "read_release_by_id", return_value=make_release()), patch.object(
            publisher, "remote_tag_commit", side_effect=["a" * 40, "b" * 40]
        ), patch.object(publisher, "read_release") as by_tag, patch.object(publisher, "run") as api_run:
            with self.assertRaisesRegex(SystemExit, "changed during"):
                publisher.publish_release("datatug/datatug-core", 42, "v1.2.3", "a" * 40, 77, "b" * 64, "package.tar.gz")
        api_run.assert_called_once()
        self.assertIn("PATCH", api_run.call_args.args[0])
        by_tag.assert_not_called()

    def test_published_readback_must_preserve_exact_uploaded_asset_id(self):
        with patch.object(publisher, "read_release_by_id", side_effect=[make_release(), make_release(draft=False)]), patch.object(
            publisher, "remote_tag_commit", return_value="a" * 40
        ), patch.object(publisher, "read_release", return_value=make_release(draft=False, assets=[{"id": 88, "name": "package.tar.gz"}])), patch.object(
            publisher, "verify_download"
        ) as verify, patch.object(publisher, "run", return_value="{}"):
            with self.assertRaisesRegex(SystemExit, "missing its exact immutable package asset"):
                publisher.publish_release("datatug/datatug-core", 42, "v1.2.3", "a" * 40, 77, "b" * 64, "package.tar.gz")
        verify.assert_not_called()

    def test_creation_error_with_invisible_draft_fails_closed_without_rediscovery_guess(self):
        absent = subprocess.CompletedProcess([], 1, "", "HTTP 404: Not Found")
        conflict = subprocess.CalledProcessError(1, ["gh", "api"], stderr="HTTP 422: Validation Failed")
        with patch.object(publisher, "run", side_effect=[json.dumps([]), conflict]) as api_run, patch.object(
            publisher.subprocess, "run", return_value=absent
        ) as tag_lookup:
            with self.assertRaisesRegex(RuntimeError, "422"):
                publisher.ensure_release("datatug/datatug-core", "v1.2.3", "a" * 40)
        self.assertEqual(api_run.call_count, 2)
        tag_lookup.assert_called_once()

    def test_read_release_by_id_requires_exact_tag_and_numeric_id(self):
        wrong_tag = {**make_release(), "tag_name": "v9.9.9"}
        with patch.object(publisher, "run", return_value=json.dumps(wrong_tag)) as api_run:
            with self.assertRaisesRegex(RuntimeError, "did not match exact tag"):
                publisher.read_release_by_id("datatug/datatug-core", 42, "v1.2.3")
        self.assertEqual(api_run.call_args.args[0][-1], "repos/datatug/datatug-core/releases/42")

        with self.assertRaisesRegex(RuntimeError, "ID is invalid"):
            publisher.read_release_by_id("datatug/datatug-core", 0, "v1.2.3")

        wrong_id = {**make_release(), "id": 43}
        with patch.object(publisher, "run", return_value=json.dumps(wrong_id)):
            with self.assertRaisesRegex(RuntimeError, "did not match the requested numeric ID"):
                publisher.read_release_by_id("datatug/datatug-core", 42, "v1.2.3")

    def test_download_refuses_asset_not_listed_on_exact_release_id(self):
        with patch.object(publisher, "run", return_value=json.dumps([{"id": 88, "name": "package.tar.gz"}])), patch.object(
            publisher.subprocess, "run"
        ) as download:
            with self.assertRaisesRegex(SystemExit, "not the unique exact-name asset"):
                publisher.verify_download("datatug/datatug-core", 42, {"id": 77}, "package.tar.gz", "0" * 64)
        download.assert_not_called()

    def test_publisher_rejects_self_asserted_archive_before_any_github_request(self):
        for invalid, source, tag in (
            ({"missing_license": True}, "a" * 40, "v1.2.3"),
            ({"corrupt_hash": True}, "a" * 40, "v1.2.3"),
            ({}, "b" * 40, "v1.2.3"),
            ({}, "a" * 40, "v1.2.4"),
        ):
            with self.subTest(invalid=invalid, source=source, tag=tag), tempfile.TemporaryDirectory() as directory:
                archive = Path(directory) / "package.tar.gz"
                make_archive(archive, source_commit=source, **invalid)
                with patch.object(sys, "argv", [str(SCRIPT), tag, str(archive)]), patch.dict(
                    "os.environ", {"GITHUB_REPOSITORY": "datatug/datatug-core", "GITHUB_SHA": "a" * 40}
                ), patch.object(publisher.subprocess, "run") as github_call:
                    with self.assertRaises(SystemExit):
                        publisher.main()
                github_call.assert_not_called()


if __name__ == "__main__":
    unittest.main()
