#!/usr/bin/env python3
"""Generate the ordinary /1 raw-byte conformance corpus.

This script is a checked-in fixture generator, not part of the runtime package
and not imported by tests. It writes the raw declaration bytes and the expected
corpus. The expected normalized projections come from the independent
normalizer below, written from the owner declaration tables; they are never
produced by the Go or TypeScript implementation under test.

Run from the repository root:

    python3 pkg/walkthrough/testdata/generate_corpus.py
"""

from __future__ import annotations

import copy
import hashlib
import json
import os
import shutil

HERE = os.path.dirname(os.path.abspath(__file__))
DECLARATIONS = os.path.join(HERE, "declarations")
CORPUS_VERSION = "walkthrough-contracts-v1"
SCHEMA = "datatug-walkthrough/1"
MODES_DEFAULT = ["guided", "explore"]


def pretty(value) -> bytes:
    return (json.dumps(value, indent=2, ensure_ascii=False) + "\n").encode("utf-8")


def compact(value) -> bytes:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode("utf-8")


def step(step_id, target, done, **extra):
    value = {
        "id": step_id,
        "title": f"Step {step_id}",
        "why": f"Why {step_id}",
        "target": target,
        "done": done,
    }
    value.update(extra)
    return value


def chapter(chapter_id, steps, **extra):
    value = {
        "id": chapter_id,
        "title": f"Chapter {chapter_id}",
        "why": f"Why {chapter_id}",
        "steps": steps,
    }
    value.update(extra)
    return value


def declaration(decl_id, chapters, **extra):
    value = {
        "schema": SCHEMA,
        "id": decl_id,
        "version": 1,
        "title": f"Walkthrough {decl_id}",
        "chapters": chapters,
    }
    value.update(extra)
    return value


BASE = declaration(
    "tour",
    [
        chapter(
            "start",
            [
                step(
                    "one",
                    {"kind": "check", "checkId": "check-1"},
                    {"kind": "acknowledged"},
                )
            ],
        )
    ],
)


def mutate(**changes):
    value = copy.deepcopy(BASE)
    value.update(changes)
    return value


def mutate_step(**changes):
    value = copy.deepcopy(BASE)
    value["chapters"][0]["steps"][0].update(changes)
    return value


def mutate_chapter(**changes):
    value = copy.deepcopy(BASE)
    value["chapters"][0].update(changes)
    return value


# ---------------------------------------------------------------------------
# Independent normalized projection, written from the owner tables.
# ---------------------------------------------------------------------------

TARGET_KINDS = {
    "table-record",
    "query",
    "board",
    "card",
    "related",
    "log-source",
    "public-data-provider",
    "incident",
    "check",
    "none",
}
CONDITION_KINDS = {
    "route-reached",
    "parameter-value",
    "record-selected",
    "query-run",
    "panel-open",
    "filter-applied",
    "check-run",
    "incident-open",
    "asset-exists",
    "board-tutorial",
    "acknowledged",
}


def pv_scalar(value):
    if value is None:
        return {"kind": "null"}
    if isinstance(value, bool):
        return {"kind": "boolean", "value": value}
    if isinstance(value, int):
        return {"kind": "number", "lexeme": str(value)}
    if isinstance(value, str):
        return {"kind": "string", "value": value}
    raise TypeError(f"not a scalar: {value!r}")


def pv_value(value):
    if isinstance(value, dict) and "captured" in value:
        return {
            "kind": "captured",
            "capture": value["captured"],
            "default": pv_scalar(value["default"]),
        }
    return {"kind": "literal", "value": pv_scalar(value)}


def pv_named(values):
    if not isinstance(values, dict):
        return []
    return [
        {"name": name, "value": pv_value(values[name])} for name in sorted(values)
    ]


def pv_target(target):
    kind = target["kind"]
    projected = {
        "kind": kind,
        "support": "supported" if kind in TARGET_KINDS else "needs-newer",
    }
    if kind not in TARGET_KINDS:
        return projected
    if kind == "table-record":
        projected["source"] = target["source"]
        projected["table"] = target["table"]
        projected["key"] = pv_named(target.get("key"))
    elif kind == "query":
        projected["queryId"] = target["queryId"]
        projected["parameters"] = pv_named(target.get("parameters"))
    elif kind == "board":
        projected["boardId"] = target["boardId"]
        projected["parameters"] = pv_named(target.get("parameters"))
        projected["tutorial"] = target.get("tutorial", False)
    elif kind == "card":
        projected["cardId"] = target["cardId"]
        projected["parameters"] = pv_named(target.get("parameters"))
    elif kind == "related":
        projected["source"] = target["source"]
        projected["table"] = target["table"]
        projected["key"] = pv_named(target["key"])
        projected["meaning"] = target.get("meaning", "")
    elif kind == "log-source":
        projected["source"] = target["source"]
        projected["stream"] = target.get("stream", "")
        projected["filter"] = pv_named(target.get("filter"))
    elif kind == "public-data-provider":
        projected["provider"] = target["provider"]
    elif kind == "incident":
        if "incident" not in target:
            projected["incident"] = None
        else:
            projected["incident"] = pv_value(target["incident"])
    elif kind == "check":
        projected["checkId"] = target["checkId"]
    return projected


def pv_condition(condition):
    kind = condition["kind"]
    projected = {
        "kind": kind,
        "support": "supported" if kind in CONDITION_KINDS else "needs-newer",
    }
    if kind not in CONDITION_KINDS:
        return projected
    if kind == "parameter-value":
        projected["name"] = condition["name"]
        projected["equals"] = (
            None if "equals" not in condition else pv_scalar(condition["equals"])
        )
    elif kind == "record-selected":
        projected["field"] = condition.get("field", "")
        projected["equals"] = (
            None if "equals" not in condition else pv_scalar(condition["equals"])
        )
    elif kind == "query-run":
        projected["minRows"] = (
            None if "minRows" not in condition else str(condition["minRows"])
        )
    elif kind == "panel-open":
        projected["panel"] = condition["panel"]
    elif kind == "filter-applied":
        projected["field"] = condition["field"]
        projected["equals"] = (
            None if "equals" not in condition else pv_scalar(condition["equals"])
        )
    elif kind == "check-run":
        projected["outcome"] = condition.get("outcome")
    elif kind == "asset-exists":
        projected["asset"] = condition["asset"]
        projected["id"] = condition["id"]
    elif kind == "board-tutorial":
        projected["untilCard"] = condition.get("untilCard", "")
    return projected


def pv_exit(exit_value):
    if exit_value is None:
        return {"kind": "index"}
    if "chapter" in exit_value:
        return {"kind": "chapter", "chapter": exit_value["chapter"]}
    if "walkthrough" in exit_value:
        return {"kind": "walkthrough", "walkthrough": exit_value["walkthrough"]}
    if "target" in exit_value:
        return {"kind": "target", "target": pv_target(exit_value["target"])}
    return {"kind": "index"}


def pv_step(step_value):
    target = step_value["target"]
    done = step_value["done"]
    supported = (
        target["kind"] in TARGET_KINDS and done["kind"] in CONDITION_KINDS
    )
    return {
        "id": step_value["id"],
        "modes": step_value.get("modes", MODES_DEFAULT),
        "support": "supported" if supported else "needs-newer",
        "capture": step_value.get("capture", ""),
        "needs": step_value.get("needs", ""),
        "target": pv_target(target),
        "done": pv_condition(done),
    }


def pv_chapter(chapter_value):
    steps = [pv_step(item) for item in chapter_value["steps"]]
    entry = ""
    for step_value in chapter_value["steps"]:
        if "explore" in step_value.get("modes", MODES_DEFAULT):
            entry = step_value["id"]
            break
    return {
        "id": chapter_value["id"],
        "formerIds": chapter_value.get("formerIds", []),
        "requires": chapter_value.get("requires", "anonymous"),
        "requiresReason": chapter_value.get("requiresReason", ""),
        "continuesFrom": chapter_value.get("continuesFrom", ""),
        "readiness": (
            None
            if "readiness" not in chapter_value
            else pv_condition(chapter_value["readiness"])
        ),
        "entryStepId": entry,
        "steps": steps,
    }


def pv_declaration(value):
    return {
        "id": value["id"],
        "version": str(value["version"]),
        "summary": value.get("summary", ""),
        "chapters": [pv_chapter(item) for item in value["chapters"]],
        "finish": pv_exit(value.get("finish")),
    }


# ---------------------------------------------------------------------------
# Escaped-surrogate normalization for byte fixtures
# ---------------------------------------------------------------------------

def normalize_text(text):
    out = []
    index = 0
    while index < len(text):
        code = ord(text[index])
        if 0xD800 <= code <= 0xDBFF:
            if index + 1 < len(text) and 0xDC00 <= ord(text[index + 1]) <= 0xDFFF:
                low = ord(text[index + 1])
                out.append(chr(0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00)))
                index += 2
                continue
            out.append("\ufffd")
        elif 0xDC00 <= code <= 0xDFFF:
            out.append("\ufffd")
        else:
            out.append(text[index])
        index += 1
    return "".join(out)


def normalize_obj(value):
    if isinstance(value, str):
        return normalize_text(value)
    if isinstance(value, list):
        return [normalize_obj(item) for item in value]
    if isinstance(value, dict):
        return {normalize_text(key): normalize_obj(item) for key, item in value.items()}
    return value


# ---------------------------------------------------------------------------
# Cases
# ---------------------------------------------------------------------------

CASES = []


def accept(case_id, description, fixture, directory=None, note=None):
    CASES.append(
        {
            "id": case_id,
            "description": description,
            "fixture": fixture,
            "directory": directory or fixture["id"],
            "note": note,
        }
    )


def refuse(case_id, description, fixture, issues, directory="tour"):
    CASES.append(
        {
            "id": case_id,
            "description": description,
            "fixture": fixture,
            "directory": directory,
            "issues": issues,
        }
    )


def newer(case_id, description, fixture):
    CASES.append(
        {
            "id": case_id,
            "description": description,
            "fixture": fixture,
            "directory": fixture["id"],
            "newer": True,
        }
    )


# --- accepted: every target and condition ---------------------------------

all_targets = declaration(
    "tour",
    [
        chapter(
            "all",
            [
                step(
                    "pick-customer",
                    {"kind": "query", "queryId": "Customer Orders"},
                    {"kind": "parameter-value", "name": "CustomerID"},
                    capture="customer",
                ),
                step(
                    "table-record",
                    {
                        "kind": "table-record",
                        "source": "northwind",
                        "table": "Café Order Details",
                        "key": {"Order ID": 11045, "Naïve Column": "Chai"},
                    },
                    {"kind": "record-selected", "field": "CustomerID", "equals": "BOTTM"},
                ),
                step(
                    "query-parameters",
                    {
                        "kind": "query",
                        "queryId": "customer-orders",
                        "parameters": {
                            "CustomerID": {"captured": "customer", "default": "BOTTM"},
                            "Region": "North West",
                        },
                    },
                    {"kind": "query-run", "minRows": 1},
                ),
                step(
                    "board",
                    {
                        "kind": "board",
                        "boardId": "Orders At Risk",
                        "parameters": {"region": "North West"},
                        "tutorial": True,
                    },
                    {"kind": "board-tutorial", "untilCard": "risk-by-country"},
                ),
                step(
                    "card",
                    {
                        "kind": "card",
                        "cardId": "Customer Card",
                        "parameters": {
                            "CustomerID": {"captured": "customer", "default": "BOTTM"}
                        },
                    },
                    {"kind": "route-reached"},
                ),
                step(
                    "related",
                    {
                        "kind": "related",
                        "source": "northwind",
                        "table": "Orders",
                        "key": {"OrderID": 11045},
                        "meaning": "order-to-shipping",
                    },
                    {"kind": "panel-open", "panel": "related"},
                ),
                step(
                    "log-source",
                    {
                        "kind": "log-source",
                        "source": "northwind",
                        "stream": "requests",
                        "filter": {"Correlation ID": "abc-123"},
                    },
                    {
                        "kind": "filter-applied",
                        "field": "Correlation ID",
                        "equals": "abc-123",
                    },
                ),
                step(
                    "provider",
                    {"kind": "public-data-provider", "provider": "northwind"},
                    {"kind": "asset-exists", "asset": "meaning-mapping", "id": "order-to-shipping"},
                ),
                step(
                    "incident-open",
                    {"kind": "incident"},
                    {"kind": "incident-open"},
                    capture="incident",
                ),
                step(
                    "incident-target",
                    {
                        "kind": "incident",
                        "incident": {"captured": "incident", "default": None},
                    },
                    {"kind": "acknowledged"},
                ),
                step(
                    "check",
                    {"kind": "check", "checkId": "Orders Valid!"},
                    {"kind": "check-run", "outcome": "pass"},
                ),
                step(
                    "explanation",
                    {"kind": "none"},
                    {"kind": "acknowledged"},
                ),
            ],
        )
    ],
)
accept(
    "v1-all-targets-and-conditions",
    "Every ordinary target and condition kind, spaced/Unicode asset names, literal and captured values, and an incident null default.",
    all_targets,
)

