#!/usr/bin/env python3
"""Generate challenge fixtures, manifests and pristine-run goldens.

The browser runtime consumes the manifests; Go tests consume the observations.
Every observation is produced from the exact fixture bytes committed next to
the manifest (the bytes are deserialized into a fresh in-memory database for
each answer and each run). Run from pkg/challenge:

    python3 testdata/generate_goldens.py

The generated manifests pin the browser engine (sqlite-wasm 3.54.1-build1),
while this generator records the Python SQLite version used to build the
fixture bytes and observations. Regenerating with another SQLite version
changes the pinned bytes, which in a real content store is a new revision.
"""

from __future__ import annotations

import hashlib
import json
import platform
import sqlite3
from pathlib import Path

ROOT = Path(__file__).resolve().parent
FIXTURES = ROOT / "fixtures"
CHALLENGES = ROOT / "challenges"
GOLDENS = ROOT / "goldens"

ENGINE = {
    "package": "@sqlite.org/sqlite-wasm",
    "version": "3.54.1-build1",
    "sqliteVersion": "3.54.1",
}

CUSTOMER_COLUMNS = [
    {"name": "id", "type": "integer"},
    {"name": "email", "type": "string"},
    {"name": "created_at", "type": "string"},
]

DEDUP_SCHEMA = """
CREATE TABLE customers (
  id INTEGER PRIMARY KEY,
  email TEXT COLLATE BINARY,
  created_at TEXT NOT NULL
);
"""

DEDUP_FIXTURES = {
    "nonsequential": {
        "label": "Nonsequential ids: an earlier created_at has a higher id",
        "rows": [
            (9, "alice@example.com", "2026-01-12T11:00:00Z"),
            (2, "alice@example.com", "2026-01-12T10:00:00Z"),
            (5, "alice@example.com", "2026-01-12T09:00:00Z"),
            (8, "bob@example.com", "2026-01-12T07:00:00Z"),
            (7, "bob@example.com", "2026-01-12T08:00:00Z"),
            (12, "carol@example.com", "2026-01-12T12:00:00Z"),
            (21, None, "2026-01-12T09:00:00Z"),
            (20, None, "2026-01-12T10:00:00Z"),
            (30, "Dave@Example.com", "2026-01-12T10:00:00Z"),
            (31, "dave@example.com", "2026-01-12T10:00:00Z"),
            (41, "e@x.com", "2026-01-12T10:00:00Z"),
            (40, " e@x.com", "2026-01-12T10:00:00Z"),
            (50, "", "2026-01-12T09:00:00Z"),
            (51, "", "2026-01-12T10:00:00Z"),
        ],
    },
    "three-member": {
        "label": "Three-member duplicate groups with inverted timestamps",
        "rows": [
            (12, "gamma@example.com", "2026-01-12T10:00:00Z"),
            (10, "gamma@example.com", "2026-01-12T12:00:00Z"),
            (11, "gamma@example.com", "2026-01-12T11:00:00Z"),
            (3, "beta@example.com", "2026-01-12T09:00:00Z"),
            (1, "beta@example.com", "2026-01-12T11:00:00Z"),
            (2, "beta@example.com", "2026-01-12T10:00:00Z"),
            (99, "solo@example.com", "2026-01-12T10:00:00Z"),
        ],
    },
    "null-text": {
        "label": "NULL, empty, case and whitespace groups under BINARY collation",
        "rows": [
            (2, None, "2026-01-12T10:00:00Z"),
            (1, None, "2026-01-12T09:00:00Z"),
            (3, "", "2026-01-12T09:00:00Z"),
            (4, "", "2026-01-12T10:00:00Z"),
            (5, "A@x.com", "2026-01-12T09:00:00Z"),
            (6, "a@x.com", "2026-01-12T10:00:00Z"),
            (7, " a@x.com", "2026-01-12T09:00:00Z"),
            (8, "a@x.com ", "2026-01-12T10:00:00Z"),
            (10, "caf\u00e9@x.com", "2026-01-12T10:00:00Z"),
            (9, "caf\u00e9@x.com", "2026-01-12T09:00:00Z"),
            (11, "solo@x.com", "2026-01-12T10:00:00Z"),
        ],
    },
    "no-duplicates": {
        "label": "No duplicate groups: the answer must leave every row intact",
        "rows": [
            (1, "a@x.com", "2026-01-12T09:00:00Z"),
            (2, "A@x.com", "2026-01-12T10:00:00Z"),
            (3, " a@x.com", "2026-01-12T10:00:00Z"),
            (4, "a@x.com ", "2026-01-12T10:00:00Z"),
            (5, None, "2026-01-12T10:00:00Z"),
            (6, "", "2026-01-12T10:00:00Z"),
        ],
    },
    "repaired": {
        "label": "A repaired fixture: the duplicate check must pass",
        "rows": [
            (1, "ann@x.com", "2026-01-12T09:00:00Z"),
            (2, "ben@x.com", "2026-01-12T10:00:00Z"),
            (3, None, "2026-01-12T10:00:00Z"),
            (4, "", "2026-01-12T10:00:00Z"),
            (5, "Ann@x.com", "2026-01-12T10:00:00Z"),
        ],
    },
    "repaired-mutated": {
        "label": "A repaired fixture mutated to reintroduce one duplicate group",
        "rows": [
            (1, "ann@x.com", "2026-01-12T09:00:00Z"),
            (2, "ben@x.com", "2026-01-12T10:00:00Z"),
            (3, "ann@x.com", "2026-01-12T10:00:00Z"),
            (4, None, "2026-01-12T10:00:00Z"),
        ],
    },
}

