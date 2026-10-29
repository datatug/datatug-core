package datatug

import (
	"errors"
	"testing"
)

func TestProjectFavoriteBlocksDeleteOnlyAndCannotBeMutatedAsDefinition(t *testing.T) {
	target := ProjectObjectRef{Kind: "board", ID: "overview"}
	favorite := ProjectObjectRef{Kind: "favorite", ID: "opaque-private-index"}
	snapshot := ProjectUsageSnapshot{Complete: true, Revision: "r", Objects: []ProjectObjectUsage{
		{Ref: target, ContentRevision: "board"},
		{Ref: favorite, Historical: true, ContentRevision: "favorite", References: []ProjectObjectRef{target}},
	}}
	for _, action := range []string{"archive", "delete", "restore"} {
		_, err := PlanProjectLifecycle(snapshot, ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r", Change: ProjectLifecycleChange{Object: target, Action: action}})
		if action == "delete" && !errors.Is(err, ErrProjectObjectReferenced) || action != "delete" && err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		_, err = PlanProjectLifecycle(snapshot, ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r", Change: ProjectLifecycleChange{Object: favorite, Action: action}})
		if !errors.Is(err, ErrProjectLifecycleInvalid) {
			t.Fatal("favorite admitted as project mutation target")
		}
	}
	snapshot.Objects[1].Historical = false
	if _, err := ProjectUsageRevision(snapshot.Objects); !errors.Is(err, ErrProjectUsageIncomplete) {
		t.Fatal("active favorite accepted")
	}
}
