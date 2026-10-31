package datatug

import (
	"errors"
	"testing"
)

func TestProjectLifecycleReviewSingleObjectWithoutExposingRestrictedItems(t *testing.T) {
	board, q, check, other := ProjectObjectRef{Kind: "board", ID: "sales"}, ProjectObjectRef{Kind: "query", ID: "revenue"}, ProjectObjectRef{Kind: "check", ID: "totals"}, ProjectObjectRef{Kind: "board", ID: "private-board"}
	snapshot := ProjectUsageSnapshot{Revision: "r", Complete: true, Objects: []ProjectObjectUsage{
		{Ref: board, Title: "Sales", ContentRevision: "b", References: []ProjectObjectRef{q, check}},
		{Ref: q, Title: "Revenue", ContentRevision: "q"}, {Ref: check, Title: "Totals", ContentRevision: "c"},
		{Ref: other, Title: "Hidden board", ContentRevision: "other", References: []ProjectObjectRef{q}},
	}}
	review, err := ReviewProjectLifecycle(snapshot, board, func(ref ProjectObjectRef) bool { return ref != other })
	if err != nil {
		t.Fatal(err)
	}
	if !review.CanArchive || !review.CanDelete {
		t.Fatalf("%+v", review)
	}
	queryReview, err := ReviewProjectLifecycle(snapshot, q, func(ref ProjectObjectRef) bool { return ref != other })
	if err != nil {
		t.Fatal(err)
	}
	if !queryReview.CanArchive || queryReview.CanDelete || len(queryReview.Incoming) != 2 || !queryReview.Incoming[1].Restricted || queryReview.Incoming[1].Title != "" || queryReview.Incoming[1].Ref.ID != "" {
		t.Fatalf("%+v", queryReview)
	}
}
func TestProjectLifecycleReviewIncompleteAndMissingTarget(t *testing.T) {
	ref := ProjectObjectRef{Kind: "board", ID: "b"}
	s := ProjectUsageSnapshot{Revision: "r", Complete: false, Objects: []ProjectObjectUsage{{Ref: ref, ContentRevision: "b"}}}
	review, err := ReviewProjectLifecycle(s, ref, nil)
	if err != nil || review.CanArchive || review.CanDelete || review.CanRestore {
		t.Fatalf("%+v %v", review, err)
	}
	if _, err = ReviewProjectLifecycle(s, ProjectObjectRef{Kind: "board", ID: "missing"}, nil); err == nil {
		t.Fatal("missing target accepted")
	}
}

func TestProjectLifecycleReviewRejectsMalformedSnapshotsBeforeDisclosure(t *testing.T) {
	query, board := ProjectObjectRef{Kind: "query", ID: "q"}, ProjectObjectRef{Kind: "board", ID: "b"}
	valid := []ProjectObjectUsage{{Ref: query, ContentRevision: "q"}, {Ref: board, ContentRevision: "b", References: []ProjectObjectRef{query}}}
	for _, tc := range []struct {
		name, revision string
		objects        []ProjectObjectUsage
	}{
		{"empty revision", "", valid},
		{"duplicate source", "r", append(append([]ProjectObjectUsage{}, valid...), valid[0])},
		{"unresolved reference", "r", valid[1:]},
		{"invalid source", "r", []ProjectObjectUsage{{Ref: ProjectObjectRef{Kind: "query", ID: "../outside"}, ContentRevision: "q"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			review, err := ReviewProjectLifecycle(ProjectUsageSnapshot{Revision: tc.revision, Complete: true, Objects: tc.objects}, board, func(ProjectObjectRef) bool { return true })
			if !errors.Is(err, ErrProjectUsageIncomplete) || review.Complete || review.CanArchive || review.CanDelete || review.CanRestore || review.Target.Title != "" || len(review.Incoming) != 0 {
				t.Fatalf("malformed snapshot disclosed as valid: %+v %v", review, err)
			}
		})
	}
}

func TestProjectLifecycleReviewIncludesRetainedHistoryWithoutGrantingAccess(t *testing.T) {
	query, history := ProjectObjectRef{Kind: "query", ID: "q"}, ProjectObjectRef{Kind: "snapshot", ID: "private-history"}
	s := ProjectUsageSnapshot{Revision: "r", Complete: true, Objects: []ProjectObjectUsage{
		{Ref: query, ContentRevision: "q"},
		{Ref: history, Title: "Private execution history", Historical: true, ContentRevision: "h", References: []ProjectObjectRef{query}},
	}}
	review, err := ReviewProjectLifecycle(s, query, func(ref ProjectObjectRef) bool { return ref != history })
	if err != nil || !review.CanArchive || review.CanDelete || len(review.Incoming) != 1 || !review.Incoming[0].Restricted || !review.Incoming[0].Historical || review.Incoming[0].Ref.ID != "" || review.Incoming[0].Title != "" {
		t.Fatalf("retained private history mishandled: %+v %v", review, err)
	}
	s.Complete = false // e.g. a history page or private collection could not be scanned
	review, err = ReviewProjectLifecycle(s, query, nil)
	if err != nil || review.CanArchive || review.CanDelete || review.Complete {
		t.Fatalf("incomplete history admitted: %+v %v", review, err)
	}
}
