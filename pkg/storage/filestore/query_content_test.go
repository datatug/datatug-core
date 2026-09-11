package filestore

import (
	"encoding/json"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
)

func validQueryForWrite() datatug.QueryDef {
	return datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
		Type:        datatug.QueryTypeDTQL,
		Text:        "from:\n  name: Invoice\n",
	}
}

// Task 2: validate credentials and content before staging - none of these
// cases may reach the file system.

func TestValidateQueryForWrite_AcceptsValidQuery(t *testing.T) {
	if err := validateQueryForWrite(validQueryForWrite()); err != nil {
		t.Fatalf("expected a valid query to pass, got: %v", err)
	}
}

func TestValidateQueryForWrite_RejectsPassword(t *testing.T) {
	q := validQueryForWrite()
	q.Targets = []datatug.QueryDefTarget{{Driver: "postgres", Credentials: datatug.Credentials{Username: "alice", Password: "secret"}}}
	err := validateQueryForWrite(q)
	if err == nil {
		t.Fatal("expected a target password to be rejected")
	}
	if !validation.IsValidationError(err) {
		t.Fatalf("expected a validation error, got %T: %v", err, err)
	}
}

func TestValidateQueryForWrite_RejectsEmbeddedURLCredentials(t *testing.T) {
	q := validQueryForWrite()
	q.Targets = []datatug.QueryDefTarget{{Driver: "postgres", Host: "user:pass@db.example.com"}}
	err := validateQueryForWrite(q)
	if err == nil {
		t.Fatal("expected embedded URL credentials to be rejected")
	}
	if !validation.IsValidationError(err) {
		t.Fatalf("expected a validation error, got %T: %v", err, err)
	}
}

func TestValidateQueryForWrite_AcceptsUsernameAlone(t *testing.T) {
	q := validQueryForWrite()
	q.Targets = []datatug.QueryDefTarget{{Driver: "postgres", Credentials: datatug.Credentials{Username: "alice"}}}
	if err := validateQueryForWrite(q); err != nil {
		t.Fatalf("expected a username alone to be accepted, got: %v", err)
	}
}

func TestValidateQueryForWrite_RejectsUnknownType(t *testing.T) {
	q := validQueryForWrite()
	// A type QueryDef.Validate itself does not recognize (its own switch's
	// default case), independent of any other field - "folder" would also
	// be rejected here, but only because of an unrelated rule ("text
	// should be empty for folders"), which would leave this test passing
	// for the wrong reason once validateQueryForWrite no longer applies
	// its own separate IsKnownQueryType gate on top (see
	// TestValidateQueryForWrite_AcceptsGraphQL/S6).
	q.Type = "COBOL"
	err := validateQueryForWrite(q)
	if err == nil {
		t.Fatal("expected an unknown/unsupported query type to be rejected")
	}
}

// TestValidateQueryForWrite_AcceptsGraphQL is a regression test for S6:
// validateQueryForWrite used to additionally require datatug.IsKnownQueryType,
// which does not include "GraphQL" even though QueryDef.Validate (pkg/datatug/
// query.go, unmodified by this branch) explicitly treats it as a valid
// case - and the legacy SaveQuery/CreateQuery/UpdateQuery paths, which call
// only query.Validate(), have always accepted it. PutQuery must accept
// exactly what the legacy path accepts, so the "additive, non-breaking"
// claim holds precisely: a query type legacy already writes must not
// become newly rejected by the revisioned API.
func TestValidateQueryForWrite_AcceptsGraphQL(t *testing.T) {
	q := validQueryForWrite()
	q.Type = "GraphQL"
	if err := validateQueryForWrite(q); err != nil {
		t.Fatalf("expected GraphQL to be accepted, matching the legacy write path, got: %v", err)
	}
}

func TestValidateQueryForWrite_RejectsInvalidRecord(t *testing.T) {
	q := validQueryForWrite()
	q.ID = "" // required field
	if err := validateQueryForWrite(q); err == nil {
		t.Fatal("expected a missing id to be rejected")
	}
}

func TestQueryJSONBytes_ExcludesText(t *testing.T) {
	q := validQueryForWrite()
	b, err := queryJSONBytes(q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unexpected error unmarshalling: %v", err)
	}
	if _, present := raw["text"]; present {
		t.Fatalf("expected persisted JSON to exclude \"text\", got: %s", b)
	}
	if raw["id"] != "q1" {
		t.Fatalf("expected persisted JSON to keep other metadata, got: %s", b)
	}
}

func TestQueryJSONBytes_DoesNotMutateCaller(t *testing.T) {
	q := validQueryForWrite()
	if _, err := queryJSONBytes(q); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Text == "" {
		t.Fatalf("expected queryJSONBytes to leave the caller's copy of Text untouched")
	}
}

func TestQueryBodyFileName(t *testing.T) {
	cases := []struct {
		id       string
		typ      datatug.QueryType
		expected string
	}{
		{"q1", datatug.QueryTypeDTQL, "q1.query.dtql"},
		{"q1", datatug.QueryTypeSQL, "q1.query.sql"},
		{"customer-invoices", datatug.QueryTypeHTTP, "customer-invoices.query.http"},
	}
	for _, c := range cases {
		if got := queryBodyFileName(c.id, c.typ); got != c.expected {
			t.Errorf("queryBodyFileName(%q, %q) = %q, want %q", c.id, c.typ, got, c.expected)
		}
	}
}
