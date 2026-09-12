package datatug

import (
	"strings"
	"testing"

	"github.com/strongo/validation"
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

// captureCredentialProbes are the credential syntaxes no provenance field
// and no purpose may carry into a git-tracked query file: a connection
// string, a URL with a password, the key/value forms, and a JSON member.
var captureCredentialProbes = []struct{ name, value string }{
	{"a connection string", "Server=h;User Id=u;Password=s3cr3t;"},
	{"a URL with a password", "postgres://u:s3cr3t@db.example.com/app"},
	{"password=", "password=s3cr3t"},
	{"AccountKey=", "AccountKey=s3cr3t"},
	{"token=", "token=s3cr3t"},
	{"a JSON password member", `{"password": "s3cr3t"}`},
}

// captureFieldSetter names one provenance field and writes to it.
type captureFieldSetter struct {
	field string
	set   func(c *QueryCapture, value string)
}

func captureFieldSetters() []captureFieldSetter {
	return []captureFieldSetter{
		{"author", func(c *QueryCapture, v string) { c.Author = v }},
		{"environment", func(c *QueryCapture, v string) { c.Environment = v }},
		{"source", func(c *QueryCapture, v string) { c.Source = v }},
		{"collection", func(c *QueryCapture, v string) { c.Collection = v }},
		{"bindings[0]", func(c *QueryCapture, v string) { c.Bindings[0].ParameterID = v }},
	}
}

// TestQueryCapture_Validate_RefusesCredentialsInEveryProvenanceField: no
// string a capture records may carry a credential into the git-tracked
// "<id>.query.json", whichever field it is written to.
func TestQueryCapture_Validate_RefusesCredentialsInEveryProvenanceField(t *testing.T) {
	for _, f := range captureFieldSetters() {
		for _, probe := range captureCredentialProbes {
			t.Run(f.field+"/"+probe.name, func(t *testing.T) {
				c := validQueryCapture()
				f.set(c, probe.value)
				err := c.Validate()
				if err == nil {
					t.Fatalf("expected %q in %s to be refused", probe.value, f.field)
				}
				if !validation.IsBadRecordError(err) || !strings.Contains(err.Error(), f.field) {
					t.Errorf("expected a bad-record error naming %q, got: %v", f.field, err)
				}
			})
		}
	}
}

// TestQueryCapture_Validate_AcceptsRealisticProvenance is the other half
// of the same decision: the values a real capture records must survive,
// including the ones a credential screen would misread (an e-mail author,
// a collection named after passwords).
func TestQueryCapture_Validate_AcceptsRealisticProvenance(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(c *QueryCapture)
	}{
		{"an e-mail author", func(c *QueryCapture) { c.Author = "alice@example.com" }},
		{"a plus-addressed e-mail author", func(c *QueryCapture) { c.Author = "alice+datatug@example.com" }},
		{"a non-ASCII author", func(c *QueryCapture) { c.Author = "Ольга" }},
		{"an opaque principal id", func(c *QueryCapture) { c.Author = "uid_7f3a91b2" }},
		{"no author", func(c *QueryCapture) { c.Author = "" }},
		{"environment prod", func(c *QueryCapture) { c.Environment = "prod" }},
		{"environment staging", func(c *QueryCapture) { c.Environment = "staging" }},
		{"a hyphenated environment", func(c *QueryCapture) { c.Environment = "dev-eu-1" }},
		{"a source id holding @", func(c *QueryCapture) { c.Source = "chinook@v2" }},
		{"a dotted source id", func(c *QueryCapture) { c.Source = "crm.readonly" }},
		{"a collection named passwords", func(c *QueryCapture) { c.Collection = "passwords" }},
		{"a collection named password_resets", func(c *QueryCapture) { c.Collection = "password_resets" }},
		{"a nested collection path", func(c *QueryCapture) { c.Collection = "Customer/5/Invoice" }},
		{"no collection", func(c *QueryCapture) { c.Collection = "" }},
		{"a parameter id naming a password", func(c *QueryCapture) { c.Bindings[0].ParameterID = "PasswordResetId" }},
		{"a token-named parameter id", func(c *QueryCapture) { c.Bindings[0].ParameterID = "api_token_id" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := validQueryCapture()
			tt.mutate(c)
			if err := c.Validate(); err != nil {
				t.Fatalf("expected valid provenance, got: %v", err)
			}
		})
	}
}