RETRY_SCHEMA = """
CREATE TABLE orders (id INTEGER PRIMARY KEY, customer_id INTEGER, total_cents INTEGER);
CREATE TABLE payments (id INTEGER PRIMARY KEY, order_id INTEGER, attempt_id TEXT, status TEXT, amount_cents INTEGER, created_at TEXT);
CREATE TABLE retry_events (attempt_id TEXT, request_id TEXT, idempotency_key TEXT, outcome TEXT, created_at TEXT);
CREATE TABLE deployments (version TEXT, deployed_at TEXT, idempotency_enabled INTEGER);
"""

RETRY_ROWS = [
    "INSERT INTO orders VALUES (1001,11,5000),(1002,12,3000),(1003,13,2000),(1004,14,8000)",
    """INSERT INTO payments VALUES
        (1,1001,'a1','succeeded',5000,'2026-01-12T09:58:00Z'),
        (2,1002,'a2','succeeded',3000,'2026-01-12T10:01:00Z'),
        (3,1002,'a3','succeeded',3000,'2026-01-12T10:01:03Z'),
        (4,1003,'a4','failed',2000,'2026-01-12T10:02:00Z'),
        (5,1003,'a5','succeeded',2000,'2026-01-12T10:02:05Z'),
        (6,1004,'a6','succeeded',8000,'2026-01-12T10:03:00Z'),
        (7,1004,'a7','succeeded',8000,'2026-01-12T10:03:02Z'),
        (8,1004,'a8','refunded',8000,'2026-01-12T10:05:00Z')""",
    """INSERT INTO retry_events VALUES
        ('a1','r1','k1','succeeded','2026-01-12T09:58:00Z'),
        ('a2','r2',NULL,'timeout','2026-01-12T10:01:00Z'),
        ('a3','r2',NULL,'succeeded','2026-01-12T10:01:03Z'),
        ('a4','r3',NULL,'succeeded','2026-01-12T10:02:00Z'),
        ('a5','r3',NULL,'succeeded','2026-01-12T10:02:05Z'),
        ('a6','r4',NULL,'timeout','2026-01-12T10:03:00Z'),
        ('a7','r4',NULL,'succeeded','2026-01-12T10:03:02Z'),
        ('a8','r5',NULL,'succeeded','2026-01-12T10:05:00Z')""",
    """INSERT INTO deployments VALUES
        ('pay-16','2026-01-12T09:00:00Z',1),
        ('pay-17','2026-01-12T10:00:00Z',0)""",
]

REFERENCE_DELETE = """DELETE FROM customers
WHERE EXISTS (
  SELECT 1 FROM customers AS keep
  WHERE keep.email IS customers.email COLLATE BINARY
    AND keep.id < customers.id
)"""

