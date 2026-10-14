package apicontract

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func sessionInputFixture() SessionInputReference {
	return SessionInputReference{SchemaVersion: 1, StorageID: "firestore", SpaceID: "space-1", DatabaseID: "private-db", ProjectID: "project-1", NamespaceID: "namespace-1", SessionID: "session-1", InputSnapshotID: "input-1", ContextSetID: "context-1", EnvironmentID: "qa", IdentityDomain: "identity-1", DefinitionDigest: strings.Repeat("a", 64), Bindings: []SessionInputBindingRef{
		{InputName: "InvoiceId", Origin: "bag", Entries: []SessionInputEntryRef{{BagID: "bag-1", BagVersion: 2, EntryID: "entry-2", RevisionID: "revision-2"}, {BagID: "bag-1", BagVersion: 2, EntryID: "entry-1", RevisionID: "revision-1"}}},
		{InputName: "CustomerId", Origin: "case", CaseID: "case-1", CaseVersion: 3},
		{InputName: "Currency", Origin: "manual"},
	}}
}

func TestSessionInputReferenceCanonicalOwnsAndOrdersMetadata(t *testing.T) {
	original := sessionInputFixture()
	canonical, err := original.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if canonical.Bindings[0].InputName != "Currency" || canonical.Bindings[2].Entries[0].EntryID != "entry-1" {
		t.Fatalf("not canonical: %+v", canonical)
	}
	original.Bindings[0].Entries[0].RevisionID = "changed"
	original.Bindings[1].CaseID = "changed"
	if canonical.Bindings[2].Entries[1].RevisionID != "revision-2" || canonical.Bindings[1].CaseID != "case-1" {
		t.Fatal("canonical reference aliases source")
	}
	canonical.Bindings[2].Entries[0].BagID = "changed"
	if original.Bindings[0].Entries[1].BagID != "bag-1" {
		t.Fatal("source aliases canonical reference")
	}
	second := sessionInputFixture()
	second.Bindings[0], second.Bindings[2] = second.Bindings[2], second.Bindings[0]
	second.Bindings[2].Entries[0], second.Bindings[2].Entries[1] = second.Bindings[2].Entries[1], second.Bindings[2].Entries[0]
	firstCanonical, _ := sessionInputFixture().Canonical()
	secondCanonical, _ := second.Canonical()
	if !reflect.DeepEqual(firstCanonical, secondCanonical) {
		t.Fatal("equivalent order differs")
	}
	local := sessionInputFixture()
	local.StorageID, local.SpaceID, local.Bindings = "local", "", nil
	if _, err = local.Canonical(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionInputReferenceRejectsMalformedNamespaceAndProvenance(t *testing.T) {
	for _, field := range []string{"StorageID", "SpaceID", "DatabaseID", "ProjectID", "NamespaceID", "SessionID", "InputSnapshotID", "ContextSetID", "EnvironmentID", "IdentityDomain"} {
		for _, value := range []string{"", "../secret", "_bad", "with space", "é", strings.Repeat("a", 129)} {
			t.Run(field+"/"+value, func(t *testing.T) {
				r := sessionInputFixture()
				reflect.ValueOf(&r).Elem().FieldByName(field).SetString(value)
				if _, err := r.Canonical(); err == nil {
					t.Fatal("malformed namespace accepted")
				}
			})
		}
	}
	for _, tc := range []struct {
		name   string
		mutate func(*SessionInputReference)
	}{
		{"schema", func(r *SessionInputReference) { r.SchemaVersion = 2 }},
		{"digest missing", func(r *SessionInputReference) { r.DefinitionDigest = "" }},
		{"digest length", func(r *SessionInputReference) { r.DefinitionDigest = "aa" }},
		{"digest hex", func(r *SessionInputReference) { r.DefinitionDigest = strings.Repeat("z", 64) }},
		{"digest case", func(r *SessionInputReference) { r.DefinitionDigest = strings.Repeat("A", 64) }},
		{"local Space", func(r *SessionInputReference) { r.StorageID = "local" }},
		{"bindings bound", func(r *SessionInputReference) { r.Bindings = make([]SessionInputBindingRef, MaxSessionInputBindings+1) }},
		{"parameter start", func(r *SessionInputReference) { r.Bindings[0].InputName = "1id" }},
		{"parameter dot", func(r *SessionInputReference) { r.Bindings[0].InputName = "Invoice.Id" }},
		{"duplicate input", func(r *SessionInputReference) { r.Bindings[1].InputName = r.Bindings[0].InputName }},
		{"unknown origin", func(r *SessionInputReference) { r.Bindings[0].Origin = "secret value" }},
		{"bag empty", func(r *SessionInputReference) { r.Bindings[0].Entries = nil }},
		{"bag Case", func(r *SessionInputReference) { r.Bindings[0].CaseID = "case-1" }},
		{"bag Case version", func(r *SessionInputReference) { r.Bindings[0].CaseVersion = 1 }},
		{"Case entries", func(r *SessionInputReference) { r.Bindings[1].Entries = r.Bindings[0].Entries }},
		{"Case missing", func(r *SessionInputReference) { r.Bindings[1].CaseID = "" }},
		{"Case version", func(r *SessionInputReference) { r.Bindings[1].CaseVersion = 0 }},
		{"manual entries", func(r *SessionInputReference) { r.Bindings[2].Entries = r.Bindings[0].Entries }},
		{"manual Case", func(r *SessionInputReference) { r.Bindings[2].CaseID = "case-1" }},
		{"manual Case version", func(r *SessionInputReference) { r.Bindings[2].CaseVersion = -1 }},
		{"bag ID", func(r *SessionInputReference) { r.Bindings[0].Entries[0].BagID = "../bad" }},
		{"entry ID", func(r *SessionInputReference) { r.Bindings[0].Entries[0].EntryID = "" }},
		{"revision ID", func(r *SessionInputReference) { r.Bindings[0].Entries[0].RevisionID = "" }},
		{"bag version", func(r *SessionInputReference) { r.Bindings[0].Entries[0].BagVersion = 0 }},
		{"duplicate entry", func(r *SessionInputReference) { r.Bindings[0].Entries[1] = r.Bindings[0].Entries[0] }},
		{"conflicting entry across inputs", func(r *SessionInputReference) {
			r.Bindings[2] = r.Bindings[0]
			r.Bindings[2].InputName = "Other"
			r.Bindings[2].Entries = append([]SessionInputEntryRef(nil), r.Bindings[2].Entries...)
			r.Bindings[2].Entries[0].RevisionID = "other"
		}},
		{"total entries bound", func(r *SessionInputReference) {
			r.Bindings[0].Entries = make([]SessionInputEntryRef, MaxSessionInputEntryRefs+1)
			for i := range r.Bindings[0].Entries {
				r.Bindings[0].Entries[i] = SessionInputEntryRef{BagID: "bag", BagVersion: 1, EntryID: "entry-" + strconv.Itoa(i), RevisionID: "revision"}
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := sessionInputFixture()
			tc.mutate(&r)
			if _, err := r.Canonical(); err == nil {
				t.Fatal("invalid reference accepted")
			}
		})
	}
}

func TestSessionInputReferenceStrictMetadataBoundary(t *testing.T) {
	r := sessionInputFixture()
	r.DatabaseID, r.SessionID = "db_1.0", "Session-1"
	r.Bindings[0].InputName = "invoice_id"
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.Bindings[2] = r.Bindings[0]
	r.Bindings[2].InputName = "another-input"
	r.Bindings[2].Entries = append([]SessionInputEntryRef(nil), r.Bindings[2].Entries...)
	r.Bindings[2].Entries[0].BagVersion++
	r.Bindings[0].Entries[1].BagVersion++
	if err := r.Validate(); err != nil {
		t.Fatal("historical attachment bag versions may differ while exact entry revisions agree:", err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`"value"`, `"text"`, `"title"`, `"note"`, `"parameters"`, `"credentials"`} {
		if strings.Contains(string(data), raw) {
			t.Fatalf("reference contains raw-content field %s", raw)
		}
		forged := append(append([]byte(nil), data[:len(data)-1]...), []byte(","+raw+`:"PRIVATE_SENTINEL"}`)...)
		var decoded SessionInputReference
		if DecodeStrict(forged, &decoded) == nil {
			t.Fatalf("raw content accepted: %s", raw)
		}
	}
	var decoded SessionInputReference
	if err = DecodeStrict(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r, decoded) {
		t.Fatal("reference wire roundtrip changed metadata")
	}
	r.NamespaceID = "PRIVATE_SENTINEL /"
	if err := r.Validate(); err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
		t.Fatal("malformed private ID leaked through validation error")
	}
}

func TestSessionInputReferenceRequiresOneVersionPerCase(t *testing.T) {
	r := sessionInputFixture()
	r.Bindings[2] = SessionInputBindingRef{InputName: "Currency", Origin: "case", CaseID: "case-1", CaseVersion: 3}
	if _, err := r.Canonical(); err != nil {
		t.Fatal("multiple facts from the same accepted Case version must be valid:", err)
	}
	r.Bindings[2].CaseVersion = 4
	if _, err := r.Canonical(); err == nil {
		t.Fatal("contradictory versions of one authoritative Case accepted")
	}
}
