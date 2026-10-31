package detective

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRestoreCasePreservesOwnedHistoryAndReferences(t *testing.T) {
	old := Case{ID: "legacy", Title: "Private case title", Status: "archived", Version: 4, ClosureReason: "Private closure", Context: map[string]InputValue{}, IncidentRefs: []string{"incident"}, ScenarioRefs: []string{"scenario"}, RunRefs: []string{"run"}, Evidence: []EvidenceRef{{ID: "evidence", Kind: "check-run", Ref: "run"}}, Timeline: []CaseEvent{{ID: "created", At: lifecycleAt, Actor: "owner", Type: "created"}}, TimelineOffset: 7, CreatedAt: lifecycleAt, UpdatedAt: lifecycleAt}
	before, err := cloneCase(old)
	if err != nil {
		t.Fatal(err)
	}
	at := lifecycleAt.Add(time.Minute)
	restored, audit, err := RestoreCase(old, "owner", at)
	if err != nil || restored.Status != "active" || restored.Version != 5 || restored.ClosureReason != "" || restored.UpdatedAt != at {
		t.Fatal(restored, err)
	}
	expected := before
	expected.Status = "active"
	expected.Version++
	expected.ClosureReason = ""
	expected.UpdatedAt = at
	expected.Timeline = append(expected.Timeline, CaseEvent{ID: "lifecycle-5", At: at, Actor: "owner", Type: "restored"})
	if !reflect.DeepEqual(expected, restored) {
		t.Fatalf("restoration replaced retained data: %+v", restored)
	}
	if !reflect.DeepEqual(old, before) {
		t.Fatal("restore mutated caller's stored Case")
	}
	restored.RunRefs[0] = "other"
	if old.RunRefs[0] != "run" {
		t.Fatal("retained references alias caller")
	}
	b, _ := json.Marshal(audit)
	if audit.Action != "restore-case" || audit.CaseID != "legacy" || audit.ID != "case-legacy-5" || strings.Contains(string(b), "Private") {
		t.Fatal("unsafe audit", string(b))
	}
}
func TestRestoreCaseRejectsUnsafeOrForeignOwnedState(t *testing.T) {
	base := Case{ID: "legacy", Title: "Case", Status: "archived", Version: 1}
	for _, mutate := range []func(*Case){func(c *Case) { c.WorkItemID = "work" }, func(c *Case) { c.Status = "active" }, func(c *Case) { c.ID = "../outside" }, func(c *Case) { c.Version = 0 }, func(c *Case) { c.Version = int64(^uint64(0) >> 1) }, func(c *Case) { c.Context = map[string]InputValue{"bad": {}} }, func(c *Case) { c.IncidentRefs = []string{""} }, func(c *Case) { c.CreatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }} {
		bad := base
		mutate(&bad)
		if _, _, err := RestoreCase(bad, "owner", lifecycleAt); !errors.Is(err, ErrInvalid) {
			t.Fatal("unsafe restore", bad, err)
		}
	}
	for _, actor := range []string{"", " "} {
		if _, _, err := RestoreCase(base, actor, lifecycleAt); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, _, err := RestoreCase(base, "owner", time.Time{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestRestoreCaseRetainsCollidingUserTimelineIDs(t *testing.T) {
	old := Case{ID: "legacy", Title: "Case", Status: "archived", Version: 4, Timeline: []CaseEvent{{ID: "lifecycle-5", Type: "note", Actor: "owner", At: lifecycleAt}, {ID: "restore-5-0", Type: "note", Actor: "owner", At: lifecycleAt}}}
	restored, _, err := RestoreCase(old, "owner", lifecycleAt.Add(time.Minute))
	if err != nil || len(restored.Timeline) != 3 || restored.Timeline[2].ID != "restore-5-1" {
		t.Fatal(restored, err)
	}
	if restored.Timeline[0] != old.Timeline[0] || restored.Timeline[1] != old.Timeline[1] || len(old.Timeline) != 2 {
		t.Fatal("restore replaced retained user event")
	}
}
