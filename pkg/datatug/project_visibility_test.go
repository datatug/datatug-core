package datatug

import (
	"errors"
	"testing"
)

func TestVisibilityPreservesReferencesWithoutClaimingCompleteGraph(t *testing.T) {
	q := ProjectObjectRef{Kind: "query", ID: "sales"}
	catalog := ProjectLifecycleCatalog{Revision: "v1", ArchiveSupported: true, Items: []ProjectUsageItem{{Ref: q, Title: "Sales"}}}
	review, err := ReviewProjectVisibility(catalog, q)
	if err != nil || !review.CanArchive || review.CanDelete || review.Complete || !review.ArchiveSupported {
		t.Fatalf("unexpected visibility review: %+v, %v", review, err)
	}
	write := ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "v1", Change: ProjectLifecycleChange{Object: q, Action: "archive"}}
	if _, err = PlanProjectVisibility(catalog, write); err != nil {
		t.Fatal(err)
	}
	write.Change.Action = "delete"
	if _, err = PlanProjectVisibility(catalog, write); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatalf("visibility granted destructive deletion: %v", err)
	}
	catalog.Items[0].Archived = true
	review, err = ReviewProjectVisibility(catalog, q)
	if err != nil || review.CanArchive || !review.CanRestore {
		t.Fatalf("restore unavailable: %+v, %v", review, err)
	}
}

func TestVisibilityRejectsStaleRestrictedUnknownAndHistoricalTargets(t *testing.T) {
	q := ProjectObjectRef{Kind: "query", ID: "q"}
	base := ProjectLifecycleCatalog{Revision: "v1", ArchiveSupported: true, Items: []ProjectUsageItem{{Ref: q}}}
	w := ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "stale", Change: ProjectLifecycleChange{Object: q, Action: "archive"}}
	if _, err := PlanProjectVisibility(base, w); !errors.Is(err, ErrProjectLifecycleConflict) {
		t.Fatal(err)
	}
	w.ExpectedRevision = "v1"
	for _, item := range []ProjectUsageItem{{Ref: q, Restricted: true}, {Ref: q, Historical: true}, {Ref: ProjectObjectRef{Kind: "run", ID: "run"}}} {
		base.Items = []ProjectUsageItem{item}
		if _, err := PlanProjectVisibility(base, w); !errors.Is(err, ErrProjectUsageIncomplete) {
			t.Fatalf("unsafe catalog admitted: %+v %v", item, err)
		}
	}
	base.Items = []ProjectUsageItem{{Ref: q}, {Ref: q}}
	if _, err := PlanProjectVisibility(base, w); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal(err)
	}
	base.Items = nil
	if _, err := PlanProjectVisibility(base, w); !errors.Is(err, ErrProjectLifecycleConflict) {
		t.Fatal(err)
	}
}

func TestVisibilityRejectsInvalidWriteAndUnsupportedCatalog(t *testing.T) {
	q := ProjectObjectRef{Kind: "query", ID: "q"}
	base := ProjectLifecycleCatalog{Revision: "r", ArchiveSupported: true, Items: []ProjectUsageItem{{Ref: q}}}
	w := ProjectLifecycleWrite{OperationID: "x", ExpectedRevision: "r", Change: ProjectLifecycleChange{Object: q, Action: "archive"}}
	invalid := w
	invalid.OperationID = ""
	if _, err := PlanProjectVisibility(base, invalid); !errors.Is(err, ErrProjectLifecycleInvalid) {
		t.Fatal(err)
	}
	base.ArchiveSupported = false
	if _, err := PlanProjectVisibility(base, w); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal(err)
	}
	if _, err := ReviewProjectVisibility(base, q); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal(err)
	}
	opts := GetStoreOptions(Depth(2), ActiveItemsOnly())
	copied := GetStoreOptions(opts.ToSlice()...)
	if !copied.ActiveItemsOnly() || copied.Depth() != 2 || !copied.Next().ActiveItemsOnly() {
		t.Fatal(copied)
	}
}