optional_conditions = declaration(
    "tour",
    [
        chapter(
            "optional",
            [
                step(
                    "parameter-set",
                    {"kind": "query", "queryId": "q"},
                    {"kind": "parameter-value", "name": "p", "equals": None},
                ),
                step(
                    "record-selected-empty",
                    {"kind": "table-record", "source": "s", "table": "t"},
                    {"kind": "record-selected"},
                ),
                step(
                    "query-run-any",
                    {"kind": "query", "queryId": "q2"},
                    {"kind": "query-run"},
                ),
                step(
                    "check-run-any",
                    {"kind": "check", "checkId": "c"},
                    {"kind": "check-run"},
                ),
                step(
                    "board-tutorial-complete",
                    {"kind": "board", "boardId": "b", "tutorial": True},
                    {"kind": "board-tutorial"},
                ),
                step(
                    "filter-null",
                    {"kind": "log-source", "source": "s"},
                    {"kind": "filter-applied", "field": "f", "equals": None},
                ),
            ],
        )
    ],
)
accept(
    "v1-condition-optionals",
    "Optional condition fields take their documented absence defaults; explicit null equals stays a literal null.",
    optional_conditions,
)

accept(
    "v1-minimal",
    "A minimal valid declaration exercises every default: anonymous access, both modes, index finish and no captures.",
    BASE,
)
accept(
    "v1-minimal-compact",
    "Semantically identical compact bytes are a separate declaration revision with their own digest.",
    compact(BASE),
    directory="tour",
)

v2_kinds = declaration(
    "tour",
    [
        chapter(
            "one",
            [
                step(
                    "future-target",
                    {"kind": "challenge", "challengeId": "x", "revision": 1},
                    {"kind": "acknowledged"},
                ),
                step(
                    "future-condition",
                    {"kind": "check", "checkId": "c"},
                    {"kind": "dialogue-finished"},
                ),
                step(
                    "ordinary",
                    {"kind": "check", "checkId": "c"},
                    {"kind": "acknowledged"},
                ),
            ],
        )
    ],
)
accept(
    "v1-v2-kinds-step-local",
    "A /2 target or condition discriminator in /1 is an unknown kind: step-local needs-newer, never fatal and never supported admission.",
    v2_kinds,
)

# --- accepted: chapters, aliases, access, readiness, captures ----------------

structure = declaration(
    "tour",
    [
        chapter(
            "find",
            [
                step(
                    "pick",
                    {"kind": "query", "queryId": "customer-orders"},
                    {"kind": "parameter-value", "name": "CustomerID"},
                    capture="customer",
                ),
                step(
                    "guided-only",
                    {"kind": "check", "checkId": "check-1"},
                    {"kind": "acknowledged"},
                    modes=["guided"],
                ),
            ],
            formerIds=["find-old"],
        ),
        chapter(
            "follow",
            [
                step(
                    "use-customer",
                    {
                        "kind": "card",
                        "cardId": "customer",
                        "parameters": {
                            "CustomerID": {"captured": "customer", "default": "BOTTM"}
                        },
                    },
                    {"kind": "route-reached"},
                )
            ],
            requires="signed-in",
            requiresReason="Sign in to follow this customer.",
            continuesFrom="find",
            readiness={"kind": "asset-exists", "asset": "query", "id": "customer-orders"},
        ),
        chapter(
            "incident",
            [
                step(
                    "raise",
                    {"kind": "incident"},
                    {"kind": "incident-open"},
                    capture="incident",
                    needs="readiness",
                )
            ],
            requires="training-copy",
            requiresReason="Raising an incident writes to a project.",
            continuesFrom="follow",
            readiness={"kind": "asset-exists", "asset": "incident", "id": "risk-1"},
        ),
        chapter(
            "reuse",
            [
                step(
                    "ack",
                    {"kind": "incident", "incident": {"captured": "incident", "default": None}},
                    {"kind": "acknowledged"},
                )
            ],
            formerIds=["reuse-old", "find-old-2"],
            continuesFrom="incident",
        ),
    ],
    summary="A structure fixture.",
    finish={
        "target": {
            "kind": "query",
            "queryId": "next-steps",
            "parameters": {"CustomerID": {"captured": "customer", "default": "BOTTM"}},
        }
    },
)
accept(
    "v1-structure",
    "Former IDs, all access levels with reasons, readiness/continuesFrom, needs, guided-only mode and a captured target finish.",
    structure,
)

for exit_name, finish in [
    ("chapter", {"chapter": "start"}),
    ("walkthrough", {"walkthrough": "next-steps"}),
    ("index", {"index": True}),
    ("target", {"target": {"kind": "check", "checkId": "check-2"}}),
]:
    accept(
        f"v1-finish-{exit_name}",
        f"Finish exit variant {exit_name}.",
        mutate(finish=finish),
    )

# --- accepted: unknown kinds stay step-local --------------------------------

unknown_target = declaration(
    "tour",
    [
        chapter(
            "future",
            [
                step(
                    "future-entry",
                    {"kind": "future-target", "opaque": {"nested": [1, 2, 3]}},
                    {"kind": "acknowledged"},
                ),
                step(
                    "known",
                    {"kind": "check", "checkId": "check-1"},
                    {"kind": "acknowledged"},
                ),
            ],
        )
    ],
)
accept(
    "v1-unknown-target-entry",
    "An unknown first Explore target stays the declared entry, disabled/needs-newer, while later known steps stay usable.",
    unknown_target,
)

unknown_condition = declaration(
    "tour",
    [
        chapter(
            "future",
            [
                step(
                    "future-producer",
                    {"kind": "query", "queryId": "q"},
                    {
                        "kind": "future-condition",
                        "future": {"payload": True},
                    },
                    capture="future-value",
                ),
                step(
                    "defaulted",
                    {
                        "kind": "card",
                        "cardId": "c",
                        "parameters": {
                            "CustomerID": {
                                "captured": "future-value",
                                "default": "fallback",
                            }
                        },
                    },
                    {"kind": "route-reached"},
                ),
            ],
        )
    ],
)
accept(
    "v1-unknown-condition-capture",
    "An unknown condition keeps its step needs-newer, validates and registers its capture, emits no value, and the later known target keeps its mandatory default.",
    unknown_condition,
)

# --- accepted: escaped surrogates and non-BMP ------------------------------

surrogates = (
    '{"schema":"datatug-walkthrough/1","id":"tour","version":1,'
    '"title":"Pair \\ud83d\\ude00 lone \\ud800 literal \ufffd end",'
    '"chapters":[{"id":"start","title":"Start","why":"Why","steps":['
    '{"id":"one","title":"One","why":"Why",'
    '"target":{"kind":"table-record","source":"s","table":"Caf\\ud800 Orders"},'
    '"done":{"kind":"acknowledged"}},'
    '{"id":"two","title":"Two","why":"Why",'
    '"target":{"kind":"card","cardId":"c","parameters":{"label":"Pair \\ud83d\\ude00 literal \ufffd"}},'
    '"done":{"kind":"acknowledged"}}]}]}\n'
).encode("utf-8")
CASES.append(
    {
        "id": "v1-escaped-surrogates",
        "description": "A valid surrogate pair and a literal U+FFFD stay distinct from a lone surrogate, which normalizes to U+FFFD in both languages.",
        "fixture": surrogates,
        "directory": "tour",
    }
)

surrogate_identity = (
    '{"schema":"datatug-walkthrough/1","id":"\\ud800","version":1,"title":"T",'
    '"chapters":[{"id":"start","title":"S","why":"W","steps":['
    '{"id":"one","title":"O","why":"W","target":{"kind":"check","checkId":"c"},"done":{"kind":"acknowledged"}}]}]}\n'
).encode("utf-8")
refuse(
    "v1-escaped-surrogate-identity",
    "A lone surrogate in an identity key normalizes to U+FFFD and then fails the ordinary ID grammar.",
    surrogate_identity,
    [("invalid_value", "/id")],
    directory="tour",
)

# --- accepted: schema newer -------------------------------------------------

newer_cases = [
    (
        "v1-schema-2-newer",
        "A /2 declaration is a newer-client boundary for this /1 slice and is not parsed.",
        {"schema": "datatug-walkthrough/2", "id": "tour", "trainingPackage": {"opaque": True}},
    ),
    (
        "v1-schema-99-newer",
        "An unknown future schema major is a newer-client boundary.",
        {"schema": "datatug-walkthrough/99", "id": "tour", "future": True},
    ),
    (
        "v1-schema-0-newer",
        "Any other schema major, including zero, is a newer-client boundary.",
        {"schema": "datatug-walkthrough/0", "id": "tour", "future": True},
    ),
    (
        "v1-schema-10-newer",
        "A two-digit major is a newer-client boundary, not an invalid schema.",
        {"schema": "datatug-walkthrough/10", "id": "tour", "future": True},
    ),
    (
        "v1-schema-11-newer",
        "A two-digit major is a newer-client boundary, not an invalid schema.",
        {"schema": "datatug-walkthrough/11", "id": "tour", "future": True},
    ),
    (
        "v1-schema-100-newer",
        "A multi-digit major is a newer-client boundary, not an invalid schema.",
        {"schema": "datatug-walkthrough/100", "id": "tour", "future": True},
    ),
    (
        "v1-schema-large-major-newer",
        "A major larger than any machine integer is still a canonical decimal newer-client boundary.",
        {"schema": "datatug-walkthrough/100000000000000000000", "id": "tour", "future": True},
    ),
]
for case_id, description, fixture in newer_cases:
    newer(case_id, description, fixture)

# --- boundary fixtures -----------------------------------------------------


def chapters(count):
    return declaration(
        "tour",
        [
            chapter(f"ch-{index:02d}", [step(f"s-{index:02d}", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})])
            for index in range(1, count + 1)
        ],
    )


def steps_in_one_chapter(count):
    return declaration(
        "tour",
        [
            chapter(
                "only",
                [
                    step(f"s-{index:02d}", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})
                    for index in range(1, count + 1)
                ],
            )
        ],
    )


