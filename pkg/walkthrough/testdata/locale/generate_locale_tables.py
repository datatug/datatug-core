#!/usr/bin/env python3
"""Generate the pinned BCP-47 profile tables for Go and the TypeScript mirror.

This script is a checked-in fixture generator, not part of the runtime package.
It reads the two official IANA registry snapshots retained next to this file,
verifies their exact SHA-256 digests, derives membership / private-use /
preferred-value / extlang-prefix / grandfathered tables mechanically from the
records, and writes:

    pkg/walkthrough/locale_tables_generated.go
    pkg/walkthrough/testdata/locale/locale-tables.json
    pkg/walkthrough/testdata/locale/locale-provenance.json

The generated tables are data only. No expected admission or canonicalization
result is produced here: the expected vectors in locale-corpus.json are
authored separately from RFC 5646 and the same registry records. Neither this
script nor the tests calls Intl, ICU, CLDR or golang.org/x/text.

Run from the repository root:

    python3 pkg/walkthrough/testdata/locale/generate_locale_tables.py
"""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
REPO_ROOT = os.path.abspath(os.path.join(HERE, "..", "..", "..", ".."))
LANGUAGE_REGISTRY = os.path.join(HERE, "language-subtag-registry.txt")
EXTENSIONS_REGISTRY = os.path.join(HERE, "language-tag-extensions-registry.txt")
TABLES_JSON = os.path.join(HERE, "locale-tables.json")
PROVENANCE_JSON = os.path.join(HERE, "locale-provenance.json")
GO_OUT = os.path.join(REPO_ROOT, "pkg", "walkthrough", "locale_tables_generated.go")

LANGUAGE_REGISTRY_URL = (
    "https://www.iana.org/assignments/language-subtag-registry/language-subtag-registry"
)
EXTENSIONS_REGISTRY_URL = (
    "https://www.iana.org/assignments/language-tag-extensions-registry/"
    "language-tag-extensions-registry"
)
LANGUAGE_REGISTRY_SHA256 = (
    "755fad43283be7b41ebe3c89ad054b6eaf928f404f9c0edb74799e0eab74beb1"
)
EXTENSIONS_REGISTRY_SHA256 = (
    "fdf7764455c493c245a9b3b5b9cd3938391f0637302c3e943fde86aee652e376"
)
RETRIEVED = "2026-10-26"
PROFILE = "datatug-bcp47-profile/1"


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def read_verified(path: str, expected: str) -> bytes:
    with open(path, "rb") as handle:
        data = handle.read()
    actual = sha256_bytes(data)
    if actual != expected:
        sys.exit(f"{path}: SHA-256 {actual} does not match pinned {expected}")
    return data


def parse_registry(text: str) -> tuple[str, list[dict[str, list[str]]]]:
    """Return (File-Date, records), each record mapping field name to values."""
    file_date = ""
    records: list[dict[str, list[str]]] = []
    for chunk in text.split("%%"):
        chunk = chunk.strip()
        if not chunk:
            continue
        fields: dict[str, list[str]] = {}
        last = ""
        for line in chunk.splitlines():
            if line.startswith("  ") and last:
                fields[last][-1] += " " + line.strip()
                continue
            if ":" not in line:
                continue
            name, value = line.split(":", 1)
            last = name.strip()
            fields.setdefault(last, []).append(value.strip())
        if "Type" not in fields and "File-Date" in fields:
            file_date = fields["File-Date"][0]
            continue
        records.append(fields)
    return file_date, records


def one(record: dict[str, list[str]], name: str, default: str = "") -> str:
    values = record.get(name)
    return values[0] if values else default


def title_script(value: str) -> str:
    return value[:1].upper() + value[1:].lower()


def parse_extension_registry(text: str) -> tuple[str, list[str]]:
    file_date = ""
    singletons: list[str] = []
    for chunk in text.split("%%"):
        chunk = chunk.strip()
        if not chunk:
            continue
        fields: dict[str, str] = {}
        for line in chunk.splitlines():
            if ":" in line and not line.startswith("  "):
                name, value = line.split(":", 1)
                fields[name.strip()] = value.strip()
        if "File-Date" in fields:
            file_date = fields["File-Date"]
        if "Identifier" in fields:
            singletons.append(fields["Identifier"].lower())
    return file_date, sorted(singletons)