// TestQueryCapture_Validate_RefusesUnusableProvenanceShapes covers the
// rest of the shape rule: a second line, a control character, invalid
// UTF-8, padding and an unbounded value.
func TestQueryCapture_Validate_RefusesUnusableProvenanceShapes(t *testing.T) {
	for _, tt := range []struct{ name, value, want string }{
		{"a smuggled header line", "prod\nAuthorization: Bearer abc123", captureShapeControlReason},
		{"a control character", "prod\x07", captureShapeControlReason},
		{"invalid UTF-8", "prod\xff", captureShapeEncodingReason},
		{"padding", " prod ", captureShapeWhitespaceReason},
		{"an oversized value", strings.Repeat("p", maxCaptureFieldLength+1), "exceeds max length"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := validQueryCapture()
			c.Environment = tt.value
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected %q to be refused", tt.value)
			}
			if !validation.IsBadRecordError(err) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected a bad-record error saying %q, got: %v", tt.want, err)
			}
		})
	}
}

// TestCaptureShape_IsNotWeakerThanTheCredentialScreen proves the trade the
// shape rule makes (query_capture.go), in both directions: it refuses
// everything the credential screen refuses among the probes, its accepted
// values are ones the credential screen would also pass, and every
// credential syntax owes its detection to punctuation the shape rule
// forbids - strip ':', '=' and '"' and EmbeddedCredentialReason no longer
// fires on any of them, which is why a value free of those three
// characters cannot express a credential at all.
func TestCaptureShape_IsNotWeakerThanTheCredentialScreen(t *testing.T) {
	t.Run("both rules refuse every probe", func(t *testing.T) {
		for _, probe := range captureCredentialProbes {
			if _, found := EmbeddedCredentialReason(probe.value); !found {
				t.Errorf("%s: the credential screen no longer refuses %q", probe.name, probe.value)
			}
			if _, found := captureShapeReason(probe.value); !found {
				t.Errorf("%s: the shape rule does not refuse %q", probe.name, probe.value)
			}
		}
	})

	t.Run("every credential syntax needs the forbidden punctuation", func(t *testing.T) {
		strip := func(r rune) rune {
			if r == ':' || r == '=' || r == '"' {
				return -1
			}
			return r
		}
		values := []string{
			"user:secret@tcp(db.example.com:3306)/app",
			"jdbc:postgresql://user:secret@host/db",
			"Authorization: Bearer abcd1234",
			"X-API-Key: abcd1234",
			"https://h/api?api_key=abc123",
			"Endpoint=sb://ns.servicebus.windows.net/;SharedAccessKey=abc",
		}
		for _, probe := range captureCredentialProbes {
			values = append(values, probe.value)
		}
		for _, value := range values {
			if _, found := EmbeddedCredentialReason(value); !found {
				t.Errorf("%q is expected to read as a credential before stripping", value)
			}
			stripped := strings.Map(strip, value)
			if reason, found := EmbeddedCredentialReason(stripped); found {
				t.Errorf("%q still reads as a credential without ':', '=' and '\"': %s", stripped, reason)
			}
		}
	})

	t.Run("accepted provenance is credential-free either way", func(t *testing.T) {
		for _, value := range []string{
			"alice@example.com", "alice+datatug@example.com", "uid_7f3a91b2", "prod", "staging",
			"dev-eu-1", "chinook@v2", "crm.readonly", "passwords", "password_resets",
			"Customer/5/Invoice", "CustomerId", "PasswordResetId", "api_token_id",
		} {
			if reason, found := captureShapeReason(value); found {
				t.Errorf("the shape rule refuses realistic provenance %q: %s", value, reason)
			}
			if reason, found := EmbeddedCredentialReason(value); found {
				t.Errorf("%q is accepted by shape but reads as a credential: %s", value, reason)
			}
		}
	})
}

// TestQueryDef_Validate_PurposeIsScreenedLikeATitle: Purpose is free text,
// so it gets the credential screen, and prose that merely talks about
// passwords keeps saving.
func TestQueryDef_Validate_PurposeIsScreenedLikeATitle(t *testing.T) {
	t.Run("refused", func(t *testing.T) {
		for _, probe := range captureCredentialProbes {
			t.Run(probe.name, func(t *testing.T) {
				q := capturedQueryDef()
				q.Purpose = "invoices for the customer, via " + probe.value
				err := q.Validate()
				if err == nil {
					t.Fatalf("expected %q in the purpose to be refused", probe.value)
				}
				if !validation.IsBadRecordError(err) || !strings.Contains(err.Error(), "purpose") {
					t.Errorf("expected a bad-record error naming %q, got: %v", "purpose", err)
				}
			})
		}
	})

	t.Run("accepted", func(t *testing.T) {
		for _, purpose := range []string{
			"Which customers reset their password last week?",
			"Find accounts whose password_hash is null so support can invite them to set one.",
			"Escalation path: ask the on-call DBA before running this against prod.",
			"Who paid late?\nUsed by the Monday revenue review.",
			"",
		} {
			t.Run(purpose, func(t *testing.T) {
				q := capturedQueryDef()
				q.Purpose = purpose
				if err := q.Validate(); err != nil {
					t.Fatalf("expected the purpose to be accepted, got: %v", err)
				}
			})
		}
	})
}