ALTERNATIVE_DELETE = """DELETE FROM customers
WHERE id NOT IN (
  SELECT MIN(id) FROM customers GROUP BY email
)"""

WRONG_EARLIEST_CREATED_AT = """DELETE FROM customers
WHERE id NOT IN (
  SELECT current.id FROM customers AS current
  WHERE NOT EXISTS (
    SELECT 1 FROM customers AS keep
    WHERE keep.email IS current.email COLLATE BINARY
      AND (keep.created_at < current.created_at
           OR (keep.created_at = current.created_at AND keep.id < current.id))
  )
)"""

WRONG_MAX_ID = """DELETE FROM customers
WHERE id NOT IN (
  SELECT MAX(id) FROM customers GROUP BY email
)"""

WRONG_NOCASE = """DELETE FROM customers
WHERE EXISTS (
  SELECT 1 FROM customers AS keep
  WHERE keep.email = customers.email COLLATE NOCASE
    AND keep.id < customers.id
)"""

WRONG_HARDCODED = """DELETE FROM customers WHERE id IN (2,7,12,30,40)"""

CHECK_QUERY = """SELECT email
FROM customers
GROUP BY email
HAVING COUNT(*) > 1"""

RETRY_REFERENCE = """SELECT r.request_id, COUNT(*) AS successes
FROM retry_events r
JOIN payments p ON p.attempt_id = r.attempt_id
WHERE p.status = 'succeeded'
GROUP BY r.request_id
HAVING COUNT(*) > 1"""

RETRY_ALTERNATIVE = """WITH successful AS (
  SELECT r.request_id
  FROM retry_events r
  JOIN payments p ON p.attempt_id = r.attempt_id
  WHERE p.status = 'succeeded'
)
SELECT request_id, COUNT(*) AS successes
FROM successful
GROUP BY request_id
HAVING COUNT(*) > 1"""

RETRY_WRONG_SAME_ROW_COUNT = """SELECT request_id, COUNT(*) AS successes
FROM retry_events
WHERE request_id IN ('r3','r4')
GROUP BY request_id"""

ROW_COUNT_ZERO = {
    "id": "no-duplicates",
    "scope": "set",
    "kind": "row-count",
    "severity": "critical",
    "expected": {"type": "integer", "value": "0"},
}


def typed(value):
    if value is None:
        return {"type": "null", "value": None}
    if isinstance(value, bool):
        return {"type": "boolean", "value": value}
    if isinstance(value, int):
        return {"type": "integer", "value": str(value)}
    if isinstance(value, float):
        return {"type": "number", "value": value}
    return {"type": "string", "value": value}


def table_state(rows):
    return {
        "name": "customers",
        "columns": CUSTOMER_COLUMNS,
        "rows": [[typed(value) for value in row] for row in rows],
    }


def recordset(columns, rows):
    return {
        "columns": [{"name": name, "type": typ} for name, typ in columns],
        "rows": [[typed(value) for value in row] for row in rows],
    }


def build_fixture(path: Path, statements: list[str]):
    path.parent.mkdir(parents=True, exist_ok=True)
    if path.exists():
        path.unlink()
    connection = sqlite3.connect(path)
    try:
        connection.execute("PRAGMA page_size = 4096")
        connection.execute("PRAGMA auto_vacuum = NONE")
        for statement in statements:
            connection.executescript(statement)
        connection.commit()
        connection.execute("VACUUM")
        connection.commit()
    finally:
        connection.close()
    data = path.read_bytes()
    return {
        "path": path,
        "sha256": hashlib.sha256(data).hexdigest(),
        "bytes": len(data),
    }


def pristine_run(path: Path, callback):
    data = path.read_bytes()
    connection = sqlite3.connect(":memory:")
    try:
        connection.deserialize(data)
        return callback(connection)
    finally:
        connection.close()


def customer_rows(connection):
    cursor = connection.execute("SELECT id, email, created_at FROM customers ORDER BY id")
    return cursor.fetchall()


def run_delete(path: Path, sql: str, runs: int):
    def callback(connection):
        once = None
        for index in range(runs):
            connection.execute(sql)
            if index == 0:
                once = customer_rows(connection)
        repeated = customer_rows(connection)
        return once, repeated

    return pristine_run(path, callback)


