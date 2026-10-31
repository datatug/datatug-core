package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestRetainedContextBindingsRejectDetachedRevisionWithoutPartialOutput(t *testing.T) {
	inventory, _ := retainedContextFixture(t)
	if _, err := ProjectRetainedContextBindings(context.Background(), inventory); err != nil {
		t.Fatal(err)
	}
	var detached RetainedDocument
	for _, document := range inventory.Documents {
		if document.Collection == Revisions && document.ID == "revision-a" {
			detached = document
			break
		}
	}
	if detached.ID == "" {
		t.Fatal("historical revision fixture missing")
	}
	var revision EntryRevision
	if err := json.Unmarshal(detached.Payload, &revision); err != nil {
		t.Fatal(err)
	}
	// Keep all existing heads, parent chains and applied sets valid. This
	// independent historical row must be validated even though no current head
	// or other revision points at it. Its only broken join is its missing entry.
	revision.ID, revision.EntryID, revision.ParentRevisionID = "detached-revision", "missing-entry", ""
	detached.ID = revision.ID
	var err error
	detached.Payload, err = json.Marshal(revision)
	if err != nil {
		t.Fatal(err)
	}
	inventory.Documents = append(inventory.Documents, detached)
	if out, err := ProjectRetainedContextBindings(context.Background(), inventory); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatalf("detached historical revision produced %d partial bindings and error %v", len(out), err)
	}
}