def total_steps(count):
    chapters_needed = min(20, max(1, (count + 9) // 10))
    base, extra = divmod(count, chapters_needed)
    sizes = [base + (1 if index < extra else 0) for index in range(chapters_needed)]
    if max(sizes) > 30:
        raise SystemExit("total_steps fixture exceeds a per-chapter cap")
    return declaration(
        "tour",
        [
            chapter(
                f"ch-{chapter_index:02d}",
                [
                    step(
                        f"s-{chapter_index:02d}-{step_index:02d}",
                        {"kind": "check", "checkId": "c"},
                        {"kind": "acknowledged"},
                    )
                    for step_index in range(1, size + 1)
                ],
            )
            for chapter_index, size in enumerate(sizes, start=1)
        ],
    )


accept("v1-boundary-chapters-20", "20 chapters is the legal maximum.", chapters(20))
refuse(
    "v1-boundary-chapters-21",
    "21 chapters is one beyond the legal maximum.",
    chapters(21),
    [("too_many", "/chapters")],
)
accept("v1-boundary-steps-30", "30 steps in one chapter is the legal maximum.", steps_in_one_chapter(30))
refuse(
    "v1-boundary-steps-31",
    "31 steps in one chapter is one beyond the legal maximum.",
    steps_in_one_chapter(31),
    [("too_many", "/chapters/0/steps")],
)
def sized_chapters(sizes):
    return declaration(
        "tour",
        [
            chapter(
                f"ch-{chapter_index:02d}",
                [
                    step(
                        f"s-{chapter_index:02d}-{step_index:02d}",
                        {"kind": "check", "checkId": "c"},
                        {"kind": "acknowledged"},
                    )
                    for step_index in range(1, size + 1)
                ],
            )
            for chapter_index, size in enumerate(sizes, start=1)
        ],
    )


accept(
    "v1-boundary-steps-200-seven-chapters",
    "200 steps in total is legal with only seven chapters, each within 30.",
    sized_chapters([30, 30, 30, 30, 30, 30, 20]),
)
refuse(
    "v1-boundary-steps-201-seven-chapters",
    "201 steps in total is refused with only seven chapters, each within 30.",
    sized_chapters([30, 30, 30, 30, 30, 30, 21]),
    [("too_many", "/chapters")],
)
accept("v1-boundary-steps-200", "200 steps in total is the legal maximum.", total_steps(200))
refuse(
    "v1-boundary-steps-201",
    "201 steps in total is one beyond the legal maximum even though every chapter is legal.",
    total_steps(201),
    [("too_many", "/chapters")],
)

title_120 = mutate(title="t" * 120)
accept("v1-boundary-title-120", "A 120-character title is the legal maximum.", title_120)
refuse(
    "v1-boundary-title-121",
    "A 121-character title is one beyond the legal maximum.",
    mutate(title="t" * 121),
    [("too_long", "/title")],
)
casing = dict(BASE)
casing["Title"] = "Wrong case"
refuse(
    "v1-field-casing",
    "Field names are matched exactly, not case-insensitively.",
    casing,
    [("unknown_field", "/Title")],
)

emoji = "\U0001F600"
accept(
    "v1-boundary-title-120-nonbmp",
    "A 120-code-point title of non-BMP characters is the legal maximum; the budget counts characters, not bytes.",
    mutate(title=emoji * 120),
)
refuse(
    "v1-boundary-title-121-nonbmp",
    "A 121-code-point non-BMP title is one beyond the character maximum.",
    mutate(title=emoji * 121),
    [("too_long", "/title")],
)
refuse(
    "v1-former-id-invalid",
    "A former ID uses the ordinary ID grammar.",
    mutate_chapter(formerIds=["Bad"]),
    [("invalid_value", "/chapters/0/formerIds/0")],
)

why_400 = mutate_step(why="w" * 400)
accept("v1-boundary-why-400", "A 400-character why is the legal maximum.", why_400)
refuse(
    "v1-boundary-why-401",
    "A 401-character why is one beyond the legal maximum.",
    mutate_step(why="w" * 401),
    [("too_long", "/chapters/0/steps/0/why")],
)
id_48 = mutate_step(id="a" + "b" * 47)
accept("v1-boundary-id-48", "A 48-character ID is the legal maximum.", id_48)
refuse(
    "v1-boundary-id-49",
    "A 49-character ID is one beyond the legal maximum.",
    mutate_step(id="a" + "b" * 48),
    [("invalid_value", "/chapters/0/steps/0/id")],
)

# 64 KiB exact boundary: pad the summary to the exact byte length.
padded = mutate(summary="")
pad = 65536 - len(pretty(padded))
if pad < 0:
    raise SystemExit("base fixture already exceeds 64 KiB")
padded["summary"] = "s" * pad
exact_bytes = pretty(padded)
if len(exact_bytes) != 65536:
    raise SystemExit(f"boundary fixture is {len(exact_bytes)} bytes, want 65536")
CASES.append(
    {
        "id": "v1-boundary-bytes-65536",
        "description": "Exactly 64 KiB is the legal maximum declaration size.",
        "fixture": exact_bytes,
        "directory": "tour",
    }
)
CASES.append(
    {
        "id": "v1-boundary-bytes-65537",
        "description": "One byte beyond 64 KiB is refused before parsing.",
        "fixture": exact_bytes[:-2] + b"xx\n",
        "directory": "tour",
        "issues": [("too_large", "")],
    }
)

deep_opaque = 0
for _ in range(300):
    deep_opaque = [deep_opaque]
deep_unknown = declaration(
    "tour",
    [
        chapter(
            "start",
            [
                step(
                    "one",
                    {"kind": "future-deep-target", "opaque": deep_opaque},
                    {"kind": "acknowledged"},
                )
            ],
        )
    ],
)
accept(
    "v1-deep-opaque-unknown-target",
    "A 300-level opaque body inside an unknown target kind is preserved step-locally; no invented nesting cap refuses the declaration.",
    compact(deep_unknown),
    directory="tour",
)

# --- structural and scalar negatives --------------------------------------

refuse("v1-empty-bytes", "Empty input is not JSON.", b"", [("invalid_json", "")])
refuse(
    "v1-malformed-utf8",
    "Invalid UTF-8 is refused before JSON parsing.",
    b'{"schema":"datatug-walkthrough/1","id":"tour","version":1,"title":"bad \xc3\x28","chapters":[]}',
    [("invalid_utf8", "")],
)
refuse(
    "v1-utf8-bom",
    "A UTF-8 BOM is valid UTF-8 but not JSON.",
    b"\xef\xbb\xbf" + compact(BASE),
    [("invalid_json", "")],
)
for case_id, payload in [
    ("v1-root-array", b"[]"),
    ("v1-root-null", b"null"),
    ("v1-root-string", b'"tour"'),
]:
    refuse(case_id, f"Root {case_id.rsplit('-', 1)[1]} is not a declaration object.", payload, [("not_object", "")])

base_compact = compact(BASE).decode("utf-8")
duplicate_top = base_compact.replace('"id":"tour"', '"id":"tour","\\u0069d":"tour"', 1)
refuse(
    "v1-duplicate-key-escaped",
    "An escaped spelling of an existing key is a duplicate.",
    duplicate_top.encode("utf-8"),
    [("duplicate_key", "/id")],
)
duplicate_nested = base_compact.replace(
    '"kind":"check"', '"kind":"check","\\u006bind":"check"', 1
)
refuse(
    "v1-duplicate-key-nested",
    "A nested escaped duplicate key is a duplicate.",
    duplicate_nested.encode("utf-8"),
    [("duplicate_key", "/chapters/0/steps/0/target/kind")],
)
refuse(
    "v1-trailing-data",
    "Bytes after the JSON value are refused.",
    compact(BASE) + b"{}",
    [("trailing_data", "")],
)
refuse(
    "v1-schema-invalid",
    "A schema string outside the datatug-walkthrough grammar is invalid.",
    mutate(schema="walkthrough/1"),
    [("invalid_value", "/schema")],
)
refuse(
    "v1-v2-top-field",
    "A /1 declaration rejects the /2-only trainingPackage top-level field.",
    mutate(trainingPackage={"id": "x"}),
    [("unknown_field", "/trainingPackage")],
)
refuse(
    "v1-v2-narrative-field",
    "A /1 declaration rejects the /2-only narrative top-level field.",
    mutate(narrative={"path": "x"}),
    [("unknown_field", "/narrative")],
)
refuse(
    "v1-v2-step-kind",
    "A /1 declaration rejects the /2-only step kind field.",
    mutate_step(kind="standard"),
    [("unknown_field", "/chapters/0/steps/0/kind")],
)
refuse(
    "v1-v2-segment-id",
    "A /1 declaration rejects the /2-only segmentId step field.",
    mutate_step(segmentId="discussion"),
    [("unknown_field", "/chapters/0/steps/0/segmentId")],
)
refuse(
    "v1-directory-mismatch",
    "The declaration id must equal the directory id.",
    BASE,
    [("invalid_value", "/id")],
    directory="other",
)
refuse("v1-missing-id", "id is required.", {k: v for k, v in BASE.items() if k != "id"}, [("missing_field", "/id")])
refuse("v1-missing-title", "title is required.", {k: v for k, v in BASE.items() if k != "title"}, [("missing_field", "/title")])
refuse("v1-missing-chapters", "chapters is required.", {k: v for k, v in BASE.items() if k != "chapters"}, [("missing_field", "/chapters")])
refuse("v1-null-summary", "An explicit null summary is not an absent summary.", mutate(summary=None), [("invalid_type", "/summary")])
refuse("v1-null-title", "A required title cannot be null.", mutate(title=None), [("invalid_type", "/title")])
refuse("v1-version-string", "version must be a JSON number.", mutate(version="1"), [("invalid_type", "/version")])
refuse("v1-version-zero", "version must be at least 1.", mutate(version=0), [("invalid_value", "/version")])
refuse("v1-version-fraction", "version must be a canonical integer.", mutate(version=1.5), [("invalid_value", "/version")])

# Lexical 1.0 and 1e3 cannot be produced by json.dumps from a Python number
# without losing the lexical form, so write those bytes explicitly.
version_fraction_bytes = compact(mutate(version=1)).replace(b'"version":1', b'"version":1.0', 1)
refuse(
    "v1-version-lexical-fraction",
    "A lexical fraction is not an integer version.",
    version_fraction_bytes,
    [("invalid_value", "/version")],
)
version_exponent_bytes = compact(mutate(version=1)).replace(b'"version":1', b'"version":1e3', 1)
refuse(
    "v1-version-exponent",
    "A lexical exponent is not an integer version.",
    version_exponent_bytes,
    [("invalid_value", "/version")],
)
refuse("v1-version-negative", "version cannot be negative.", mutate(version=-1), [("invalid_value", "/version")])

refuse(
    "v1-id-uppercase",
    "IDs are lowercase only.",
    mutate_step(id="One"),
    [("invalid_value", "/chapters/0/steps/0/id")],
)
refuse(
    "v1-id-leading-hyphen",
    "IDs cannot start with a hyphen.",
    mutate_step(id="-one"),
    [("invalid_value", "/chapters/0/steps/0/id")],
)
refuse(
    "v1-id-underscore",
    "IDs cannot contain underscores.",
    mutate_step(id="one_two"),
    [("invalid_value", "/chapters/0/steps/0/id")],
)
refuse(
    "v1-chapter-missing-title",
    "A chapter title is required.",
    {**BASE, "chapters": [{k: v for k, v in BASE["chapters"][0].items() if k != "title"}]},
    [("missing_field", "/chapters/0/title")],
)
refuse(
    "v1-step-missing-why",
    "A step why is required.",
    {**BASE, "chapters": [{**BASE["chapters"][0], "steps": [{k: v for k, v in BASE["chapters"][0]["steps"][0].items() if k != "why"}]}]},
    [("missing_field", "/chapters/0/steps/0/why")],
)
refuse(
    "v1-step-missing-target",
    "A step target is required.",
    {**BASE, "chapters": [{**BASE["chapters"][0], "steps": [{k: v for k, v in BASE["chapters"][0]["steps"][0].items() if k != "target"}]}]},
    [("missing_field", "/chapters/0/steps/0/target")],
)
refuse(
    "v1-step-missing-done",
    "A step done condition is required.",
    {**BASE, "chapters": [{**BASE["chapters"][0], "steps": [{k: v for k, v in BASE["chapters"][0]["steps"][0].items() if k != "done"}]}]},
    [("missing_field", "/chapters/0/steps/0/done")],
)
refuse(
    "v1-target-missing-kind",
    "A target kind is required.",
    mutate_step(target={"checkId": "check-1"}),
    [("missing_field", "/chapters/0/steps/0/target/kind")],
)
refuse(
    "v1-condition-missing-kind",
    "A condition kind is required.",
    mutate_step(done={}),
    [("missing_field", "/chapters/0/steps/0/done/kind")],
)
refuse(
    "v1-known-target-extra-field",
    "A known target variant rejects unknown fields.",
    mutate_step(target={"kind": "query", "queryId": "q", "selector": ".x"}),
    [("unknown_field", "/chapters/0/steps/0/target/selector")],
)
refuse(
    "v1-known-condition-extra-field",
    "A known condition variant rejects unknown fields.",
    mutate_step(done={"kind": "route-reached", "minRows": 1}),
    [("unknown_field", "/chapters/0/steps/0/done/minRows")],
)
refuse(
    "v1-chapter-extra-field",
    "A chapter rejects unknown fields.",
    mutate_chapter(mode="guided"),
    [("unknown_field", "/chapters/0/mode")],
)
refuse(
    "v1-step-extra-field",
    "A step rejects unknown fields.",
    mutate_step(hidden=True),
    [("unknown_field", "/chapters/0/steps/0/hidden")],
)
refuse(
    "v1-exit-extra-field",
    "An exit rejects unknown fields.",
    mutate(finish={"chapter": "start", "selector": ".x"}),
    [("unknown_field", "/finish/selector")],
)
refuse(
    "v1-exit-two-variants",
    "An exit declares exactly one variant.",
    mutate(finish={"chapter": "start", "index": True}),
    [("invalid_value", "/finish")],
)
refuse(
    "v1-exit-index-false",
    "The index exit requires true.",
    mutate(finish={"index": False}),
    [("invalid_value", "/finish/index")],
)
refuse(
    "v1-exit-chapter-dangling",
    "A chapter exit must name a declared chapter.",
    mutate(finish={"chapter": "missing"}),
    [("invalid_reference", "/finish/chapter")],
)
refuse(
    "v1-requires-invalid",
    "requires is a closed union.",
    mutate_chapter(requires="admin"),
    [("invalid_value", "/chapters/0/requires")],
)
refuse(
    "v1-gated-missing-reason",
    "A gated chapter requires requiresReason.",
    mutate_chapter(requires="signed-in"),
    [("missing_field", "/chapters/0/requiresReason")],
)
refuse(
    "v1-gated-empty-reason",
    "A gated chapter's requiresReason cannot be empty.",
    mutate_chapter(requires="signed-in", requiresReason=""),
    [("invalid_value", "/chapters/0/requiresReason")],
)
modes_invalid = declaration(
    "tour",
    [
        chapter(
            "one",
            [
                step("one", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"}, modes=["guided", "admin"]),
                step("two", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"}),
            ],
        )
    ],
)
refuse(
    "v1-modes-invalid",
    "modes is a closed union.",
    modes_invalid,
    [("invalid_value", "/chapters/0/steps/0/modes/1")],
)
refuse(
    "v1-modes-null",
    "An explicit null modes is not an absent modes.",
    mutate_step(modes=None),
    [("invalid_type", "/chapters/0/steps/0/modes")],
)
refuse(
    "v1-tutorial-integer",
    "tutorial must be a boolean, not a number.",
    mutate_step(target={"kind": "board", "boardId": "b", "tutorial": 1}),
    [("invalid_type", "/chapters/0/steps/0/target/tutorial")],
)
refuse(
    "v1-minrows-negative",
    "minRows cannot be negative.",
    mutate_step(done={"kind": "query-run", "minRows": -1}),
    [("invalid_value", "/chapters/0/steps/0/done/minRows")],
)
refuse(
    "v1-minrows-fraction",
    "minRows is an integer.",
    mutate_step(done={"kind": "query-run", "minRows": 1.5}),
    [("invalid_value", "/chapters/0/steps/0/done/minRows")],
)
refuse(
    "v1-minrows-boolean",
    "minRows is not a boolean.",
    mutate_step(done={"kind": "query-run", "minRows": True}),
    [("invalid_type", "/chapters/0/steps/0/done/minRows")],
)
refuse(
    "v1-panel-invalid",
    "panel is a closed union.",
    mutate_step(done={"kind": "panel-open", "panel": "details"}),
    [("invalid_value", "/chapters/0/steps/0/done/panel")],
)
refuse(
    "v1-check-outcome-invalid",
    "check-run outcome is a closed union.",
    mutate_step(done={"kind": "check-run", "outcome": "ok"}),
    [("invalid_value", "/chapters/0/steps/0/done/outcome")],
)
refuse(
    "v1-asset-invalid",
    "asset-exists asset is a closed union.",
    mutate_step(done={"kind": "asset-exists", "asset": "widget", "id": "x"}),
    [("invalid_value", "/chapters/0/steps/0/done/asset")],
)
refuse(
    "v1-key-not-object",
    "A key value map must be an object.",
    mutate_step(target={"kind": "related", "source": "s", "table": "t", "key": []}),
    [("invalid_type", "/chapters/0/steps/0/target/key")],
)
refuse(
    "v1-equals-captured",
    "A condition equals is always a literal.",
    mutate_step(done={"kind": "parameter-value", "name": "p", "equals": {"captured": "x", "default": "d"}}),
    [("invalid_type", "/chapters/0/steps/0/done/equals")],
)

# --- identity and capture negatives ---------------------------------------

duplicate_chapter = declaration(
    "tour",
    [
        chapter("same", [step("one", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})]),
        chapter("same", [step("two", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})]),
    ],
)
refuse(
    "v1-duplicate-chapter-id",
    "Chapter IDs are unique across the walkthrough.",
    duplicate_chapter,
    [("duplicate_id", "/chapters/1/id")],
)
former_collision = declaration(
    "tour",
    [
        chapter("same", [step("one", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})]),
        chapter("other", [step("two", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})], formerIds=["same"]),
    ],
)
refuse(
    "v1-former-collides-chapter",
    "Chapter IDs and former IDs share one uniqueness scope.",
    former_collision,
    [("duplicate_id", "/chapters/1/formerIds/0")],
)
duplicate_step = declaration(
    "tour",
    [
        chapter("one", [step("same", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})]),
        chapter("two", [step("same", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})]),
    ],
)
refuse(
    "v1-duplicate-step-id",
    "Step IDs are unique across the walkthrough.",
    duplicate_step,
    [("duplicate_id", "/chapters/1/steps/0/id")],
)
duplicate_capture = declaration(
    "tour",
    [
        chapter(
            "one",
            [
                step("a", {"kind": "query", "queryId": "q"}, {"kind": "parameter-value", "name": "p"}, capture="value"),
                step("b", {"kind": "query", "queryId": "q"}, {"kind": "parameter-value", "name": "p"}, capture="value"),
            ],
        )
    ],
)
refuse(
    "v1-duplicate-capture",
    "Capture names are unique across the walkthrough.",
    duplicate_capture,
    [("duplicate_id", "/chapters/0/steps/1/capture")],
)
duplicate_capture_unsupported = declaration(
    "tour",
    [
        chapter(
            "one",
            [
                step("a", {"kind": "query", "queryId": "q"}, {"kind": "future-condition"}, capture="value"),
                step("b", {"kind": "query", "queryId": "q"}, {"kind": "future-condition"}, capture="value"),
            ],
        )
    ],
)
refuse(
    "v1-duplicate-capture-unsupported",
    "Duplicate capture IDs are refused even when their producers are unsupported.",
    duplicate_capture_unsupported,
    [("duplicate_id", "/chapters/0/steps/1/capture")],
)
unknown_capture = mutate_step(
    target={"kind": "card", "cardId": "c", "parameters": {"p": {"captured": "missing", "default": "d"}}}
)
refuse(
    "v1-capture-unknown",
    "A captured reference needs a declared producer.",
    unknown_capture,
    [("unknown_capture", "/chapters/0/steps/0/target/parameters/p/captured")],
)
forward_capture = declaration(
    "tour",
    [
        chapter(
            "one",
            [
                step(
                    "a",
                    {"kind": "card", "cardId": "c", "parameters": {"p": {"captured": "later", "default": "d"}}},
                    {"kind": "route-reached"},
                ),
                step("b", {"kind": "query", "queryId": "q"}, {"kind": "parameter-value", "name": "p"}, capture="later"),
            ],
        )
    ],
)
refuse(
    "v1-capture-forward",
    "A captured reference must name an earlier producer, not a later one.",
    forward_capture,
    [("capture_not_earlier", "/chapters/0/steps/0/target/parameters/p/captured")],
)
missing_default = declaration(
    "tour",
    [
        chapter(
            "one",
            [
                step("a", {"kind": "query", "queryId": "q"}, {"kind": "parameter-value", "name": "p"}, capture="value"),
                step(
                    "b",
                    {"kind": "card", "cardId": "c", "parameters": {"p": {"captured": "value"}}},
                    {"kind": "route-reached"},
                ),
            ],
        )
    ],
)
refuse(
    "v1-capture-missing-default",
    "A captured value requires a literal default.",
    missing_default,
    [("missing_default", "/chapters/0/steps/1/target/parameters/p/default")],
)
capture_on_query_run = mutate_step(done={"kind": "query-run"}, capture="value")
refuse(
    "v1-capture-on-query-run",
    "Only parameter-value, record-selected and incident-open may capture.",
    capture_on_query_run,
    [("capture_not_allowed", "/chapters/0/steps/0/capture")],
)
refuse(
    "v1-capture-null-default",
    "A null captured default is allowed only where the target kind defines a fallback.",
    declaration(
        "tour",
        [
            chapter(
                "one",
                [
                    step("a", {"kind": "query", "queryId": "q"}, {"kind": "parameter-value", "name": "p"}, capture="value"),
                    step(
                        "b",
                        {"kind": "query", "queryId": "q2", "parameters": {"p": {"captured": "value", "default": None}}},
                        {"kind": "route-reached"},
                    ),
                ],
            )
        ],
    ),
    [("invalid_value", "/chapters/0/steps/1/target/parameters/p/default")],
)
value_extra = declaration(
    "tour",
    [
        chapter(
            "one",
            [
                step("a", {"kind": "query", "queryId": "q"}, {"kind": "parameter-value", "name": "p"}, capture="value"),
                step(
                    "b",
                    {"kind": "card", "cardId": "c", "parameters": {"p": {"captured": "value", "default": "d", "extra": 1}}},
                    {"kind": "route-reached"},
                ),
            ],
        )
    ],
)
refuse(
    "v1-value-object-extra-field",
    "A captured value object is closed.",
    value_extra,
    [("unknown_field", "/chapters/0/steps/1/target/parameters/p/extra")],
)
value_array_default = declaration(
    "tour",
    [
        chapter(
            "one",
            [
                step("a", {"kind": "query", "queryId": "q"}, {"kind": "parameter-value", "name": "p"}, capture="value"),
                step(
                    "b",
                    {"kind": "card", "cardId": "c", "parameters": {"p": {"captured": "value", "default": [1]}}},
                    {"kind": "route-reached"},
                ),
            ],
        )
    ],
)
refuse(
    "v1-value-default-not-scalar",
    "A captured default is a JSON scalar.",
    value_array_default,
    [("invalid_type", "/chapters/0/steps/1/target/parameters/p/default")],
)