def build_tables(language_text: str, extension_text: str) -> dict:
    language_file_date, records = parse_registry(language_text)
    extension_file_date, singletons = parse_extension_registry(extension_text)

    languages: set[str] = set()
    scripts: set[str] = set()
    regions: set[str] = set()
    variants: set[str] = set()
    extlangs: dict[str, str] = {}
    preferred_languages: dict[str, str] = {}
    preferred_scripts: dict[str, str] = {}
    preferred_regions: dict[str, str] = {}
    preferred_variants: dict[str, str] = {}
    grandfathered: list[dict] = []
    redundant: list[dict] = []
    private_ranges: dict[str, list[list[str]]] = {
        "language": [],
        "script": [],
        "region": [],
    }

    for record in records:
        record_type = one(record, "Type")
        subtag = one(record, "Subtag")
        if record_type in ("language", "script", "region"):
            descriptions = record.get("Description", [])
            if any(description.lower() == "private use" for description in descriptions):
                if ".." in subtag:
                    low, high = subtag.split("..", 1)
                else:
                    low = high = subtag
                private_ranges[record_type].append([low, high])
                continue
        if record_type == "language":
            languages.add(subtag.lower())
            if "Preferred-Value" in record:
                preferred_languages[subtag.lower()] = one(record, "Preferred-Value").lower()
        elif record_type == "extlang":
            prefix = one(record, "Prefix").lower()
            if not prefix:
                sys.exit(f"extlang {subtag} has no Prefix")
            extlangs[subtag.lower()] = prefix
        elif record_type == "script":
            scripts.add(title_script(subtag))
            if "Preferred-Value" in record:
                preferred_scripts[title_script(subtag)] = title_script(
                    one(record, "Preferred-Value")
                )
        elif record_type == "region":
            regions.add(subtag.upper())
            if "Preferred-Value" in record:
                preferred_regions[subtag.upper()] = one(record, "Preferred-Value").upper()
        elif record_type == "variant":
            variants.add(subtag.lower())
            if "Preferred-Value" in record:
                preferred_variants[subtag.lower()] = one(record, "Preferred-Value").lower()
        elif record_type in ("grandfathered", "redundant"):
            tag = one(record, "Tag")
            item = {"tag": tag}
            if "Preferred-Value" in record:
                item["preferred"] = one(record, "Preferred-Value")
            (grandfathered if record_type == "grandfathered" else redundant).append(item)

    for ranges in private_ranges.values():
        ranges.sort()
    grandfathered.sort(key=lambda item: item["tag"].lower())
    redundant.sort(key=lambda item: item["tag"].lower())

    tables = {
        "profile": PROFILE,
        "languageRegistry": {
            "url": LANGUAGE_REGISTRY_URL,
            "fileDate": language_file_date,
            "sha256": LANGUAGE_REGISTRY_SHA256,
        },
        "extensionsRegistry": {
            "url": EXTENSIONS_REGISTRY_URL,
            "fileDate": extension_file_date,
            "sha256": EXTENSIONS_REGISTRY_SHA256,
        },
        "languages": sorted(languages),
        "scripts": sorted(scripts),
        "regions": sorted(regions),
        "variants": sorted(variants),
        "extlangs": {key: extlangs[key] for key in sorted(extlangs)},
        "preferredLanguages": {
            key: preferred_languages[key] for key in sorted(preferred_languages)
        },
        "preferredScripts": {
            key: preferred_scripts[key] for key in sorted(preferred_scripts)
        },
        "preferredRegions": {
            key: preferred_regions[key] for key in sorted(preferred_regions)
        },
        "preferredVariants": {
            key: preferred_variants[key] for key in sorted(preferred_variants)
        },
        "grandfathered": grandfathered,
        "redundant": redundant,
        "extensionSingletons": singletons,
        "privateUseRanges": private_ranges,
    }
    return tables


