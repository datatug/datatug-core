package detective

import (
	"testing"
	"time"
)

func TestCaseWorkAssociationIsServerEstablishedAndImmutable(t *testing.T) {
	at := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	if _, _, err := CreateCase(Case{ID: "case", Title: "Case", WorkItemID: "foreign-work"}, "actor", at); err == nil {
		t.Fatal("client Work association accepted")
	}
	item, _, err := CreateCase(Case{ID: "case", Title: "Case"}, "actor", at)
	if err != nil {
		t.Fatal(err)
	}
	// The trusted backend establishes the reciprocal association after creation.
	item.WorkItemID = "work-item"
	next := item
	next.Title = "Updated title"
	updated, _, err := UpdateCase(item, next, "actor", at.Add(time.Second))
	if err != nil || updated.WorkItemID != "work-item" {
		t.Fatalf("association not retained: %+v %v", updated, err)
	}
	for _, id := range []string{"", "other-work"} {
		next.WorkItemID = id
		if _, _, err = UpdateCase(item, next, "actor", at.Add(time.Second)); err == nil {
			t.Fatalf("association changed to %q", id)
		}
	}
}
