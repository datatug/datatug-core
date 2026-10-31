package datatug

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestProjectLifecycleAdmissionDoesNotSerializeHiddenSourceIDs(t *testing.T) {
	query, hidden := ProjectObjectRef{Kind: "query", ID: "q"}, ProjectObjectRef{Kind: "board", ID: "private-payroll-board"}
	snapshot := ProjectUsageSnapshot{Revision: "r", Complete: true, Objects: []ProjectObjectUsage{
		{Ref: query, ContentRevision: "q"},
		{Ref: hidden, ContentRevision: "b", References: []ProjectObjectRef{query}},
	}}
	plan, err := PlanProjectLifecycle(snapshot, ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r", Change: ProjectLifecycleChange{query, "delete"}})
	if !errors.Is(err, ErrProjectObjectReferenced) || len(plan.Blockers) != 1 {
		t.Fatalf("expected internal blocker: %+v %v", plan, err)
	}
	public, err := json.Marshal(plan)
	if err != nil || strings.Contains(string(public), hidden.ID) {
		t.Fatalf("raw admission disclosed hidden source: %s %v", public, err)
	}
}

func TestProjectLifecycleReferencePolicy(t *testing.T) {
	query, board := ProjectObjectRef{Kind: "query", ID: "revenue"}, ProjectObjectRef{Kind: "board", ID: "overview"}
	for _, tc := range []struct {
		name, action                  string
		referenced, archived, blocked bool
	}{
		{"active use blocks delete", "delete", true, false, true},
		{"active use preserves archive", "archive", true, false, false},
		{"archived use blocks delete", "delete", true, true, true},
		{"archived use allows archive", "archive", true, true, false},
		{"unreferenced allows delete", "delete", false, false, false},
		{"unreferenced allows archive", "archive", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := ProjectUsageSnapshot{Revision: "r1", Complete: true, Objects: []ProjectObjectUsage{{Ref: query, ContentRevision: "q1"}}}
			if tc.referenced {
				s.Objects = append(s.Objects, ProjectObjectUsage{Ref: board, ContentRevision: "b1", Archived: tc.archived, References: []ProjectObjectRef{query}})
			}
			plan, err := PlanProjectLifecycle(s, ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r1", Change: ProjectLifecycleChange{query, tc.action}})
			if errors.Is(err, ErrProjectObjectReferenced) != tc.blocked {
				t.Fatalf("blocked=%v err=%v", tc.blocked, err)
			}
			if tc.blocked && len(plan.Blockers) != 1 {
				t.Fatal("missing exact blocker")
			}
		})
	}
}
func TestProjectLifecycleFailsClosed(t *testing.T) {
	ref := ProjectObjectRef{Kind: "query", ID: "q"}
	s := ProjectUsageSnapshot{Complete: true, Revision: "r", Objects: []ProjectObjectUsage{{Ref: ref, ContentRevision: "q"}}}
	w := ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r", Change: ProjectLifecycleChange{ref, "delete"}}
	s.Complete = false
	if _, err := PlanProjectLifecycle(s, w); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal(err)
	}
	s.Complete = true
	w.ExpectedRevision = "old"
	if _, err := PlanProjectLifecycle(s, w); !errors.Is(err, ErrProjectLifecycleConflict) {
		t.Fatal(err)
	}
	w.ExpectedRevision = "r"
	s.Objects = append(s.Objects, s.Objects[0])
	if _, err := PlanProjectLifecycle(s, w); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal(err)
	}
}

func TestProjectLifecycleRestorePreservesArchivedDependencies(t *testing.T) {
	query, board := ProjectObjectRef{Kind: "query", ID: "q"}, ProjectObjectRef{Kind: "board", ID: "b"}
	s := ProjectUsageSnapshot{Revision: "r", Complete: true, Objects: []ProjectObjectUsage{
		{Ref: query, Archived: true, ContentRevision: "q"},
		{Ref: board, Archived: true, ContentRevision: "b", References: []ProjectObjectRef{query}},
	}}
	w := ProjectLifecycleWrite{OperationID: "restore", ExpectedRevision: "r", Change: ProjectLifecycleChange{board, "restore"}}
	if _, err := PlanProjectLifecycle(s, w); err != nil {
		t.Fatalf("restore must preserve the still accessible archived query: %v", err)
	}
	s.Objects[0].Archived = false // separate query restore committed
	s.Revision = "after"
	if _, err := PlanProjectLifecycle(s, w); !errors.Is(err, ErrProjectLifecycleConflict) {
		t.Fatalf("old review must not authorize a later write: %v", err)
	}
	w.ExpectedRevision = "after"
	if _, err := PlanProjectLifecycle(s, w); err != nil {
		t.Fatal(err)
	}
}

func TestProjectLifecycleRetainedHistoryBlocksDeletion(t *testing.T) {
	query := ProjectObjectRef{Kind: "query", ID: "revenue"}
	for _, kind := range []string{"run", "execution", "snapshot"} {
		for _, historical := range []bool{false, true} {
			for _, action := range []string{"archive", "delete"} {
				t.Run(fmt.Sprintf("%s/historical=%t/%s", kind, historical, action), func(t *testing.T) {
					source := ProjectObjectRef{Kind: kind, ID: "retained-record"}
					s := ProjectUsageSnapshot{Revision: "r", Complete: true, Objects: []ProjectObjectUsage{
						{Ref: query, ContentRevision: "q"},
						{Ref: source, Historical: historical, ContentRevision: "h", References: []ProjectObjectRef{query}},
					}}
					plan, err := PlanProjectLifecycle(s, ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r", Change: ProjectLifecycleChange{query, action}})
					blocked := action == "delete"
					if blocked && (!errors.Is(err, ErrProjectObjectReferenced) || len(plan.Blockers) != 1 || plan.Blockers[0].UsedBy != source) {
						t.Fatalf("retained reference lost: %+v %v", plan, err)
					}
					if !blocked && err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

func TestProjectLifecycleCannotPurgeHistoryToUnblockDefinitionDeletion(t *testing.T) {
	history := ProjectObjectRef{Kind: "snapshot", ID: "retained"}
	s := ProjectUsageSnapshot{Revision: "r", Complete: true, Objects: []ProjectObjectUsage{{Ref: history, Historical: true, ContentRevision: "h"}}}
	for _, action := range []string{"archive", "restore", "delete"} {
		if _, err := PlanProjectLifecycle(s, ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r", Change: ProjectLifecycleChange{history, action}}); !errors.Is(err, ErrProjectLifecycleInvalid) {
			t.Fatalf("definition lifecycle must not mutate retained history (%s): %v", action, err)
		}
	}
}

func TestProjectLifecycleHistoryCannotBeMadeInactiveByArchiveFlag(t *testing.T) {
	query := ProjectObjectRef{Kind: "query", ID: "q"}
	for _, historical := range []bool{false, true} {
		s := ProjectUsageSnapshot{Revision: "r", Complete: true, Objects: []ProjectObjectUsage{
			{Ref: query, ContentRevision: "q"},
			{Ref: ProjectObjectRef{Kind: "run", ID: "r"}, Archived: true, Historical: historical, ContentRevision: "run", References: []ProjectObjectRef{query}},
		}}
		for _, action := range []string{"archive", "delete"} {
			if _, err := PlanProjectLifecycle(s, ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r", Change: ProjectLifecycleChange{query, action}}); !errors.Is(err, ErrProjectUsageIncomplete) {
				t.Fatalf("ambiguous history state must fail closed (%t/%s): %v", historical, action, err)
			}
		}
	}
}
