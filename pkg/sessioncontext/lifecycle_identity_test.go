package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/dal-go/record"
)

func TestSessionLifecycleIDGoldenAndNamespaceSeparation(t *testing.T) {
	repo, db := retainedInventoryFixture(t)
	const golden = "3ecd26ca2ae6c1717a591cd1fe866599ea04a8cc39fcf2731e3eac5280833407/same-session"
	got, err := SessionLifecycleID(repo.NativeNamespace(), "same-session")
	if err != nil || got != golden {
		t.Fatalf("owning repository identity = %q, %v; want %q", got, err, golden)
	}
	for _, change := range []string{"parent", "mount", "space", "project"} {
		t.Run(change, func(t *testing.T) {
			mount, parent, space, project := "session-db", "project-a", "space-a", "project-a"
			switch change {
			case "parent":
				parent = "other-parent"
			case "mount":
				mount = "other-mount"
			case "space":
				space = "other-space"
			case "project":
				project = "other-project"
			}
			other, err := NewNativePinRepository(testMountedDatabase{id: mount, db: db}, record.NewKeyWithID("sessionProjects", parent), "firestore", space, project)
			if err != nil {
				t.Fatal(err)
			}
			id, err := SessionLifecycleID(other.NativeNamespace(), "same-session")
			if err != nil || id == got || !strings.HasSuffix(id, "/same-session") {
				t.Fatalf("colliding raw ID lost %s authority: %q %v", change, id, err)
			}
		})
	}
	maxID := "A" + strings.Repeat("x", 127)
	if id, err := SessionLifecycleID(repo.NativeNamespace(), maxID); err != nil || len(id) != 64+1+128 {
		t.Fatalf("maximum safe session ID rejected: %q %v", id, err)
	}
}

func TestSessionLifecycleIDRejectsUnboundMalformedAndUnsafeIdentity(t *testing.T) {
	repo, _ := retainedInventoryFixture(t)
	for name, mutate := range map[string]func(*NativeNamespace){
		"unbound":              func(n *NativeNamespace) { *n = NativeNamespace{} },
		"missing digest":       func(n *NativeNamespace) { n.NamespaceID = "" },
		"short digest":         func(n *NativeNamespace) { n.NamespaceID = strings.Repeat("a", 63) },
		"long digest":          func(n *NativeNamespace) { n.NamespaceID = strings.Repeat("a", 65) },
		"uppercase digest":     func(n *NativeNamespace) { n.NamespaceID = strings.Repeat("A", 64) },
		"nonhex digest":        func(n *NativeNamespace) { n.NamespaceID = strings.Repeat("g", 64) },
		"unsafe storage":       func(n *NativeNamespace) { n.StorageID = "host/path" },
		"missing database":     func(n *NativeNamespace) { n.DatabaseID = "" },
		"unsafe database":      func(n *NativeNamespace) { n.DatabaseID = "db/path" },
		"missing project":      func(n *NativeNamespace) { n.ProjectID = "" },
		"unsafe project":       func(n *NativeNamespace) { n.ProjectID = "project/path" },
		"missing hosted space": func(n *NativeNamespace) { n.SpaceID = "" },
		"unsafe hosted space":  func(n *NativeNamespace) { n.SpaceID = "space/path" },
		"local with space":     func(n *NativeNamespace) { n.StorageID = "local" },
	} {
		t.Run(name, func(t *testing.T) {
			n := repo.NativeNamespace()
			mutate(&n)
			if got, err := SessionLifecycleID(n, "session"); got != "" || !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid namespace returned %q %v", got, err)
			}
			inventory := RetainedInventory{Namespace: n, Collections: Collections()}
			if out, err := ProjectRetainedSessionSources(context.Background(), inventory); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
				t.Fatalf("unbound namespace returned descriptors: %+v %v", out, err)
			}
		})
	}
	for _, session := range []string{"", "../session", "a/b", "a\\b", "a b", "_session", "a\n", strings.Repeat("a", 129)} {
		if got, err := SessionLifecycleID(repo.NativeNamespace(), session); got != "" || !errors.Is(err, ErrInvalid) {
			t.Fatalf("unsafe session %q returned %q %v", session, got, err)
		}
	}
	local, err := NewNativePinRepository(testMountedDatabase{id: "local-db", db: repo.db}, nil, "local", "", "project")
	if err != nil {
		t.Fatal(err)
	}
	if id, err := SessionLifecycleID(local.NativeNamespace(), "local-session"); err != nil || id != local.NativeNamespace().NamespaceID+"/local-session" {
		t.Fatalf("bound local namespace rejected: %q %v", id, err)
	}
}

func TestRetainedSessionSourcesUseOwningLifecycleSessionIdentity(t *testing.T) {
	_, service, principal, _, _, _, read := retainedDetectiveFixture(t, false)
	principal.Actor = "bob"
	createTestSession(t, service, principal, "ordinary", "ordinary-bag")
	executeOK(t, service, principal, Command{Action: "pinInput", OpID: "ordinary-pin", SessionID: "ordinary", ExpectedVersion: 1, InputSnapshotID: "ordinary-input", RunID: "ordinary-run", Kind: "query", DefinitionID: "opaque", DefinitionRevision: "revision", DefinitionDocument: ordinaryLocalDefinition, Scope: &Scope{"uat", "tenant-a"}})
	inventory := read()
	sources, err := ProjectRetainedSessionSources(context.Background(), inventory)
	if err != nil || len(sources.Native) != 1 || len(sources.Ordinary) != 1 || len(sources.Contexts) == 0 || len(sources.Provenance) == 0 {
		t.Fatalf("fixture did not project all descriptor families: %+v %v", sources, err)
	}
	check := func(namespace NativeNamespace, session, actual string) {
		t.Helper()
		want, err := SessionLifecycleID(namespace, session)
		if err != nil || actual != want {
			t.Fatalf("descriptor identity %q, want %q: %v", actual, want, err)
		}
	}
	for _, item := range sources.Contexts {
		check(item.Namespace, item.SessionID, item.LifecycleSessionID)
	}
	for _, item := range sources.Native {
		check(item.Namespace, item.SessionID, item.LifecycleSessionID)
		if item.Dispatch.NativeProjectID == item.Namespace.ProjectID {
			t.Fatal("fixture lost native-target separation")
		}
	}
	for _, item := range sources.Ordinary {
		check(item.Namespace, item.SessionID, item.LifecycleSessionID)
	}
	for _, item := range sources.Provenance {
		check(item.Namespace, item.SessionID, item.LifecycleSessionID)
	}
	// Changing ownership consistently in the retained records cannot change the
	// stored session key. Use the context projector to avoid altering receipt hashes.
	for i := range inventory.Documents {
		d := &inventory.Documents[i]
		if d.SessionID != "ordinary" {
			continue
		}
		d.Owner = "charlie"
		if d.Collection == Sessions {
			var session Session
			if err := json.Unmarshal(d.Payload, &session); err != nil {
				t.Fatal(err)
			}
			session.Owner = "charlie"
			d.Payload, err = json.Marshal(session)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	contexts, err := ProjectRetainedContextBindings(context.Background(), inventory)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range contexts {
		if item.SessionID == "ordinary" {
			if item.Owner != "charlie" {
				t.Fatal("owner fixture did not change")
			}
			check(item.Namespace, item.SessionID, item.LifecycleSessionID)
			if item.LifecycleSessionID != sources.Ordinary[0].LifecycleSessionID {
				t.Fatal("owner changed stored session identity")
			}
		}
	}
}