def run_query(path: Path, sql: str):
    def callback(connection):
        cursor = connection.execute(sql)
        names = [description[0] for description in cursor.description]
        return names, cursor.fetchall()

    return pristine_run(path, callback)


def write_json(path: Path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")


def fixture_entry(fixture_id: str, pin):
    return {
        "id": fixture_id,
        "label": DEDUP_FIXTURES[fixture_id]["label"],
        "sha256": pin["sha256"],
        "bytes": pin["bytes"],
        "runs": 2,
    }


def generate_deduplicate():
    pins = {}
    for fixture_id, fixture in DEDUP_FIXTURES.items():
        statements = [DEDUP_SCHEMA]
        values = ",".join(
            "({},{},{})".format(
                row[0],
                "NULL" if row[1] is None else "'" + row[1].replace("'", "''") + "'",
                "'" + row[2] + "'",
            )
            for row in fixture["rows"]
        )
        statements.append("INSERT INTO customers VALUES " + values)
        pins[fixture_id] = build_fixture(FIXTURES / "deduplicate-customers" / (fixture_id + ".db"), statements)

    expected = {}
    for fixture_id, pin in pins.items():
        once, repeated = run_delete(pin["path"], REFERENCE_DELETE, 2)
        if once != repeated:
            raise AssertionError("reference delete is not idempotent on " + fixture_id)
        expected[fixture_id] = once

    cases = []
    for fixture_id in DEDUP_FIXTURES:
        cases.append(
            {
                "fixture": fixture_id,
                "oracle": {
                    "kind": "final-state",
                    "tables": [table_state(expected[fixture_id])],
                    "invariants": [
                        {
                            "id": "keep-lowest-id",
                            "kind": "group-min",
                            "table": "customers",
                            "groupBy": ["email"],
                            "column": "id",
                            "description": "Preserve the lowest numeric id per exact BINARY email value; NULL is one group.",
                        }
                    ],
                },
            }
        )

    check_counts = {}
    for fixture_id, pin in pins.items():
        _, rows = run_query(pin["path"], CHECK_QUERY)
        check_counts[fixture_id] = len(rows)

    check_cases = [
        {
            "fixture": fixture_id,
            "expectedOutcome": "fail" if check_counts[fixture_id] else "pass",
        }
        for fixture_id in DEDUP_FIXTURES
    ]

    manifest = {
        "schemaVersion": 1,
        "gradingVersion": "challenge-grading-v1",
        "challengeId": "deduplicate-customers",
        "revision": 1,
        "title": "Delete duplicate customers",
        "objective": "Delete duplicate customer rows while preserving the lowest numeric id for each exact email value. BINARY equality is significant, NULL is one group, and unrelated rows must survive.",
        "difficulty": "intro",
        "dialect": "sqlite",
        "allowedOperation": "write",
        "timing": "off",
        "engine": ENGINE,
        "fixtureSuite": {
            "id": "dedup-customers-v1",
            "fixtures": [fixture_entry(fixture_id, pins[fixture_id]) for fixture_id in DEDUP_FIXTURES],
        },
        "visibleSchema": {
            "tables": [
                {
                    "name": "customers",
                    "columns": [
                        {"name": "id", "type": "integer", "primaryKey": True},
                        {"name": "email", "type": "string", "collation": "BINARY"},
                        {"name": "created_at", "type": "string", "notNull": True},
                    ],
                }
            ]
        },
        "parts": [
            {
                "id": "delete-duplicates",
                "title": "Delete duplicate customer rows",
                "required": True,
                "answerKind": "sql",
                "sql": {"cases": cases},
            },
            {
                "id": "prevention-check",
                "title": "Detect recurring duplicate customers",
                "required": True,
                "answerKind": "check-definition",
                "checkDefinition": {
                    "queryIds": ["queries/duplicate-emails"],
                    "expectation": ROW_COUNT_ZERO,
                    "cases": check_cases,
                },
            },
        ],
        "hints": [
            {"id": "hint-1", "text": "Group rows by the exact stored email value; NULL is still one group."},
            {"id": "hint-2", "text": "The retention rule is the lowest numeric id, not the earliest created_at."},
            {"id": "hint-3", "text": "Delete rows for which a lower id exists in the same BINARY email group."},
        ],
        "referenceSolutions": [
            {"partId": "delete-duplicates", "answer": {"sql": REFERENCE_DELETE}},
            {
                "partId": "prevention-check",
                "answer": {
                    "checkDefinition": {
                        "queryId": "queries/duplicate-emails",
                        "expectation": ROW_COUNT_ZERO,
                    }
                },
            },
        ],
    }
    write_json(CHALLENGES / "deduplicate-customers" / "1.json", manifest)

    sql_answers = {
        "reference": REFERENCE_DELETE,
        "alternative": ALTERNATIVE_DELETE,
        "wrong-earliest-created-at": WRONG_EARLIEST_CREATED_AT,
        "wrong-max-id": WRONG_MAX_ID,
        "wrong-nocase": WRONG_NOCASE,
        "wrong-hardcoded": WRONG_HARDCODED,
    }
    answers = []
    for answer_id, sql in sql_answers.items():
        fixtures = []
        for fixture_id, pin in pins.items():
            once, repeated = run_delete(pin["path"], sql, 2)
            fixtures.append(
                {
                    "fixture": fixture_id,
                    "fixtureSha256": pin["sha256"],
                    "runs": 2,
                    "onceTables": [table_state(once)],
                    "tables": [table_state(repeated)],
                }
            )
        answers.append({"id": answer_id, "kind": "sql", "sql": sql, "fixtures": fixtures})

    check_fixtures = []
    for fixture_id, pin in pins.items():
        names, rows = run_query(pin["path"], CHECK_QUERY)
        if names != ["email"]:
            raise AssertionError("unexpected check columns: " + repr(names))
        check_fixtures.append({"fixture": fixture_id, "fixtureSha256": pin["sha256"], "runs": 1, "rows": recordset([("email", "string")], rows)})
    answers.append({"id": "check-duplicate-groups", "kind": "check-observation", "sql": CHECK_QUERY, "fixtures": check_fixtures})

    write_json(
        GOLDENS / "deduplicate-customers.json",
        {
            "generatedWith": {"python": platform.python_version(), "sqlite": sqlite3.sqlite_version},
            "challenge": "deduplicate-customers",
            "revision": 1,
            "answers": answers,
        },
    )


def generate_retry_evidence():
    pin = build_fixture(FIXTURES / "retry-evidence" / "retry-evidence.db", [RETRY_SCHEMA, *RETRY_ROWS])
    names, reference_rows = run_query(pin["path"], RETRY_REFERENCE)
    if names != ["request_id", "successes"]:
        raise AssertionError("unexpected reference columns: " + repr(names))
    if reference_rows != [("r2", 2), ("r4", 2)]:
        raise AssertionError("unexpected reference rows: " + repr(reference_rows))

    manifest = {
        "schemaVersion": 1,
        "gradingVersion": "challenge-grading-v1",
        "challengeId": "retry-evidence",
        "revision": 1,
        "title": "Correlate retries with successful payments",
        "objective": "Return the request IDs that produced more than one successful payment, then select the exact evidence pairs that support the conclusion.",
        "difficulty": "intermediate",
        "dialect": "sqlite",
        "allowedOperation": "read",
        "timing": "off",
        "engine": ENGINE,
        "fixtureSuite": {
            "id": "retry-evidence-v1",
            "fixtures": [
                {
                    "id": "retry-evidence",
                    "label": "Synthetic payments, retry events and deployments",
                    "sha256": pin["sha256"],
                    "bytes": pin["bytes"],
                    "runs": 1,
                }
            ],
        },
        "visibleSchema": {
            "tables": [
                {
                    "name": "orders",
                    "columns": [
                        {"name": "id", "type": "integer", "primaryKey": True},
                        {"name": "customer_id", "type": "integer"},
                        {"name": "total_cents", "type": "integer"},
                    ],
                },
                {
                    "name": "payments",
                    "columns": [
                        {"name": "id", "type": "integer", "primaryKey": True},
                        {"name": "order_id", "type": "integer"},
                        {"name": "attempt_id", "type": "string"},
                        {"name": "status", "type": "string"},
                        {"name": "amount_cents", "type": "integer"},
                        {"name": "created_at", "type": "string"},
                    ],
                },
                {
                    "name": "retry_events",
                    "columns": [
                        {"name": "attempt_id", "type": "string"},
                        {"name": "request_id", "type": "string"},
                        {"name": "idempotency_key", "type": "string"},
                        {"name": "outcome", "type": "string"},
                        {"name": "created_at", "type": "string"},
                    ],
                },
            ]
        },
        "parts": [
            {
                "id": "requests",
                "title": "Find requests with more than one successful payment",
                "required": True,
                "answerKind": "sql",
                "sql": {
                    "cases": [
                        {
                            "fixture": "retry-evidence",
                            "oracle": {
                                "kind": "result",
                                "columns": [
                                    {"name": "request_id", "type": "string"},
                                    {"name": "successes", "type": "integer"},
                                ],
                                "rows": [[typed(value) for value in row] for row in reference_rows],
                                "ordering": "unordered",
                            },
                        }
                    ]
                },
            },
            {
                "id": "evidence-pairs",
                "title": "Select the exact duplicate evidence pairs",
                "required": True,
                "answerKind": "structured",
                "structured": {
                    "fields": [
                        {
                            "id": "evidence-pairs",
                            "label": "Evidence pairs",
                            "cardinality": "multi",
                            "required": True,
                            "minSelections": 2,
                            "maxSelections": 2,
                            "options": [
                                {"id": "r1-a1-trace", "label": "r1 / a1 / trace-r1"},
                                {"id": "r2-a2-a3-trace", "label": "r2 / a2+a3 / trace-r2"},
                                {"id": "r3-a4-a5-trace", "label": "r3 / a4+a5 / trace-r3"},
                                {"id": "r4-a6-a7-trace", "label": "r4 / a6+a7 / trace-r4"},
                            ],
                        }
                    ],
                    "expected": {
                        "selections": {
                            "evidence-pairs": ["r2-a2-a3-trace", "r4-a6-a7-trace"],
                        }
                    },
                },
            },
        ],
        "hints": [
            {"id": "hint-1", "text": "Join retry events to payments by attempt_id."},
            {"id": "hint-2", "text": "Count only succeeded payments, then group by request_id."},
            {"id": "hint-3", "text": "A request with a failed attempt and one success is not a duplicate."},
        ],
        "referenceSolutions": [
            {"partId": "requests", "answer": {"sql": RETRY_REFERENCE}},
            {
                "partId": "evidence-pairs",
                "answer": {
                    "structured": {
                        "selections": {
                            "evidence-pairs": ["r2-a2-a3-trace", "r4-a6-a7-trace"],
                        }
                    }
                },
            },
        ],
    }
    write_json(CHALLENGES / "retry-evidence" / "1.json", manifest)

    answers = []
    for answer_id, sql in {
        "reference": RETRY_REFERENCE,
        "alternative": RETRY_ALTERNATIVE,
        "wrong-same-row-count": RETRY_WRONG_SAME_ROW_COUNT,
    }.items():
        names, rows = run_query(pin["path"], sql)
        if names != ["request_id", "successes"]:
            raise AssertionError("unexpected columns for " + answer_id + ": " + repr(names))
        answers.append(
            {
                "id": answer_id,
                "kind": "sql",
                "sql": sql,
                "fixtures": [
                    {
                        "fixture": "retry-evidence",
                        "fixtureSha256": pin["sha256"],
                        "runs": 1,
                        "rows": recordset([("request_id", "string"), ("successes", "integer")], rows),
                    }
                ],
            }
        )

    write_json(
        GOLDENS / "retry-evidence.json",
        {
            "generatedWith": {"python": platform.python_version(), "sqlite": sqlite3.sqlite_version},
            "challenge": "retry-evidence",
            "revision": 1,
            "answers": answers,
        },
    )


if __name__ == "__main__":
    generate_deduplicate()
    generate_retry_evidence()
    print("generated fixtures and goldens with SQLite", sqlite3.sqlite_version)
