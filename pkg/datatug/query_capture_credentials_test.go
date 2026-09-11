package datatug

import (
	"strings"
	"testing"
)

// capturedQueryDefWithScreenedParam is capturedQueryDef() plus a
// string-typed parameter, the one a credential case below gives a secret
// default: a non-string default on the integer parameter would be refused
// by ParameterDef.Validate's type check before any credential screening
// runs, which would prove nothing.
func capturedQueryDefWithScreenedParam() QueryDef {
	q := capturedQueryDef()
	q.Parameters = append(q.Parameters, ParameterDef{ID: "Note", Type: "string", DefaultValue: "unpaid"})
	return q
}

// TestQueryDef_Validate_KeepsBothScreens proves QueryDef.Validate enforces
// both protections that meet in it, on one and the same query: the
// credential screening every save path relies on (a query's title, its body
// text, every target and every parameter default, via
// EmbeddedCredentialReason - query_credentials.go) and the
// capture-provenance rules a query captured from exploration must satisfy
// (QueryCapture.validateCaptureAgainst - query_capture.go).
//
// Every case runs against a valid captured query, so the test fails if
// either half is ever dropped: the credentials table fails if only the
// capture rules survive, and the provenance table fails if only the
// credential screening does.
func TestQueryDef_Validate_KeepsBothScreens(t *testing.T) {
	t.Run("a valid captured query passes both screens", func(t *testing.T) {
		if err := capturedQueryDefWithScreenedParam().Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("credentials", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			mutate  func(q *QueryDef)
			wantErr string
		}{
			{
				name:    "in a target",
				mutate:  func(q *QueryDef) { q.Targets = []QueryDefTarget{{Host: "postgres://user:secret@db.example.com/app"}} },
				wantErr: "host",
			},
			{
				name: "in a target's password",
				mutate: func(q *QueryDef) {
					target := QueryDefTarget{Host: "db.example.com"}
					target.Password = "s3cret"
					q.Targets = []QueryDefTarget{target}
				},
				wantErr: "password",
			},
			{
				name:    "in a parameter default",
				mutate:  func(q *QueryDef) { q.Parameters[1].DefaultValue = "postgres://user:secret@db.example.com/app" },
				wantErr: "parameters[1].defaultValue",
			},
			{
				name:    "in the title",
				mutate:  func(q *QueryDef) { q.Title = "rows from postgres://user:secret@db.example.com/app" },
				wantErr: "title",
			},
			{
				name:    "in the body",
				mutate:  func(q *QueryDef) { q.Text = "# Authorization: Bearer abcd1234\nfrom: Invoice\n" },
				wantErr: "text",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				q := capturedQueryDefWithScreenedParam()
				tt.mutate(&q)
				err := q.Validate()
				if err == nil {
					t.Fatalf("expected a refusal naming %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("refusal %q does not name %q", err, tt.wantErr)
				}
			})
		}
	})

	t.Run("capture provenance", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			mutate  func(q *QueryDef)
			wantErr string
		}{
			{
				name:    "capture without an environment",
				mutate:  func(q *QueryDef) { q.Capture.Environment = "" },
				wantErr: "environment",
			},
			{
				name:    "capture without a source",
				mutate:  func(q *QueryDef) { q.Capture.Source = "" },
				wantErr: "source",
			},
			{
				name:    "a binding with an unknown origin",
				mutate:  func(q *QueryDef) { q.Capture.Bindings[0].Origin = "default" },
				wantErr: "bindings[0]",
			},
			{
				name:    "a binding naming an undeclared parameter",
				mutate:  func(q *QueryDef) { q.Capture.Bindings[0].ParameterID = "InvoiceId" },
				wantErr: "InvoiceId",
			},
			{
				name: "a duplicate binding",
				mutate: func(q *QueryDef) {
					q.Capture.Bindings = append(q.Capture.Bindings, q.Capture.Bindings[0])
				},
				wantErr: "bindings[1]",
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				q := capturedQueryDefWithScreenedParam()
				tt.mutate(&q)
				err := q.Validate()
				if err == nil {
					t.Fatalf("expected a refusal naming %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), "capture") {
					t.Errorf("refusal %q is not reported as a capture failure", err)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("refusal %q does not name %q", err, tt.wantErr)
				}
			})
		}
	})
}