# --- chapter entry, readiness and continuesFrom ---------------------------

none_entry = declaration(
    "tour",
    [chapter("one", [step("one", {"kind": "none"}, {"kind": "acknowledged"})])],
)
refuse(
    "v1-known-none-entry",
    "A known none target cannot be a chapter entry.",
    none_entry,
    [("invalid_entry_target", "/chapters/0/steps/0/target")],
)
missing_entry = declaration(
    "tour",
    [chapter("one", [step("one", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"}, modes=["guided"])])],
)
refuse(
    "v1-missing-explore-entry",
    "A chapter needs at least one Explore step.",
    missing_entry,
    [("missing_explore_entry", "/chapters/0/steps")],
)
readiness_without_continues = mutate_chapter(
    readiness={"kind": "asset-exists", "asset": "query", "id": "q"}
)
refuse(
    "v1-readiness-without-continues",
    "readiness requires continuesFrom.",
    readiness_without_continues,
    [("missing_field", "/chapters/0/continuesFrom")],
)
readiness_wrong_kind = declaration(
    "tour",
    [
        chapter("one", [step("one", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})]),
        chapter(
            "two",
            [step("two", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})],
            continuesFrom="one",
            readiness={"kind": "route-reached"},
        ),
    ],
)
refuse(
    "v1-readiness-wrong-kind",
    "readiness must be an asset-exists condition.",
    readiness_wrong_kind,
    [("invalid_value", "/chapters/1/readiness/kind")],
)
continues_forward = declaration(
    "tour",
    [
        chapter("one", [step("one", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})], continuesFrom="two"),
        chapter("two", [step("two", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})]),
    ],
)
refuse(
    "v1-continues-forward",
    "continuesFrom must name an earlier chapter.",
    continues_forward,
    [("invalid_reference", "/chapters/0/continuesFrom")],
)
continues_self = declaration(
    "tour",
    [chapter("one", [step("one", {"kind": "check", "checkId": "c"}, {"kind": "acknowledged"})], continuesFrom="one")],
)
refuse(
    "v1-continues-self",
    "continuesFrom cannot name its own chapter.",
    continues_self,
    [("invalid_reference", "/chapters/0/continuesFrom")],
)
needs_without_readiness = mutate_step(needs="readiness")
refuse(
    "v1-needs-without-readiness",
    "needs readiness requires the chapter to declare readiness.",
    needs_without_readiness,
    [("invalid_reference", "/chapters/0/steps/0/needs")],
)
needs_invalid = mutate_step(needs="later")
refuse(
    "v1-needs-invalid",
    "needs is a closed union.",
    needs_invalid,
    [("invalid_value", "/chapters/0/steps/0/needs")],
)

