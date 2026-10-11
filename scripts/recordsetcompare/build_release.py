#!/usr/bin/env python3
"""Build and deterministically package the linked recordset comparer."""

from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[2]
EXPECTED_GO = "1.27.1"
ABI = "recordsetcompare/1"
ASSET = "recordsetcompare.wasm"
GO_ENV = {"GOTOOLCHAIN": "local", "GOWORK": "off", "GOFLAGS": "-mod=readonly"}


def run(args: list[str], *, cwd: Path = ROOT, env: dict[str, str] | None = None) -> str:
    result = subprocess.run(args, cwd=cwd, env=env, check=True, text=True, capture_output=True)
    return result.stdout.strip()


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def linked_modules(env: dict[str, str]) -> list[tuple[str, str]]:
    details = subprocess.run(
        ["go", "list", "-deps", "-json", "./cmd/recordsetcompare-wasm"],
        cwd=ROOT,
        env=env | GO_ENV | {"GOOS": "js", "GOARCH": "wasm"},
        check=True,
        text=True,
        capture_output=True,
    ).stdout
    decoder = json.JSONDecoder()
    modules: set[tuple[str, str]] = set()
    index = 0
    while index < len(details):
        while index < len(details) and details[index].isspace():
            index += 1
        if index == len(details):
            break
        package, index = decoder.raw_decode(details, index)
        module = package.get("Module")
        if not module:
            continue
        path = module.get("Path", "")
        version = module.get("Version", "")
        if module.get("Replace"):
            raise RuntimeError(f"linked module {path} uses a replacement; release packaging requires the locked upstream module")
        if module.get("Main"):
            directory = module.get("Dir", "")
            module_dir = Path(directory).resolve() if directory else None
            if path != module_path() or module_dir != ROOT.resolve():
                raise RuntimeError(f"linked dependency {path or '<unknown>'} is an unexpected main/workspace module")
            continue
        if not path or not version:
            raise RuntimeError(f"linked module {path or '<unknown>'} has no locked version; refusing incomplete module closure")
        modules.add((path, version))
    return sorted(modules)


def module_path() -> str:
    for line in (ROOT / "go.mod").read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if len(fields) == 2 and fields[0] == "module":
            return fields[1]
    raise RuntimeError("go.mod has no module path")


def module_notice(module: str, version: str, env: dict[str, str]) -> tuple[str, list[tuple[str, bytes]]]:
    encoded = run(["go", "list", "-m", "-json", f"{module}@{version}"], env=env)
    metadata = json.loads(encoded)
    module_root = Path(metadata["Dir"])
    license_files = []
    for candidate in module_root.iterdir():
        if candidate.is_file() and re.fullmatch(r"(?i)(license|copying|notice)(\..*)?", candidate.name):
            license_files.append(candidate)
    if not license_files:
        return module, []
    content = []
    for path in sorted(license_files, key=lambda item: item.name.lower()):
        content.append((f"{module}@{version}/{path.name}", path.read_bytes()))
    return module, content


def build_wasm(path: Path, env: dict[str, str]) -> None:
    build_env = env | GO_ENV | {"GOOS": "js", "GOARCH": "wasm", "CGO_ENABLED": "0"}
    run(["go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-buildid=", "-o", str(path), "./cmd/recordsetcompare-wasm"], env=build_env)


