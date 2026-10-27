#!/usr/bin/env python3
"""Author the independent training-package inventory corpus.

Every raw byte, expected typed projection and expected refusal below is
authored from
`sneat-co/backstage spec/features/project-walkthroughs/training-package-format.md`
and its declaration companion. This script never imports, runs or reads output
from the Go implementation, so the checked-in bytes stay an oracle the Go and
TypeScript stages consume rather than a recording of either.

Run: python3 pkg/walkthrough/testdata/package-corpus-v1/generate.py
"""

import copy
import hashlib
import json
import pathlib
import re
import shutil

ROOT = pathlib.Path(__file__).resolve().parent
CASES = ROOT / "cases"
INDEX_PATH = "training/package.json"

GIT_PIN = {"kind": "git", "commit": "0123456789abcdef0123456789abcdef01234567"}
DECLARATION_SHA = "aa" * 32
NARRATIVE_SHA = "bb" * 32
PROJECT = "project-alpha"


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def pin(data: bytes) -> dict:
    return {"sha256": sha256(data), "bytes": len(data)}


def dumps(value, indent=2) -> bytes:
    return (json.dumps(value, indent=indent, ensure_ascii=False) + "\n").encode("utf-8")


def compact(value) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True)


def asset(asset_id, kind, media_type, path, data, pinned=None):
    entry = {
        "id": asset_id,
        "kind": kind,
        "mediaType": media_type,
        "path": path,
    }
    entry.update(pinned if pinned is not None else pin(data))
    return entry


def base_index(package_id="training-case", revision=1):
    return {
        "schema": "datatug-training-package/1",
        "id": package_id,
        "revision": revision,
        "assets": [],
        "fictionalActors": [],
        "sources": [],
        "cases": [],
        "incidents": [],
        "checks": [],
        "queries": [],
        "challenges": [],
        "proposals": [],
        "presentationTargets": [],
    }


# ---------------------------------------------------------------------------
# Owner example: a nonempty locally closed zero-row recordset package.
# ---------------------------------------------------------------------------

EMPTY_RECORDS = b'{"columns":[{"name":"value","type":"integer"}],"rows":[]}\n'


def minimal():
    index = base_index("example-training", 1)
    index["assets"] = [
        asset("sample", "evidence-records", "application/json",
              "training/empty-records.json", EMPTY_RECORDS),
    ]
    index["fictionalActors"] = [{"id": "narrator", "name": "Fictional narrator"}]
    index["cases"] = [{
        "id": "example-case",
        "title": "Example Case",
        "status": "active",
        "context": {},
        "incidentIds": [],
        "evidenceIds": ["sample"],
        "findings": [],
        "timeline": [{
            "id": "introduced",
            "at": "1970-01-01T00:00:00Z",
            "actorId": "narrator",
            "type": "evidence-revealed",
            "ref": {"kind": "evidence", "id": "sample"},
        }],
    }]
    index["presentationTargets"] = [{"id": "case-panel", "kind": "case", "refId": "example-case"}]
    files = {"training/empty-records.json": EMPTY_RECORDS}
    return index, files


# ---------------------------------------------------------------------------
# Full nine-codec package.
# ---------------------------------------------------------------------------

TEXT_BODY = "Retry storm notes: requests repeat per status.\n"
RECORDS_BODY = dumps({
    "columns": [
        {"name": "order_id", "type": "integer"},
        {"name": "status", "type": "string"},
        {"name": "seen_at", "type": "datetime"},
        {"name": "note", "type": "null"},
    ],
    "rows": [[
        {"type": "integer", "value": "11045"},
        {"type": "string", "value": "pending"},
        {"type": "datetime", "value": "2026-01-02T03:04:05Z"},
        {"type": "null", "value": None},
    ]],
})
LOG_BODY = (
    '{"id":"line-1","at":"2026-01-02T03:04:05Z","actorId":"guide","kind":"order-seen","message":"First retry."}\n'
    '{"id":"line-2","at":"2026-01-02T03:05:05Z","actorId":"analyst","kind":"retry-seen","message":"Second retry."}\n'
).encode()
SQL_BODY = b"select status, count(*) from retries where attempts >= @attempts group by status;\n"

METADATA = {
    "id": "retries/by-status",
    "title": "Retries by status",
    "type": "SQL",
    "connectionId": "training-fixture",
    "parameters": [
        {"id": "status", "type": "string", "title": "Status", "isRequired": True, "defaultValue": "pending"},
        {"id": "attempts", "type": "integer", "defaultValue": 3},
        {"id": "ratio", "type": "number", "defaultValue": 2.5},
        {"id": "strict", "type": "boolean", "defaultValue": False},
        {"id": "optional", "type": "string"},
    ],
}

CHECK = {
    "id": "retry-observation",
    "title": "Retry observation",
    "description": "Fictional retry check.",
    "version": 1,
    "inputs": [
        {"name": "status", "type": "string", "cardinality": "scalar", "required": True},
        # A second input omits the optional native `required` field so the
        # complete projection can prove absence is not turned into false.
        {"name": "note", "type": "string", "cardinality": "scalar"},
    ],
    "steps": [{
        "id": "run",
        "kind": "query",
        "queryId": "retries/by-status",
        "validators": [{
            "id": "status-present",
            "scope": "row",
            "kind": "non-null",
            "severity": "error",
            "field": "status",
        }],
        "onUnavailable": "warn",
    }],
    "updatedAt": "2026-01-02T03:04:05Z",
    "updatedBy": "guide",
    "createdAt": "2026-01-01T00:00:00Z",
}

FIXTURE_BODY = b"SQLite format 3\x00fictional-retry-fixture-bytes"

MANIFEST = {
    "schemaVersion": 1,
    "gradingVersion": "challenge-grading-v1",
    "challengeId": "retry-query",
    "revision": 1,
    "title": "Retry query",
    "objective": "Find repeated requests by status.",
    "difficulty": "intro",
    "dialect": "sqlite",
    "allowedOperation": "read",
    "timing": "off",
    "engine": {
        "package": "@sqlite.org/sqlite-wasm",
        "version": "3.54.1-build1",
        "sqliteVersion": "3.54.1",
    },
    "fixtureSuite": {
        "id": "retry-suite",
        "fixtures": [{
            "id": "base",
            "label": "Base fixture",
            "sha256": sha256(FIXTURE_BODY),
            "bytes": len(FIXTURE_BODY),
            "runs": 1,
        }],
    },
    "visibleSchema": {
        "tables": [{
            "name": "retries",
            "columns": [
                {"name": "status", "type": "string", "primaryKey": True,
                 "notNull": True, "collation": "NOCASE"},
                {"name": "attempts", "type": "integer"},
            ],
        }],
    },
    "parts": [{
        "id": "result",
        "title": "Rows",
        "required": True,
        "answerKind": "sql",
        "sql": {
            "cases": [{
                "fixture": "base",
                "oracle": {
                    "kind": "result",
                    "columns": [{"name": "status", "type": "string"}],
                    "rows": [[{"type": "string", "value": "pending"}]],
                    "ordering": "unordered",
                },
            }],
        },
    }],
    "hints": [
        {"id": "retry-hint", "text": "Look at the attempts column."},
    ],
    "referenceSolutions": [{
        "partId": "result",
        "answer": {
            "sql": "select status, count(*) from retries where attempts >= 3 group by status;",
        },
    }],
}

WATCH = {
    "title": "Alert when retries appear",
    "checkId": "retry-observation",
    "checkVersion": "1",
    "inputs": {"status": {"type": "string", "value": "pending"}},
    "schedule": {"intervalSeconds": 300},
    "trigger": "becomes_true",
    "initialObservation": "baseline",
    "mode": "one_shot",
    "notificationPolicy": {"cooldownSeconds": 0, "recovery": False},
}


def full():
    metadata_body = dumps(METADATA)
    check_body = dumps(CHECK)
    manifest_body = dumps(MANIFEST)
    watch_body = dumps(WATCH)
    files = {
        "training/notes.txt": TEXT_BODY.encode(),
        "training/orders.json": RECORDS_BODY,
        "training/ops.ndjson": LOG_BODY,
        "training/retry.meta.json": metadata_body,
        "training/retry.sql": SQL_BODY,
        "training/retry-check.json": check_body,
        "training/challenge.json": manifest_body,
        "training/retry.sqlite": FIXTURE_BODY,
        "training/watch.json": watch_body,
    }
    index = base_index("training-full", 7)
    index["assets"] = [
        asset("narrative-text", "evidence-text", "text/plain", "training/notes.txt", files["training/notes.txt"]),
        asset("orders-records", "evidence-records", "application/json", "training/orders.json", files["training/orders.json"]),
        asset("ops-log", "evidence-log", "application/x-ndjson", "training/ops.ndjson", files["training/ops.ndjson"]),
        asset("retry-metadata", "query-metadata", "application/json", "training/retry.meta.json", files["training/retry.meta.json"]),
        asset("retry-body", "query-body-sql", "application/sql", "training/retry.sql", files["training/retry.sql"]),
        asset("retry-check", "check-definition", "application/json", "training/retry-check.json", files["training/retry-check.json"]),
        asset("retry-manifest", "challenge-manifest", "application/json", "training/challenge.json", files["training/challenge.json"]),
        asset("retry-fixture", "challenge-fixture", "application/vnd.sqlite3", "training/retry.sqlite", files["training/retry.sqlite"]),
        asset("retry-watch", "watch-recommendation", "application/json", "training/watch.json", files["training/watch.json"]),
    ]
    index["fictionalActors"] = [
        {"id": "guide", "name": "Guide"},
        {"id": "analyst", "name": "Fictional analyst 😀"},
    ]
    index["sources"] = [{"id": "training-fixture", "fixtureAssetId": "retry-fixture"}]
    index["cases"] = [{
        "id": "retry-case",
        "title": "Retry case",
        "status": "active",
        "context": {"status": {"type": "string", "value": "pending"}},
        "incidentIds": ["retry-incident"],
        "evidenceIds": ["narrative-text", "orders-records", "ops-log"],
        "findings": [
            {
                "id": "first-finding",
                "text": "Retries cluster by status.",
                "status": "hypothesis",
                "evidenceRefs": ["orders-records", "ops-log"],
            },
            {"id": "second-finding", "text": "Retry loop confirmed.", "status": "confirmed"},
        ],
        "timeline": [
            {"id": "log-seen", "at": "2026-01-02T03:04:05Z", "actorId": "guide",
             "type": "evidence-revealed", "ref": {"kind": "evidence", "id": "ops-log"}},
            {"id": "finding-seen", "at": "2026-01-02T03:04:06Z", "actorId": "analyst",
             "type": "finding-recorded", "ref": {"kind": "finding", "id": "first-finding"}},
            {"id": "check-seen", "at": "2026-01-02T03:04:07Z", "actorId": "guide",
             "type": "check-observed", "ref": {"kind": "check", "id": "retry-observation"}},
        ],
    }]
    index["incidents"] = [{
        "id": "retry-incident",
        "title": "Retry storm",
        "description": "Fictional retry storm.",
        "status": "investigating",
        "evidenceIds": ["ops-log"],
    }]
    index["checks"] = [{"id": "retry-observation", "definitionAssetId": "retry-check"}]
    index["queries"] = [{
        "id": "retries/by-status",
        "metadataAssetId": "retry-metadata",
        "bodyAssetId": "retry-body",
        "sourceId": "training-fixture",
    }]
    index["challenges"] = [{
        "id": "retry-query",
        "revision": 1,
        "manifestAssetId": "retry-manifest",
        "fixtures": [{"fixtureId": "base", "assetId": "retry-fixture"}],
    }]
    index["proposals"] = [
        {"id": "query-proposal", "kind": "query", "definitionId": "retries/by-status",
         "evidenceIds": ["narrative-text"], "rationale": "Keep the retry query."},
        {"id": "check-proposal", "kind": "check", "definitionId": "retry-observation",
         "evidenceIds": ["orders-records"], "rationale": "Keep the retry check."},
        {"id": "watch-proposal", "kind": "watch", "definitionId": "retry-watch",
         "evidenceIds": ["ops-log"], "rationale": "Watch the retry rate."},
    ]
    index["presentationTargets"] = [
        {"id": "case-view", "kind": "case", "refId": "retry-case"},
        {"id": "incident-view", "kind": "incident", "refId": "retry-incident"},
        {"id": "evidence-view", "kind": "evidence", "refId": "narrative-text"},
        {"id": "check-view", "kind": "check", "refId": "retry-observation"},
        {"id": "proposal-view", "kind": "proposal", "refId": "watch-proposal"},
    ]
    return index, files


