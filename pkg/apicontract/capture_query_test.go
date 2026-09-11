package apicontract

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func validCapturedQuery() CapturedQuery {
	return CapturedQuery{
		FolderPath: "customers",
		ID:         "customer-invoices",
		Title:      "Customer invoices",
		Purpose:    "Which invoices does this customer have?",
		Source:     "chinook",
		DTQL:       "from: Invoice\n",
		Parameters: []CapturedParameter{{
			ID: "CustomerId", Type: "integer", Title: "Customer", IsRequired: true,
			Meta: &EntityFieldRef{Entity: "Customer", Field: "ID"},
		}},
		BindingOrigins: []CaptureBindingOrigin{{ParameterID: "CustomerId", Origin: BindingOriginSelection}},
	}
}

func validCaptureCreate() CaptureQueryRequest {
	return CaptureQueryRequest{
		Project: "demo-project-1", Environment: "local", SecurityContextID: "sc1",
		IfNoneMatch: true,
		Query:       validCapturedQuery(),
	}
}

func validCaptureResponse() CaptureQueryResponse {
	return CaptureQueryResponse{
		QueryID:    "customers/customer-invoices",
		Revision:   "rev-1",
		Query:      validCapturedQuery(),
		Provenance: CaptureProvenance{Author: "admin", Environment: "local", Collection: "Invoice"},
	}
}

