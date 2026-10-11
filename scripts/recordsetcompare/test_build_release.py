import importlib.util
import json
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch


SCRIPT = Path(__file__).with_name("build_release.py")
SPEC = importlib.util.spec_from_file_location("build_release", SCRIPT)
builder = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(builder)

TAG_SPEC = importlib.util.spec_from_file_location("resolve_source_tag", Path(__file__).with_name("resolve_source_tag.py"))
tag_resolver = importlib.util.module_from_spec(TAG_SPEC)
TAG_SPEC.loader.exec_module(tag_resolver)


class BuildReleaseTests(unittest.TestCase):
    def test_source_tag_requires_one_semver_tag(self):
        with self.assertRaisesRegex(SystemExit, "found 0"):
            tag_resolver.resolve_tag(["release-candidate"], "a" * 40, "a" * 40, lambda _: "a" * 40)
        with self.assertRaisesRegex(SystemExit, "found 2"):
            tag_resolver.resolve_tag(["v1.2.3", "v1.2.4"], "a" * 40, "a" * 40, lambda _: "a" * 40)

    def test_source_tag_must_still_point_at_exact_checkout(self):
        with self.assertRaisesRegex(SystemExit, "found 0"):
            tag_resolver.resolve_tag(["v1.2.3"], "a" * 40, "a" * 40, lambda _: "b" * 40)
        with self.assertRaisesRegex(SystemExit, "identity mismatch"):
            tag_resolver.resolve_tag(["v1.2.3"], "a" * 40, "b" * 40, lambda _: "a" * 40)

    def test_annotated_tag_resolution_ignores_other_source_versions(self):
        commits = {"v1.2.3": "b" * 40, "v1.2.4": "a" * 40}
        tag = tag_resolver.resolve_tag(list(commits), "a" * 40, "a" * 40, commits.__getitem__)
        self.assertEqual(tag, "v1.2.4")

    def test_linked_module_closure_keeps_only_locked_external_modules(self):
        details = '\n'.join([
            json.dumps({"Module": {"Path": "github.com/datatug/datatug-core", "Dir": str(builder.ROOT), "Main": True}}),
            '{"Module":{"Path":"example.com/dep","Version":"v1.2.3"}}',
        ])
        result = subprocess.CompletedProcess([], 0, details, "")
        with patch.object(builder.subprocess, "run", return_value=result) as run:
            self.assertEqual(builder.linked_modules({}), [("example.com/dep", "v1.2.3")])
        self.assertEqual(run.call_args.kwargs["env"]["GOWORK"], "off")
        self.assertEqual(run.call_args.kwargs["env"]["GOFLAGS"], "-mod=readonly")

    def test_wasm_build_overrides_external_workspace_and_module_flags(self):
        with patch.object(builder, "run") as run:
            builder.build_wasm(Path("/tmp/recordsetcompare-test.wasm"), {"GOWORK": "/tmp/foreign/go.work", "GOFLAGS": "-mod=mod"})
        build_env = run.call_args.kwargs["env"]
        self.assertEqual(build_env["GOWORK"], "off")
        self.assertEqual(build_env["GOFLAGS"], "-mod=readonly")

    def test_local_replacement_in_linked_closure_is_rejected(self):
        details = '{"Module":{"Path":"example.com/dep","Version":"v1.2.3","Replace":{"Path":"../dep"}}}'
        result = subprocess.CompletedProcess([], 0, details, "")
        with patch.object(builder.subprocess, "run", return_value=result):
            with self.assertRaisesRegex(RuntimeError, "uses a replacement"):
                builder.linked_modules({})

    def test_unexpected_workspace_main_module_is_rejected(self):
        details = '{"Module":{"Path":"example.com/workspace-dependency","Dir":"/tmp/workspace-dependency","Main":true}}'
        result = subprocess.CompletedProcess([], 0, details, "")
        with patch.object(builder.subprocess, "run", return_value=result):
            with self.assertRaisesRegex(RuntimeError, "unexpected main/workspace module"):
                builder.linked_modules({})

    def test_versionless_external_module_is_rejected(self):
        details = '{"Module":{"Path":"example.com/dep","Version":""}}'
        result = subprocess.CompletedProcess([], 0, details, "")
        with patch.object(builder.subprocess, "run", return_value=result):
            with self.assertRaisesRegex(RuntimeError, "no locked version"):
                builder.linked_modules({})


if __name__ == "__main__":
    unittest.main()
