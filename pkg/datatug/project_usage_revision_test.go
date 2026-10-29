package datatug

import (
	"errors"
	"testing"
)

func TestProjectUsageRevisionBindsArchiveAndContent(t *testing.T) {
	a := ProjectObjectUsage{Ref: ProjectObjectRef{Kind: "query", ID: "q"}, ContentRevision: "q"}
	b := ProjectObjectUsage{Ref: ProjectObjectRef{Kind: "board", ID: "b"}, ContentRevision: "b", References: []ProjectObjectRef{a.Ref}}
	first, err := ProjectUsageRevision([]ProjectObjectUsage{a, b})
	if err != nil {
		t.Fatal(err)
	}
	reverse, _ := ProjectUsageRevision([]ProjectObjectUsage{b, a})
	if first != reverse {
		t.Fatal("order changed revision")
	}
	b.Archived = true
	changed, _ := ProjectUsageRevision([]ProjectObjectUsage{a, b})
	if first == changed {
		t.Fatal("archive state not bound")
	}
	if _, err = ProjectUsageRevision([]ProjectObjectUsage{a, a}); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal(err)
	}
}

func TestProjectUsageRevisionBindsHistoryCompletion(t *testing.T) {
	q := ProjectObjectUsage{Ref: ProjectObjectRef{Kind: "query", ID: "q"}, ContentRevision: "q"}
	run := ProjectObjectUsage{Ref: ProjectObjectRef{Kind: "run", ID: "r"}, ContentRevision: "r", References: []ProjectObjectRef{q.Ref}}
	active, err := ProjectUsageRevision([]ProjectObjectUsage{q, run})
	if err != nil {
		t.Fatal(err)
	}
	run.Historical = true
	completed, err := ProjectUsageRevision([]ProjectObjectUsage{q, run})
	if err != nil || active == completed {
		t.Fatalf("completion must invalidate prior review: %v", err)
	}
}