func TestCaptureQueryRequest_JSONFieldNames(t *testing.T) {
	data, err := json.Marshal(validCaptureCreate())
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"project", "environment", "securityContextId", "ifNoneMatch", "query"} {
		if _, ok := top[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
	if _, ok := top["ifMatch"]; ok {
		t.Errorf("expected ifMatch to be omitted for a create, got %s", data)
	}
	var query map[string]json.RawMessage
	if err := json.Unmarshal(top["query"], &query); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"folderPath", "id", "title", "purpose", "source", "dtql", "parameters", "bindingOrigins"} {
		if _, ok := query[key]; !ok {
			t.Errorf("missing query key %q in %s", key, top["query"])
		}
	}
	var params []map[string]json.RawMessage
	if err := json.Unmarshal(query["parameters"], &params); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"id", "type", "title", "isRequired", "meta"} {
		if _, ok := params[0][key]; !ok {
			t.Errorf("missing parameter key %q in %s", key, query["parameters"])
		}
	}

	update := validCaptureCreate()
	update.IfNoneMatch, update.IfMatch = false, "rev-1"
	data, err = json.Marshal(update)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"ifNoneMatch"`) || !strings.Contains(string(data), `"ifMatch":"rev-1"`) {
		t.Errorf("an update carries ifMatch only, got %s", data)
	}
}

func TestCaptureQueryRequest_Validate(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(r *CaptureQueryRequest)
		wantField string // "" means valid
	}{
		{name: "create", mutate: func(*CaptureQueryRequest) {}},
		{name: "update", mutate: func(r *CaptureQueryRequest) { r.IfNoneMatch, r.IfMatch = false, "rev-1" }},
		{name: "root folder", mutate: func(r *CaptureQueryRequest) { r.Query.FolderPath = "" }},
		{name: "nested folder", mutate: func(r *CaptureQueryRequest) { r.Query.FolderPath = "sales/emea" }},
		{name: "no parameters", mutate: func(r *CaptureQueryRequest) {
			r.Query.Parameters, r.Query.BindingOrigins = []CapturedParameter{}, []CaptureBindingOrigin{}
		}},
		{name: "parameter without meta", mutate: func(r *CaptureQueryRequest) { r.Query.Parameters[0].Meta = nil }},
		{name: "context and manual origins", mutate: func(r *CaptureQueryRequest) {
			r.Query.Parameters = append(r.Query.Parameters, CapturedParameter{ID: "Since", Type: "date"})
			r.Query.BindingOrigins = []CaptureBindingOrigin{
				{ParameterID: "CustomerId", Origin: BindingOriginContext},
				{ParameterID: "Since", Origin: BindingOriginManual},
			}
		}},
		{name: "a parameter need not have a binding origin", mutate: func(r *CaptureQueryRequest) { r.Query.BindingOrigins = nil }},

		{name: "missing project", mutate: func(r *CaptureQueryRequest) { r.Project = "" }, wantField: "project"},
		{name: "missing environment", mutate: func(r *CaptureQueryRequest) { r.Environment = " " }, wantField: "environment"},
		{name: "missing securityContextId", mutate: func(r *CaptureQueryRequest) { r.SecurityContextID = "" }, wantField: "securityContextId"},
		{name: "neither condition", mutate: func(r *CaptureQueryRequest) { r.IfNoneMatch = false }, wantField: "ifNoneMatch/ifMatch"},
		{name: "both conditions", mutate: func(r *CaptureQueryRequest) { r.IfMatch = "rev-1" }, wantField: "ifNoneMatch/ifMatch"},

		{name: "missing id", mutate: func(r *CaptureQueryRequest) { r.Query.ID = "" }, wantField: "query.id"},
		{name: "id with a separator", mutate: func(r *CaptureQueryRequest) { r.Query.ID = "a/b" }, wantField: "query.id"},
		{name: "id with a backslash", mutate: func(r *CaptureQueryRequest) { r.Query.ID = `a\b` }, wantField: "query.id"},
		{name: "dot-dot id", mutate: func(r *CaptureQueryRequest) { r.Query.ID = ".." }, wantField: "query.id"},
		{name: "dot id", mutate: func(r *CaptureQueryRequest) { r.Query.ID = "." }, wantField: "query.id"},
		{name: "id with NUL", mutate: func(r *CaptureQueryRequest) { r.Query.ID = "a\x00b" }, wantField: "query.id"},
		{name: "absolute folder", mutate: func(r *CaptureQueryRequest) { r.Query.FolderPath = "/etc" }, wantField: "query.folderPath"},
		{name: "traversing folder", mutate: func(r *CaptureQueryRequest) { r.Query.FolderPath = "customers/../../x" }, wantField: "query.folderPath"},
		{name: "empty folder segment", mutate: func(r *CaptureQueryRequest) { r.Query.FolderPath = "a//b" }, wantField: "query.folderPath"},
		{name: "trailing folder separator", mutate: func(r *CaptureQueryRequest) { r.Query.FolderPath = "a/" }, wantField: "query.folderPath"},
		{name: "backslash folder", mutate: func(r *CaptureQueryRequest) { r.Query.FolderPath = `a\..\b` }, wantField: "query.folderPath"},
		{name: "missing title", mutate: func(r *CaptureQueryRequest) { r.Query.Title = "" }, wantField: "query.title"},
		{name: "missing purpose", mutate: func(r *CaptureQueryRequest) { r.Query.Purpose = "  " }, wantField: "query.purpose"},
		{name: "missing source", mutate: func(r *CaptureQueryRequest) { r.Query.Source = "" }, wantField: "query.source"},
		{name: "missing dtql", mutate: func(r *CaptureQueryRequest) { r.Query.DTQL = "" }, wantField: "query.dtql"},

		{name: "parameter without id", mutate: func(r *CaptureQueryRequest) { r.Query.Parameters[0].ID = "" }, wantField: "query.parameters"},
		{name: "parameter without type", mutate: func(r *CaptureQueryRequest) { r.Query.Parameters[0].Type = "" }, wantField: "query.parameters"},
		{name: "null parameter type", mutate: func(r *CaptureQueryRequest) { r.Query.Parameters[0].Type = "null" }, wantField: "query.parameters"},
		{name: "unknown parameter type", mutate: func(r *CaptureQueryRequest) { r.Query.Parameters[0].Type = "text" }, wantField: "query.parameters"},
		{name: "duplicate parameter", mutate: func(r *CaptureQueryRequest) {
			r.Query.Parameters = append(r.Query.Parameters, r.Query.Parameters[0])
		}, wantField: "query.parameters"},
		{name: "meta without entity", mutate: func(r *CaptureQueryRequest) { r.Query.Parameters[0].Meta.Entity = "" }, wantField: "query.parameters"},
		{name: "meta without field", mutate: func(r *CaptureQueryRequest) { r.Query.Parameters[0].Meta.Field = " " }, wantField: "query.parameters"},

		{name: "binding origin for an undeclared parameter", mutate: func(r *CaptureQueryRequest) {
			r.Query.BindingOrigins[0].ParameterID = "InvoiceId"
		}, wantField: "query.bindingOrigins"},
		{name: "binding origin without parameter", mutate: func(r *CaptureQueryRequest) {
			r.Query.BindingOrigins[0].ParameterID = ""
		}, wantField: "query.bindingOrigins"},
		{name: "default origin is not captured", mutate: func(r *CaptureQueryRequest) {
			r.Query.BindingOrigins[0].Origin = BindingOriginDefault
		}, wantField: "query.bindingOrigins"},
		{name: "unknown origin", mutate: func(r *CaptureQueryRequest) { r.Query.BindingOrigins[0].Origin = "guess" }, wantField: "query.bindingOrigins"},
		{name: "duplicate binding origin", mutate: func(r *CaptureQueryRequest) {
			r.Query.BindingOrigins = append(r.Query.BindingOrigins, r.Query.BindingOrigins[0])
		}, wantField: "query.bindingOrigins"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validCaptureCreate()
			tt.mutate(&r)
			err := r.Validate()
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected a *ValidationError naming %q, got %v", tt.wantField, err)
			}
			if ve.Field != tt.wantField {
				t.Errorf("Field = %q, want %q (%v)", ve.Field, tt.wantField, err)
			}
		})
	}
}

func TestCanonicalQueryID(t *testing.T) {
	tests := []struct{ folder, id, want string }{
		{"", "q", "q"},
		{"customers", "customer-invoices", "customers/customer-invoices"},
		{"sales/emea", "q", "sales/emea/q"},
	}
	for _, tt := range tests {
		if got := CanonicalQueryID(tt.folder, tt.id); got != tt.want {
			t.Errorf("CanonicalQueryID(%q, %q) = %q, want %q", tt.folder, tt.id, got, tt.want)
		}
	}
}

func TestCaptureQueryResponse_Validate(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(r *CaptureQueryResponse)
		wantField string
	}{
		{name: "valid", mutate: func(*CaptureQueryResponse) {}},
		{name: "author is optional", mutate: func(r *CaptureQueryResponse) { r.Provenance.Author = "" }},
		{name: "root query", mutate: func(r *CaptureQueryResponse) { r.Query.FolderPath, r.QueryID = "", "customer-invoices" }},
		{name: "missing queryId", mutate: func(r *CaptureQueryResponse) { r.QueryID = "" }, wantField: "queryId"},
		{name: "queryId disagrees with the query", mutate: func(r *CaptureQueryResponse) { r.QueryID = "customer-invoices" }, wantField: "queryId"},
		{name: "missing revision", mutate: func(r *CaptureQueryResponse) { r.Revision = "" }, wantField: "revision"},
		{name: "invalid query", mutate: func(r *CaptureQueryResponse) { r.Query.Title = "" }, wantField: "query.title"},
		{name: "missing provenance environment", mutate: func(r *CaptureQueryResponse) { r.Provenance.Environment = "" }, wantField: "provenance.environment"},
		{name: "missing provenance collection", mutate: func(r *CaptureQueryResponse) { r.Provenance.Collection = "" }, wantField: "provenance.collection"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := validCaptureResponse()
			tt.mutate(&r)
			err := r.Validate()
			if tt.wantField == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected a *ValidationError naming %q, got %v", tt.wantField, err)
			}
			if ve.Field != tt.wantField {
				t.Errorf("Field = %q, want %q (%v)", ve.Field, tt.wantField, err)
			}
		})
	}
}

func TestCaptureQuery_DecodeStrictRejectsResultRowsAndIdentity(t *testing.T) {
	base, err := json.Marshal(validCaptureCreate())
	if err != nil {
		t.Fatal(err)
	}
	var ok CaptureQueryRequest
	if err := DecodeStrict(base, &ok); err != nil {
		t.Fatalf("a valid request must decode strictly: %v", err)
	}
	for name, body := range map[string]string{
		"result rows at the top level": strings.Replace(string(base), `"query":`, `"rows":[[1]],"query":`, 1),
		"recordset inside the query":   strings.Replace(string(base), `"dtql":`, `"recordset":{"rows":[]},"dtql":`, 1),
		"a captured default value":     strings.Replace(string(base), `"isRequired":`, `"defaultValue":5,"isRequired":`, 1),
		"a session-local fact id":      strings.Replace(string(base), `"origin":"selection"`, `"origin":"selection","factId":"f1"`, 1),
		"a client-claimed author":      strings.Replace(string(base), `"query":`, `"author":"root","query":`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			var r CaptureQueryRequest
			if err := DecodeStrict([]byte(body), &r, "principal", "role"); err == nil {
				t.Fatalf("expected strict decode to reject %s", body)
			}
		})
	}
}

func TestErrCodeRevisionConflict(t *testing.T) {
	if !ErrCodeRevisionConflict.Valid() {
		t.Fatal("REVISION_CONFLICT must be a valid error code")
	}
	if got := ErrCodeRevisionConflict.HTTPStatus(); got != 409 {
		t.Errorf("HTTPStatus() = %d, want 409", got)
	}
	env := ErrorEnvelope{Error: ErrorBody{Code: string(ErrCodeRevisionConflict), Message: "stale revision", Field: "ifMatch", RequestID: "req-1"}}
	if err := env.Validate(); err != nil {
		t.Errorf("a REVISION_CONFLICT envelope must validate: %v", err)
	}
}
