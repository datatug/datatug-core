#!/usr/bin/env python3
"""Independent generator for the walkthrough narrative conformance corpus.

This script authors raw bytes and expected projections from the owner tables.
It never calls the Go or TypeScript implementation. Both languages assert the
same complete inventory and copy these bytes byte-for-byte.
"""
from __future__ import annotations

import hashlib
import json
from copy import deepcopy
from pathlib import Path

ROOT = Path(__file__).resolve().parent
NARRATIVE = ROOT / "narrative"
PACKAGE_ID = "training-demo"
PACKAGE_REVISION = 1
PACKAGE_SHA = "a" * 64
PACKAGE_BYTES = 100
PHOTO_SHA = "b" * 64
EVIDENCE_SHA = "c" * 64
CHECK_SHA = "d" * 64


def sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def dump(path: Path, value) -> bytes:
    data = json.dumps(value, separators=(",", ":"), ensure_ascii=False).encode()
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    return data


def dump_pretty(path: Path, value) -> bytes:
    data = (json.dumps(value, indent=2, ensure_ascii=False) + "\n").encode()
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(data)
    return data


def catalogue(locale: str, messages: dict[str, str]) -> dict:
    return {"schemaVersion": 1, "locale": locale, "messages": messages}


def base_events() -> list[dict]:
    return [
        {
            "id": "await-result",
            "chapterId": "retries",
            "type": "challenge.await_validation",
            "payload": {"stepId": "solve", "checkpointId": "result"},
        },
        {
            "id": "explain",
            "chapterId": "retries",
            "type": "chat.message",
            "payload": {
                "actorId": "guide",
                "messageKey": "retry.explanation",
                "attachmentIds": ["photo"],
            },
            "presentation": {
                "delayBeforeMs": 10,
                "pauseAfterMs": 20,
                "typingSpeed": "fast",
                "typingIndicator": True,
                "transition": "fade",
            },
        },
    ]


def target_events() -> list[dict]:
    events = base_events()
    events.extend(
        [
            {
                "id": "check-event",
                "chapterId": "retries",
                "type": "check.run",
                "payload": {
                    "checkId": "owner-check",
                    "referenceOutcome": "passed",
                    "referenceAssisted": False,
                },
            },
            {
                "id": "recommend-event",
                "chapterId": "retries",
                "type": "postmortem.recommend",
                "payload": {"proposalId": "patch"},
            },
            {
                "id": "focus-event",
                "chapterId": "retries",
                "type": "camera.focus",
                "payload": {"targetId": "case-panel"},
            },
        ]
    )
    return events


def nested_target_events() -> list[dict]:
    events = base_events()
    events.extend(
        [
            {
                "id": "check-event",
                "chapterId": "retries",
                "type": "check.run",
                "payload": {
                    "checkId": "owner-check",
                    "referenceOutcome": "passed",
                    "referenceAssisted": False,
                },
            },
            {
                "id": "recommend-event",
                "chapterId": "retries",
                "type": "postmortem.recommend",
                "payload": {"proposalId": "patch"},
            },
            {
                "id": "focus-check",
                "chapterId": "retries",
                "type": "camera.focus",
                "payload": {"targetId": "check-panel"},
            },
            {
                "id": "focus-proposal",
                "chapterId": "retries",
                "type": "camera.focus",
                "payload": {"targetId": "proposal-panel"},
            },
        ]
    )
    return events


def sidecar(catalogues: list[dict], events: list[dict], segment_ids: list[str]) -> dict:
    return {
        "schemaVersion": 1,
        "scenarioId": PACKAGE_ID,
        "revision": PACKAGE_REVISION,
        "locale": "en",
        "fallbackLocale": "en",
        "messageCatalogues": [
            {
                "locale": ref["locale"],
                "path": ref["path"],
                "sha256": ref["sha256"],
                "bytes": ref["bytes"],
            }
            for ref in catalogues
        ],
        "chapterRefs": ["retries"],
        "actors": [{"id": "guide", "name": "Guide"}],
        "assets": [
            {
                "id": "photo",
                "mediaType": "image/png",
                "path": "training/photo.png",
                "sha256": PHOTO_SHA,
                "bytes": 5,
            }
        ],
        "segments": [
            {
                "id": "discussion",
                "chapterId": "retries",
                "stepId": "discussion",
                "eventIds": segment_ids,
                "prerequisiteContextEventIds": [],
            }
        ],
        "events": events,
    }


def declaration(narrative_ref: dict) -> dict:
    return {
        "schema": "datatug-walkthrough/2",
        "id": PACKAGE_ID,
        "version": 1,
        "title": "Retry investigation",
        "trainingPackage": {
            "id": PACKAGE_ID,
            "revision": PACKAGE_REVISION,
            "path": "training/package.json",
            "sha256": PACKAGE_SHA,
            "bytes": PACKAGE_BYTES,
        },
        "narrative": narrative_ref,
        "chapters": [
            {
                "id": "retries",
                "title": "Inspect retries",
                "why": "Find repeated requests.",
                "steps": [
                    {
                        "id": "solve",
                        "kind": "challenge",
                        "title": "Inspect requests",
                        "why": "Validate the retry query.",
                        "target": {
                            "kind": "challenge",
                            "challengeId": "retry-query",
                            "revision": 1,
                        },
                        "done": {
                            "kind": "challenge-passed",
                            "checkpoints": [
                                {"id": "result", "completion": "whole-challenge"}
                            ],
                        },
                    },
                    {
                        "id": "discussion",
                        "kind": "scripted-dialogue",
                        "segmentId": "discussion",
                        "title": "Discuss",
                        "why": "Explain the result.",
                        "modes": ["guided"],
                        "target": {"kind": "none"},
                        "done": {"kind": "dialogue-finished"},
                    },
                ],
            }
        ],
    }