def union_manifest():
    """One manifest that exercises every native answer/oracle/reference union."""
    fixture = b"SQLite format 3\x00fictional-union-fixture-bytes"
    expectation = {
        "id": "union-expectation",
        "scope": "set",
        "kind": "row-count",
        "severity": "error",
        "expected": {"type": "integer", "value": "1"},
    }
    manifest = {
        "schemaVersion": 1,
        "gradingVersion": "challenge-grading-v1",
        "challengeId": "union-challenge",
        "revision": 1,
        "title": "Union challenge",
        "objective": "Exercise every closed union.",
        "difficulty": "advanced",
        "dialect": "sqlite",
        "allowedOperation": "write",
        "timing": "optional",
        "engine": {
            "package": "@sqlite.org/sqlite-wasm",
            "version": "3.54.1-build1",
            "sqliteVersion": "3.54.1",
        },
        "fixtureSuite": {
            "id": "union-suite",
            "fixtures": [{
                "id": "base",
                "label": "Base fixture",
                "sha256": sha256(fixture),
                "bytes": len(fixture),
                "runs": 2,
            }],
        },
        "visibleSchema": {
            "tables": [{
                "name": "retries",
                "columns": [
                    {"name": "status", "type": "string", "primaryKey": True,
                     "notNull": True, "collation": "RTRIM"},
                    {"name": "attempts", "type": "integer"},
                ],
            }],
        },
        "parts": [
            {
                "id": "result",
                "title": "Rows",
                "required": True,
                "answerKind": "sql",
                "sql": {
                    "cases": [{
                        "fixture": "base",
                        "oracle": {
                            "kind": "result",
                            "columns": [{"name": "status", "type": "string"}],
                            "rows": [[{"type": "string", "value": "pending"}]],
                            "ordering": "unordered",
                            "requireColumnNames": True,
                            "numericTolerance": "0.01",
                        },
                    }],
                },
            },
            {
                "id": "pick",
                "title": "Pick",
                "required": True,
                "answerKind": "structured",
                "structured": {
                    "fields": [{
                        "id": "choice",
                        "label": "Choice",
                        "cardinality": "multi",
                        "required": True,
                        "minSelections": 1,
                        "maxSelections": 2,
                        "options": [
                            {"id": "a", "label": "A"},
                            {"id": "b", "label": "B"},
                        ],
                    }],
                    "expected": {"selections": {"choice": ["a"]}},
                },
            },
            {
                "id": "check",
                "title": "Check",
                "required": False,
                "answerKind": "check-definition",
                "checkDefinition": {
                    "queryIds": ["union/query"],
                    "expectation": expectation,
                    "cases": [{"fixture": "base", "expectedOutcome": "pass"}],
                },
            },
            {
                "id": "state",
                "title": "State",
                "required": False,
                "answerKind": "sql",
                "sql": {
                    "cases": [{
                        "fixture": "base",
                        "oracle": {
                            "kind": "final-state",
                            "tables": [{
                                "name": "retries",
                                "columns": [
                                    {"name": "status", "type": "string"},
                                    {"name": "attempts", "type": "integer"},
                                ],
                                "rows": [[
                                    {"type": "string", "value": "pending"},
                                    {"type": "integer", "value": "1"},
                                ]],
                            }],
                            "invariants": [{
                                "id": "keep-lowest",
                                "kind": "group-min",
                                "table": "retries",
                                "groupBy": ["status"],
                                "column": "attempts",
                                "description": "Keep the lowest attempt count.",
                            }],
                        },
                    }],
                },
            },
        ],
        "hints": [{"id": "union-hint", "text": "Every union is present."}],
        "referenceSolutions": [
            {"partId": "result", "answer": {"sql": "select status from retries;"}},
            {"partId": "pick", "answer": {"structured": {"selections": {"choice": ["a"]}}}},
            {"partId": "check", "answer": {
                "checkDefinition": {
                    "queryId": "union/query",
                    "expectation": expectation,
                },
            }},
            {"partId": "state", "answer": {"sql": "delete from retries;"}},
        ],
    }
    manifest_body = dumps(manifest)
    union_metadata = {
        "id": "union/query",
        "title": "Union query",
        "type": "SQL",
        "connectionId": "union-source",
        "parameters": [],
    }
    metadata_body = dumps(union_metadata)
    query_body = b"select status from retries;\n"
    files = {
        "training/union-challenge.json": manifest_body,
        "training/union.sqlite": fixture,
        "training/union.meta.json": metadata_body,
        "training/union.sql": query_body,
    }
    index = base_index("training-unions", 1)
    index["assets"] = [
        asset("union-manifest", "challenge-manifest", "application/json",
              "training/union-challenge.json", manifest_body),
        asset("union-fixture", "challenge-fixture", "application/vnd.sqlite3",
              "training/union.sqlite", fixture),
        asset("union-metadata", "query-metadata", "application/json",
              "training/union.meta.json", metadata_body),
        asset("union-body", "query-body-sql", "application/sql",
              "training/union.sql", query_body),
    ]
    # The manifest's check-definition part names a saved query the learner may
    # select, and its reference answer selects that same id, so the package has
    # to declare the query it closes against.
    index["sources"] = [{"id": "union-source", "fixtureAssetId": "union-fixture"}]
    index["queries"] = [{
        "id": "union/query",
        "metadataAssetId": "union-metadata",
        "bodyAssetId": "union-body",
        "sourceId": "union-source",
    }]
    index["challenges"] = [{
        "id": "union-challenge",
        "revision": 1,
        "manifestAssetId": "union-manifest",
        "fixtures": [{"fixtureId": "base", "assetId": "union-fixture"}],
    }]
    return index, files


# ---------------------------------------------------------------------------
# Expected projection mirror (authored from the owner tables).
# ---------------------------------------------------------------------------

NATIVE_TIMESTAMP = re.compile(
    r"^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$")


def native_timestamp(text):
    """Render an RFC3339 instant the way the native time value encodes it.

    The native Check stores `updatedAt` as a time value whose JSON encoding is
    RFC3339 with a fractional second only when it is non-zero and without
    trailing fractional zeros; a zero numeric offset encodes as `Z`. Authored
    optional strings such as `createdAt` are projected verbatim.
    """
    match = NATIVE_TIMESTAMP.match(text)
    if match is None:
        raise ValueError(f"not an RFC3339 timestamp: {text}")
    seconds, fraction, offset = match.groups()
    fraction = (fraction or "").rstrip("0")
    if offset in ("+00:00", "-00:00"):
        offset = "Z"
    return seconds + (f".{fraction}" if fraction else "") + offset


def p_tv(value):
    kind = value["type"]
    return {"type": kind, "value": value["value"]}


def p_input(value):
    kind = value["type"]
    if kind == "set":
        return {"set": [p_tv(item) for item in value["values"]]}
    if kind == "range":
        return {"range": {"start": p_tv(value["start"]), "end": p_tv(value["end"])}}
    return {"scalar": p_tv(value)}


def p_parameter(parameter):
    projected = {"id": parameter["id"], "type": parameter["type"]}
    if "title" in parameter:
        projected["title"] = parameter["title"]
    if parameter.get("isRequired"):
        projected["isRequired"] = True
    if "defaultValue" in parameter:
        default = parameter["defaultValue"]
        kind = parameter["type"]
        projected["defaultValue"] = {
            "type": kind,
            "value": str(default) if kind == "integer" else default,
        }
    return projected


def p_metadata(metadata):
    return {
        "id": metadata["id"],
        "title": metadata["title"],
        "type": metadata["type"],
        "connectionId": metadata["connectionId"],
        "parameters": [p_parameter(item) for item in metadata["parameters"]],
    }


def p_binding(binding):
    projected = {"input": binding["input"]}
    for key in ("fromInput", "part", "fromColumn"):
        if key in binding:
            projected[key] = binding[key]
    return projected


def p_validator(validator):
    projected = {"id": validator["id"]}
    for key in ("validatorId", "scope", "kind", "severity", "field", "otherField",
                "checkId", "onUnavailable"):
        if key in validator:
            projected[key] = validator[key]
    if "expected" in validator:
        projected["expected"] = p_tv(validator["expected"])
    if validator.get("bindings"):
        projected["bindings"] = [p_binding(item) for item in validator["bindings"]]
    return projected


def p_step(step):
    projected = {"id": step["id"], "kind": step["kind"]}
    for key in ("queryId", "checkId", "onUnavailable"):
        if key in step:
            projected[key] = step[key]
    if step.get("bindings"):
        projected["bindings"] = [p_binding(item) for item in step["bindings"]]
    if step.get("validators"):
        projected["validators"] = [p_validator(item) for item in step["validators"]]
    return projected


def p_check(check):
    inputs = []
    for item in check["inputs"]:
        projected_input = {
            "name": item["name"],
            "type": item["type"],
            "cardinality": item["cardinality"],
        }
        # `required` is a native omitempty boolean: only a true value is
        # projected, so an omitted optional field never becomes false.
        if item.get("required"):
            projected_input["required"] = True
        inputs.append(projected_input)
    projected = {
        "id": check["id"],
        "title": check["title"],
        "version": str(check["version"]),
        "inputs": inputs,
        "steps": [p_step(item) for item in check["steps"]],
        "updatedAt": native_timestamp(check["updatedAt"]),
        "updatedBy": check["updatedBy"],
    }
    if "description" in check:
        projected["description"] = check["description"]
    if "createdAt" in check:
        projected["createdAt"] = check["createdAt"]
    return projected


def p_manifest(manifest):
    engine = manifest["engine"]
    fixtures = [
        {
            "id": item["id"],
            "label": item["label"],
            "sha256": item["sha256"],
            "bytes": item["bytes"],
            "runs": item["runs"],
        }
        for item in manifest["fixtureSuite"]["fixtures"]
    ]
    tables = []
    for table in manifest["visibleSchema"]["tables"]:
        columns = []
        for column in table["columns"]:
            projected_column = {"name": column["name"], "type": column["type"]}
            if column.get("primaryKey"):
                projected_column["primaryKey"] = True
            if column.get("notNull"):
                projected_column["notNull"] = True
            if column.get("collation"):
                projected_column["collation"] = column["collation"]
            columns.append(projected_column)
        tables.append({"name": table["name"], "columns": columns})
    return {
        "schemaVersion": manifest["schemaVersion"],
        "gradingVersion": manifest["gradingVersion"],
        "challengeId": manifest["challengeId"],
        "revision": manifest["revision"],
        "title": manifest["title"],
        "objective": manifest["objective"],
        "difficulty": manifest["difficulty"],
        "dialect": manifest["dialect"],
        "allowedOperation": manifest["allowedOperation"],
        "timing": manifest["timing"],
        "engine": {
            "package": engine["package"],
            "version": engine["version"],
            "sqliteVersion": engine["sqliteVersion"],
        },
        "fixtureSuite": {"id": manifest["fixtureSuite"]["id"], "fixtures": fixtures},
        "visibleSchema": {"tables": tables},
        "parts": [p_manifest_part(item) for item in manifest["parts"]],
        "hints": [{"id": item["id"], "text": item["text"]} for item in manifest["hints"]],
        "referenceSolutions": [
            p_reference_solution(item) for item in manifest["referenceSolutions"]
        ],
    }


def p_manifest_part(part):
    projected = {
        "id": part["id"],
        "title": part["title"],
        "required": part["required"],
        "answerKind": part["answerKind"],
    }
    if "sql" in part:
        projected["sql"] = {
            "cases": [
                {"fixture": item["fixture"], "oracle": p_oracle(item["oracle"])}
                for item in part["sql"]["cases"]
            ],
        }
    if "structured" in part:
        projected["structured"] = p_structured_part(part["structured"])
    if "checkDefinition" in part:
        projected["checkDefinition"] = p_check_definition_part(part["checkDefinition"])
    return projected


def p_oracle(oracle):
    if oracle["kind"] == "result":
        projected = {
            "kind": "result",
            "columns": [
                {"name": item["name"], "type": item["type"]}
                for item in oracle["columns"]
            ],
            "rows": [[p_tv(cell) for cell in row] for row in oracle["rows"]],
            "ordering": oracle["ordering"],
        }
        if oracle.get("requireColumnNames"):
            projected["requireColumnNames"] = True
        if oracle.get("numericTolerance"):
            projected["numericTolerance"] = oracle["numericTolerance"]
        return projected
    if oracle["kind"] == "final-state":
        projected = {
            "kind": "final-state",
            "tables": [
                {
                    "name": table["name"],
                    "columns": [
                        {"name": item["name"], "type": item["type"]}
                        for item in table["columns"]
                    ],
                    "rows": [[p_tv(cell) for cell in row] for row in table["rows"]],
                }
                for table in oracle["tables"]
            ],
        }
        if oracle.get("invariants"):
            projected["invariants"] = [
                {
                    "id": item["id"],
                    "kind": item["kind"],
                    "table": item["table"],
                    "groupBy": list(item["groupBy"]),
                    "column": item["column"],
                    "description": item["description"],
                }
                for item in oracle["invariants"]
            ]
        return projected
    return {"kind": oracle["kind"]}