# ---------------------------------------------------------------------------
# Write fixtures and corpus
# ---------------------------------------------------------------------------


def main():
    write_corpus(
        CORPUS_VERSION,
        CASES,
        "corpus.json",
        "provenance.json",
        os.path.join("declarations"),
    )
    with open(os.path.join(HERE, "corpus.json"), "rb") as handle:
        base_sha = hashlib.sha256(handle.read()).hexdigest()
    write_corpus(
        CORPUS_VERSION_V2,
        TRAINING_CASES,
        "corpus-v2.json",
        "provenance-v2.json",
        os.path.join("declarations-v2"),
        extra={
            "baseCorpusVersion": CORPUS_VERSION,
            "baseCorpusSha256": base_sha,
        },
    )
    print(f"wrote {len(CASES)} ordinary vectors and {len(TRAINING_CASES)} training vectors")


PROJECTORS = {
    "declaration": None,
    "training-declaration": None,
    "checkpoint-envelope": None,
    "story-registry-entry": None,
}


def write_corpus(corpus_version, cases, corpus_name, provenance_name, declarations_dir, extra=None):
    directory = os.path.join(HERE, declarations_dir)
    if os.path.isdir(directory):
        shutil.rmtree(directory)
    os.makedirs(directory)
    corpus = {"corpusVersion": corpus_version, "vectors": []}
    provenance = {"corpusVersion": corpus_version}
    if extra:
        provenance.update(extra)
    provenance["files"] = {}
    seen = set()
    for case in cases:
        if case["id"] in seen:
            raise SystemExit(f"duplicate case id {case['id']}")
        seen.add(case["id"])
        operation = case.get("operation", "declaration")
        fixture = case["fixture"]
        data = fixture if isinstance(fixture, bytes) else pretty(fixture)
        relative = os.path.join(declarations_dir, case["id"] + ".json")
        path = os.path.join(HERE, relative)
        with open(path, "wb") as handle:
            handle.write(data)
        digest = hashlib.sha256(data).hexdigest()
        provenance["files"][relative] = digest
        vector = {
            "id": case["id"],
            "description": case["description"],
            "operation": operation,
            "inputFile": relative,
            "directoryId": case.get("directory", ""),
        }
        want = {"accepted": False, "newerClient": False}
        if "issues" in case:
            want["issues"] = [
                {"code": code, "path": path_value}
                for code, path_value in case["issues"]
            ]
        elif case.get("newer"):
            want["newerClient"] = True
        else:
            if isinstance(fixture, bytes):
                parsed = normalize_obj(json.loads(fixture.decode("utf-8")))
            else:
                parsed = fixture
            want.update(
                {
                    "accepted": True,
                    "digest": digest,
                    "definition": project(operation, parsed),
                }
            )
        if "digest" not in want:
            want["digest"] = digest
        vector["want"] = want
        corpus["vectors"].append(vector)
    corpus_path = os.path.join(HERE, corpus_name)
    with open(corpus_path, "w", encoding="utf-8") as handle:
        json.dump(corpus, handle, indent=2, ensure_ascii=False)
        handle.write("\n")
    with open(corpus_path, "rb") as handle:
        provenance["corpusSha256"] = hashlib.sha256(handle.read()).hexdigest()
    with open(os.path.join(HERE, provenance_name), "w", encoding="utf-8") as handle:
        json.dump(provenance, handle, indent=2, ensure_ascii=False)
        handle.write("\n")


def project(operation, parsed):
    if operation == "declaration":
        return pv_declaration(parsed)
    if operation == "training-declaration":
        return pv2_declaration(parsed)
    if operation == "checkpoint-envelope":
        return pv2_envelope(parsed)
    if operation == "story-registry-entry":
        return pv2_registry(parsed)
    raise SystemExit(f"unknown corpus operation {operation}")


# ---------------------------------------------------------------------------
# Training /2 declarations, public registry shapes and checkpoint envelopes.
# These vectors select the explicit /2 entry points only; the original v1
# vectors keep selecting ordinary /1 admission unchanged.
# ---------------------------------------------------------------------------

CORPUS_VERSION_V2 = "walkthrough-contracts-v2"
SCHEMA_V2 = "datatug-walkthrough/2"
DECLARATIONS_V2 = os.path.join(HERE, "declarations-v2")

TARGET_KINDS_V2 = set(TARGET_KINDS) | {"challenge", "training-case"}
CONDITION_KINDS_V2 = set(CONDITION_KINDS) | {"dialogue-finished", "challenge-passed"}

PACKAGE = {
    "id": "training-demo",
    "revision": 1,
    "path": "training/package.json",
    "sha256": "a" * 64,
    "bytes": 100,
}
NARRATIVE = {
    "path": "walkthroughs/training-demo/narrative.json",
    "sha256": "b" * 64,
    "bytes": 879,
}
CHALLENGE = {"kind": "challenge", "challengeId": "retry-query", "revision": 1}


def pv2_asset(value):
    return {"path": value["path"], "sha256": value["sha256"], "bytes": str(value["bytes"])}


def pv2_package(value):
    return {
        "id": value["id"],
        "revision": str(value["revision"]),
        **pv2_asset(value),
    }


def pv2_pin(value):
    if value["kind"] == "git":
        return {"kind": "git", "commit": value["commit"].lower()}
    return {"kind": "digest", "sha256": value["sha256"]}


def pv2_target(target):
    kind = target["kind"]
    if kind == "challenge":
        return {
            "kind": kind,
            "support": "supported",
            "challengeId": target["challengeId"],
            "revision": str(target["revision"]),
        }
    if kind == "training-case":
        return {
            "kind": kind,
            "support": "supported",
            "manifestCaseId": target["manifestCaseId"],
            "trainingPackage": pv2_package(target["trainingPackage"]),
        }
    return pv_target(target)


def pv2_condition(condition):
    kind = condition["kind"]
    if kind == "dialogue-finished":
        return {"kind": kind, "support": "supported"}
    if kind == "challenge-passed":
        return {
            "kind": kind,
            "support": "supported",
            "checkpoints": [
                {"id": checkpoint["id"], "completion": checkpoint["completion"]}
                for checkpoint in condition["checkpoints"]
            ],
        }
    return pv_condition(condition)


def pv2_exit(exit_value):
    if exit_value is None:
        return {"kind": "index"}
    if "target" in exit_value:
        return {"kind": "target", "target": pv2_target(exit_value["target"])}
    return pv_exit(exit_value)


def pv2_step(step_value):
    target = step_value["target"]
    done = step_value["done"]
    supported = target["kind"] in TARGET_KINDS_V2 and done["kind"] in CONDITION_KINDS_V2
    return {
        "id": step_value["id"],
        "kind": step_value.get("kind", "standard"),
        "segmentId": step_value.get("segmentId", ""),
        "modes": step_value.get("modes", MODES_DEFAULT),
        "support": "supported" if supported else "needs-newer",
        "capture": step_value.get("capture", ""),
        "needs": step_value.get("needs", ""),
        "target": pv2_target(target),
        "done": pv2_condition(done),
    }


def pv2_chapter(chapter_value):
    steps = [pv2_step(item) for item in chapter_value["steps"]]
    entry = ""
    for step_value in chapter_value["steps"]:
        if "explore" in step_value.get("modes", MODES_DEFAULT):
            entry = step_value["id"]
            break
    return {
        "id": chapter_value["id"],
        "formerIds": chapter_value.get("formerIds", []),
        "requires": chapter_value.get("requires", "anonymous"),
        "requiresReason": chapter_value.get("requiresReason", ""),
        "continuesFrom": chapter_value.get("continuesFrom", ""),
        "readiness": (
            None
            if "readiness" not in chapter_value
            else pv2_condition(chapter_value["readiness"])
        ),
        "entryStepId": entry,
        "steps": steps,
    }


def pv2_declaration(value):
    return {
        "id": value["id"],
        "version": str(value["version"]),
        "summary": value.get("summary", ""),
        "trainingPackage": pv2_package(value["trainingPackage"]),
        "narrative": pv2_asset(value["narrative"]),
        "chapters": [pv2_chapter(item) for item in value["chapters"]],
        "finish": pv2_exit(value.get("finish")),
    }


def pv2_envelope(value):
    return {
        "projectPresent": True,
        "authorityGeneration": str(value["authorityGeneration"]),
        "contentPin": pv2_pin(value["contentPin"]),
        "declarationSha256": value["declarationSha256"].lower(),
        "narrativeSha256": value["narrativeSha256"].lower(),
        "trainingPackageSha256": value["trainingPackageSha256"].lower(),
        "walkthroughId": value["walkthroughId"],
        "chapterId": value["chapterId"],
        "stepId": value["stepId"],
        "challengeId": value["challengeId"],
        "challengeRevision": str(value["challengeRevision"]),
        "checkpointId": value["checkpointId"],
        "attemptId": value["attemptId"],
        "submissionGeneration": str(value["submissionGeneration"]),
        "outcome": value["outcome"],
        "complete": value["complete"],
        "assisted": value["assisted"],
    }