def project_presentation(value: dict | None) -> dict | None:
    if value is None:
        return None
    return {
        "delayBeforeMs": value.get("delayBeforeMs", 0),
        "pauseAfterMs": value.get("pauseAfterMs", 0),
        "typingSpeed": value.get("typingSpeed", "natural"),
        "typingIndicator": value.get("typingIndicator", False),
        "transition": value.get("transition", "none"),
        "cameraTargetId": value.get("cameraTargetId", ""),
    }


def project_event(event: dict) -> dict:
    projected = {
        "id": event["id"],
        "chapterId": event["chapterId"],
        "type": event["type"],
    }
    payload = event["payload"]
    kind = event["type"]
    if kind == "chat.message":
        projected["payload"] = {
            "actorId": payload["actorId"],
            "messageKey": payload["messageKey"],
            "attachmentIds": payload["attachmentIds"],
        }
    elif kind in ("detective.create", "detective.open"):
        projected["payload"] = {"manifestCaseId": payload["manifestCaseId"]}
    elif kind == "evidence.reveal":
        projected["payload"] = {"assetId": payload["assetId"]}
    elif kind == "challenge.prompt":
        projected["payload"] = {"stepId": payload["stepId"]}
    elif kind == "challenge.await_validation":
        projected["payload"] = {
            "stepId": payload["stepId"],
            "checkpointId": payload["checkpointId"],
        }
    elif kind == "check.run":
        projected["payload"] = {
            "checkId": payload["checkId"],
            "referenceOutcome": payload["referenceOutcome"],
            "referenceAssisted": payload["referenceAssisted"],
        }
    elif kind == "postmortem.recommend":
        projected["payload"] = {"proposalId": payload["proposalId"]}
    elif kind == "camera.focus":
        projected["payload"] = {"targetId": payload["targetId"]}
    elif kind == "branch":
        projected["payload"] = {
            "stepId": payload["stepId"],
            "checkpointId": payload["checkpointId"],
            "outcome": payload["outcome"],
            "assisted": payload["assisted"],
            "targetEventId": payload["targetEventId"],
        }
    elif kind == "checkpoint":
        projected["payload"] = {"stateKey": payload["stateKey"]}
    projected["presentation"] = project_presentation(event.get("presentation"))
    return projected


def project_sidecar(value: dict) -> dict:
    return {
        "schemaVersion": str(value["schemaVersion"]),
        "scenarioId": value["scenarioId"],
        "revision": str(value["revision"]),
        "locale": value["locale"],
        "fallbackLocale": value["fallbackLocale"],
        "messageCatalogues": [
            {
                "locale": ref["locale"],
                "path": ref["path"],
                "sha256": ref["sha256"],
                "bytes": str(ref["bytes"]),
            }
            for ref in value["messageCatalogues"]
        ],
        "chapterRefs": value["chapterRefs"],
        "actors": value["actors"],
        "assets": [
            {
                "id": asset["id"],
                "mediaType": asset["mediaType"],
                "path": asset["path"],
                "sha256": asset["sha256"],
                "bytes": str(asset["bytes"]),
            }
            for asset in value["assets"]
        ],
        "segments": [
            {
                "id": segment["id"],
                "chapterId": segment["chapterId"],
                "stepId": segment["stepId"],
                "eventIds": segment["eventIds"],
                "prerequisiteContextEventIds": segment["prerequisiteContextEventIds"],
            }
            for segment in value["segments"]
        ],
        "events": [project_event(event) for event in value["events"]],
    }


def project_catalogue(value: dict) -> dict:
    return {
        "schemaVersion": str(value["schemaVersion"]),
        "locale": value["locale"],
        "messages": {key: value["messages"][key] for key in sorted(value["messages"])},
    }


def owner_fixture(
    assets: list[dict],
    *,
    cases=None,
    incidents=None,
    checks=None,
    queries=None,
    proposals=None,
    presentation_targets=None,
    check_bindings=None,
    proposal_bindings=None,
    target_bindings=None,
    mutate_on_second_read=False,
    wrong_package_binding=False,
):
    return {
        "assets": assets,
        "cases": cases or [],
        "incidents": incidents or [],
        "checks": checks or [],
        "queries": queries or [],
        "proposals": proposals or [],
        "presentationTargets": presentation_targets or [],
        "checkBindings": check_bindings or [],
        "proposalBindings": proposal_bindings or [],
        "targetBindings": target_bindings or [],
        "mutateOnSecondRead": mutate_on_second_read,
        "wrongPackageBinding": wrong_package_binding,
    }


def base_assets() -> list[dict]:
    return [
        {
            "assetId": "evidence",
            "kind": "evidence-text",
            "path": "training/evidence.txt",
            "sha256": EVIDENCE_SHA,
            "bytes": "10",
        },
        {
            "assetId": "owner-check-definition",
            "kind": "check-definition",
            "path": "training/check.json",
            "sha256": CHECK_SHA,
            "bytes": "20",
        },
    ]


