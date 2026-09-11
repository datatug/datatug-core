package filestore

import "testing"

// Task 1: freeze the additive revision contract. computeQueryRevision must
// derive a QueryRevision from the exact persisted bytes - JSON metadata,
// the body sidecar's type-derived extension and body bytes - with
// unambiguous framing, so any external hand edit to either persisted file
// (including one that only shifts bytes across the json/body-ext/body
// boundary) changes the revision and invalidates a stale writer. It never
// takes the id-bearing sidecar file name - see
// TestLoadQueryRevision_RevisionIgnoresTheRequestedIDSpelling.

func TestComputeQueryRevision_Deterministic(t *testing.T) {
	a := computeQueryRevision([]byte(`{"id":"q1"}`), "dtql", []byte("from:\n  name: Invoice\n"))
	b := computeQueryRevision([]byte(`{"id":"q1"}`), "dtql", []byte("from:\n  name: Invoice\n"))
	if a != b {
		t.Fatalf("expected identical inputs to produce identical revisions, got %q vs %q", a, b)
	}
	if a == "" {
		t.Fatalf("expected a non-empty revision")
	}
}

func TestComputeQueryRevision_JSONHandEditChangesRevision(t *testing.T) {
	before := computeQueryRevision([]byte(`{"id":"q1","title":"Q1"}`), "dtql", []byte("body"))
	// Simulate an external hand edit of the JSON sidecar only.
	after := computeQueryRevision([]byte(`{"id":"q1","title":"Q1 edited"}`), "dtql", []byte("body"))
	if before == after {
		t.Fatalf("expected a hand edit of the JSON bytes to change the revision")
	}
}

func TestComputeQueryRevision_BodyHandEditChangesRevision(t *testing.T) {
	before := computeQueryRevision([]byte(`{"id":"q1"}`), "dtql", []byte("SELECT 1"))
	after := computeQueryRevision([]byte(`{"id":"q1"}`), "dtql", []byte("SELECT 2"))
	if before == after {
		t.Fatalf("expected a hand edit of the body bytes to change the revision")
	}
}

func TestComputeQueryRevision_BodyTypeChangeChangesRevision(t *testing.T) {
	// A type change (e.g. SQL -> DTQL) swaps the body sidecar's extension
	// while possibly leaving the body bytes identical; the revision must
	// still change since the persisted pair is different.
	before := computeQueryRevision([]byte(`{"id":"q1"}`), "sql", []byte("SELECT 1"))
	after := computeQueryRevision([]byte(`{"id":"q1"}`), "dtql", []byte("SELECT 1"))
	if before == after {
		t.Fatalf("expected a body sidecar type change to change the revision")
	}
}

func TestComputeQueryRevision_NoBodyIsDistinctFromEmptyBody(t *testing.T) {
	noBody := computeQueryRevision([]byte(`{"id":"q1"}`), "", nil)
	emptyBody := computeQueryRevision([]byte(`{"id":"q1"}`), "sql", []byte(""))
	if noBody == emptyBody {
		t.Fatalf("expected an absent body sidecar to hash differently from an empty one")
	}
}

// TestComputeQueryRevision_FramingPreventsBoundaryAmbiguity proves the
// length-prefixed framing: without it, concatenating jsonBytes with
// bodyExt could let bytes shift across the boundary and still hash the
// same, silently hiding a real difference in what was persisted.
func TestComputeQueryRevision_FramingPreventsBoundaryAmbiguity(t *testing.T) {
	a := computeQueryRevision([]byte("ab"), "c", []byte("body"))
	b := computeQueryRevision([]byte("a"), "bc", []byte("body"))
	if a == b {
		t.Fatalf("expected framing to distinguish (%q,%q) from (%q,%q)", "ab", "c", "a", "bc")
	}
}