def pv2_registry(value):
    return {
        "storyId": value["storyId"],
        "revision": str(value["revision"]),
        "projectUrl": value["projectUrl"],
        "walkthroughId": value["walkthroughId"],
        "contentPin": pv2_pin(value["contentPin"]),
        "declarationSha256": value["declarationSha256"].lower(),
    }


TRAINING_CASES = []


def accept2(case_id, description, fixture, directory=None):
    TRAINING_CASES.append(
        {
            "id": case_id,
            "description": description,
            "operation": "training-declaration",
            "fixture": fixture,
            "directory": directory or fixture["id"],
        }
    )


def refuse2(case_id, description, fixture, issues, directory="training-demo"):
    TRAINING_CASES.append(
        {
            "id": case_id,
            "description": description,
            "operation": "training-declaration",
            "fixture": fixture,
            "directory": directory,
            "issues": issues,
        }
    )


def newer2(case_id, description, fixture):
    TRAINING_CASES.append(
        {
            "id": case_id,
            "description": description,
            "operation": "training-declaration",
            "fixture": fixture,
            "directory": fixture["id"],
            "newer": True,
        }
    )


def shape_case(case_id, description, operation, fixture, issues=None):
    case = {
        "id": case_id,
        "description": description,
        "operation": operation,
        "fixture": fixture,
        "directory": "",
    }
    if issues is not None:
        case["issues"] = issues
    TRAINING_CASES.append(case)


def challenge_step(step_id="solve", **extra):
    value = {
        "id": step_id,
        "kind": "challenge",
        "title": f"Step {step_id}",
        "why": f"Why {step_id}",
        "target": copy.deepcopy(CHALLENGE),
        "done": {
            "kind": "challenge-passed",
            "checkpoints": [{"id": "result", "completion": "whole-challenge"}],
        },
    }
    value.update(extra)
    return value


def dialogue_step(step_id="discussion", **extra):
    value = {
        "id": step_id,
        "kind": "scripted-dialogue",
        "segmentId": step_id,
        "title": f"Step {step_id}",
        "why": f"Why {step_id}",
        "modes": ["guided"],
        "target": {"kind": "none"},
        "done": {"kind": "dialogue-finished"},
    }
    value.update(extra)
    return value


def standard_step(step_id="inspect", target=None, done=None, **extra):
    value = {
        "id": step_id,
        "title": f"Step {step_id}",
        "why": f"Why {step_id}",
        "target": copy.deepcopy(target)
        if target is not None
        else {"kind": "check", "checkId": "check-1"},
        "done": copy.deepcopy(done)
        if done is not None
        else {"kind": "acknowledged"},
    }
    value.update(extra)
    return value


def training_chapter(chapter_id="retries", steps=None, **extra):
    value = {
        "id": chapter_id,
        "title": f"Chapter {chapter_id}",
        "why": f"Why {chapter_id}",
        "steps": copy.deepcopy(steps)
        if steps is not None
        else [challenge_step(), dialogue_step()],
    }
    value.update(extra)
    return value


def training_declaration(decl_id="training-demo", chapters=None, **extra):
    value = {
        "schema": SCHEMA_V2,
        "id": decl_id,
        "version": 1,
        "title": f"Walkthrough {decl_id}",
        "trainingPackage": copy.deepcopy(PACKAGE),
        "narrative": copy.deepcopy(NARRATIVE),
        "chapters": copy.deepcopy(chapters)
        if chapters is not None
        else [training_chapter()],
    }
    value.update(extra)
    return value


def mutate2(**changes):
    value = training_declaration()
    value.update(changes)
    return value


def mutate2_step(step_index, **changes):
    value = training_declaration()
    value["chapters"][0]["steps"][step_index].update(changes)
    return value


def mutate2_target(**changes):
    value = training_declaration()
    value["chapters"][0]["steps"][0]["target"].update(changes)
    return value


def mutate2_done(**changes):
    value = training_declaration()
    value["chapters"][0]["steps"][0]["done"].update(changes)
    return value


def mutate2_package(**changes):
    value = training_declaration()
    value["trainingPackage"].update(changes)
    return value


def mutate2_narrative(**changes):
    value = training_declaration()
    value["narrative"].update(changes)
    return value


# --- accepted training declarations ----------------------------------------

accept2(
    "v2-minimal",
    "A structurally valid /2 declaration: challenge and guided dialogue steps, pinned package and narrative.",
    training_declaration(),
)
accept2(
    "v2-standard-training-case",
    "An ordinary standard step may present a training Case whose package reference equals the top reference.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    standard_step(
                        "open-case",
                        {
                            "kind": "training-case",
                            "manifestCaseId": "case-1",
                            "trainingPackage": copy.deepcopy(PACKAGE),
                        },
                        {"kind": "route-reached"},
                    )
                ]
            )
        ]
    ),
)
accept2(
    "v2-ordinary-fields",
    "All ordinary /1 fields remain admissible in a /2 declaration.",
    training_declaration(
        chapters=[
            training_chapter(
                "find",
                [
                    standard_step(
                        "pick",
                        {"kind": "query", "queryId": "customer-orders"},
                        {"kind": "parameter-value", "name": "CustomerID"},
                        capture="customer",
                    ),
                    standard_step(
                        "open",
                        {"kind": "check", "checkId": "check-1"},
                        {"kind": "check-run"},
                    ),
                ],
            )
        ]
    ),
)
accept2(
    "v2-unknown-target-step-local",
    "An unknown target kind inside a challenge step stays step-local newer-client, without a fatal pairing error.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "future-challenge"})]
            )
        ]
    ),
)
accept2(
    "v2-unknown-condition-step-local",
    "An unknown condition kind inside a dialogue step stays step-local newer-client.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(),
                    dialogue_step(done={"kind": "future-dialogue"}),
                ]
            )
        ]
    ),
)
accept2(
    "v2-challenge-id-64",
    "A 64-character challenge slug is checked by the challenge contract, not the 48-character walkthrough grammar.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        target={
                            "kind": "challenge",
                            "challengeId": "a" * 64,
                            "revision": 1,
                        }
                    )
                ]
            )
        ]
    ),
)
accept2(
    "v2-revision-1000000",
    "The challenge revision upper bound 1000000 is admitted.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        target={
                            "kind": "challenge",
                            "challengeId": "retry-query",
                            "revision": 1000000,
                        }
                    )
                ]
            )
        ]
    ),
)
accept2(
    "v2-large-integers",
    "2^53, the uint64 boundary and a far larger byte length survive as exact lexemes with no machine-integer cap.",
    training_declaration(
        version=9007199254740992,
        trainingPackage={
            "id": "training-demo",
            "revision": 18446744073709551615,
            "path": "training/package.json",
            "sha256": "c" * 64,
            "bytes": 9007199254740993,
        },
        narrative={
            "path": "walkthroughs/training-demo/narrative.json",
            "sha256": "d" * 64,
            "bytes": 10 ** 30,
        },
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        target={
                            "kind": "challenge",
                            "challengeId": "retry-query",
                            "revision": 1000000,
                        }
                    ),
                    dialogue_step(),
                ]
            )
        ],
    ),
)
accept2(
    "v2-checkpoint-same-id-across-chapters",
    "Checkpoint ids are unique within a chapter, not across the whole walkthrough.",
    training_declaration(
        chapters=[
            training_chapter("first", [challenge_step("solve-first")]),
            training_chapter("second", [challenge_step("solve-second")]),
        ]
    ),
)
accept2(
    "v2-finish-challenge-target",
    "A finish exit may use the /2 challenge target registry.",
    training_declaration(
        finish={
            "target": {
                "kind": "challenge",
                "challengeId": "retry-query",
                "revision": 1,
            }
        }
    ),
)

# --- refused training declarations -----------------------------------------

missing_package = training_declaration()
del missing_package["trainingPackage"]
refuse2(
    "v2-missing-package",
    "The top-level trainingPackage reference is required.",
    missing_package,
    [("missing_field", "/trainingPackage")],
)

missing_narrative = training_declaration()
del missing_narrative["narrative"]
refuse2(
    "v2-missing-narrative",
    "The top-level narrative asset reference is required.",
    missing_narrative,
    [("missing_field", "/narrative")],
)