def project_admission(
    declaration_value: dict,
    sidecar_value: dict,
    catalogues: dict[str, dict],
    owner: dict,
    request: str | None,
) -> dict:
    catalogue_locales = [ref["locale"] for ref in sidecar_value["messageCatalogues"]]
    if request is None:
        requested = sidecar_value["locale"]
        selected = requested
        matched = "exact"
    else:
        requested = request
        if request in catalogue_locales:
            selected = request
            matched = "exact"
        else:
            primary = request.split("-")[0]
            if primary in catalogue_locales:
                selected = primary
                matched = "primary"
            else:
                selected = sidecar_value["fallbackLocale"]
                matched = "fallback"
    fallback_locale = sidecar_value["fallbackLocale"]
    messages = []
    for event in sidecar_value["events"]:
        if event["type"] != "chat.message":
            continue
        key = event["payload"]["messageKey"]
        selected_messages = catalogues[selected]["messages"]
        fallback_messages = catalogues[fallback_locale]["messages"]
        if key in selected_messages:
            messages.append(
                {
                    "eventId": event["id"],
                    "key": key,
                    "text": selected_messages[key],
                    "locale": selected,
                    "fromFallback": False,
                }
            )
        elif key in fallback_messages:
            messages.append(
                {
                    "eventId": event["id"],
                    "key": key,
                    "text": fallback_messages[key],
                    "locale": fallback_locale,
                    "fromFallback": True,
                }
            )
    text_paths: dict[str, tuple[str, int]] = {}
    sidecar_bytes = sidecar_value["_bytes"]
    text_paths[declaration_value["narrative"]["path"]] = (
        declaration_value["narrative"]["sha256"],
        len(sidecar_bytes),
    )
    for ref in sidecar_value["messageCatalogues"]:
        text_paths[ref["path"]] = (ref["sha256"], ref["bytes"])
    for asset in owner["assets"]:
        if asset["kind"].startswith("evidence-") or asset["kind"] in (
            "query-metadata",
            "query-body-sql",
            "check-definition",
            "challenge-manifest",
            "watch-recommendation",
        ):
            text_paths[asset["path"]] = (asset["sha256"], int(asset["bytes"]))
    text_bytes = sum(actual for _, actual in text_paths.values())
    checks = []
    for binding in owner["checkBindings"]:
        if not any(event["type"] == "check.run" for event in sidecar_value["events"]):
            continue
        checks.append(
            {
                "checkId": binding["checkId"],
                "definitionAssetId": binding["definitionAssetId"],
            }
        )
    proposals = []
    if any(event["type"] == "postmortem.recommend" for event in sidecar_value["events"]):
        for binding in owner["proposalBindings"]:
            proposals.append(
                {
                    "proposalId": binding["proposalId"],
                    "kind": binding["kind"],
                    "definitionId": binding["definitionId"],
                    "evidenceIds": binding["evidenceIds"],
                    "rationale": binding["rationale"],
                }
            )
    targets = []
    if any(event["type"] == "camera.focus" for event in sidecar_value["events"]):
        for binding in owner["targetBindings"]:
            targets.append(
                {
                    "targetId": binding["targetId"],
                    "kind": binding["kind"],
                    "refId": binding["refId"],
                }
            )
    return {
        "scenarioId": sidecar_value["scenarioId"],
        "revision": str(sidecar_value["revision"]),
        "package": {
            "id": declaration_value["trainingPackage"]["id"],
            "revision": str(declaration_value["trainingPackage"]["revision"]),
            "path": declaration_value["trainingPackage"]["path"],
            "sha256": declaration_value["trainingPackage"]["sha256"],
            "bytes": str(declaration_value["trainingPackage"]["bytes"]),
        },
        "locale": {
            "requestedLocale": requested,
            "selectedLocale": selected,
            "matched": matched,
            "fallbackLocale": fallback_locale,
        },
        "textBytes": str(text_bytes),
        "catalogueLocales": catalogue_locales,
        "messages": messages,
        "checks": checks,
        "proposals": proposals,
        "presentationTargets": targets,
    }