def copy_sources(destination: Path, wasm: Path, goroot: Path, env: dict[str, str], source_commit: str, source_tag: str) -> None:
    destination.mkdir(parents=True)
    shutil.copyfile(wasm, destination / ASSET)
    shutil.copyfile(goroot / "lib" / "wasm" / "wasm_exec.js", destination / "wasm_exec.js")
    sources = ROOT / "web" / "recordsetcompare"
    for name in ("index.mjs", "recordsetcompare-worker.js", "node-loader.mjs", "recordsetcompare-node-worker.mjs", "index.d.ts"):
        shutil.copyfile(sources / name, destination / name)
    shutil.copyfile(ROOT / "LICENSE", destination / "DATATUG-LICENSE.txt")
    go_license = next((path for path in (goroot / "LICENSE", goroot.parent / "LICENSE") if path.is_file()), None)
    if go_license is None:
        raise RuntimeError(f"Go {EXPECTED_GO} distribution has no LICENSE adjacent to {goroot}")
    shutil.copyfile(go_license, destination / "GO-LICENSE.txt")

    notices = ["# Third-party notices", "", f"The linked adapter uses Go {EXPECTED_GO} and these linked Go modules:", ""]
    module_files: list[tuple[str, bytes]] = []
    missing_notices = []
    for module, version in linked_modules(env):
        name, files = module_notice(module, version, env)
        notices.append(f"- `{name}`")
        if not files:
            missing_notices.append(name)
        module_files.extend(files)
    notices.extend(["", "The corresponding upstream license and notice files follow.", ""])
    for name, content in module_files:
        notices.extend([f"## {name}", "", content.decode("utf-8", errors="replace").rstrip(), ""])
    if missing_notices:
        notices.extend(["## Linked modules without bundled license metadata", ""])
        for module in missing_notices:
            notices.append(f"- `{module}`: the exact Go module archive contained no top-level license or notice file. This entry is disclosure only and does not grant redistribution rights.")
        notices.append("")
    (destination / "THIRD-PARTY-NOTICES.md").write_text("\n".join(notices), encoding="utf-8")

    files = {}
    for path in sorted(destination.iterdir(), key=lambda item: item.name):
        if path.is_file():
            files[path.name] = {"bytes": path.stat().st_size, "sha256": sha256(path)}
    manifest = {
        "abi": ABI,
        "files": files,
        "goVersion": EXPECTED_GO,
        "sourceCommit": source_commit,
        "sourceTag": source_tag or None,
    }
    (destination / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def create_archive(source: Path, archive: Path) -> None:
    archive.parent.mkdir(parents=True, exist_ok=True)
    with archive.open("wb") as output, gzip.GzipFile(filename="", mode="wb", fileobj=output, mtime=0) as zipped:
        with tarfile.open(fileobj=zipped, mode="w", format=tarfile.USTAR_FORMAT) as tar:
            for path in sorted(source.iterdir(), key=lambda item: item.name):
                if not path.is_file():
                    continue
                data = path.read_bytes()
                entry = tarfile.TarInfo(path.name)
                entry.size = len(data)
                entry.mode = 0o644
                entry.uid = entry.gid = 0
                entry.uname = entry.gname = ""
                entry.mtime = 0
                tar.addfile(entry, io.BytesIO(data))


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--require-tag", action="store_true")
    parser.add_argument("--require-licenses", action="store_true")
    parser.add_argument("--tag", default=os.environ.get("SOURCE_TAG", ""))
    args = parser.parse_args()
    env = os.environ | GO_ENV
    version = run(["go", "env", "GOVERSION"], env=env)
    if version != f"go{EXPECTED_GO}":
        raise SystemExit(f"expected Go go{EXPECTED_GO}, got {version}")
    source_commit = run(["git", "rev-parse", "HEAD"])
    if args.require_tag:
        if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", args.tag):
            raise SystemExit("release packaging requires exactly one resolved Core semver tag")
        peeled = run(["git", "rev-parse", f"{args.tag}^{{commit}}"])
        if peeled != source_commit:
            raise SystemExit(f"tag {args.tag} peels to {peeled}, not source {source_commit}")
    if args.require_tag and run(["git", "status", "--porcelain"]).strip():
        raise SystemExit("release packaging requires a clean, source-bound checkout")
    goroot = Path(run(["go", "env", "GOROOT"], env=env))
    output = args.output.resolve()
    if output.exists():
        shutil.rmtree(output)
    output.mkdir(parents=True)
    with tempfile.TemporaryDirectory(prefix="recordsetcompare-build-") as temporary:
        first = Path(temporary) / "first.wasm"
        second = Path(temporary) / "second.wasm"
        build_wasm(first, env)
        build_wasm(second, env)
        if sha256(first) != sha256(second):
            raise SystemExit("independent linked builds produced different WASM bytes")
        stage_one = Path(temporary) / "stage-one"
        stage_two = Path(temporary) / "stage-two"
        copy_sources(stage_one, first, goroot, env, source_commit, args.tag)
        copy_sources(stage_two, second, goroot, env, source_commit, args.tag)
        if args.require_licenses and "without bundled license metadata" in (stage_one / "THIRD-PARTY-NOTICES.md").read_text(encoding="utf-8"):
            raise SystemExit("one or more linked modules have no bundled license/notice; refusing publication")
        version_name = args.tag[1:] if args.tag else f"local-{source_commit[:12]}"
        archive = output / f"datatug-recordsetcompare-{version_name}.tar.gz"
        independent_archive = Path(temporary) / "independent.tar.gz"
        create_archive(stage_one, archive)
        create_archive(stage_two, independent_archive)
        if sha256(archive) != sha256(independent_archive):
            raise SystemExit("independent complete package builds produced different archive bytes")
        shutil.copytree(stage_one, output / "files")
    print(json.dumps({"abi": ABI, "archive": str(archive), "archiveBytes": archive.stat().st_size, "archiveSha256": sha256(archive), "sourceCommit": source_commit, "sourceTag": args.tag or None}, sort_keys=True))


if __name__ == "__main__":
    main()