refuse2(
    "v2-unknown-top-field",
    "An unknown top-level field rejects; /2 does not ignore typos.",
    mutate2(extra=True),
    [("unknown_field", "/extra")],
)
refuse2(
    "v2-unknown-step-kind",
    "The /2 step kind registry is closed.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(kind="future-step"), dialogue_step()]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/kind")],
)
refuse2(
    "v2-standard-segment-id",
    "A standard step cannot carry a dialogue segmentId.",
    training_declaration(
        chapters=[
            training_chapter(steps=[standard_step("inspect", segmentId="segment")])
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/segmentId")],
)
missing_segment = training_declaration()
del missing_segment["chapters"][0]["steps"][1]["segmentId"]
refuse2(
    "v2-dialogue-missing-segment",
    "A scripted-dialogue step requires its sidecar segmentId.",
    missing_segment,
    [("missing_field", "/chapters/0/steps/1/segmentId")],
)
refuse2(
    "v2-dialogue-target-not-none",
    "A scripted-dialogue target must be none.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(),
                    dialogue_step(target={"kind": "query", "queryId": "q"}),
                ]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/1/target/kind")],
)
refuse2(
    "v2-dialogue-wrong-done",
    "A scripted-dialogue step ends only with dialogue-finished.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(),
                    dialogue_step(done={"kind": "route-reached"}),
                ]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/1/done/kind")],
)
refuse2(
    "v2-dialogue-capture",
    "A scripted-dialogue step cannot capture.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(), dialogue_step(capture="incident")]
            )
        ]
    ),
    [("capture_not_allowed", "/chapters/0/steps/1/capture")],
)
refuse2(
    "v2-dialogue-needs",
    "A scripted-dialogue step cannot declare needs.",
    training_declaration(
        chapters=[
            training_chapter("find", [standard_step("pick")]),
            training_chapter(
                "retries",
                [challenge_step(), dialogue_step(needs="readiness")],
                continuesFrom="find",
                readiness={"kind": "asset-exists", "asset": "query", "id": "q"},
            ),
        ]
    ),
    [("invalid_value", "/chapters/1/steps/1/needs")],
)
refuse2(
    "v2-challenge-segment-id",
    "A challenge step cannot carry a dialogue segmentId.",
    training_declaration(
        chapters=[training_chapter(steps=[challenge_step(segmentId="segment")])]
    ),
    [("invalid_value", "/chapters/0/steps/0/segmentId")],
)
refuse2(
    "v2-challenge-wrong-target",
    "A challenge step target must be a challenge target.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(target={"kind": "query", "queryId": "q"})
                ]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/kind")],
)
refuse2(
    "v2-challenge-wrong-done",
    "A challenge step done must be challenge-passed.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(done={"kind": "route-reached"})]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/done/kind")],
)
refuse2(
    "v2-standard-challenge-target",
    "A challenge target requires a challenge step.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[standard_step("inspect", copy.deepcopy(CHALLENGE))]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/kind")],
)
refuse2(
    "v2-standard-challenge-passed",
    "A challenge-passed condition requires a challenge step.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    standard_step(
                        "inspect",
                        done={
                            "kind": "challenge-passed",
                            "checkpoints": [
                                {"id": "result", "completion": "whole-challenge"}
                            ],
                        },
                    )
                ]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/done/kind")],
)
refuse2(
    "v2-challenge-capture",
    "A challenge step cannot capture.",
    training_declaration(
        chapters=[
            training_chapter(steps=[challenge_step(capture="incident")])
        ]
    ),
    [("capture_not_allowed", "/chapters/0/steps/0/capture")],
)
refuse2(
    "v2-checkpoints-missing",
    "challenge-passed requires its one checkpoint mapping.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(done={"kind": "challenge-passed"})]
            )
        ]
    ),
    [("missing_field", "/chapters/0/steps/0/done/checkpoints")],
)
refuse2(
    "v2-checkpoints-two",
    "Exactly one checkpoint mapping is admitted per challenge step.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        done={
                            "kind": "challenge-passed",
                            "checkpoints": [
                                {"id": "result", "completion": "whole-challenge"},
                                {"id": "other", "completion": "whole-challenge"},
                            ],
                        }
                    ),
                    dialogue_step(),
                ]
            )
        ]
    ),
    [("too_many", "/chapters/0/steps/0/done/checkpoints")],
)
refuse2(
    "v2-checkpoints-empty",
    "An empty checkpoint mapping array is refused.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        done={"kind": "challenge-passed", "checkpoints": []}
                    ),
                    dialogue_step(),
                ]
            )
        ]
    ),
    [("too_many", "/chapters/0/steps/0/done/checkpoints")],
)
refuse2(
    "v2-checkpoints-not-array",
    "checkpoints must be an array.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        done={"kind": "challenge-passed", "checkpoints": {}}
                    ),
                    dialogue_step(),
                ]
            )
        ]
    ),
    [("invalid_type", "/chapters/0/steps/0/done/checkpoints")],
)
refuse2(
    "v2-checkpoint-extra-field",
    "A checkpoint mapping has exactly id and completion.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        done={
                            "kind": "challenge-passed",
                            "checkpoints": [
                                {
                                    "id": "result",
                                    "completion": "whole-challenge",
                                    "extra": True,
                                }
                            ],
                        }
                    ),
                    dialogue_step(),
                ]
            )
        ]
    ),
    [("unknown_field", "/chapters/0/steps/0/done/checkpoints/0/extra")],
)
refuse2(
    "v2-checkpoint-completion-part",
    "Only whole-challenge completion is representable; part-level mappings are not invented.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        done={
                            "kind": "challenge-passed",
                            "checkpoints": [
                                {"id": "result", "completion": "part"}
                            ],
                        }
                    ),
                    dialogue_step(),
                ]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/done/checkpoints/0/completion")],
)
refuse2(
    "v2-checkpoint-missing-id",
    "A checkpoint mapping requires its walkthrough-owned id.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        done={
                            "kind": "challenge-passed",
                            "checkpoints": [
                                {"completion": "whole-challenge"}
                            ],
                        }
                    ),
                    dialogue_step(),
                ]
            )
        ]
    ),
    [("missing_field", "/chapters/0/steps/0/done/checkpoints/0/id")],
)
refuse2(
    "v2-checkpoint-duplicate-in-chapter",
    "A checkpoint id repeated within one chapter rejects.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step("solve"), challenge_step("solve-again")]
            )
        ]
    ),
    [("duplicate_id", "/chapters/0/steps/1/done/checkpoints/0/id")],
)
refuse2(
    "v2-training-case-package-mismatch",
    "A training-case package reference must equal the top reference field for field.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    standard_step(
                        "open-case",
                        {
                            "kind": "training-case",
                            "manifestCaseId": "case-1",
                            "trainingPackage": {
                                "id": "other-package",
                                "revision": 1,
                                "path": "training/package.json",
                                "sha256": "a" * 64,
                                "bytes": 100,
                            },
                        },
                        {"kind": "route-reached"},
                    )
                ]
            )
        ]
    ),
    [("mismatch", "/chapters/0/steps/0/target/trainingPackage/id")],
)
refuse2(
    "v2-training-case-missing-package",
    "A training-case target requires its package reference.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    standard_step(
                        "open-case",
                        {"kind": "training-case", "manifestCaseId": "case-1"},
                        {"kind": "route-reached"},
                    )
                ]
            )
        ]
    ),
    [("missing_field", "/chapters/0/steps/0/target/trainingPackage")],
)
refuse2(
    "v2-challenge-id-uppercase",
    "A challenge id is a lowercase slug.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "challenge", "challengeId": "Retry", "revision": 1})]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/challengeId")],
)
refuse2(
    "v2-challenge-id-65",
    "A 65-character challenge id exceeds the owner contract.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        target={
                            "kind": "challenge",
                            "challengeId": "a" * 65,
                            "revision": 1,
                        }
                    )
                ]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/challengeId")],
)
refuse2(
    "v2-challenge-id-leading-digit",
    "A challenge id starts with a letter.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "challenge", "challengeId": "1retry", "revision": 1})]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/challengeId")],
)
refuse2(
    "v2-revision-1000001",
    "A challenge revision above 1000000 is refused by the owner contract.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "challenge", "challengeId": "retry-query", "revision": 1000001})]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/revision")],
)
refuse2(
    "v2-revision-zero",
    "A challenge revision is at least 1.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "challenge", "challengeId": "retry-query", "revision": 0})]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/revision")],
)
refuse2(
    "v2-revision-fraction",
    "A challenge revision is an integer.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "challenge", "challengeId": "retry-query", "revision": 1.5})]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/revision")],
)
refuse2(
    "v2-revision-exponent",
    "A challenge revision is not an exponent spelling.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "challenge", "challengeId": "retry-query", "revision": 1e3})]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/revision")],
)
refuse2(
    "v2-revision-negative",
    "A challenge revision is positive.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "challenge", "challengeId": "retry-query", "revision": -1})]
            )
        ]
    ),
    [("invalid_value", "/chapters/0/steps/0/target/revision")],
)
refuse2(
    "v2-revision-null",
    "A challenge revision is a number, not null.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "challenge", "challengeId": "retry-query", "revision": None})]
            )
        ]
    ),
    [("invalid_type", "/chapters/0/steps/0/target/revision")],
)
refuse2(
    "v2-package-path-absolute",
    "An asset path is project-root-relative, never absolute.",
    mutate2_package(path="/training/package.json"),
    [("invalid_path", "/trainingPackage/path")],
)
refuse2(
    "v2-package-path-dotdot",
    "An asset path has no .. segment.",
    mutate2_package(path="training/../package.json"),
    [("invalid_path", "/trainingPackage/path")],
)
refuse2(
    "v2-package-path-dot",
    "An asset path has no . segment.",
    mutate2_package(path="training/./package.json"),
    [("invalid_path", "/trainingPackage/path")],
)
refuse2(
    "v2-package-path-backslash",
    "An asset path has no backslash.",
    mutate2_package(path="training\\package.json"),
    [("invalid_path", "/trainingPackage/path")],
)
refuse2(
    "v2-package-path-scheme",
    "An asset path has no URL scheme.",
    mutate2_package(path="https://example.com/package.json"),
    [("invalid_path", "/trainingPackage/path")],
)
refuse2(
    "v2-package-path-query",
    "An asset path has no query or fragment.",
    mutate2_package(path="training/package.json?v=1"),
    [("invalid_path", "/trainingPackage/path")],
)
refuse2(
    "v2-package-path-trailing-slash",
    "An asset path has no empty segment.",
    mutate2_package(path="training/"),
    [("invalid_path", "/trainingPackage/path")],
)
refuse2(
    "v2-narrative-path-dot",
    "The narrative asset path follows the same safe path rules.",
    mutate2_narrative(path="./narrative.json"),
    [("invalid_path", "/narrative/path")],
)
refuse2(
    "v2-package-sha-uppercase",
    "An asset digest is 64 lowercase hexadecimal characters.",
    mutate2_package(sha256="A" * 64),
    [("invalid_value", "/trainingPackage/sha256")],
)
refuse2(
    "v2-package-bytes-zero",
    "An asset byte count is positive.",
    mutate2_package(bytes=0),
    [("invalid_value", "/trainingPackage/bytes")],
)
refuse2(
    "v2-package-revision-zero",
    "A package revision is positive.",
    mutate2_package(revision=0),
    [("invalid_value", "/trainingPackage/revision")],
)
refuse2(
    "v2-package-extra-field",
    "A package reference has exactly its five fields.",
    mutate2_package(extra=True),
    [("unknown_field", "/trainingPackage/extra")],
)
missing_package_id = training_declaration()
del missing_package_id["trainingPackage"]["id"]
refuse2(
    "v2-package-missing-id",
    "A package reference requires its id.",
    missing_package_id,
    [("missing_field", "/trainingPackage/id")],
)
missing_narrative_bytes = training_declaration()
del missing_narrative_bytes["narrative"]["bytes"]
refuse2(
    "v2-narrative-missing-bytes",
    "An asset reference requires its byte length.",
    missing_narrative_bytes,
    [("missing_field", "/narrative/bytes")],
)
refuse2(
    "v2-schema-v1-in-training",
    "The explicit /2 entry point does not down-convert an ordinary /1 declaration.",
    mutate2(schema="datatug-walkthrough/1"),
    [("invalid_value", "/schema")],
)
refuse2(
    "v2-schema-0",
    "Schema major 0 is not later than /2.",
    mutate2(schema="datatug-walkthrough/0"),
    [("invalid_value", "/schema")],
)
newer2(
    "v2-schema-3-newer",
    "A later /2+1 schema major needs a newer client.",
    mutate2(schema="datatug-walkthrough/3"),
)
newer2(
    "v2-schema-99-newer",
    "A far later schema major needs a newer client.",
    mutate2(schema="datatug-walkthrough/99"),
)
refuse2(
    "v2-dialogue-entry-target",
    "A chapter's first Explore step must not be a none-target dialogue.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    dialogue_step(modes=["guided", "explore"]),
                    challenge_step(),
                ]
            )
        ]
    ),
    [("invalid_entry_target", "/chapters/0/steps/0/target")],
)
refuse2(
    "v2-missing-explore-entry",
    "Every /2 chapter still requires a navigable Explore entry.",
    training_declaration(
        chapters=[training_chapter(steps=[dialogue_step()])]
    ),
    [("missing_explore_entry", "/chapters/0/steps")],
)

# --- checkpoint envelope shapes --------------------------------------------

PROJECT_REF = {
    "storeId": "github.com",
    "projectId": "datatug-demo-project@datatug@demo-project-1",
    "branch": "main",
}
ENVELOPE_COMMIT = "0123456789abcdef0123456789abcdef01234567"


def envelope(**overrides):
    value = {
        "project": copy.deepcopy(PROJECT_REF),
        "authorityGeneration": 1,
        "contentPin": {"kind": "git", "commit": ENVELOPE_COMMIT},
        "declarationSha256": "c" * 64,
        "narrativeSha256": "b" * 64,
        "trainingPackageSha256": "a" * 64,
        "walkthroughId": "training-demo",
        "chapterId": "retries",
        "stepId": "solve",
        "challengeId": "retry-query",
        "challengeRevision": 1,
        "checkpointId": "result",
        "attemptId": "attempt-1",
        "submissionGeneration": 1,
        "outcome": "passed",
        "complete": True,
        "assisted": False,
    }
    value.update(overrides)
    return value


def mutate_envelope(**changes):
    value = envelope()
    value.update(changes)
    return value


def envelope_case(case_id, description, fixture, issues=None):
    shape_case(case_id, description, "checkpoint-envelope", fixture, issues)