def main() -> None:
    NARRATIVE.mkdir(parents=True, exist_ok=True)

    en = dump(NARRATIVE / "catalogues" / "en.json", catalogue(
        "en", {"retry.explanation": "Repeated requests need an idempotency key."}
    ))
    fr = dump(NARRATIVE / "catalogues" / "fr.json", catalogue("fr", {"other.key": "Autre"}))
    unknown_catalogue = dump(
        NARRATIVE / "catalogues" / "unknown-field.json",
        {"schemaVersion": 1, "locale": "en", "messages": {"a": "A"}, "extra": 1},
    )
    bad_key = dump(
        NARRATIVE / "catalogues" / "bad-key.json",
        {"schemaVersion": 1, "locale": "en", "messages": {"A": "A"}},
    )
    empty_catalogue = dump(
        NARRATIVE / "catalogues" / "empty.json",
        {"schemaVersion": 1, "locale": "en", "messages": {}},
    )

    refs = [
        {
            "locale": "en",
            "path": "walkthroughs/training-demo/messages.en.json",
            "sha256": sha(en),
            "bytes": len(en),
        },
        {
            "locale": "fr",
            "path": "walkthroughs/training-demo/messages.fr.json",
            "sha256": sha(fr),
            "bytes": len(fr),
        },
    ]

    base_sidecar_value = sidecar(refs[:1], base_events(), ["await-result", "explain"])
    base_sidecar = dump_pretty(NARRATIVE / "sidecars" / "minimal.json", base_sidecar_value)
    base_sidecar_value["_bytes"] = base_sidecar
    fallback_sidecar_value = sidecar(refs, base_events(), ["await-result", "explain"])
    fallback_sidecar = dump_pretty(NARRATIVE / "sidecars" / "fallback.json", fallback_sidecar_value)
    fallback_sidecar_value["_bytes"] = fallback_sidecar
    target_sidecar_value = sidecar(
        refs[:1],
        target_events(),
        ["await-result", "explain", "check-event", "recommend-event", "focus-event"],
    )
    target_sidecar = dump_pretty(NARRATIVE / "sidecars" / "targets.json", target_sidecar_value)
    target_sidecar_value["_bytes"] = target_sidecar
    nested_target_sidecar_value = sidecar(
        refs[:1],
        nested_target_events(),
        [
            "await-result",
            "explain",
            "check-event",
            "recommend-event",
            "focus-check",
            "focus-proposal",
        ],
    )
    nested_target_sidecar = dump_pretty(
        NARRATIVE / "sidecars" / "targets-nested.json", nested_target_sidecar_value
    )
    nested_target_sidecar_value["_bytes"] = nested_target_sidecar
    def padded_sidecar(source: bytes, size: int) -> bytes:
        padding = size - len(source)
        if padding < 0 or not source.startswith(b"{\n"):
            raise ValueError("sidecar cannot be padded in place")
        # Indent the first member line with JSON whitespace: valid, and not
        # trailing whitespace at end of line.
        return source[:2] + b" " * padding + source[2:]

    at_limit_sidecar = padded_sidecar(base_sidecar, 64 * 1024)
    at_limit_sidecar_value = dict(base_sidecar_value)
    at_limit_sidecar_value["_bytes"] = at_limit_sidecar
    (NARRATIVE / "sidecars" / "minimal-at-limit.json").write_bytes(at_limit_sidecar)
    over_limit_sidecar = padded_sidecar(base_sidecar, 64 * 1024 + 1)
    over_limit_sidecar_value = dict(base_sidecar_value)
    over_limit_sidecar_value["_bytes"] = over_limit_sidecar
    (NARRATIVE / "sidecars" / "minimal-over-limit.json").write_bytes(over_limit_sidecar)

    unknown_sidecar_value = sidecar(refs[:1], base_events(), ["await-result", "explain"])
    unknown_sidecar_value["extra"] = 1
    dump_pretty(NARRATIVE / "sidecars" / "unknown-field.json", unknown_sidecar_value)
    dup_events = base_events() + [base_events()[1]]
    dup_sidecar_value = sidecar(refs[:1], dup_events, ["await-result", "explain"])
    dump_pretty(NARRATIVE / "sidecars" / "duplicate-event.json", dup_sidecar_value)
    branch_events = [
        base_events()[0],
        {
            "id": "branch-back",
            "chapterId": "retries",
            "type": "branch",
            "payload": {
                "stepId": "solve",
                "checkpointId": "result",
                "outcome": "passed",
                "assisted": False,
                "targetEventId": "await-result",
            },
        },
        base_events()[1],
    ]
    branch_sidecar_value = sidecar(
        refs[:1], branch_events, ["await-result", "branch-back"]
    )
    dump_pretty(NARRATIVE / "sidecars" / "branch-order.json", branch_sidecar_value)
    presentation_value = sidecar(refs[:1], base_events(), ["await-result", "explain"])
    presentation_value["events"][1]["presentation"]["delayBeforeMs"] = 10001
    dump_pretty(NARRATIVE / "sidecars" / "presentation-bounds.json", presentation_value)
    chapter_value = sidecar(refs[:1], base_events(), ["await-result", "explain"])
    chapter_value["events"][1]["chapterId"] = "other"
    dump_pretty(NARRATIVE / "sidecars" / "chapter-mismatch.json", chapter_value)

    def narrative_ref(data: bytes) -> dict:
        return {
            "path": "walkthroughs/training-demo/narrative.json",
            "sha256": sha(data),
            "bytes": len(data),
        }

    declarations = {
        "minimal": dump_pretty(
            NARRATIVE / "declarations" / "minimal.json",
            declaration(narrative_ref(base_sidecar)),
        ),
        "fallback": dump_pretty(
            NARRATIVE / "declarations" / "fallback.json",
            declaration(narrative_ref(fallback_sidecar)),
        ),
        "targets": dump_pretty(
            NARRATIVE / "declarations" / "targets.json",
            declaration(narrative_ref(target_sidecar)),
        ),
        "targets-nested": dump_pretty(
            NARRATIVE / "declarations" / "targets-nested.json",
            declaration(narrative_ref(nested_target_sidecar)),
        ),
        "minimal-at-limit": dump_pretty(
            NARRATIVE / "declarations" / "minimal-at-limit.json",
            declaration(narrative_ref(at_limit_sidecar)),
        ),
    }

    vectors = []

    def add_sidecar(name: str, description: str, data: bytes, want: dict) -> None:
        vectors.append(
            {
                "id": name,
                "description": description,
                "operation": "narrative-sidecar",
                "inputFile": f"narrative/sidecars/{name.split('-', 1)[1]}.json",
                "want": want,
            }
        )

    for name, description, data, want in [
        (
            "sidecar-minimal",
            "A structurally valid sidecar with a challenge wait and a chat message.",
            base_sidecar,
            {"accepted": True, "digest": sha(base_sidecar), "definition": project_sidecar(base_sidecar_value)},
        ),
        (
            "sidecar-unknown-field",
            "An unknown sidecar top-level field rejects.",
            (NARRATIVE / "sidecars" / "unknown-field.json").read_bytes(),
            {"accepted": False, "digest": sha((NARRATIVE / "sidecars" / "unknown-field.json").read_bytes()), "issues": [{"code": "unknown_field", "path": "/extra"}]},
        ),
        (
            "sidecar-duplicate-event",
            "A duplicate event id rejects.",
            (NARRATIVE / "sidecars" / "duplicate-event.json").read_bytes(),
            {"accepted": False, "digest": sha((NARRATIVE / "sidecars" / "duplicate-event.json").read_bytes()), "issues": [{"code": "duplicate_id", "path": "/events/2/id"}]},
        ),
        (
            "sidecar-branch-order",
            "A branch target must be later in the same segment.",
            (NARRATIVE / "sidecars" / "branch-order.json").read_bytes(),
            {"accepted": False, "digest": sha((NARRATIVE / "sidecars" / "branch-order.json").read_bytes()), "issues": [{"code": "invalid_reference", "path": "/events/1/payload/targetEventId"}]},
        ),
        (
            "sidecar-presentation-bounds",
            "A presentation delay above 10000 ms rejects.",
            (NARRATIVE / "sidecars" / "presentation-bounds.json").read_bytes(),
            {"accepted": False, "digest": sha((NARRATIVE / "sidecars" / "presentation-bounds.json").read_bytes()), "issues": [{"code": "invalid_value", "path": "/events/1/presentation/delayBeforeMs"}]},
        ),
        (
            "sidecar-chapter-mismatch",
            "An event chapter outside chapterRefs rejects.",
            (NARRATIVE / "sidecars" / "chapter-mismatch.json").read_bytes(),
            {"accepted": False, "digest": sha((NARRATIVE / "sidecars" / "chapter-mismatch.json").read_bytes()), "issues": [{"code": "invalid_reference", "path": "/events/1/chapterId"}, {"code": "invalid_reference", "path": "/segments/0/eventIds/1"}]},
        ),
    ]:
        vectors.append(
            {
                "id": name,
                "description": description,
                "operation": "narrative-sidecar",
                "inputFile": f"narrative/sidecars/{name.split('-', 1)[1]}.json",
                "want": want,
            }
        )

    vectors.append(
        {
            "id": "sidecar-at-limit",
            "description": "A sidecar of exactly 64 KiB parses.",
            "operation": "narrative-sidecar",
            "inputFile": "narrative/sidecars/minimal-at-limit.json",
            "want": {
                "accepted": True,
                "digest": sha(at_limit_sidecar),
                "definition": project_sidecar(at_limit_sidecar_value),
            },
        }
    )
    vectors.append(
        {
            "id": "sidecar-over-limit",
            "description": "A sidecar padded past 64 KiB refuses at the parser.",
            "operation": "narrative-sidecar",
            "inputFile": "narrative/sidecars/minimal-over-limit.json",
            "want": {
                "accepted": False,
                "digest": sha(over_limit_sidecar),
                "issues": [{"code": "too_large", "path": ""}],
            },
        }
    )

    for name, description, data, want in [
        ("catalogue-valid", "A valid catalogue with stable keys.", en, {"accepted": True, "digest": sha(en), "definition": project_catalogue(catalogue("en", {"retry.explanation": "Repeated requests need an idempotency key."}))}),
        ("catalogue-unknown-field", "An unknown catalogue field rejects.", unknown_catalogue, {"accepted": False, "digest": sha(unknown_catalogue), "issues": [{"code": "unknown_field", "path": "/extra"}]}),
        ("catalogue-bad-key", "A catalogue key outside the stable grammar rejects.", bad_key, {"accepted": False, "digest": sha(bad_key), "issues": [{"code": "invalid_value", "path": "/messages/A"}]}),
        ("catalogue-empty", "An empty catalogue message map rejects.", empty_catalogue, {"accepted": False, "digest": sha(empty_catalogue), "issues": [{"code": "too_many", "path": "/messages"}]}),
    ]:
        vectors.append(
            {
                "id": name,
                "description": description,
                "operation": "message-catalogue",
                "inputFile": "narrative/catalogues/en.json" if name == "catalogue-valid" else f"narrative/catalogues/{name.split('-', 1)[1]}.json",
                "want": want,
            }
        )

    selections = [
        ("selection-default", None, ["en", "fr"], "en", "fr", "en", "exact", "en"),
        ("selection-exact", "en-us", ["en-US", "fr"], "en-US", "fr", "en-US", "exact", "en-US"),
        ("selection-primary", "en-GB", ["en", "fr"], "en", "fr", "en", "primary", "en-GB"),
        ("selection-fallback", "de-DE", ["en", "fr"], "en", "fr", "fr", "fallback", "de-DE"),
        ("selection-private", "x-private", ["en", "x-private"], "en", "en", "x-private", "exact", "x-private"),
    ]
    for name, request, declared, default_locale, fallback, selected, matched, requested in selections:
        payload = {
            "request": request,
            "declared": declared,
            "defaultLocale": default_locale,
            "fallbackLocale": fallback,
        }
        data = dump(NARRATIVE / "selections" / f"{name.split('-', 1)[1]}.json", payload)
        vectors.append(
            {
                "id": name,
                "description": "Deterministic /2 locale selection.",
                "operation": "locale-selection",
                "inputFile": f"narrative/selections/{name.split('-', 1)[1]}.json",
                "want": {
                    "accepted": True,
                    "digest": sha(data),
                    "selection": {
                        "requestedLocale": requested,
                        "selectedLocale": selected,
                        "matched": matched,
                        "fallbackLocale": fallback,
                    },
                },
            }
        )
    malformed = dump(
        NARRATIVE / "selections" / "malformed.json",
        {"request": "en_US", "declared": ["en"], "defaultLocale": "en", "fallbackLocale": "en"},
    )
    vectors.append(
        {
            "id": "selection-malformed",
            "description": "A malformed visitor request is a structured refusal.",
            "operation": "locale-selection",
            "inputFile": "narrative/selections/malformed.json",
            "want": {"accepted": False, "digest": sha(malformed), "issues": [{"code": "malformed_locale", "path": "/request"}]},
        }
    )

    def add_admission(name, description, declaration_name, sidecar_name, sidecar_data, catalogues, request, owner, want):
        case = {
            "declarationFile": f"narrative/declarations/{declaration_name}.json",
            "sidecarFile": f"narrative/sidecars/{sidecar_name}.json",
            "catalogueFiles": catalogues,
            "request": request,
            "owner": owner,
        }
        data = dump_pretty(NARRATIVE / "admissions" / f"{name.split('-', 1)[1]}.json", case)
        vectors.append(
            {
                "id": name,
                "description": description,
                "operation": "narrative-admission",
                "inputFile": f"narrative/admissions/{name.split('-', 1)[1]}.json",
                "want": {"digest": sha(data), **want},
            }
        )

    minimal_owner = owner_fixture(base_assets(), cases=["case-one"], incidents=["incident-one"])
    minimal_value = project_admission(
        declaration(narrative_ref(base_sidecar)),
        base_sidecar_value,
        {"en": catalogue("en", {"retry.explanation": "Repeated requests need an idempotency key."})},
        minimal_owner,
        None,
    )
    add_admission(
        "admission-minimal",
        "Minimal narrative closure with package text accounting.",
        "minimal",
        "minimal",
        base_sidecar,
        ["en"],
        None,
        minimal_owner,
        {"accepted": True, "admission": minimal_value},
    )

    target_owner = owner_fixture(
        base_assets(),
        cases=["case-one"],
        incidents=["incident-one"],
        checks=["owner-check"],
        queries=["queries/retries"],
        proposals=["patch"],
        presentation_targets=["case-panel"],
        check_bindings=[{"checkId": "owner-check", "definitionAssetId": "owner-check-definition"}],
        proposal_bindings=[
            {
                "proposalId": "patch",
                "kind": "query",
                "definitionId": "queries/retries",
                "evidenceIds": ["evidence"],
                "rationale": "Try the idempotency key.",
            }
        ],
        target_bindings=[
            {"targetId": "case-panel", "kind": "case", "refId": "case-one", "closureKind": "case", "closureId": "case-one"}
        ],
    )
    target_value = project_admission(
        declaration(narrative_ref(target_sidecar)),
        target_sidecar_value,
        {"en": catalogue("en", {"retry.explanation": "Repeated requests need an idempotency key."})},
        target_owner,
        None,
    )
    add_admission(
        "admission-targets",
        "Check, proposal and presentation-target closure.",
        "targets",
        "targets",
        target_sidecar,
        ["en"],
        None,
        target_owner,
        {"accepted": True, "admission": target_value},
    )

    fallback_owner = owner_fixture(base_assets(), cases=["case-one"], incidents=["incident-one"])
    fallback_value = project_admission(
        declaration(narrative_ref(fallback_sidecar)),
        fallback_sidecar_value,
        {
            "en": catalogue("en", {"retry.explanation": "Repeated requests need an idempotency key."}),
            "fr": catalogue("fr", {"other.key": "Autre"}),
        },
        fallback_owner,
        "fr",
    )
    add_admission(
        "admission-fallback",
        "A selected catalogue key resolves through the pinned fallback.",
        "fallback",
        "fallback",
        fallback_sidecar,
        ["en", "fr"],
        "fr",
        fallback_owner,
        {"accepted": True, "admission": fallback_value},
    )

    missing_owner = owner_fixture(
        base_assets(), cases=["case-one"], incidents=["incident-one"], checks=["owner-check"], queries=["queries/retries"], proposals=["patch"], presentation_targets=["case-panel"], proposal_bindings=target_owner["proposalBindings"], target_bindings=target_owner["targetBindings"]
    )
    add_admission(
        "admission-check-missing",
        "A missing Check owner binding refuses before executable output.",
        "targets",
        "targets",
        target_sidecar,
        ["en"],
        None,
        missing_owner,
        {"accepted": False, "issues": [{"code": "not_admitted", "path": "/events/2/payload/checkId"}]},
    )

    wrong_owner = owner_fixture(
        base_assets(),
        cases=["case-one"],
        incidents=["incident-one"],
        checks=["owner-check"],
        queries=["queries/retries"],
        proposals=["patch"],
        presentation_targets=["case-panel"],
        check_bindings=target_owner["checkBindings"],
        proposal_bindings=target_owner["proposalBindings"],
        target_bindings=target_owner["targetBindings"],
        wrong_package_binding=True,
    )
    add_admission(
        "admission-wrong-package",
        "A Check binding tied to another package refuses.",
        "targets",
        "targets",
        target_sidecar,
        ["en"],
        None,
        wrong_owner,
        {"accepted": False, "issues": [{"code": "mismatch", "path": "/events/2/payload/checkId"}]},
    )

    alias_assets = base_assets() + [
        {
            "assetId": "evidence-alias",
            "kind": "evidence-text",
            "path": "training/evidence.txt",
            "sha256": EVIDENCE_SHA,
            "bytes": "10",
        }
    ]
    alias_owner = owner_fixture(alias_assets, cases=["case-one"], incidents=["incident-one"])
    alias_value = project_admission(
        declaration(narrative_ref(base_sidecar)),
        base_sidecar_value,
        {"en": catalogue("en", {"retry.explanation": "Repeated requests need an idempotency key."})},
        alias_owner,
        None,
    )
    add_admission(
        "admission-alias",
        "Identical source+path aliases count once.",
        "minimal",
        "minimal",
        base_sidecar,
        ["en"],
        None,
        alias_owner,
        {"accepted": True, "admission": alias_value},
    )

    different_assets = base_assets() + [
        {
            "assetId": "evidence-second",
            "kind": "evidence-text",
            "path": "training/evidence-2.txt",
            "sha256": EVIDENCE_SHA,
            "bytes": "10",
        }
    ]
    different_owner = owner_fixture(different_assets, cases=["case-one"], incidents=["incident-one"])
    different_value = project_admission(
        declaration(narrative_ref(base_sidecar)),
        base_sidecar_value,
        {"en": catalogue("en", {"retry.explanation": "Repeated requests need an idempotency key."})},
        different_owner,
        None,
    )
    add_admission(
        "admission-different-path",
        "Different paths count separately even with equal bytes and digest.",
        "minimal",
        "minimal",
        base_sidecar,
        ["en"],
        None,
        different_owner,
        {"accepted": True, "admission": different_value},
    )

    conflict_assets = base_assets() + [
        {
            "assetId": "evidence-conflict",
            "kind": "evidence-text",
            "path": "training/evidence.txt",
            "sha256": "e" * 64,
            "bytes": "10",
        }
    ]
    conflict_owner = owner_fixture(conflict_assets, cases=["case-one"], incidents=["incident-one"])
    add_admission(
        "admission-conflict",
        "Conflicting pins at one source+path refuse.",
        "minimal",
        "minimal",
        base_sidecar,
        ["en"],
        None,
        conflict_owner,
        {"accepted": False, "issues": [{"code": "mismatch", "path": ""}]},
    )

    over_assets = base_assets() + [
        {
            "assetId": "large-text",
            "kind": "evidence-text",
            "path": "training/large.txt",
            "sha256": "f" * 64,
            "bytes": str(64 * 1024 + 1),
        }
    ]
    over_owner = owner_fixture(over_assets, cases=["case-one"], incidents=["incident-one"])
    add_admission(
        "admission-over-text",
        "A text asset above 64 KiB refuses.",
        "minimal",
        "minimal",
        base_sidecar,
        ["en"],
        None,
        over_owner,
        {"accepted": False, "issues": [{"code": "too_long", "path": ""}]},
    )

    drift_owner = owner_fixture(
        base_assets(), cases=["case-one"], incidents=["incident-one"], mutate_on_second_read=True
    )
    add_admission(
        "admission-inventory-drift",
        "A changing owner inventory refuses instead of mixing snapshots.",
        "minimal",
        "minimal",
        base_sidecar,
        ["en"],
        None,
        drift_owner,
        {"accepted": False, "issues": [{"code": "mismatch", "path": ""}]},
    )

    proposal_binding = {
        "proposalId": "patch",
        "kind": "query",
        "definitionId": "queries/retries",
        "evidenceIds": ["evidence"],
        "rationale": "Try the idempotency key.",
    }
    nested_target_owner = owner_fixture(
        base_assets(),
        cases=["case-one"],
        incidents=["incident-one"],
        checks=["owner-check"],
        queries=["queries/retries"],
        proposals=["patch"],
        presentation_targets=["check-panel", "proposal-panel"],
        check_bindings=[
            {"checkId": "owner-check", "definitionAssetId": "owner-check-definition"}
        ],
        proposal_bindings=[proposal_binding],
        target_bindings=[
            {
                "targetId": "check-panel",
                "kind": "check",
                "refId": "owner-check",
                "closureKind": "check",
                "closureId": "owner-check",
                "closureCheck": {
                    "checkId": "owner-check",
                    "definitionAssetId": "owner-check-definition",
                },
            },
            {
                "targetId": "proposal-panel",
                "kind": "proposal",
                "refId": "patch",
                "closureKind": "proposal",
                "closureId": "patch",
                "closureProposal": dict(proposal_binding),
            },
        ],
    )
    nested_target_value = project_admission(
        declaration(narrative_ref(nested_target_sidecar)),
        nested_target_sidecar_value,
        {"en": catalogue("en", {"retry.explanation": "Repeated requests need an idempotency key."})},
        nested_target_owner,
        None,
    )
    add_admission(
        "admission-target-closures",
        "Check and proposal presentation targets require matching typed nested closures.",
        "targets-nested",
        "targets-nested",
        nested_target_sidecar,
        ["en"],
        None,
        nested_target_owner,
        {"accepted": True, "admission": nested_target_value},
    )

    def nested_variant(name, description, mutate, want):
        owner = deepcopy(nested_target_owner)
        mutate(owner)
        add_admission(
            name,
            description,
            "targets-nested",
            "targets-nested",
            nested_target_sidecar,
            ["en"],
            None,
            owner,
            want,
        )

    def target_binding(owner, target_id):
        for binding in owner["targetBindings"]:
            if binding["targetId"] == target_id:
                return binding
        raise AssertionError(f"missing target {target_id}")

    nested_variant(
        "admission-check-not-inventory",
        "A Check binding must be a member of the admitted package Check inventory.",
        lambda owner: owner.update({"checks": []}),
        {
            "accepted": False,
            "issues": [
                {"code": "not_admitted", "path": "/events/2/payload/checkId"},
                {"code": "invalid_reference", "path": "/events/4/payload/targetId"},
            ],
        },
    )
    nested_variant(
        "admission-target-check-closure-absent",
        "A Check target without its typed closure refuses.",
        lambda owner: target_binding(owner, "check-panel").update({"omitClosure": True}),
        {"accepted": False, "issues": [{"code": "mismatch", "path": "/events/4/payload/targetId"}]},
    )
    nested_variant(
        "admission-target-check-nested-absent",
        "A Check target without its nested Check binding refuses.",
        lambda owner: target_binding(owner, "check-panel").pop("closureCheck", None),
        {"accepted": False, "issues": [{"code": "mismatch", "path": "/events/4/payload/targetId"}]},
    )
    nested_variant(
        "admission-target-check-nested-mismatch",
        "A contradictory nested Check definition refuses.",
        lambda owner: target_binding(owner, "check-panel")["closureCheck"].update(
            {"definitionAssetId": "evidence"}
        ),
        {"accepted": False, "issues": [{"code": "mismatch", "path": "/events/4/payload/targetId"}]},
    )
    nested_variant(
        "admission-target-check-nested-pin",
        "A nested Check binding tied to another package pin refuses.",
        lambda owner: target_binding(owner, "check-panel")["closureCheck"].update(
            {"wrongPackageBinding": True}
        ),
        {"accepted": False, "issues": [{"code": "mismatch", "path": "/events/4/payload/targetId"}]},
    )
    nested_variant(
        "admission-target-proposal-nested-absent",
        "A proposal target without its nested proposal binding refuses.",
        lambda owner: target_binding(owner, "proposal-panel").pop("closureProposal", None),
        {"accepted": False, "issues": [{"code": "mismatch", "path": "/events/5/payload/targetId"}]},
    )
    nested_variant(
        "admission-target-proposal-nested-mismatch",
        "A contradictory nested proposal binding refuses.",
        lambda owner: target_binding(owner, "proposal-panel")["closureProposal"].update(
            {"rationale": "Different rationale."}
        ),
        {"accepted": False, "issues": [{"code": "mismatch", "path": "/events/5/payload/targetId"}]},
    )
    nested_variant(
        "admission-target-case-extra-nested",
        "An untyped target carrying an unrelated nested Check binding refuses.",
        lambda owner: target_binding(owner, "check-panel").update(
            {
                "kind": "case",
                "refId": "case-one",
                "closureKind": "case",
                "closureId": "case-one",
                "closureCheck": {
                    "checkId": "owner-check",
                    "definitionAssetId": "owner-check-definition",
                },
            }
        ),
        {"accepted": False, "issues": [{"code": "mismatch", "path": "/events/4/payload/targetId"}]},
    )

    for name, kind in [
        ("admission-codec-constructor", "constructor"),
        ("admission-codec-proto", "__proto__"),
        ("admission-codec-tostring", "toString"),
    ]:
        owner = deepcopy(minimal_owner)
        owner["assets"][1]["kind"] = kind
        add_admission(
            name,
            f"An inherited Object property name {kind!r} is not a codec kind.",
            "minimal",
            "minimal",
            base_sidecar,
            ["en"],
            None,
            owner,
            {"accepted": False, "issues": [{"code": "invalid_value", "path": ""}]},
        )

    at_limit_value = project_admission(
        declaration(narrative_ref(at_limit_sidecar)),
        at_limit_sidecar_value,
        {"en": catalogue("en", {"retry.explanation": "Repeated requests need an idempotency key."})},
        minimal_owner,
        None,
    )
    add_admission(
        "admission-sidecar-at-limit",
        "A sidecar of exactly 64 KiB counts once and admits.",
        "minimal-at-limit",
        "minimal-at-limit",
        at_limit_sidecar,
        ["en"],
        None,
        minimal_owner,
        {"accepted": True, "admission": at_limit_value},
    )

    corpus = {"corpusVersion": "walkthrough-contracts-v3", "vectors": vectors}
    corpus_bytes = dump_pretty(ROOT / "corpus-v3.json", corpus)
    base_corpus = (ROOT / "corpus-v2.json").read_bytes()
    files = {}
    for path in sorted((ROOT / "narrative").rglob("*")):
        if path.is_file():
            files[str(path.relative_to(ROOT)).replace("\\", "/")] = sha(path.read_bytes())
    provenance = {
        "corpusVersion": "walkthrough-contracts-v3",
        "baseCorpusVersion": "walkthrough-contracts-v2",
        "baseCorpusSha256": sha(base_corpus),
        "corpusSha256": sha(corpus_bytes),
        "files": files,
    }
    dump_pretty(ROOT / "provenance-v3.json", provenance)
    print(f"wrote {len(vectors)} vectors and {len(files)} file hashes")


if __name__ == "__main__":
    main()
