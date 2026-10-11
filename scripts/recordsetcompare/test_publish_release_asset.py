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


SCRIPT = Path(__file__).with_name("publish_release_asset.py")
SPEC = importlib.util.spec_from_file_location("publish_release_asset", SCRIPT)
publisher = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(publisher)
verify_spec = importlib.util.spec_from_file_location("verify_package", Path(__file__).with_name("verify_package.py"))
verify_package = importlib.util.module_from_spec(verify_spec)
verify_spec.loader.exec_module(verify_package)


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
        "goVersion": "1.27.1",
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
        created = subprocess.CompletedProcess([], 0, "", "")
        draft = {"tag_name": "v1.2.3", "draft": True, "assets": []}
        with patch.object(publisher.subprocess, "run", side_effect=[absent, created, subprocess.CompletedProcess([], 0, json.dumps(draft), "")]) as run:
            release = publisher.ensure_release("datatug/datatug-core", "v1.2.3")
        self.assertTrue(release["draft"])
        create_args = run.call_args_list[1].args[0]
        self.assertIn("--verify-tag", create_args)
        self.assertIn("--draft", create_args)
        self.assertEqual(create_args[-2:], ["--repo", "datatug/datatug-core"])

    def test_non_not_found_release_lookup_fails_closed(self):
        failure = subprocess.CompletedProcess([], 1, "", "HTTP 403: Forbidden")
        with patch.object(publisher.subprocess, "run", return_value=failure) as run:
            with self.assertRaisesRegex(RuntimeError, "403"):
                publisher.ensure_release("datatug/datatug-core", "v1.2.3")
        self.assertEqual(run.call_count, 1)

    def test_moved_tag_blocks_release_publication(self):
        with patch.object(publisher, "remote_tag_commit", return_value="f" * 40), patch.object(publisher, "ensure_release") as ensure:
            with self.assertRaisesRegex(SystemExit, "changed before"):
                publisher.publish_release("datatug/datatug-core", "v1.2.3", "0" * 40, "a" * 64, "package.tar.gz")
        ensure.assert_not_called()

    def test_downloaded_asset_mismatch_is_rejected(self):
        def download(args):
            target = Path(args[args.index("--dir") + 1]) / "package.tar.gz"
            target.write_bytes(b"different immutable bytes")
            return ""

        with patch.object(publisher, "run", side_effect=download) as run:
            with self.assertRaisesRegex(SystemExit, "differs"):
                publisher.verify_download("datatug/datatug-core", "v1.2.3", "package.tar.gz", "0" * 64)
        self.assertEqual(run.call_args.args[0][-2:], ["--repo", "datatug/datatug-core"])

    def test_edit_and_upload_commands_are_repository_scoped(self):
        with patch.object(publisher, "remote_tag_commit", return_value="a" * 40), patch.object(
            publisher, "ensure_release", return_value={"tag_name": "v1.2.3", "draft": True, "assets": []}
        ), patch.object(publisher.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "", "")) as run:
            publisher.publish_release("datatug/datatug-core", "v1.2.3", "a" * 40, "b" * 64, "package.tar.gz")
        self.assertEqual(run.call_args.args[0][-2:], ["--repo", "datatug/datatug-core"])

        with patch.object(publisher.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, "", "")) as run:
            publisher.upload_asset("datatug/datatug-core", "v1.2.3", Path("package.tar.gz"))
        self.assertEqual(run.call_args.args[0][-2:], ["--repo", "datatug/datatug-core"])

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
