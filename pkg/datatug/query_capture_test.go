package datatug

import (
	"encoding/json"
	"strings"
	"testing"
)

func validQueryCapture() *QueryCapture {
	return &QueryCapture{
		Author:      "admin",
		Environment: "local",
		Source:      "chinook",
		Collection:  "Invoice",
		Bindings:    []QueryCaptureBinding{{ParameterID: "CustomerId", Origin: QueryCaptureOriginSelection}},
	}
}

func capturedQueryDef() QueryDef {
	return QueryDef{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "customer-invoices", Title: "Customer invoices"}},
		Type:        QueryTypeDTQL,
		Text:        "from: Invoice\n",
		Purpose:     "Which invoices does this customer have?",
		Parameters: Parameters{{
			ID: "CustomerId", Type: "integer", IsRequired: true,
			Meta: &EntityFieldRef{Entity: "Customer", Field: "ID"},
		}},
		Capture: validQueryCapture(),
	}
}

func TestQueryCapture_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(c *QueryCapture)
		wantErr string
	}{
		{name: "valid", mutate: func(*QueryCapture) {}},
		{name: "author is optional", mutate: func(c *QueryCapture) { c.Author = "" }},
		{name: "collection is optional", mutate: func(c *QueryCapture) { c.Collection = "" }},
		{name: "no bindings", mutate: func(c *QueryCapture) { c.Bindings = nil }},
		{name: "context origin", mutate: func(c *QueryCapture) { c.Bindings[0].Origin = QueryCaptureOriginContext }},
		{name: "manual origin", mutate: func(c *QueryCapture) { c.Bindings[0].Origin = QueryCaptureOriginManual }},
		{name: "missing environment", mutate: func(c *QueryCapture) { c.Environment = " " }, wantErr: "environment"},
		{name: "missing source", mutate: func(c *QueryCapture) { c.Source = "" }, wantErr: "source"},
		{name: "binding without parameter", mutate: func(c *QueryCapture) { c.Bindings[0].ParameterID = "" }, wantErr: "bindings[0]"},
		{name: "default origin is not captured", mutate: func(c *QueryCapture) { c.Bindings[0].Origin = "default" }, wantErr: "bindings[0]"},
		{name: "unknown origin", mutate: func(c *QueryCapture) { c.Bindings[0].Origin = "guess" }, wantErr: "bindings[0]"},
		{name: "duplicate binding", mutate: func(c *QueryCapture) {
			c.Bindings = append(c.Bindings, c.Bindings[0])
		}, wantErr: "bindings[1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validQueryCapture()
			tt.mutate(c)
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error naming %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not name %q", err, tt.wantErr)
			}
		})
	}
}

func TestQueryDef_Validate_Capture(t *testing.T) {
	t.Run("valid captured query", func(t *testing.T) {
		if err := capturedQueryDef().Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})
	t.Run("invalid capture is rejected", func(t *testing.T) {
		q := capturedQueryDef()
		q.Capture.Source = ""
		if err := q.Validate(); err == nil || !strings.Contains(err.Error(), "capture") {
			t.Fatalf("expected a capture error, got %v", err)
		}
	})
	t.Run("binding must name a declared parameter", func(t *testing.T) {
		q := capturedQueryDef()
		q.Capture.Bindings[0].ParameterID = "InvoiceId"
		if err := q.Validate(); err == nil || !strings.Contains(err.Error(), "InvoiceId") {
			t.Fatalf("expected an undeclared-parameter error, got %v", err)
		}
	})
	t.Run("a query with no capture is unaffected", func(t *testing.T) {
		q := capturedQueryDef()
		q.Capture = nil
		q.Purpose = ""
		if err := q.Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})
}

func TestQueryDef_PurposeAndCapture_JSON(t *testing.T) {
	data, err := json.Marshal(capturedQueryDef())
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"purpose", "capture"} {
		if _, ok := generic[key]; !ok {
			t.Errorf("missing key %q in %s", key, data)
		}
	}
	var capture map[string]json.RawMessage
	if err := json.Unmarshal(generic["capture"], &capture); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"author", "environment", "source", "collection", "bindings"} {
		if _, ok := capture[key]; !ok {
			t.Errorf("missing capture key %q in %s", key, generic["capture"])
		}
	}

	plain := capturedQueryDef()
	plain.Purpose, plain.Capture = "", nil
	data, err = json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"purpose"`, `"capture"`} {
		if strings.Contains(string(data), key) {
			t.Errorf("expected %s to be omitted from an uncaptured query, got %s", key, data)
		}
	}
}