def p_structured_part(part):
    fields = []
    for field in part["fields"]:
        projected = {
            "id": field["id"],
            "label": field["label"],
            "cardinality": field["cardinality"],
            "required": field["required"],
            "options": [{"id": item["id"], "label": item["label"]}
                        for item in field["options"]],
        }
        if field.get("minSelections"):
            projected["minSelections"] = field["minSelections"]
        if field.get("maxSelections"):
            projected["maxSelections"] = field["maxSelections"]
        fields.append(projected)
    return {
        "fields": fields,
        "expected": {"selections": dict(part["expected"]["selections"])},
    }


def p_check_definition_part(part):
    return {
        "queryIds": list(part["queryIds"]),
        "expectation": p_validator(part["expectation"]),
        "cases": [
            {"fixture": item["fixture"], "expectedOutcome": item["expectedOutcome"]}
            for item in part["cases"]
        ],
    }


def p_reference_solution(solution):
    answer = {}
    if solution["answer"].get("sql"):
        answer["sql"] = solution["answer"]["sql"]
    if "structured" in solution["answer"]:
        answer["structured"] = {
            "selections": dict(solution["answer"]["structured"]["selections"]),
        }
    if "checkDefinition" in solution["answer"]:
        answer["checkDefinition"] = {
            "queryId": solution["answer"]["checkDefinition"]["queryId"],
            "expectation": p_validator(
                solution["answer"]["checkDefinition"]["expectation"]),
        }
    return {"partId": solution["partId"], "answer": answer}


def p_watch(watch):
    return {
        "title": watch["title"],
        "checkId": watch["checkId"],
        "checkVersion": watch["checkVersion"],
        "inputs": {name: p_input(value) for name, value in watch["inputs"].items()},
        "intervalSeconds": str(watch["schedule"]["intervalSeconds"]),
        "trigger": watch["trigger"],
        "initialObservation": watch["initialObservation"],
        "mode": watch["mode"],
        "cooldownSeconds": str(watch["notificationPolicy"]["cooldownSeconds"]),
        "recovery": watch["notificationPolicy"]["recovery"],
    }


def p_records(body):
    records = json.loads(body)
    return {
        "columns": [{"name": item["name"], "type": item["type"]} for item in records["columns"]],
        "rows": [[p_tv(cell) for cell in row] for row in records["rows"]],
    }


def p_log(body):
    entries = []
    for line in body.decode().splitlines():
        if line:
            entries.append(json.loads(line))
    return [
        {"id": item["id"], "at": item["at"], "actorId": item["actorId"],
         "kind": item["kind"], "message": item["message"]}
        for item in entries
    ]


def codec_map_for(index, files, metadata=None, check=None, manifest=None, watch=None, records_body=None):
    codecs = {}
    for entry in index["assets"]:
        kind = entry["kind"]
        aid = entry["id"]
        body = files.get(entry["path"], b"")
        if kind in ("evidence-text", "query-body-sql"):
            codecs[aid] = {"assetId": aid, "kind": kind, "text": body.decode()}
        elif kind == "evidence-records":
            codecs[aid] = {"assetId": aid, "kind": kind,
                           "records": p_records(records_body if records_body is not None else body)}
        elif kind == "evidence-log":
            codecs[aid] = {"assetId": aid, "kind": kind, "log": p_log(body)}
        elif kind == "query-metadata":
            codecs[aid] = {"assetId": aid, "kind": kind,
                           "metadata": p_metadata(metadata if metadata is not None else json.loads(body))}
        elif kind == "check-definition":
            codecs[aid] = {"assetId": aid, "kind": kind,
                           "check": p_check(check if check is not None else json.loads(body))}
        elif kind == "challenge-manifest":
            codecs[aid] = {"assetId": aid, "kind": kind,
                           "manifest": p_manifest(manifest if manifest is not None else json.loads(body))}
        elif kind == "watch-recommendation":
            codecs[aid] = {"assetId": aid, "kind": kind,
                           "watch": p_watch(watch if watch is not None else json.loads(body))}
        elif kind == "challenge-fixture":
            codecs[aid] = {"assetId": aid, "kind": kind, "binary": True}
    return codecs


def contribution_class(kind):
    return "binary" if kind == "challenge-fixture" else "text"


def expected_projection(index, files, ref, binding, codecs):
    assets = []
    contributions = []
    seen = {}
    for entry in index["assets"]:
        assets.append({
            "id": entry["id"], "kind": entry["kind"], "mediaType": entry["mediaType"],
            "path": entry["path"], "sha256": entry["sha256"], "bytes": str(entry["bytes"]),
        })
        path = entry["path"]
        if path in seen:
            seen[path]["assetIds"].append(entry["id"])
            seen[path]["classes"].add(contribution_class(entry["kind"]))
            continue
        seen[path] = {
            "assetIds": [entry["id"]],
            "classes": {contribution_class(entry["kind"])},
        }
        contributions.append({
            "path": path, "sha256": entry["sha256"], "class": None,
            "bytes": str(entry["bytes"]), "assetIds": seen[path]["assetIds"],
        })
    # One physical source+path entry is summed once per logical subset it
    # belongs to. The owner keeps the text and binary quotas separate, so a
    # mixed entry occupies one buffer and charges its bytes to both subsets.
    text_bytes = 0
    binary_bytes = 0
    for contribution in contributions:
        classes = seen[contribution["path"]]["classes"]
        contribution["class"] = "+".join(
            name for name in ("text", "binary") if name in classes)
        if "text" in classes:
            text_bytes += int(contribution["bytes"])
        if "binary" in classes:
            binary_bytes += int(contribution["bytes"])
    codec_list = [codecs[entry["id"]] for entry in index["assets"]]
    cases = []
    for case in index["cases"]:
        findings = []
        for finding in case["findings"]:
            item = {"id": finding["id"], "text": finding["text"], "status": finding["status"]}
            if "evidenceRefs" in finding:
                item["evidenceRefs"] = list(finding["evidenceRefs"])
            findings.append(item)
        cases.append({
            "id": case["id"], "title": case["title"], "status": case["status"],
            "context": {name: p_input(value) for name, value in case["context"].items()},
            "incidentIds": list(case["incidentIds"]),
            "evidenceIds": list(case["evidenceIds"]),
            "findings": findings,
            "timeline": [
                {"id": item["id"], "at": item["at"], "actorId": item["actorId"],
                 "type": item["type"], "ref": {"kind": item["ref"]["kind"], "id": item["ref"]["id"]}}
                for item in case["timeline"]
            ],
        })
    asset_by_id = {entry["id"]: entry for entry in index["assets"]}
    queries = []
    for query in index["queries"]:
        metadata = codecs[query["metadataAssetId"]]["metadata"]
        body = codecs[query["bodyAssetId"]]["text"]
        parameters = []
        for parameter in metadata["parameters"]:
            item = {"id": parameter["id"], "type": parameter["type"]}
            if "title" in parameter:
                item["title"] = parameter["title"]
            if parameter.get("isRequired"):
                item["isRequired"] = True
            if "defaultValue" in parameter:
                value = parameter["defaultValue"]
                item["defaultValue"] = (
                    value["value"] if isinstance(value, dict) else value
                )
            parameters.append(item)
        queries.append({
            "id": query["id"], "metadataAssetId": query["metadataAssetId"],
            "bodyAssetId": query["bodyAssetId"], "sourceId": query["sourceId"],
            "hydrated": {
                "id": metadata["id"], "type": metadata["type"], "text": body,
                "connectionId": metadata["connectionId"], "parameters": parameters,
            },
        })
    challenges = []
    for challenge in index["challenges"]:
        fixtures = []
        for fixture in challenge["fixtures"]:
            entry = asset_by_id[fixture["assetId"]]
            fixtures.append({
                "fixtureId": fixture["fixtureId"], "assetId": fixture["assetId"],
                "path": entry["path"], "sha256": entry["sha256"], "bytes": str(entry["bytes"]),
            })
        challenges.append({
            "id": challenge["id"], "revision": str(challenge["revision"]),
            "manifestAssetId": challenge["manifestAssetId"], "fixtures": fixtures,
        })
    return {
        "schema": index["schema"],
        "id": index["id"],
        "revision": str(index["revision"]),
        "package": {
            "id": ref["id"], "revision": ref["revision"], "path": ref["path"],
            "sha256": ref["sha256"], "bytes": ref["bytes"],
        },
        "binding": {
            "contentPin": {"kind": binding["contentPin"]["kind"],
                           "commit": binding["contentPin"]["commit"]},
            "declarationSha256": binding["declarationSha256"],
            "narrativeSha256": binding["narrativeSha256"],
            "packageSha256": binding["packageSha256"],
            "scenarioId": binding["scenarioId"],
            "scenarioRevision": binding["scenarioRevision"],
        },
        "assets": assets,
        "codecs": codec_list,
        "fictionalActors": [{"id": item["id"], "name": item["name"]} for item in index["fictionalActors"]],
        "sources": [{"id": item["id"], "fixtureAssetId": item["fixtureAssetId"]} for item in index["sources"]],
        "cases": cases,
        "incidents": [
            {"id": item["id"], "title": item["title"], "description": item["description"],
             "status": item["status"], "evidenceIds": list(item["evidenceIds"])}
            for item in index["incidents"]
        ],
        "checks": [{"id": item["id"], "definitionAssetId": item["definitionAssetId"]}
                   for item in index["checks"]],
        "queries": queries,
        "challenges": challenges,
        "proposals": [
            {"id": item["id"], "kind": item["kind"], "definitionId": item["definitionId"],
             "evidenceIds": list(item["evidenceIds"]), "rationale": item["rationale"]}
            for item in index["proposals"]
        ],
        "presentationTargets": [
            {"id": item["id"], "kind": item["kind"], "refId": item["refId"]}
            for item in index["presentationTargets"]
        ],
        "textBytes": str(text_bytes),
        "binaryBytes": str(binary_bytes),
        "contributions": contributions,
    }


# ---------------------------------------------------------------------------
# Emission.
# ---------------------------------------------------------------------------

VECTORS = []
PROVENANCE = {}
PROVENANCE_BYTES = {}


def emit(case_id, description, index, files, want, *, raw_index=None,
         ref_id=None, ref_revision=None, binding=None, index_path=INDEX_PATH):
    case_dir = CASES / case_id
    if case_dir.exists():
        shutil.rmtree(case_dir)
    index_bytes = raw_index if raw_index is not None else dumps(index)
    case_dir.mkdir(parents=True)
    for relative, data in files.items():
        target = case_dir / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
        PROVENANCE[f"cases/{case_id}/{relative}"] = sha256(data)
        PROVENANCE_BYTES[f"cases/{case_id}/{relative}"] = len(data)
    (case_dir / index_path).parent.mkdir(parents=True, exist_ok=True)
    (case_dir / index_path).write_bytes(index_bytes)
    PROVENANCE[f"cases/{case_id}/{index_path}"] = sha256(index_bytes)
    PROVENANCE_BYTES[f"cases/{case_id}/{index_path}"] = len(index_bytes)
    package_id = ref_id if ref_id is not None else index["id"]
    revision = str(ref_revision if ref_revision is not None else index["revision"])
    reference = {
        "id": package_id,
        "revision": revision,
        "path": index_path,
        "sha256": sha256(index_bytes),
        "bytes": str(len(index_bytes)),
    }
    effective_binding = {
        "project": PROJECT,
        "contentPin": dict(GIT_PIN),
        "declarationSha256": DECLARATION_SHA,
        "narrativeSha256": NARRATIVE_SHA,
        "packageSha256": reference["sha256"],
        "scenarioId": reference["id"],
        "scenarioRevision": reference["revision"],
    }
    if binding:
        effective_binding.update(binding)
    VECTORS.append({
        "id": case_id,
        "description": description,
        "directory": case_id,
        "indexPath": index_path,
        "reference": reference,
        "binding": effective_binding,
        "want": want,
    })
    return reference, effective_binding


def refused(code, path):
    return {"admitted": False, "issues": [{"code": code, "path": path}]}


def admitted_definition(index, files, ref, binding, **codec_kwargs):
    codecs = codec_map_for(index, files, **codec_kwargs)
    return {"admitted": True, "definition": expected_projection(index, files, ref, binding, codecs)}


def admitted_digest(index, files, ref, binding, **codec_kwargs):
    codecs = codec_map_for(index, files, **codec_kwargs)
    projection = expected_projection(index, files, ref, binding, codecs)
    return {"admitted": True, "definitionSha256": sha256(compact(projection).encode())}


def mutate(index, *mutators):
    index = copy.deepcopy(index)
    for mutator in mutators:
        mutator(index)
    return index


def advance_bytes(index, asset_id, new_bytes):
    entry = next(item for item in index["assets"] if item["id"] == asset_id)
    entry.update(pin(new_bytes))
    return entry


# ---------------------------------------------------------------------------
# Cases.
# ---------------------------------------------------------------------------

def full_negative(case_id, description, mutator=None, code=None, path=None,
                  edit=None, binding=None, files=None):
    index, case_files = full()
    if files is not None:
        case_files = dict(files)
    if mutator is not None:
        mutator(index)
    if edit is not None:
        edit(index, case_files)
    emit(case_id, description, index, case_files, refused(code, path), binding=binding)