def json_bytes(value: object) -> bytes:
    return (json.dumps(value, ensure_ascii=True, indent=2, sort_keys=True) + "\n").encode(
        "utf-8"
    )


def go_string(value: str) -> str:
    return json.dumps(value, ensure_ascii=True)


def packed_set(name: str, values: list[str], indent: str = "\t") -> str:
    tokens = [go_string(value + " ") for value in values]
    lines: list[list[str]] = []
    current: list[str] = []
    width = 0
    for token in tokens:
        piece = len(token) + (3 if current else 0)
        if current and width + piece > 96:
            lines.append(current)
            current = []
            width = 0
        width += piece
        current.append(token)
    if current:
        lines.append(current)
    rendered = " +\n".join(
        indent + " + ".join(line) for line in lines
    )
    return f"var {name} = newLocaleSet(\n" + rendered + ",\n)\n"


def go_map(name: str, value_type: str, entries: list[tuple[str, str]]) -> str:
    lines = [f"var {name} = map[string]{value_type}{{"]
    for key, value in entries:
        lines.append(f"\t{go_string(key)}: {go_string(value)},")
    lines.append("}\n")
    return "\n".join(lines)


def go_records(name: str, records: list[dict]) -> str:
    lines = [f"var {name} = []localeWholeTagRecord{{"]
    for record in records:
        preferred = record.get("preferred")
        if preferred:
            lines.append(
                f"\t{{tag: {go_string(record['tag'])}, preferred: {go_string(preferred)}}},"
            )
        else:
            lines.append(f"\t{{tag: {go_string(record['tag'])}}},")
    lines.append("}\n")
    return "\n".join(lines)


def go_ranges(name: str, ranges: list[list[str]]) -> str:
    lines = [f"var {name} = []localeRange{{"]
    for low, high in ranges:
        lines.append(f"\t{{low: {go_string(low)}, high: {go_string(high)}}},")
    lines.append("}\n")
    return "\n".join(lines)


def write_go(tables: dict, tables_digest: str) -> None:
    language_registry = tables["languageRegistry"]
    extensions_registry = tables["extensionsRegistry"]
    parts = [
        "// Code generated by testdata/locale/generate_locale_tables.py; DO NOT EDIT.\n",
        "package walkthrough\n",
        "const (\n"
        "\t// LocaleRegistryFileDate is the pinned IANA Language Subtag Registry File-Date.\n"
        f'\tLocaleRegistryFileDate = {go_string(language_registry["fileDate"])}\n'
        "\t// LocaleRegistrySHA256 is the SHA-256 of the exact retained registry bytes.\n"
        f'\tLocaleRegistrySHA256 = {go_string(language_registry["sha256"])}\n'
        "\t// LocaleRegistryURL is the official source of LocaleRegistrySHA256.\n"
        f'\tLocaleRegistryURL = {go_string(language_registry["url"])}\n'
        "\t// LocaleExtensionsRegistryFileDate is the pinned extension registry File-Date.\n"
        f'\tLocaleExtensionsRegistryFileDate = {go_string(extensions_registry["fileDate"])}\n'
        "\t// LocaleExtensionsRegistrySHA256 is the SHA-256 of the retained extension bytes.\n"
        f'\tLocaleExtensionsRegistrySHA256 = {go_string(extensions_registry["sha256"])}\n'
        "\t// LocaleExtensionsRegistryURL is the official source of that snapshot.\n"
        f'\tLocaleExtensionsRegistryURL = {go_string(extensions_registry["url"])}\n'
        "\t// LocaleTablesSHA256 is the SHA-256 of testdata/locale/locale-tables.json.\n"
        f'\tLocaleTablesSHA256 = {go_string(tables_digest)}\n'
        ")\n",
        packed_set("localeLanguages", tables["languages"]),
        packed_set("localeScripts", tables["scripts"]),
        packed_set("localeRegions", tables["regions"]),
        packed_set("localeVariants", tables["variants"]),
        go_map(
            "localeExtlangPrefixes",
            "string",
            sorted(tables["extlangs"].items()),
        ),
        go_map(
            "localePreferredLanguages",
            "string",
            sorted(tables["preferredLanguages"].items()),
        ),
        go_map(
            "localePreferredScripts",
            "string",
            sorted(tables["preferredScripts"].items()),
        ),
        go_map(
            "localePreferredRegions",
            "string",
            sorted(tables["preferredRegions"].items()),
        ),
        go_map(
            "localePreferredVariants",
            "string",
            sorted(tables["preferredVariants"].items()),
        ),
        go_records("localeGrandfatheredRecords", tables["grandfathered"]),
        go_records("localeRedundantRecords", tables["redundant"]),
        packed_set("localeExtensionSingletons", tables["extensionSingletons"]),
        go_ranges(
            "localePrivateUseLanguageRanges",
            tables["privateUseRanges"]["language"],
        ),
        go_ranges(
            "localePrivateUseScriptRanges",
            tables["privateUseRanges"]["script"],
        ),
        go_ranges(
            "localePrivateUseRegionRanges",
            tables["privateUseRanges"]["region"],
        ),
    ]
    with open(GO_OUT, "w", encoding="utf-8", newline="\n") as handle:
        handle.write("\n".join(parts))
    subprocess.run(["gofmt", "-w", GO_OUT], check=True)