envelope_case(
    "envelope-valid",
    "A fully consistent passed and complete validation envelope with independent assistance.",
    envelope(assisted=True),
)
envelope_case(
    "envelope-hex-case",
    "Repeated digests and a Git commit are accepted in either hex case and normalized lowercase.",
    envelope(
        contentPin={"kind": "git", "commit": ENVELOPE_COMMIT.upper()},
        declarationSha256="C" * 64,
        narrativeSha256="B" * 64,
        trainingPackageSha256="A" * 64,
    ),
)
envelope_case(
    "envelope-attempt-min-1",
    "A one-character attempt id is the minimum.",
    envelope(attemptId="x"),
)
envelope_case(
    "envelope-attempt-max-128",
    "A 128-character printable ASCII attempt id is the maximum.",
    envelope(attemptId="a" * 128),
)
envelope_case(
    "envelope-attempt-empty",
    "An empty attempt id is refused.",
    envelope(attemptId=""),
    [("invalid_value", "/attemptId")],
)
envelope_case(
    "envelope-attempt-129",
    "A 129-character attempt id exceeds the owner bound.",
    envelope(attemptId="a" * 129),
    [("invalid_value", "/attemptId")],
)
envelope_case(
    "envelope-attempt-control",
    "A control character is not printable ASCII.",
    envelope(attemptId="line\nbreak"),
    [("invalid_value", "/attemptId")],
)
envelope_case(
    "envelope-attempt-non-ascii",
    "A non-ASCII attempt id is refused.",
    envelope(attemptId="attempté"),
    [("invalid_value", "/attemptId")],
)
envelope_case(
    "envelope-authority-2-53",
    "2^53 survives exactly as an authority generation lexeme.",
    envelope(authorityGeneration=9007199254740992),
)
envelope_case(
    "envelope-authority-uint64",
    "The uint64 boundary survives exactly as an authority generation lexeme.",
    envelope(authorityGeneration=18446744073709551615),
)
envelope_case(
    "envelope-authority-huge",
    "A far larger authority generation lexeme is not truncated.",
    envelope(authorityGeneration=10 ** 30),
)
envelope_case(
    "envelope-authority-zero",
    "An authority generation is positive.",
    envelope(authorityGeneration=0),
    [("invalid_value", "/authorityGeneration")],
)
envelope_case(
    "envelope-authority-negative",
    "An authority generation is positive.",
    envelope(authorityGeneration=-1),
    [("invalid_value", "/authorityGeneration")],
)
envelope_case(
    "envelope-authority-fraction",
    "An authority generation is an integer.",
    envelope(authorityGeneration=1.5),
    [("invalid_value", "/authorityGeneration")],
)
envelope_case(
    "envelope-authority-exponent",
    "An authority generation is not an exponent spelling.",
    envelope(authorityGeneration=1e3),
    [("invalid_value", "/authorityGeneration")],
)
envelope_case(
    "envelope-authority-null",
    "An authority generation is a number, not null.",
    envelope(authorityGeneration=None),
    [("invalid_type", "/authorityGeneration")],
)
envelope_case(
    "envelope-submission-2-53",
    "A submission generation uses the same lossless positive-integer rule.",
    envelope(submissionGeneration=9007199254740992),
)
envelope_case(
    "envelope-challenge-revision-1000001",
    "A challenge revision above the owner bound is refused.",
    envelope(challengeRevision=1000001),
    [("invalid_value", "/challengeRevision")],
)
envelope_case(
    "envelope-challenge-revision-zero",
    "A challenge revision is at least 1.",
    envelope(challengeRevision=0),
    [("invalid_value", "/challengeRevision")],
)
envelope_case(
    "envelope-outcome-invalid",
    "The outcome registry is closed.",
    envelope(outcome="skipped"),
    [("invalid_value", "/outcome")],
)
envelope_case(
    "envelope-passed-incomplete",
    "A passed outcome with complete false is an inconsistent summary.",
    envelope(complete=False),
    [("inconsistent_summary", "/complete")],
)
envelope_case(
    "envelope-failed-complete",
    "A non-passed outcome with complete true is an inconsistent summary.",
    envelope(outcome="failed", complete=True, assisted=True),
    [("inconsistent_summary", "/complete")],
)
envelope_case(
    "envelope-reference-namespace",
    "A reference-only result is not this envelope type; a namespace label is an unknown field.",
    envelope(namespace="reference"),
    [("unknown_field", "/namespace")],
)
envelope_case(
    "envelope-test-namespace",
    "A Test-mode result is not this envelope type.",
    envelope(mode="test"),
    [("unknown_field", "/mode")],
)
envelope_case(
    "envelope-extra-sql",
    "No SQL, rows, evidence or transcript may travel in the envelope.",
    envelope(sql="select 1"),
    [("unknown_field", "/sql")],
)
envelope_case(
    "envelope-missing-project",
    "The project reference is required.",
    {key: value for key, value in envelope().items() if key != "project"},
    [("missing_field", "/project")],
)
envelope_case(
    "envelope-null-project",
    "The project reference is not null.",
    envelope(project=None),
    [("invalid_type", "/project")],
)
envelope_case(
    "envelope-digest-short",
    "Each repeated digest is a full 64-hex digest.",
    envelope(declarationSha256="abc"),
    [("invalid_value", "/declarationSha256")],
)
envelope_case(
    "envelope-pin-branch",
    "A content pin is one exact immutable pin, never a branch name.",
    envelope(contentPin={"kind": "git", "commit": "main"}),
    [("invalid_value", "/contentPin/commit")],
)
envelope_case(
    "envelope-pin-uppercase-digest",
    "A digest pin is lowercase as the owner specifies.",
    envelope(contentPin={"kind": "digest", "sha256": "A" * 64}),
    [("invalid_value", "/contentPin/sha256")],
)
envelope_case(
    "envelope-complete-missing",
    "Every envelope field is required.",
    {key: value for key, value in envelope().items() if key != "complete"},
    [("missing_field", "/complete")],
)
envelope_case(
    "envelope-assisted-missing",
    "Assistance is an explicit required field.",
    {key: value for key, value in envelope().items() if key != "assisted"},
    [("missing_field", "/assisted")],
)
envelope_case(
    "envelope-walkthrough-id-uppercase",
    "Envelope identifiers use the walkthrough ID grammar.",
    envelope(walkthroughId="Training-Demo"),
    [("invalid_value", "/walkthroughId")],
)

# --- additional structural boundaries --------------------------------------

refuse2(
    "v2-step-kind-not-string",
    "A /2 step kind is a string.",
    training_declaration(
        chapters=[training_chapter(steps=[challenge_step(kind=1)])]
    ),
    [("invalid_type", "/chapters/0/steps/0/kind")],
)
refuse2(
    "v2-package-not-object",
    "A package reference is a JSON object.",
    mutate2(trainingPackage="training/package.json"),
    [("invalid_type", "/trainingPackage")],
)
refuse2(
    "v2-narrative-not-object",
    "An asset reference is a JSON object.",
    mutate2(narrative=[]),
    [("invalid_type", "/narrative")],
)
refuse2(
    "v2-package-path-not-string",
    "An asset path is a string.",
    mutate2_package(path=1),
    [("invalid_type", "/trainingPackage/path")],
)
missing_package_sha = training_declaration()
del missing_package_sha["trainingPackage"]["sha256"]
refuse2(
    "v2-package-sha-missing",
    "A package reference requires its digest.",
    missing_package_sha,
    [("missing_field", "/trainingPackage/sha256")],
)
refuse2(
    "v2-challenge-id-missing",
    "A challenge target requires its challenge id.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[challenge_step(target={"kind": "challenge", "revision": 1})]
            )
        ]
    ),
    [("missing_field", "/chapters/0/steps/0/target/challengeId")],
)
refuse2(
    "v2-checkpoint-not-object",
    "A checkpoint mapping is a JSON object.",
    training_declaration(
        chapters=[
            training_chapter(
                steps=[
                    challenge_step(
                        done={"kind": "challenge-passed", "checkpoints": ["result"]}
                    ),
                    dialogue_step(),
                ]
            )
        ]
    ),
    [("invalid_type", "/chapters/0/steps/0/done/checkpoints/0")],
)
envelope_case(
    "envelope-pin-not-object",
    "A content pin is a JSON object.",
    envelope(contentPin="git"),
    [("invalid_type", "/contentPin")],
)
envelope_case(
    "envelope-pin-missing-kind",
    "A content pin requires its kind.",
    envelope(contentPin={"commit": ENVELOPE_COMMIT}),
    [("missing_field", "/contentPin/kind")],
)
envelope_case(
    "envelope-pin-kind-not-string",
    "A content pin kind is a string.",
    envelope(contentPin={"kind": 1}),
    [("invalid_type", "/contentPin/kind")],
)
envelope_case(
    "envelope-pin-git-missing-commit",
    "A Git content pin requires its commit.",
    envelope(contentPin={"kind": "git"}),
    [("missing_field", "/contentPin/commit")],
)
envelope_case(
    "envelope-pin-unknown-kind",
    "A content pin kind is git or digest.",
    envelope(contentPin={"kind": "future", "sha256": "e" * 64}),
    [("invalid_value", "/contentPin/kind")],
)
envelope_case(
    "envelope-missing-content-pin",
    "The content pin is required.",
    {key: value for key, value in envelope().items() if key != "contentPin"},
    [("missing_field", "/contentPin")],
)
envelope_case(
    "envelope-missing-attempt",
    "The attempt id is required.",
    {key: value for key, value in envelope().items() if key != "attemptId"},
    [("missing_field", "/attemptId")],
)
envelope_case(
    "envelope-complete-not-boolean",
    "Whole-attempt completion is a boolean.",
    envelope(complete="true"),
    [("invalid_type", "/complete")],
)
envelope_case(
    "envelope-missing-declaration-sha",
    "Every repeated digest is required.",
    {key: value for key, value in envelope().items() if key != "declarationSha256"},
    [("missing_field", "/declarationSha256")],
)

# --- public story registry entry shapes ------------------------------------

REGISTRY_URL = "/project/github.com/datatug/datatug-demo-project/demo-project-1"


def registry(**overrides):
    value = {
        "storyId": "phantom-orders",
        "revision": 1,
        "projectUrl": REGISTRY_URL,
        "walkthroughId": "training-demo",
        "contentPin": {"kind": "git", "commit": ENVELOPE_COMMIT},
        "declarationSha256": "c" * 64,
    }
    value.update(overrides)
    return value


def registry_case(case_id, description, fixture, issues=None):
    shape_case(case_id, description, "story-registry-entry", fixture, issues)


registry_case(
    "registry-valid",
    "A closed registry entry shape with a Git commit pin.",
    registry(),
)
registry_case(
    "registry-hex-case",
    "The repeated declaration digest accepts either hex case and normalizes lowercase.",
    registry(declarationSha256="C" * 64, contentPin={"kind": "git", "commit": ENVELOPE_COMMIT.upper()}),
)
registry_case(
    "registry-digest-pin",
    "A cloud/local registry entry pins an exact content digest.",
    registry(contentPin={"kind": "digest", "sha256": "e" * 64}),
)
registry_case(
    "registry-extra-field",
    "A registry entry has exactly its six fields.",
    registry(extra=True),
    [("unknown_field", "/extra")],
)
registry_case(
    "registry-story-id-invalid",
    "A story id uses the walkthrough ID grammar.",
    registry(storyId="Phantom"),
    [("invalid_value", "/storyId")],
)
registry_case(
    "registry-revision-zero",
    "A story revision is positive.",
    registry(revision=0),
    [("invalid_value", "/revision")],
)
registry_case(
    "registry-revision-2-53",
    "A story revision is a lossless positive integer lexeme.",
    registry(revision=9007199254740992),
)
registry_case(
    "registry-project-url-empty",
    "A project URL must not be empty.",
    registry(projectUrl=""),
    [("invalid_value", "/projectUrl")],
)
registry_case(
    "registry-missing-content-pin",
    "A registry entry requires its content pin.",
    {key: value for key, value in registry().items() if key != "contentPin"},
    [("missing_field", "/contentPin")],
)
registry_case(
    "registry-walkthrough-id-49",
    "A walkthrough id is at most 48 characters.",
    registry(walkthroughId="a" * 49),
    [("invalid_value", "/walkthroughId")],
)
registry_case(
    "registry-declaration-sha-short",
    "A declaration digest is a full 64-hex digest.",
    registry(declarationSha256="abc"),
    [("invalid_value", "/declarationSha256")],
)


if __name__ == "__main__":
    main()