def minimal_negative(case_id, description, mutator, code, path, files=None):
    index, case_files = minimal()
    mutator(index)
    emit(case_id, description, index, case_files if files is None else files,
         refused(code, path))


def empty_index(package_id="training-empty", revision=1):
    return base_index(package_id, revision)


def main():
    # --- Positive: owner example and all nine codecs ---------------------
    index, files = minimal()
    ref, binding = emit("minimal", "The owner's structurally valid zero-row package.",
                        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    index, files = full()
    emitted_ref, emitted_binding = emit(
        "full", "All nine owner codecs in one closed package.", index, files,
        {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, emitted_ref, emitted_binding)

    index, files = union_manifest()
    ref, binding = emit(
        "manifest-unions",
        "A manifest projection keeps every native answer, oracle and reference union.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    # --- Positive: aliases and different paths ---------------------------
    index, files = minimal()
    alias_body = b"alias text\n"
    files["training/a.txt"] = alias_body
    files["training/b.txt"] = alias_body
    index["assets"].extend([
        asset("alias-one", "evidence-text", "text/plain", "training/a.txt", alias_body),
        asset("alias-two", "evidence-text", "text/plain", "training/a.txt", alias_body),
        asset("different-path", "evidence-text", "text/plain", "training/b.txt", alias_body),
    ])
    ref, binding = emit("aliases", "Identical source+path aliases and different paths.", index, files,
                        {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    # A same-source+path alias may use a different text codec. Both aliases must
    # independently validate; the unique raw buffer is counted once.
    index, files = minimal()
    shared_sql = b"select status from retries;\n"
    files["training/shared.sql"] = shared_sql
    index["assets"].extend([
        asset("shared-body", "query-body-sql", "application/sql",
              "training/shared.sql", shared_sql),
        asset("shared-text", "evidence-text", "text/plain",
              "training/shared.sql", shared_sql),
    ])
    ref, binding = emit(
        "alias-cross-codec-text",
        "One SQL buffer is valid as both query-body-sql and evidence-text.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    # --- Positive: count and size boundaries -----------------------------
    index = empty_index()
    files = {"training/one.txt": b"x"}
    for number in range(1000):
        index["assets"].append(asset(f"a{number:04d}", "evidence-text", "text/plain",
                                     "training/one.txt", files["training/one.txt"]))
    ref, binding = emit("boundary-assets-1000", "One thousand asset declarations alias one path.",
                        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_digest(index, files, ref, binding)

    index, files = minimal()
    for number in range(999):
        index["presentationTargets"].append(
            {"id": f"target-{number:04d}", "kind": "case", "refId": "example-case"})
    ref, binding = emit("boundary-targets-1000", "One thousand presentation targets.",
                        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_digest(index, files, ref, binding)

    index, files = minimal()
    index["cases"][0]["context"]["padding"] = {"type": "string", "value": ""}
    base_length = len(dumps(index))
    padding = 1048576 - base_length
    index["cases"][0]["context"]["padding"] = {"type": "string", "value": "p" * padding}
    raw = dumps(index)
    assert len(raw) == 1048576, len(raw)
    ref, binding = emit("boundary-index-1mib", "A valid index at exactly 1 MiB.",
                        index, files, {"admitted": True, "definition": {}}, raw_index=raw)
    VECTORS[-1]["want"] = admitted_digest(index, files, ref, binding)

    index = empty_index()
    body = b"a" * 65536
    files = {"training/exactly-64kib.txt": body}
    index["assets"] = [asset("large-text", "evidence-text", "text/plain",
                             "training/exactly-64kib.txt", body)]
    ref, binding = emit("boundary-text-64kib", "One text asset at exactly 64 KiB.",
                        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_digest(index, files, ref, binding)

    # --- Negatives: index and root shape --------------------------------
    index, files = full()
    index["schema"] = "datatug-training-package/2"
    emit("schema-newer", "A later schema major is a recoverable newer-client boundary.",
         index, files, {"admitted": False, "newerClient": True, "issues": []})

    index, files = full()
    index["schema"] = "datatug-training-package/0"
    negative = refused("invalid_value", "/schema")
    emit("schema-unknown", "A non-current schema major refuses.", index, files, negative)

    index, files = full()
    index["schema"] = "datatug-training-package/one"
    emit("schema-invalid", "A malformed schema spelling refuses.", index, files,
         refused("invalid_value", "/schema"))

    index, files = minimal()
    index["cases"][0]["context"]["padding"] = {"type": "string", "value": ""}
    base_length = len(dumps(index))
    padding = 1048576 + 1 - base_length
    index["cases"][0]["context"]["padding"] = {"type": "string", "value": "p" * padding}
    raw = dumps(index)
    assert len(raw) == 1048576 + 1, len(raw)
    emit("index-over-1mib", "One byte over the index bound.", index, files,
         refused("too_large", ""), raw_index=raw)

    index, files = full()
    raw = dumps(index).replace(b'"id": "training-full"', b'"id": "training-full", "id": "training-full"', 1)
    emit("duplicate-key", "A decoded duplicate member name rejects.", index, files,
         refused("duplicate_key", "/id"), raw_index=raw)

    index, files = full()
    emit("trailing-data", "Trailing bytes after the index object reject.", index, files,
         refused("trailing_data", ""), raw_index=dumps(index) + b"{}")

    index, files = full()
    emit("root-array", "A non-object index root rejects.", index, files,
         refused("not_object", ""), raw_index=b"[]")

    index, files = full()
    raw = b'{"schema":"\xff"}'
    emit("malformed-utf8", "Invalid UTF-8 index bytes reject.", index, files,
         refused("invalid_utf8", ""), raw_index=raw, ref_id="training-full", ref_revision="7")

    index, files = full()
    index["extra"] = True
    emit("unknown-root-field", "Unknown root fields reject.", index, files,
         refused("unknown_field", "/extra"))

    index, files = full()
    index["assets"] = None
    emit("null-assets", "Explicit null where an array is required rejects.", index, files,
         refused("invalid_type", "/assets"))

    index, files = full()
    index["assets"][0]["kind"] = "evidence-video"
    emit("unknown-kind", "An unreviewed codec kind rejects.", index, files,
         refused("invalid_value", "/assets/0/kind"))

    index, files = full()
    index["assets"][0]["mediaType"] = "application/json"
    emit("media-mismatch", "An author mediaType may not relabel a text codec.", index, files,
         refused("invalid_value", "/assets/0/mediaType"))

    index, files = full()
    index["assets"][0]["path"] = INDEX_PATH
    emit("self-reference", "The index is not one of its own assets.", index, files,
         refused("invalid_value", "/assets/0/path"))

    index, files = full()
    conflict = copy.deepcopy(index["assets"][0])
    conflict["id"] = "narrative-alias-conflict"
    conflict["sha256"] = "cc" * 32
    conflict["bytes"] = 5
    index["assets"].append(conflict)
    emit("alias-conflict", "Conflicting pins at one canonical source+path reject.", index, files,
         refused("mismatch", "/assets/9/sha256"))

    index, files = full()
    length_conflict = copy.deepcopy(index["assets"][0])
    length_conflict["id"] = "narrative-alias-length-conflict"
    length_conflict["bytes"] = 5
    index["assets"].append(length_conflict)
    emit("alias-length-conflict",
         "A conflicting byte length at one canonical source+path rejects.",
         index, files, refused("mismatch", "/assets/9/bytes"))

    index, files = full()
    mixed = copy.deepcopy(index["assets"][0])
    mixed["id"] = "narrative-mixed-kind"
    mixed["kind"] = "evidence-records"
    mixed["mediaType"] = "application/json"
    index["assets"].append(mixed)
    emit("alias-mixed-kind", "A different text codec still has to parse the shared body.",
         index, files, refused("invalid_json", "/assets/9"))

    # A same-source+path alias may also pair a text codec with the fixture
    # codec. The owner only requires identical digest and length at one
    # source+path and validates each alias independently; the unique physical
    # buffer then belongs to both logical subsets without being counted twice in
    # either one.
    index, files = full()
    shared_text = files["training/notes.txt"]
    index["assets"].append(asset("narrative-text-fixture", "challenge-fixture",
                                 "application/vnd.sqlite3", "training/notes.txt", shared_text))
    ref, binding = emit(
        "alias-mixed-class",
        "One text buffer is valid as both evidence text and a challenge fixture.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    # The same pair in the opposite declaration order must produce the same
    # admission, the same physical contribution and the same logical totals.
    index, files = full()
    shared_text = files["training/notes.txt"]
    index["assets"].insert(0, asset("narrative-text-fixture", "challenge-fixture",
                                    "application/vnd.sqlite3", "training/notes.txt", shared_text))
    ref, binding = emit(
        "alias-mixed-class-fixture-first",
        "The mixed text/fixture alias admits in either declaration order.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    # A mixed entry obeys every applicable logical rule: the fixture alias is
    # inside the 10 MiB fixture bound, but the text alias over the same buffer is
    # over the 64 KiB text bound and refuses at the text alias.
    index = empty_index()
    mixed_body = b"a" * 65537
    index["assets"] = [
        asset("mixed-fixture", "challenge-fixture", "application/vnd.sqlite3",
              "training/mixed.sqlite", mixed_body),
        asset("mixed-text", "evidence-text", "text/plain",
              "training/mixed.sqlite", mixed_body),
    ]
    emit("alias-mixed-text-over-64kib",
         "A text alias over a fixture-sized buffer still obeys the 64 KiB text bound.",
         index, {}, refused("too_long", "/assets/1/bytes"))

    # A physical union above 64 MiB is not itself a refusal: the owner caps the
    # unique binary subset and the text subset separately. This vector is
    # preflight accounting only; the declared fixture bytes are not supplied.
    index = empty_index()
    small_text = b"x"
    files = {"training/small.txt": small_text}
    index["assets"] = [asset("union-text", "evidence-text", "text/plain",
                             "training/small.txt", small_text)]
    for number in range(64):
        path = f"training/union-fixture-{number:02d}.sqlite"
        index["assets"].append(asset(
            f"union-fixture-{number:02d}", "challenge-fixture",
            "application/vnd.sqlite3", path, b"",
            pinned={"sha256": sha256(path.encode()), "bytes": 1048576}))
    emit("mixed-union-over-64mib",
         "A full 64 MiB binary subset plus separately allowed text passes preflight; "
         "the physical union alone never refuses (preflight accounting only).",
         index, files, refused("missing_asset", "/assets"))

    index, files = full()
    index["assets"].append(copy.deepcopy(index["assets"][0]))
    emit("duplicate-asset-id", "Asset ids are unique.", index, files,
         refused("duplicate_id", "/assets/9"))

    index, files = full()
    del files["training/notes.txt"]
    emit("missing-asset", "A declared child without supplied bytes refuses.", index, files,
         refused("missing_asset", "/assets"))

    index, files = full()
    files["training/extra.txt"] = b"extra\n"
    emit("extra-asset", "Supplied bytes for an undeclared path refuse.", index, files,
         refused("mismatch", "/assets"))

    index, files = full()
    tampered = b"X" + TEXT_BODY.encode()[1:]
    files["training/notes.txt"] = tampered
    emit("digest-tamper", "Child bytes are verified before any codec parse.", index, files,
         refused("digest_mismatch", "/assets"))

    index, files = full()
    files["training/notes.txt"] = b"short\n"
    emit("length-tamper", "A child byte length must equal its pin.", index, files,
         refused("byte_count_mismatch", "/assets"))

    # --- Negatives: external binding ------------------------------------
    full_negative("binding-package-digest", "The bound package digest must equal the pin.",
                  code="mismatch", path="/binding/packageSha256",
                  binding={"packageSha256": "cc" * 32})
    full_negative("binding-scenario-id", "The package id must equal the admitted scenario id.",
                  code="mismatch", path="/binding/scenarioId",
                  binding={"scenarioId": "other-scenario"})
    full_negative("binding-scenario-revision", "The package revision must equal the admitted scenario revision.",
                  code="mismatch", path="/binding/scenarioRevision",
                  binding={"scenarioRevision": "8"})
    full_negative("binding-declaration-digest", "The declaration digest must be 64 lowercase hex.",
                  code="digest_mismatch", path="/binding/declarationSha256",
                  binding={"declarationSha256": "xyz"})
    full_negative("binding-missing-project", "A missing resolved project identity refuses.",
                  code="missing_context", path="/binding/project",
                  binding={"project": ""})
    full_negative("binding-pin", "A content pin must be one exact immutable pin.",
                  code="pin_mismatch", path="/binding/contentPin",
                  binding={"contentPin": {"kind": "branch"}})

    # --- Negatives: Case, finding and timeline --------------------------
    full_negative("title-121", "A Case title over 120 code points rejects.",
                  lambda i: i["cases"][0].__setitem__("title", "a" * 121),
                  code="too_long", path="/cases/0/title")
    full_negative("case-work-item", "A Case projection may not fabricate Work identity.",
                  lambda i: i["cases"][0].__setitem__("workItemId", "work-1"),
                  code="unknown_field", path="/cases/0/workItemId")
    full_negative("duplicate-case-id", "Case ids are unique.",
                  lambda i: i["cases"].append(copy.deepcopy(i["cases"][0])),
                  code="duplicate_id", path="/cases/1")
    full_negative("case-status", "Case status is the native closed set.",
                  lambda i: i["cases"][0].__setitem__("status", "new"),
                  code="invalid_value", path="/cases/0/status")
    full_negative("context-name", "Case context names are native parameter names.",
                  lambda i: i["cases"][0]["context"].__setitem__("bad name", {"type": "string", "value": "x"}),
                  code="invalid_value", path="/cases/0/context/bad name")
    full_negative("context-value", "Case context values are native InputValues.",
                  lambda i: i["cases"][0]["context"].__setitem__("status", {"type": "string", "value": 3}),
                  code="invalid_type", path="/cases/0/context/status/value")
    full_negative("case-incident-duplicate", "Case incident references are unique.",
                  lambda i: i["cases"][0].__setitem__("incidentIds", ["retry-incident", "retry-incident"]),
                  code="duplicate_id", path="/cases/0/incidentIds/1")
    full_negative("case-evidence-duplicate", "Case evidence references are unique.",
                  lambda i: i["cases"][0].__setitem__("evidenceIds", ["ops-log", "ops-log"]),
                  code="duplicate_id", path="/cases/0/evidenceIds/1")
    full_negative("case-dangling-incident", "Case incident references close locally.",
                  lambda i: i["cases"][0].__setitem__("incidentIds", ["missing"]),
                  code="invalid_reference", path="/cases/0/incidentIds")
    full_negative("case-dangling-evidence", "Case evidence references close locally.",
                  lambda i: i["cases"][0].__setitem__("evidenceIds", ["missing"]),
                  code="invalid_reference", path="/cases/0/evidenceIds")
    full_negative("finding-duplicate-id", "Finding ids are Case-local unique.",
                  lambda i: i["cases"][0]["findings"][1].__setitem__("id", "first-finding"),
                  code="duplicate_id", path="/cases/0/findings/1")
    full_negative("finding-empty-text", "Native Finding text is nonempty.",
                  lambda i: i["cases"][0]["findings"][1].__setitem__("text", ""),
                  code="invalid_value", path="/cases/0/findings/1")
    full_negative("finding-dangling-evidence", "Finding evidence references close locally.",
                  lambda i: i["cases"][0]["findings"][0].__setitem__("evidenceRefs", ["missing"]),
                  code="invalid_reference", path="/cases/0/findings/0/evidenceRefs")
    full_negative("finding-duplicate-evidence", "Finding evidence references are unique.",
                  lambda i: i["cases"][0]["findings"][0].__setitem__("evidenceRefs", ["ops-log", "ops-log"]),
                  code="invalid_value", path="/cases/0/findings/0")
    full_negative("timeline-type", "Timeline type is the closed set.",
                  lambda i: i["cases"][0]["timeline"][0].__setitem__("type", "unknown"),
                  code="invalid_value", path="/cases/0/timeline/0/type")
    full_negative("timeline-non-utc", "Timeline time is a fictional RFC3339 UTC instant.",
                  lambda i: i["cases"][0]["timeline"][0].__setitem__("at", "2026-01-02T03:04:05+01:00"),
                  code="invalid_value", path="/cases/0/timeline/0/at")
    full_negative("timeline-actor", "Timeline actor resolves a fictional actor.",
                  lambda i: i["cases"][0]["timeline"][0].__setitem__("actorId", "ghost"),
                  code="invalid_reference", path="/cases/0/timeline/0/actorId")
    full_negative("timeline-ref-evidence", "Timeline evidence references close locally.",
                  lambda i: i["cases"][0]["timeline"][0]["ref"].__setitem__("id", "missing"),
                  code="invalid_reference", path="/cases/0/timeline/0/ref/id")
    full_negative("timeline-ref-finding", "Finding timeline references are Case-local.",
                  lambda i: i["cases"][0]["timeline"][1]["ref"].__setitem__("id", "missing"),
                  code="invalid_reference", path="/cases/0/timeline/1/ref/id")
    full_negative("timeline-ref-check", "Check timeline references close locally.",
                  lambda i: i["cases"][0]["timeline"][2]["ref"].__setitem__("id", "missing"),
                  code="invalid_reference", path="/cases/0/timeline/2/ref/id")

    index, files = minimal()
    index["cases"][0]["findings"] = [
        {"id": f"finding-{number:04d}", "text": "Fictional.", "status": "hypothesis"}
        for number in range(1001)
    ]
    emit("finding-over-1000", "A Case may declare at most 1000 findings.", index, files,
         refused("too_many", "/cases/0/findings"))

    index, files = full()
    index["cases"] = [copy.deepcopy(index["cases"][0]) for _ in range(201)]
    for number, case in enumerate(index["cases"]):
        case["id"] = f"case-{number:04d}"
    emit("cases-over-200", "At most 200 Case projections are admitted.", index, files,
         refused("too_many", "/cases"))

    index, files = full()
    for number in range(1000):
        index["presentationTargets"].append(
            {"id": f"extra-{number:04d}", "kind": "case", "refId": "retry-case"})
    emit("targets-over-1000", "At most 1000 presentation targets are admitted.", index, files,
         refused("too_many", "/presentationTargets"))

    # --- Negatives: incidents, sources, queries -------------------------
    full_negative("incident-status", "Incident status is the native closed set.",
                  lambda i: i["incidents"][0].__setitem__("status", "pending"),
                  code="invalid_value", path="/incidents/0/status")
    full_negative("incident-evidence-duplicate", "Incident evidence references are unique.",
                  lambda i: i["incidents"][0].__setitem__("evidenceIds", ["ops-log", "ops-log"]),
                  code="duplicate_id", path="/incidents/0/evidenceIds/1")
    full_negative("incident-evidence", "Incident evidence references close locally.",
                  lambda i: i["incidents"][0].__setitem__("evidenceIds", ["missing"]),
                  code="invalid_reference", path="/incidents/0/evidenceIds")
    full_negative("source-wrong-kind", "Every source, used or unused, binds a challenge fixture.",
                  lambda i: i["sources"][0].__setitem__("fixtureAssetId", "narrative-text"),
                  code="invalid_reference", path="/sources/0/fixtureAssetId")
    full_negative("source-unused-dangling", "An unused source still has to resolve its fixture.",
                  lambda i: i["sources"].append(
                      {"id": "ghost-source", "fixtureAssetId": "does-not-exist"}),
                  code="invalid_reference", path="/sources/1/fixtureAssetId")
    full_negative("source-unused-wrong-kind", "An unused source still has to bind a fixture kind.",
                  lambda i: i["sources"].append(
                      {"id": "ghost-source", "fixtureAssetId": "narrative-text"}),
                  code="invalid_reference", path="/sources/1/fixtureAssetId")

    index, files = full()
    index["sources"].append(
        {"id": "unused-fixture", "fixtureAssetId": "retry-fixture"})
    ref, binding = emit(
        "source-unused-valid",
        "A valid unused source fixture binding is admitted and projected.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    full_negative("source-missing", "A query source must be declared.",
                  lambda i: i["queries"][0].__setitem__("sourceId", "missing"),
                  code="invalid_reference", path="/queries/0/sourceId")
    full_negative("query-body-kind", "A query body resolves a query-body-sql asset.",
                  lambda i: i["queries"][0].__setitem__("bodyAssetId", "orders-records"),
                  code="invalid_reference", path="/queries/0/bodyAssetId")

    def metadata_id_mismatch(index, files):
        metadata = copy.deepcopy(METADATA)
        metadata["id"] = "retries/other"
        body = dumps(metadata)
        files["training/retry.meta.json"] = body
        advance_bytes(index, "retry-metadata", body)

    full_negative("query-metadata-id", "Metadata id equals the inventory query id.",
                  edit=metadata_id_mismatch,
                  code="mismatch", path="/queries/0/metadataAssetId")

    def connection_mismatch(index, files):
        metadata = copy.deepcopy(METADATA)
        metadata["connectionId"] = "other-connection"
        body = dumps(metadata)
        files["training/retry.meta.json"] = body
        advance_bytes(index, "retry-metadata", body)

    full_negative("query-connection", "Metadata connectionId equals the query sourceId.",
                  edit=connection_mismatch,
                  code="mismatch", path="/queries/0/sourceId")

    def metadata_type(index, files):
        metadata = copy.deepcopy(METADATA)
        metadata["type"] = "HTTP"
        body = dumps(metadata)
        files["training/retry.meta.json"] = body
        advance_bytes(index, "retry-metadata", body)

    full_negative("query-metadata-type", "Only the SQL metadata codec is admitted.",
                  edit=metadata_type,
                  code="invalid_value", path="/assets/3/type")

    full_negative("query-metadata-targets", "Ordinary query metadata fields do not leak in.",
                  lambda i: i["assets"][3].__setitem__("targets", []),
                  code="unknown_field", path="/assets/3/targets")

    def sql_credential(index, files):
        body = b"select '--password=hunter2';\n"
        files["training/retry.sql"] = body
        advance_bytes(index, "retry-body", body)

    full_negative("query-body-credential", "Native query privacy screening still applies.",
                  edit=sql_credential,
                  code="invalid_value", path="/queries/0")

    def sql_invalid_utf8(index, files):
        body = b"\xff"
        files["training/retry.sql"] = body
        advance_bytes(index, "retry-body", body)

    full_negative("query-body-utf8", "A SQL body must be valid UTF-8.",
                  edit=sql_invalid_utf8, code="invalid_utf8", path="/assets/4")

    # A query-metadata asset is only meaningful as one declared query's metadata:
    # its id and connectionId are compared against that record, and the format
    # has no other metadata-to-body mapping. An unbound one is an unresolved
    # reference, not a guess by filename, hash or connection default.
    def orphan_metadata(index, files, mutator=None):
        metadata = copy.deepcopy(METADATA)
        metadata["id"] = "retries/unused"
        if mutator is not None:
            mutator(metadata)
        body = dumps(metadata)
        files["training/unused.meta.json"] = body
        index["assets"].append(asset("unused-metadata", "query-metadata",
                                     "application/json", "training/unused.meta.json", body))

    def orphan_metadata_credential(index, files):
        orphan_metadata(index, files,
                        lambda m: m["parameters"][0].__setitem__("isCredential", True))

    index, files = full()
    orphan_metadata(index, files)
    emit("orphan-metadata-unbound",
         "A query-metadata asset that no declared query record binds refuses.",
         index, files, refused("invalid_reference", "/assets/9"))

    index, files = full()
    orphan_metadata_credential(index, files)
    emit("orphan-metadata-credential",
         "Credential screening runs on an unbound query-metadata asset too.",
         index, files, refused("unknown_field", "/assets/9/parameters/0/isCredential"))

    def invalid_utf8_asset(asset_id):
        def apply(index, files):
            entry = next(item for item in index["assets"] if item["id"] == asset_id)
            files[entry["path"]] = b"\xff"
            entry["sha256"] = sha256(b"\xff")
            entry["bytes"] = 1
        return apply

    full_negative("evidence-text-invalid-utf8", "Evidence text must be valid UTF-8.",
                  edit=invalid_utf8_asset("narrative-text"),
                  code="invalid_utf8", path="/assets/0")
    full_negative("manifest-invalid-utf8", "A challenge manifest must be valid UTF-8.",
                  edit=invalid_utf8_asset("retry-manifest"),
                  code="invalid_utf8", path="/assets/6")

    def manifest_with(mutator):
        def apply(index, files):
            manifest = copy.deepcopy(MANIFEST)
            mutator(manifest)
            body = dumps(manifest)
            files["training/challenge.json"] = body
            advance_bytes(index, "retry-manifest", body)
        return apply

    def manifest_raw(edit):
        def apply(index, files):
            body = edit(dumps(MANIFEST))
            files["training/challenge.json"] = body
            advance_bytes(index, "retry-manifest", body)
        return apply

    full_negative("manifest-root-case", "Root manifest keys keep exact casing.",
                  edit=manifest_with(lambda m: m.__setitem__("Title", m.pop("title"))),
                  code="unknown_field", path="/assets/6/Title")
    full_negative("manifest-hints-null", "An explicit null hint array rejects.",
                  edit=manifest_with(lambda m: m.__setitem__("hints", None)),
                  code="invalid_type", path="/assets/6/hints")
    full_negative("manifest-engine-case", "Nested manifest keys keep exact casing.",
                  edit=manifest_with(lambda m: m["engine"].__setitem__(
                      "Package", m["engine"].pop("package"))),
                  code="unknown_field", path="/assets/6/engine/Package")
    full_negative("manifest-fixture-null", "A null fixture label rejects.",
                  edit=manifest_with(lambda m: m["fixtureSuite"]["fixtures"][0].__setitem__("label", None)),
                  code="invalid_type", path="/assets/6/fixtureSuite/fixtures/0/label")
    full_negative("manifest-column-null", "A null optional visible column field rejects.",
                  edit=manifest_with(lambda m: m["visibleSchema"]["tables"][0]["columns"][0].__setitem__(
                      "collation", None)),
                  code="invalid_type",
                  path="/assets/6/visibleSchema/tables/0/columns/0/collation")
    full_negative("manifest-part-extra", "Nested manifest objects are closed.",
                  edit=manifest_with(lambda m: m["parts"][0].__setitem__("extra", True)),
                  code="unknown_field", path="/assets/6/parts/0/extra")
    full_negative("manifest-part-required-null", "A null part required flag rejects.",
                  edit=manifest_with(lambda m: m["parts"][0].__setitem__("required", None)),
                  code="invalid_type", path="/assets/6/parts/0/required")
    full_negative("manifest-integer-spelling", "Manifest integers are canonical JSON integers.",
                  edit=manifest_with(lambda m: m["fixtureSuite"]["fixtures"][0].__setitem__("bytes", "12")),
                  code="invalid_value", path="/assets/6/fixtureSuite/fixtures/0/bytes")
    full_negative("manifest-oracle-extra", "Oracle objects are closed nested objects.",
                  edit=manifest_with(lambda m: m["parts"][0]["sql"]["cases"][0]["oracle"].__setitem__(
                      "extra", 1)),
                  code="unknown_field", path="/assets/6/parts/0/sql/cases/0/oracle/extra")
    full_negative("manifest-reference-solutions-null", "An explicit null reference list rejects.",
                  edit=manifest_with(lambda m: m.__setitem__("referenceSolutions", None)),
                  code="invalid_type", path="/assets/6/referenceSolutions")
    full_negative("manifest-hint-text-null", "A null hint text rejects.",
                  edit=manifest_with(lambda m: m["hints"][0].__setitem__("text", None)),
                  code="invalid_type", path="/assets/6/hints/0/text")
    full_negative("manifest-escaped-case", "An escaped spelling of a closed key is that key.",
                  edit=manifest_raw(lambda body: body.replace(
                      b'"title": "Retry query"', b'"\\u0054itle": "Retry query"', 1)),
                  code="unknown_field", path="/assets/6/Title")
    full_negative("manifest-escaped-duplicate", "An escaped duplicate decoded key rejects.",
                  edit=manifest_raw(lambda body: body.replace(
                      b'"title": "Retry query"',
                      b'"title": "Retry query", "\\u0074itle": "Retry query"', 1)),
                  code="duplicate_key", path="/assets/6/title")

    # --- Negatives: parameter bypasses ---------------------------------
    def metadata_with(mutator):
        def apply(index, files):
            metadata = copy.deepcopy(METADATA)
            mutator(metadata)
            body = dumps(metadata)
            files["training/retry.meta.json"] = body
            advance_bytes(index, "retry-metadata", body)
        return apply

    full_negative("param-lookup-null", "lookup rejects at presence including null.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("lookup", None)),
                  code="unknown_field", path="/assets/3/parameters/0/lookup")
    full_negative("param-lookup-false", "lookup rejects at presence including false.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("lookup", False)),
                  code="unknown_field", path="/assets/3/parameters/0/lookup")
    full_negative("param-lookup-object", "lookup rejects even as an object.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__(
                      "lookup", {"db": "d", "sql": "s", "keyFields": ["k"]})),
                  code="unknown_field", path="/assets/3/parameters/0/lookup")
    full_negative("param-meta-null", "meta rejects at presence including null.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("meta", None)),
                  code="unknown_field", path="/assets/3/parameters/0/meta")
    full_negative("param-is-credential-false", "isCredential rejects at presence including false.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("isCredential", False)),
                  code="unknown_field", path="/assets/3/parameters/0/isCredential")
    full_negative("param-is-credential-null", "isCredential rejects at presence including null.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("isCredential", None)),
                  code="unknown_field", path="/assets/3/parameters/0/isCredential")
    full_negative("param-multi-value", "isMultiValue is not an admitted training parameter key.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("isMultiValue", False)),
                  code="unknown_field", path="/assets/3/parameters/0/isMultiValue")
    full_negative("param-unknown-key", "Unknown parameter keys reject.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("minLength", 1)),
                  code="unknown_field", path="/assets/3/parameters/0/minLength")
    full_negative("param-default-wrong-type", "A default must match its declared scalar type.",
                  edit=metadata_with(lambda m: m["parameters"][1].__setitem__("defaultValue", "3")),
                  code="invalid_type", path="/assets/3/parameters/1/defaultValue")
    full_negative("param-default-null", "An explicit null default rejects.",
                  edit=metadata_with(lambda m: m["parameters"][1].__setitem__("defaultValue", None)),
                  code="invalid_type", path="/assets/3/parameters/1/defaultValue")
    full_negative("param-integer-over-safe", "Integer defaults stay in the native safe range.",
                  edit=metadata_with(lambda m: m["parameters"][1].__setitem__("defaultValue", 9007199254740992)),
                  code="invalid_value", path="/assets/3/parameters/1/defaultValue")
    full_negative("param-nested-default", "A nested default cannot smuggle a source or credential.",
                  edit=metadata_with(lambda m: m["parameters"][1].__setitem__(
                      "defaultValue", {"source": "x"})),
                  code="invalid_type", path="/assets/3/parameters/1/defaultValue")
    full_negative("param-title-121", "A parameter title is at most 120 code points.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("title", "t" * 121)),
                  code="too_long", path="/assets/3/parameters/0/title")
    full_negative("param-id-empty", "A parameter id is nonempty.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("id", "")),
                  code="invalid_value", path="/assets/3/parameters/0/id")
    full_negative("param-type-unsupported", "Only the four scalar parameter types are admitted.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("type", "array")),
                  code="invalid_value", path="/assets/3/parameters/0/type")

    def escaped_lookup(index, files):
        metadata = copy.deepcopy(METADATA)
        metadata["parameters"][0]["lookup"] = None
        body = dumps(metadata).replace(b'"lookup": null', b'"\\u006cookup": null', 1)
        files["training/retry.meta.json"] = body
        advance_bytes(index, "retry-metadata", body)

    full_negative("param-escaped-lookup", "An escaped spelling of a forbidden key is that key.",
                  edit=escaped_lookup,
                  code="unknown_field", path="/assets/3/parameters/0/lookup")

    def infinite_number(index, files):
        body = dumps(METADATA).replace(b'"defaultValue": 2.5', b'"defaultValue": 1e999', 1)
        files["training/retry.meta.json"] = body
        advance_bytes(index, "retry-metadata", body)

    full_negative("param-number-infinite", "A number default must be finite binary64.",
                  edit=infinite_number,
                  code="invalid_value", path="/assets/3/parameters/2/defaultValue")

    def escaped_allowed(index, files):
        body = dumps(METADATA).replace(b'"id": "status"', b'"\\u0069d": "status"', 1)
        files["training/retry.meta.json"] = body
        advance_bytes(index, "retry-metadata", body)

    index, files = full()
    escaped_allowed(index, files)
    ref, binding = emit("param-escaped-allowed",
                        "An escaped spelling of an allowed key applies the same validation.",
                        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(
        index, files, ref, binding, metadata=json.loads(files["training/retry.meta.json"]))

    # --- Negatives: evidence codecs ------------------------------------
    def records_with(mutator):
        def apply(index, files):
            records = json.loads(RECORDS_BODY)
            mutator(records)
            body = dumps(records)
            files["training/orders.json"] = body
            advance_bytes(index, "orders-records", body)
        return apply

    full_negative("records-column-type", "Record columns use native scalar discriminators.",
                  edit=records_with(lambda r: r["columns"][0].__setitem__("type", "uuid")),
                  code="invalid_value", path="/assets/1/columns/0/type")
    full_negative("records-row-width", "Every row has exactly the declared column count.",
                  edit=records_with(lambda r: r["rows"][0].pop()),
                  code="invalid_value", path="/assets/1/rows/0")
    full_negative("records-type-agreement", "Every non-null cell agrees with its column type.",
                  edit=records_with(lambda r: r["rows"][0][0].__setitem__("type", "string")),
                  code="invalid_value", path="/assets/1/rows/0/0")
    full_negative("records-cell-unknown", "Typed cells are closed objects.",
                  edit=records_with(lambda r: r["rows"][0][0].__setitem__("extra", 1)),
                  code="unknown_field", path="/assets/1/rows/0/0/extra")
    full_negative("records-cell-integer", "Native integer values keep canonical spelling.",
                  edit=records_with(lambda r: r["rows"][0][0].__setitem__("value", "01")),
                  code="invalid_value", path="/assets/1/rows/0/0/value")
    full_negative("records-duplicate-column", "Column names are unique.",
                  edit=records_with(lambda r: r["columns"][1].__setitem__("name", "order_id")),
                  code="duplicate_id", path="/assets/1/columns/1/name")

    def records_columns_over(index, files):
        records = json.loads(RECORDS_BODY)
        records["columns"] = [{"name": f"c{number}", "type": "string"} for number in range(101)]
        records["rows"] = []
        body = dumps(records)
        files["training/orders.json"] = body
        advance_bytes(index, "orders-records", body)

    full_negative("records-columns-over-100", "A recordset has at most 100 columns.",
                  edit=records_columns_over,
                  code="too_many", path="/assets/1/columns")

    def records_zero_columns(index, files):
        body = dumps({"columns": [], "rows": []})
        files["training/orders.json"] = body
        advance_bytes(index, "orders-records", body)

    full_negative("records-zero-columns", "A recordset requires at least one column.",
                  edit=records_zero_columns,
                  code="too_many", path="/assets/1/columns")

    def records_rows_over(index, files):
        records = {
            "columns": [{"name": "v", "type": "null"}],
            "rows": [[{"type": "null", "value": None}] for _ in range(1001)],
        }
        body = compact(records).encode()
        files["training/orders.json"] = body
        advance_bytes(index, "orders-records", body)

    full_negative("records-rows-over-1000", "A recordset has at most 1000 rows.",
                  edit=records_rows_over,
                  code="too_many", path="/assets/1/rows")

    def log_with(mutator):
        def apply(index, files):
            lines = LOG_BODY.decode().splitlines()
            parsed = [json.loads(line) for line in lines]
            mutator(parsed)
            body = ("\n".join(json.dumps(item, ensure_ascii=False) for item in parsed) + "\n").encode()
            files["training/ops.ndjson"] = body
            advance_bytes(index, "ops-log", body)
        return apply

    full_negative("log-actor", "Log actors resolve declared fictional actors.",
                  edit=log_with(lambda lines: lines[0].__setitem__("actorId", "ghost")),
                  code="invalid_reference", path="/assets/2/0/actorId")
    full_negative("log-time", "Log time is a fictional RFC3339 UTC instant.",
                  edit=log_with(lambda lines: lines[0].__setitem__("at", "2026-01-02T03:04:05+01:00")),
                  code="invalid_value", path="/assets/2/0/at")
    full_negative("log-duplicate-id", "Log entry ids are unique.",
                  edit=log_with(lambda lines: lines[1].__setitem__("id", "line-1")),
                  code="duplicate_id", path="/assets/2/1/id")
    full_negative("log-unknown-field", "Log lines are closed objects.",
                  edit=log_with(lambda lines: lines[0].__setitem__("extra", 1)),
                  code="unknown_field", path="/assets/2/0/extra")
    full_negative("log-empty-kind", "Log kind is nonempty.",
                  edit=log_with(lambda lines: lines[0].__setitem__("kind", "")),
                  code="invalid_value", path="/assets/2/0/kind")
    full_negative("log-empty-message", "Log message is nonempty.",
                  edit=log_with(lambda lines: lines[0].__setitem__("message", "")),
                  code="invalid_value", path="/assets/2/0/message")

    def log_malformed(index, files):
        body = b"not json\n"
        files["training/ops.ndjson"] = body
        advance_bytes(index, "ops-log", body)

    full_negative("log-malformed", "Every nonempty log line is strict JSON.",
                  edit=log_malformed, code="invalid_json", path="/assets/2/0")

    def log_root_array(index, files):
        body = b"[]\n"
        files["training/ops.ndjson"] = body
        advance_bytes(index, "ops-log", body)

    full_negative("log-root-array", "Every log line is a JSON object.",
                  edit=log_root_array, code="not_object", path="/assets/2/0")

    # --- Negatives: Checks ---------------------------------------------
    full_negative("check-id-mismatch", "The loaded native Check id equals the inventory id.",
                  lambda i: i["checks"][0].__setitem__("id", "other-check"),
                  code="mismatch", path="/checks/0/id")
    full_negative("check-definition-kind", "A Check definition resolves a check-definition asset.",
                  lambda i: i["checks"][0].__setitem__("definitionAssetId", "orders-records"),
                  code="invalid_reference", path="/checks/0/definitionAssetId")
    full_negative("check-updated-by", "A Check updatedBy is a declared fictional actor.",
                  edit=lambda index, files: (
                      files.__setitem__("training/retry-check.json",
                                        dumps({**CHECK, "updatedBy": "ghost"})),
                      advance_bytes(index, "retry-check",
                                    dumps({**CHECK, "updatedBy": "ghost"}))),
                  code="invalid_reference", path="/assets/5")
    full_negative("check-unknown-field", "A Check definition is a closed native object.",
                  edit=lambda index, files: (
                      files.__setitem__("training/retry-check.json",
                                        dumps({**CHECK, "extra": True})),
                      advance_bytes(index, "retry-check", dumps({**CHECK, "extra": True}))),
                  code="unknown_field", path="/assets/5/extra")
    full_negative("check-query-unresolved", "A Check query step resolves a declared package query.",
                  lambda i: i["checks"][0].__setitem__("id", "retry-observation"),
                  edit=lambda index, files: (
                      files.__setitem__("training/retry-check.json", dumps({
                          **CHECK,
                          "steps": [{"id": "run", "kind": "query", "queryId": "missing/query"}],
                      })),
                      advance_bytes(index, "retry-check", dumps({
                          **CHECK,
                          "steps": [{"id": "run", "kind": "query", "queryId": "missing/query"}],
                      }))),
                  code="invalid_reference", path="/checks/0")

    def promoted_validator(index, files):
        check = copy.deepcopy(CHECK)
        check["steps"][0]["validators"][0] = {
            "id": "status-present",
            "validatorId": "promoted-validator",
        }
        body = dumps(check)
        files["training/retry-check.json"] = body
        advance_bytes(index, "retry-check", body)

    full_negative("check-promoted-validator", "Missing promoted validators refuse instead of falling back.",
                  edit=promoted_validator,
                  code="invalid_reference", path="/checks")

    def missing_child(index, files):
        check = copy.deepcopy(CHECK)
        check["steps"] = [{"id": "run", "kind": "check", "checkId": "missing-check"}]
        body = dumps(check)
        files["training/retry-check.json"] = body
        advance_bytes(index, "retry-check", body)

    full_negative("check-missing-child", "A transitive Check dependency must be promoted in the package.",
                  edit=missing_child,
                  code="invalid_reference", path="/checks")

    index = empty_index()
    files = {}
    check_a = {
        "id": "check-a", "title": "A", "version": 1, "inputs": [],
        "steps": [{"id": "run", "kind": "check", "checkId": "check-b"}],
        "updatedAt": "2026-01-02T03:04:05Z", "updatedBy": "guide",
    }
    check_b = {
        "id": "check-b", "title": "B", "version": 1, "inputs": [],
        "steps": [{"id": "run", "kind": "check", "checkId": "check-a"}],
        "updatedAt": "2026-01-02T03:04:05Z", "updatedBy": "guide",
    }
    index["fictionalActors"] = [{"id": "guide", "name": "Guide"}]
    files["training/check-a.json"] = dumps(check_a)
    files["training/check-b.json"] = dumps(check_b)
    index["assets"] = [
        asset("check-a-asset", "check-definition", "application/json", "training/check-a.json",
              files["training/check-a.json"]),
        asset("check-b-asset", "check-definition", "application/json", "training/check-b.json",
              files["training/check-b.json"]),
    ]
    index["checks"] = [
        {"id": "check-a", "definitionAssetId": "check-a-asset"},
        {"id": "check-b", "definitionAssetId": "check-b-asset"},
    ]
    emit("check-cycle", "A transitive Check cycle rejects.", index, files,
         refused("invalid_value", "/checks"))

    # --- Negatives/positives: unused Check definitions -------------------
    # An unused check-definition asset is admitted and projected, so it must
    # satisfy the same native semantics and package-local closure even though no
    # checks[] record binds it. It never gains inventory authority.

    def orphan_check(check_id="orphan-check", steps=None, validators=None):
        return {
            "id": check_id, "title": "Unused check", "version": 1, "inputs": [],
            "steps": steps if steps is not None
            else [{"id": "run", "kind": "query", "queryId": "retries/by-status"}],
            "updatedAt": "2026-01-02T03:04:05Z", "updatedBy": "guide",
        }

    def add_orphan_check(index, files, check):
        body = dumps(check)
        files["training/orphan-check.json"] = body
        index["assets"].append(asset("orphan-check", "check-definition",
                                     "application/json", "training/orphan-check.json", body))

    index, files = full()
    add_orphan_check(index, files, orphan_check())
    ref, binding = emit(
        "orphan-check-valid",
        "A fully closed unused Check definition is admitted with no inventory record.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    index, files = full()
    add_orphan_check(index, files, orphan_check(
        steps=[{"id": "run", "kind": "query", "queryId": "missing/query"}]))
    emit("orphan-check-missing-query",
         "An unused Check query step must resolve a declared package query.",
         index, files, refused("invalid_reference", "/assets/9"))

    index, files = full()
    add_orphan_check(index, files, orphan_check(
        steps=[{"id": "run", "kind": "check", "checkId": "missing-check"}]))
    emit("orphan-check-missing-child",
         "An unused Check dependency must resolve a declared package Check.",
         index, files, refused("invalid_reference", "/assets/9"))

    index, files = full()
    add_orphan_check(index, files, orphan_check(steps=[{
        "id": "run", "kind": "query", "queryId": "retries/by-status",
        "validators": [{"id": "status-present", "validatorId": "promoted-validator"}],
    }]))
    emit("orphan-check-promoted-validator",
         "An unused Check promoted-validator reference has no declared closed source.",
         index, files, refused("invalid_reference", "/assets/9"))

    index, files = full()
    add_orphan_check(index, files, orphan_check(check_id="retry-observation"))
    emit("orphan-check-duplicate-id",
         "An unused Check definition may not duplicate a declared package Check id.",
         index, files, refused("duplicate_id", "/assets/9"))

    # --- Native timestamp precision -------------------------------------
    def check_with(mutator):
        def apply(index, files):
            check = copy.deepcopy(CHECK)
            mutator(check)
            body = dumps(check)
            files["training/retry-check.json"] = body
            advance_bytes(index, "retry-check", body)
        return apply

    index, files = full()
    check_with(lambda c: (c.__setitem__("updatedAt", "2026-01-02T03:04:05.123456789Z"),
                          c.pop("createdAt")))(index, files)
    ref, binding = emit(
        "check-fractional-timestamp",
        "A native nanosecond updatedAt is projected at full precision and an omitted "
        "optional createdAt stays absent.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    index, files = full()
    check_with(lambda c: c.__setitem__("updatedAt", "2026-01-02T03:04:05.500+05:30"))(index, files)
    ref, binding = emit(
        "check-offset-timestamp",
        "A native non-zero offset keeps its offset and trailing fractional zeros are trimmed.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    full_negative("check-timestamp-invalid",
                  "A native updatedAt requires an RFC3339 instant.",
                  edit=check_with(lambda c: c.__setitem__("updatedAt", "2026-01-02T03:04:05")),
                  code="invalid_value", path="/assets/5/updatedAt")
    full_negative("check-timestamp-null",
                  "An explicit null updatedAt rejects.",
                  edit=check_with(lambda c: c.__setitem__("updatedAt", None)),
                  code="invalid_type", path="/assets/5/updatedAt")
    full_negative("check-created-at-invalid",
                  "The optional native createdAt keeps its own native validation.",
                  edit=check_with(lambda c: c.__setitem__("createdAt", "yesterday")),
                  code="invalid_value", path="/assets/5")

    # --- Negatives: challenges -----------------------------------------
    full_negative("challenge-missing-fixture", "The fixture map equals the manifest suite exactly.",
                  lambda i: i["challenges"][0].__setitem__("fixtures", []),
                  code="invalid_reference", path="/challenges/0")
    full_negative("challenge-extra-fixture", "An extra fixture id rejects.",
                  lambda i: i["challenges"][0]["fixtures"].append(
                      {"fixtureId": "extra", "assetId": "retry-fixture"}),
                  code="invalid_reference", path="/challenges/0")
    full_negative("challenge-id-mismatch", "The manifest challengeId equals the inventory id.",
                  lambda i: i["challenges"][0].__setitem__("id", "other-query"),
                  code="mismatch", path="/challenges/0/id")
    full_negative("challenge-revision-mismatch", "The manifest revision equals the inventory revision.",
                  lambda i: i["challenges"][0].__setitem__("revision", 2),
                  code="mismatch", path="/challenges/0/revision")
    full_negative("challenge-revision-over", "The existing challenge revision bound stays 1..1,000,000.",
                  lambda i: i["challenges"][0].__setitem__("revision", 1000001),
                  code="invalid_value", path="/challenges/0/revision")

    def challenge_fixture_pin(index, files):
        body = b"other-fixture-bytes"
        files["training/retry.sqlite"] = body
        advance_bytes(index, "retry-fixture", body)

    full_negative("challenge-fixture-pin", "A fixture asset pin must equal the manifest pin.",
                  edit=challenge_fixture_pin,
                  code="mismatch", path="/challenges/0")

    def challenge_manifest_grading(index, files):
        manifest = copy.deepcopy(MANIFEST)
        manifest["gradingVersion"] = "challenge-grading-v2"
        body = dumps(manifest)
        files["training/challenge.json"] = body
        advance_bytes(index, "retry-manifest", body)

    full_negative("challenge-manifest-grading", "Native manifest validation refuses an unsupported grading version.",
                  edit=challenge_manifest_grading,
                  code="invalid_value", path="/assets/6")

    index, files = full()
    files["training/retry.sqlite"] = b"X" + FIXTURE_BODY[1:]
    emit("challenge-fixture-tamper", "Actual fixture bytes are verified before native Open.",
         index, files, refused("digest_mismatch", "/assets"))

    index, files = full()
    del files["training/retry.sqlite"]
    emit("challenge-fixture-missing", "A declared fixture without bytes refuses.", index, files,
         refused("missing_asset", "/assets"))

    # A manifest asset that no declared challenge record binds cannot prove any
    # fixture byte: the only fixtureID-to-asset mapping in the format is the
    # challenge record's fixtures array. It refuses as an unresolved reference.
    index, files = full()
    orphan_manifest = copy.deepcopy(MANIFEST)
    orphan_manifest["challengeId"] = "orphan-challenge"
    orphan_manifest["fixtureSuite"]["fixtures"][0]["sha256"] = "dd" * 32
    orphan_body = dumps(orphan_manifest)
    files["training/orphan-challenge.json"] = orphan_body
    index["assets"].append(asset("orphan-manifest", "challenge-manifest",
                                 "application/json", "training/orphan-challenge.json",
                                 orphan_body))
    emit("orphan-manifest-unbound",
         "A manifest asset with no declared challenge record cannot prove its fixture pins.",
         index, files, refused("invalid_reference", "/assets/9"))

    # A manifest's check-definition part names saved queries; each must be a
    # declared package query. The reference answer selects one of those same ids
    # (native validation enforces that), so closing the part closes both.
    index, files = union_manifest()
    manifest = json.loads(files["training/union-challenge.json"])
    manifest["parts"][2]["checkDefinition"]["queryIds"] = ["missing/query"]
    manifest["referenceSolutions"][2]["answer"]["checkDefinition"]["queryId"] = "missing/query"
    manifest_body = dumps(manifest)
    files["training/union-challenge.json"] = manifest_body
    advance_bytes(index, "union-manifest", manifest_body)
    emit("manifest-query-missing",
         "A manifest saved-query reference must resolve a declared package query.",
         index, files,
         refused("invalid_reference", "/challenges/0/parts/2/checkDefinition/queryIds/0"))

    # --- Negatives: proposals and presentation -------------------------
    full_negative("proposal-query-missing", "A query proposal resolves a declared package query.",
                  lambda i: i["proposals"][0].__setitem__("definitionId", "missing/query"),
                  code="invalid_reference", path="/proposals/0/definitionId")
    full_negative("proposal-check-missing", "A check proposal resolves a declared package Check.",
                  lambda i: i["proposals"][1].__setitem__("definitionId", "missing-check"),
                  code="invalid_reference", path="/proposals/1/definitionId")
    full_negative("proposal-watch-wrong-kind", "A watch proposal resolves a watch-recommendation asset.",
                  lambda i: i["proposals"][2].__setitem__("definitionId", "retry-check"),
                  code="invalid_reference", path="/proposals/2/definitionId")
    full_negative("proposal-evidence-duplicate", "Proposal evidence references are unique.",
                  lambda i: i["proposals"][0].__setitem__("evidenceIds", ["ops-log", "ops-log"]),
                  code="duplicate_id", path="/proposals/0/evidenceIds/1")
    full_negative("proposal-evidence", "Proposal evidence references close locally.",
                  lambda i: i["proposals"][0].__setitem__("evidenceIds", ["missing"]),
                  code="invalid_reference", path="/proposals/0/evidenceIds")
    full_negative("proposal-rationale-401", "A proposal rationale is at most 400 code points.",
                  lambda i: i["proposals"][0].__setitem__("rationale", "r" * 401),
                  code="too_long", path="/proposals/0/rationale")
    full_negative("presentation-ref", "A presentation target refId resolves its declared kind.",
                  lambda i: i["presentationTargets"][0].__setitem__("refId", "missing"),
                  code="invalid_reference", path="/presentationTargets/0/refId")
    full_negative("presentation-kind", "A presentation target refId resolves the same inventory kind.",
                  lambda i: i["presentationTargets"][0].update({"kind": "check", "refId": "retry-case"}),
                  code="invalid_reference", path="/presentationTargets/0/refId")

    # --- Negatives: Watch recommendation -------------------------------
    def watch_with(mutator):
        def apply(index, files):
            watch = copy.deepcopy(WATCH)
            mutator(watch)
            body = dumps(watch)
            files["training/watch.json"] = body
            advance_bytes(index, "retry-watch", body)
        return apply

    full_negative("watch-check-missing", "A Watch recommendation Check must be declared.",
                  edit=watch_with(lambda w: w.__setitem__("checkId", "missing-check")),
                  code="invalid_reference", path="/assets/8/checkId")
    full_negative("watch-version", "A Watch recommendation pins the native Check version.",
                  edit=watch_with(lambda w: w.__setitem__("checkVersion", "2")),
                  code="mismatch", path="/assets/8/checkVersion")
    full_negative("watch-inputs-unknown", "Native input validation runs on Watch recommendation inputs.",
                  edit=watch_with(lambda w: w["inputs"].__setitem__(
                      "other", {"type": "string", "value": "x"})),
                  code="invalid_value", path="/assets/8/inputs")
    full_negative("watch-inputs-required", "A required Check input cannot be omitted.",
                  edit=watch_with(lambda w: w.__setitem__("inputs", {})),
                  code="invalid_value", path="/assets/8/inputs")
    full_negative("watch-trigger", "Watch trigger is the closed set.",
                  edit=watch_with(lambda w: w.__setitem__("trigger", "sometimes")),
                  code="invalid_value", path="/assets/8/trigger")
    full_negative("watch-schedule-zero", "A Watch schedule interval is positive.",
                  edit=watch_with(lambda w: w["schedule"].__setitem__("intervalSeconds", 0)),
                  code="invalid_value", path="/assets/8/schedule/intervalSeconds")
    full_negative("watch-cooldown-negative", "A Watch cooldown is non-negative.",
                  edit=watch_with(lambda w: w["notificationPolicy"].__setitem__("cooldownSeconds", -1)),
                  code="invalid_value", path="/assets/8/notificationPolicy/cooldownSeconds")
    full_negative("watch-recovery-type", "A Watch recovery flag is a boolean.",
                  edit=watch_with(lambda w: w["notificationPolicy"].__setitem__("recovery", "no")),
                  code="invalid_type", path="/assets/8/notificationPolicy/recovery")

    # Watch closure is independent of proposals[]: an unconsumed recommendation
    # still has to pin a declared Check version and pass native input validation.
    index, files = full()
    unused_watch = copy.deepcopy(WATCH)
    unused_watch["checkVersion"] = "2"
    unused_watch_body = dumps(unused_watch)
    files["training/unused-watch.json"] = unused_watch_body
    index["assets"].append(asset("unused-watch", "watch-recommendation", "application/json",
                                 "training/unused-watch.json", unused_watch_body))
    emit("watch-unconsumed-wrong-version",
         "An unconsumed Watch recommendation still pins the native Check version.",
         index, files, refused("mismatch", "/assets/9/checkVersion"))

    index, files = full()
    unused_watch_body = dumps(WATCH)
    files["training/unused-watch.json"] = unused_watch_body
    index["assets"].append(asset("unused-watch", "watch-recommendation", "application/json",
                                 "training/unused-watch.json", unused_watch_body))
    ref, binding = emit(
        "watch-unconsumed-valid",
        "A fully closed unconsumed Watch recommendation is admitted.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    # --- Positive: empty registries with an unused closed asset ----------
    index = empty_index("training-unused")
    files = {"training/unused.txt": b"unused fictional evidence\n"}
    index["assets"] = [asset("unused-text", "evidence-text", "text/plain",
                             "training/unused.txt", files["training/unused.txt"])]
    ref, binding = emit(
        "unused-definitions-empty",
        "Every empty inventory admits one closed unused evidence asset.",
        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_definition(index, files, ref, binding)

    # --- Negatives: size budgets ---------------------------------------
    index = empty_index()
    index["assets"] = [asset("oversize-text", "evidence-text", "text/plain",
                             "training/oversize.txt", b"a" * 65537,
                             pinned={"sha256": sha256(b"a" * 65537), "bytes": 65537})]
    emit("text-over-64kib", "One byte over the per-text-asset bound.", index, {},
         refused("too_long", "/assets/0/bytes"))

    index = empty_index()
    for number in range(96):
        path = f"training/text-{number:03d}.txt"
        index["assets"].append(asset(
            f"text-{number:03d}", "evidence-text", "text/plain", path,
            b"", pinned={"sha256": sha256(path.encode()), "bytes": 65536}))
    index["assets"].append(asset(
        "text-final-byte", "evidence-text", "text/plain", "training/text-final-byte.txt",
        b"", pinned={"sha256": sha256(b"x"), "bytes": 1}))
    emit("text-aggregate-over-6mib",
         "Exactly one byte over the 6 MiB aggregate text budget (preflight refusal).",
         index, {}, refused("too_large", ""))

    index = empty_index()
    text_body = b"a" * 65536
    files = {}
    for number in range(96):
        path = f"training/text-{number:03d}.txt"
        files[path] = text_body
        index["assets"].append(asset(
            f"text-{number:03d}", "evidence-text", "text/plain", path, text_body))
    ref, binding = emit("text-aggregate-at-6mib",
                        "Exactly 6 MiB of supplied text is admitted end to end.",
                        index, files, {"admitted": True, "definition": {}})
    VECTORS[-1]["want"] = admitted_digest(index, files, ref, binding)

    index = empty_index()
    index["assets"] = [asset("exact-fixture", "challenge-fixture",
                             "application/vnd.sqlite3", "training/exact.sqlite",
                             b"", pinned={"sha256": sha256(b"x"), "bytes": 10485760})]
    emit("fixture-at-10mib",
         "Exactly 10 MiB passes the fixture bound, then needs its bytes (preflight only).",
         index, {}, refused("missing_asset", "/assets"))

    index = empty_index()
    index["assets"] = [asset("oversize-fixture", "challenge-fixture",
                             "application/vnd.sqlite3", "training/oversize.sqlite",
                             b"", pinned={"sha256": sha256(b"x"), "bytes": 10485761})]
    emit("fixture-over-10mib", "Exactly one byte over the existing fixture bound.", index, {},
         refused("too_long", "/assets/0/bytes"))

    index = empty_index()
    for number in range(64):
        path = f"training/fixture-{number}.sqlite"
        index["assets"].append(asset(
            f"fixture-{number}", "challenge-fixture", "application/vnd.sqlite3", path,
            b"", pinned={"sha256": sha256(path.encode()), "bytes": 1048576}))
    index["assets"].append(asset(
        "fixture-final-byte", "challenge-fixture", "application/vnd.sqlite3",
        "training/fixture-final-byte.sqlite", b"",
        pinned={"sha256": sha256(b"x"), "bytes": 1}))
    emit("binary-aggregate-over-64mib",
         "Exactly one byte over the unique binary aggregate budget (preflight refusal).",
         index, {}, refused("too_large", ""))

    index = empty_index()
    for number in range(64):
        path = f"training/fixture-{number}.sqlite"
        index["assets"].append(asset(
            f"fixture-{number}", "challenge-fixture", "application/vnd.sqlite3", path,
            b"", pinned={"sha256": sha256(path.encode()), "bytes": 1048576}))
    emit("binary-aggregate-at-64mib",
         "Exactly 64 MiB passes preflight, then needs its bytes (not a transport-memory proof).",
         index, {}, refused("missing_asset", "/assets"))

    # --- Additional countable bounds and closed native refusals -----------
    index = empty_index()
    index["assets"] = [asset(f"a{number:04d}", "evidence-text", "text/plain",
                             "training/one.txt", b"", pinned={"sha256": sha256(b"x"), "bytes": 1})
                       for number in range(1001)]
    emit("assets-over-1000", "At most 1000 asset declarations are admitted.", index, {},
         refused("too_many", "/assets"))

    index = empty_index()
    index["incidents"] = [{"id": f"incident-{number:03d}", "title": "T", "description": "D",
                           "status": "open", "evidenceIds": []} for number in range(201)]
    emit("incidents-over-200", "At most 200 incident projections are admitted.", index, {},
         refused("too_many", "/incidents"))

    index = empty_index()
    index["checks"] = [{"id": f"check-{number:03d}", "definitionAssetId": "missing-asset"}
                       for number in range(201)]
    emit("checks-over-200", "At most 200 Check records are admitted.", index, {},
         refused("too_many", "/checks"))

    index = empty_index()
    index["queries"] = [{"id": f"q/{number:03d}", "metadataAssetId": "m", "bodyAssetId": "b",
                         "sourceId": "s"} for number in range(201)]
    emit("queries-over-200", "At most 200 query records are admitted.", index, {},
         refused("too_many", "/queries"))

    index = empty_index()
    index["challenges"] = [{"id": f"challenge-{number:03d}", "revision": 1,
                            "manifestAssetId": "m", "fixtures": []} for number in range(201)]
    emit("challenges-over-200", "At most 200 challenge records are admitted.", index, {},
         refused("too_many", "/challenges"))

    index = empty_index()
    index["proposals"] = [{"id": f"proposal-{number:03d}", "kind": "query",
                           "definitionId": "q/0", "evidenceIds": [], "rationale": "r"}
                          for number in range(201)]
    emit("proposals-over-200", "At most 200 proposal records are admitted.", index, {},
         refused("too_many", "/proposals"))

    full_negative("check-native-invalid", "Native Check validation refuses an empty step list.",
                  edit=lambda index, files: (
                      files.__setitem__("training/retry-check.json", dumps({**CHECK, "steps": []})),
                      advance_bytes(index, "retry-check", dumps({**CHECK, "steps": []}))),
                  code="invalid_value", path="/assets/5")

    full_negative("actor-name-121", "A fictional actor name is at most 120 code points.",
                  lambda i: i["fictionalActors"][1].__setitem__("name", "a" * 121),
                  code="too_long", path="/fictionalActors/1/name")

    full_negative("incident-description-401", "An incident description is at most 400 code points.",
                  lambda i: i["incidents"][0].__setitem__("description", "d" * 401),
                  code="too_long", path="/incidents/0/description")

    def timeline_over(index, files):
        case = copy.deepcopy(index["cases"][0])
        case["timeline"] = [
            {"id": f"t-{number:04d}", "at": "2026-01-02T03:04:05Z", "actorId": "guide",
             "type": "evidence-revealed", "ref": {"kind": "evidence", "id": "ops-log"}}
            for number in range(1001)
        ]
        index["cases"] = [case]

    full_negative("timeline-over-1000", "A Case timeline is at most 1000 entries.",
                  edit=timeline_over, code="too_many", path="/cases/0/timeline")

    full_negative("param-is-credential-true", "isCredential rejects at presence including true.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("isCredential", True)),
                  code="unknown_field", path="/assets/3/parameters/0/isCredential")
    full_negative("param-meta-false", "meta rejects at presence including false.",
                  edit=metadata_with(lambda m: m["parameters"][0].__setitem__("meta", False)),
                  code="unknown_field", path="/assets/3/parameters/0/meta")

    # --- Write corpus and provenance ------------------------------------
    corpus = {"corpusVersion": "training-package-corpus-v1", "vectors": VECTORS}
    corpus_bytes = (json.dumps(corpus, indent=1, ensure_ascii=False) + "\n").encode()
    (ROOT / "corpus.json").write_bytes(corpus_bytes)
    provenance = {
        "corpusVersion": "training-package-corpus-v1",
        "corpusSha256": sha256(corpus_bytes),
        "generatorSha256": sha256(pathlib.Path(__file__).read_bytes()),
        "verifierSha256": sha256((ROOT / "verify.py").read_bytes()),
        "files": dict(sorted(PROVENANCE.items())),
        "fileBytes": dict(sorted(PROVENANCE_BYTES.items())),
    }
    (ROOT / "provenance.json").write_bytes(
        (json.dumps(provenance, indent=1, sort_keys=True) + "\n").encode())
    print(f"wrote {len(VECTORS)} vectors and {len(PROVENANCE)} files")


if __name__ == "__main__":
    main()