def main() -> None:
    language_bytes = read_verified(LANGUAGE_REGISTRY, LANGUAGE_REGISTRY_SHA256)
    extension_bytes = read_verified(EXTENSIONS_REGISTRY, EXTENSIONS_REGISTRY_SHA256)
    tables = build_tables(
        language_bytes.decode("utf-8"), extension_bytes.decode("utf-8")
    )
    tables_data = json_bytes(tables)
    tables_digest = sha256_bytes(tables_data)
    with open(TABLES_JSON, "wb") as handle:
        handle.write(tables_data)

    inventory = {
        "languages": len(tables["languages"]),
        "scripts": len(tables["scripts"]),
        "regions": len(tables["regions"]),
        "variants": len(tables["variants"]),
        "extlangs": len(tables["extlangs"]),
        "preferredLanguages": len(tables["preferredLanguages"]),
        "preferredScripts": len(tables["preferredScripts"]),
        "preferredRegions": len(tables["preferredRegions"]),
        "preferredVariants": len(tables["preferredVariants"]),
        "grandfathered": len(tables["grandfathered"]),
        "redundant": len(tables["redundant"]),
        "extensionSingletons": len(tables["extensionSingletons"]),
    }
    provenance = {
        "profile": PROFILE,
        "generatedBy": "pkg/walkthrough/testdata/locale/generate_locale_tables.py",
        "sources": {
            "languageSubtagRegistry": {
                "url": LANGUAGE_REGISTRY_URL,
                "fileDate": tables["languageRegistry"]["fileDate"],
                "sha256": LANGUAGE_REGISTRY_SHA256,
                "bytes": len(language_bytes),
                "retrieved": RETRIEVED,
            },
            "languageTagExtensionsRegistry": {
                "url": EXTENSIONS_REGISTRY_URL,
                "fileDate": tables["extensionsRegistry"]["fileDate"],
                "sha256": EXTENSIONS_REGISTRY_SHA256,
                "bytes": len(extension_bytes),
                "retrieved": RETRIEVED,
            },
        },
        "tablesFile": "locale-tables.json",
        "tablesSha256": tables_digest,
        "inventory": inventory,
    }
    with open(PROVENANCE_JSON, "wb") as handle:
        handle.write(json_bytes(provenance))

    write_go(tables, tables_digest)
    print(f"wrote {GO_OUT}")
    print(f"wrote {TABLES_JSON} ({tables_digest})")
    print(f"wrote {PROVENANCE_JSON}")
    print(json.dumps(inventory, sort_keys=True))


if __name__ == "__main__":
    main()
