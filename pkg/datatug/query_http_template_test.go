package datatug

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
)

type httpVectorError struct {
	Code      HTTPTemplateErrorCode `json:"code"`
	Parameter string                `json:"parameter"`
	Position  HTTPPosition          `json:"position"`
	Header    string                `json:"header"`
}

type httpVectorCase struct {
	Name          string               `json:"name"`
	Mode          string               `json:"mode"`
	Template      HTTPRequestTemplate  `json:"template"`
	Parameters    Parameters           `json:"parameters"`
	RunParameters []Parameter          `json:"runParameters"`
	Values        map[string]any       `json:"values"`
	Expect        *HTTPRequestTemplate `json:"expect"`
	Error         *httpVectorError     `json:"error"`
}

type httpVectors struct {
	Cases  []json.RawMessage `json:"cases"`
	Render []struct {
		Type   string  `json:"type"`
		Value  any     `json:"value"`
		Expect *string `json:"expect"`
	} `json:"render"`
}

func loadHTTPVectors(t *testing.T) httpVectors {
	t.Helper()
	data, err := os.ReadFile("testdata/http_template_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v httpVectors
	if err = json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cases) == 0 || len(v.Render) == 0 {
		t.Fatal("no vectors")
	}
	return v
}

func TestHTTPTemplateVectors(t *testing.T) {
	for _, raw := range loadHTTPVectors(t).Cases {
		var c httpVectorCase
		if err := json.Unmarshal(raw, &c); err != nil {
			t.Fatal(err)
		}
		var shape struct {
			Expect map[string]json.RawMessage `json:"expect"`
		}
		_ = json.Unmarshal(raw, &shape)
		t.Run(c.Name, func(t *testing.T) {
			var got HTTPRequestTemplate
			var err error
			switch c.Mode {
			case "save":
				err = c.Template.ValidateParameters(c.Parameters)
			case "run":
				got, err = c.Template.Substitute(c.Parameters, c.Values)
			case "server":
				got, err = SubstituteHTTPRunParameters(c.Template, c.RunParameters)
			default:
				t.Fatalf("unknown mode %q", c.Mode)
			}
			if c.Error != nil {
				var te *HTTPTemplateError
				if !errors.As(err, &te) {
					t.Fatalf("expected %+v, got %v", *c.Error, err)
				}
				if te.Code != c.Error.Code || (c.Error.Parameter != "" && te.Parameter != c.Error.Parameter) ||
					(c.Error.Position != "" && te.Position != c.Error.Position) || (c.Error.Header != "" && te.Header != c.Error.Header) {
					t.Fatalf("expected %+v, got %+v", *c.Error, *te)
				}
				if te.Error() == "" || te.Detail == "" {
					t.Fatal("empty message")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.Expect == nil {
				t.Fatal("vector has neither expect nor error")
			}
			if _, ok := shape.Expect["url"]; ok && got.URL != c.Expect.URL {
				t.Errorf("url: got %q, want %q", got.URL, c.Expect.URL)
			}
			if _, ok := shape.Expect["method"]; ok && got.Method != c.Expect.Method {
				t.Errorf("method: got %q, want %q", got.Method, c.Expect.Method)
			}
			if _, ok := shape.Expect["body"]; ok && got.Body != c.Expect.Body {
				t.Errorf("body: got %q, want %q", got.Body, c.Expect.Body)
			}
			if _, ok := shape.Expect["headers"]; ok {
				gotJSON, _ := json.Marshal(got.Headers)
				wantJSON, _ := json.Marshal(c.Expect.Headers)
				if !bytes.Equal(gotJSON, wantJSON) {
					t.Errorf("headers: got %s, want %s", gotJSON, wantJSON)
				}
			}
		})
	}
}

func TestRenderHTTPParameterValueVectors(t *testing.T) {
	for _, r := range loadHTTPVectors(t).Render {
		got, ok := RenderHTTPParameterValue(r.Type, r.Value)
		switch {
		case r.Expect == nil && ok:
			t.Errorf("%s %#v: expected refusal, got %q", r.Type, r.Value, got)
		case r.Expect != nil && (!ok || got != *r.Expect):
			t.Errorf("%s %#v: got %q (%v), want %q", r.Type, r.Value, got, ok, *r.Expect)
		}
	}
}

func TestRenderHTTPParameterValueGoTypes(t *testing.T) {
	for _, c := range []struct {
		typ  string
		v    any
		want string
		ok   bool
	}{
		{"integer", int64(42), "42", true},
		{"integer", 42, "42", true},
		{"number", json.Number("1.5"), "1.5", true},
		{"integer", json.Number("7"), "7", true},
		{"number", json.Number("bad"), "", false},
		{"number", float32(1), "", false},
		{"number", math.NaN(), "", false},
		{"number", math.Inf(1), "", false},
	} {
		got, ok := RenderHTTPParameterValue(c.typ, c.v)
		if got != c.want || ok != c.ok {
			t.Errorf("%s %#v: got %q %v", c.typ, c.v, got, ok)
		}
	}
}

func TestHTTPTemplatePlaceholdersAndHelpers(t *testing.T) {
	tmpl := HTTPRequestTemplate{
		Method:  "POST",
		URL:     "https://x.test/a/{p}?q={q}",
		Headers: []HTTPHeaderItem{{Name: "Authorization", Value: "Bearer {tok}"}, {Name: "Content-Type", Value: "application/json"}},
		Body:    `{"b":"{b}"}`,
	}
	got, err := tmpl.Placeholders()
	if err != nil {
		t.Fatal(err)
	}
	want := []HTTPPlaceholder{
		{Name: "p", Position: HTTPPositionPath},
		{Name: "q", Position: HTTPPositionQuery},
		{Name: "tok", Position: HTTPPositionHeader, Header: "Authorization", Credential: true},
		{Name: "b", Position: HTTPPositionJSONString},
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("got %s, want %s", gotJSON, wantJSON)
	}
	if _, err = (HTTPRequestTemplate{URL: "http://{h}/"}).Placeholders(); err == nil {
		t.Fatal("expected a host error")
	}
	if !HTTPTemplateHasPlaceholders(tmpl) {
		t.Fatal("expected placeholders")
	}
	for _, tc := range []HTTPRequestTemplate{
		{URL: "https://x.test/"},
		{URL: "https://x.test/{{a}}", Headers: []HTTPHeaderItem{{Name: "X", Value: "1"}}},
	} {
		if HTTPTemplateHasPlaceholders(tc) {
			t.Fatalf("unexpected placeholders in %+v", tc)
		}
	}
	for _, tc := range []HTTPRequestTemplate{
		{Method: "{m}"},
		{Body: "{b}"},
		{Headers: []HTTPHeaderItem{{Name: "X-{a}"}}},
		{Headers: []HTTPHeaderItem{{Name: "X", Value: "{a}"}}},
	} {
		if !HTTPTemplateHasPlaceholders(tc) {
			t.Fatalf("expected placeholders in %+v", tc)
		}
	}
	e := &HTTPTemplateError{Code: HTTPErrHeaderLineBreak, Parameter: "p", Position: HTTPPositionHeader, Header: "X", Detail: "d"}
	if e.Error() != "http template: parameter p: header X: position header: d" {
		t.Fatal(e.Error())
	}
	if (&HTTPTemplateError{Code: HTTPErrInvalidURL, Detail: "d"}).Error() != "http template: d" {
		t.Fatal("bad bare message")
	}
	// A substitution failure returns the template unchanged.
	if out, err := tmpl.Substitute(Parameters{{ID: "x", Type: "bad"}}, nil); err == nil || out.URL != tmpl.URL {
		t.Fatal("expected a declaration error")
	}
	if out, err := (HTTPRequestTemplate{URL: "http://{h}/"}).Substitute(Parameters{{ID: "h", Type: "string", IsRequired: true}}, nil); err == nil || out.URL != "http://{h}/" {
		t.Fatal("expected a position error")
	}
	if _, err = (HTTPRequestTemplate{URL: "https://x.test/{a}"}).Substitute(nil, nil); err == nil {
		t.Fatal("expected an undeclared error")
	}
	if _, err = SubstituteHTTPRunParameters(HTTPRequestTemplate{URL: "http://{h}/"}, nil); err == nil {
		t.Fatal("expected a server position error")
	}
}

func TestQueryDefValidateHTTPParameters(t *testing.T) {
	base := func() QueryDef {
		return QueryDef{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "q", Title: "Q"}},
			Type:        QueryTypeHTTP,
			Text:        "https://carrier.demodb.dev/v1/shipments?request_id={request_id}\n",
			Parameters: Parameters{{
				ID: "request_id", Type: "string", IsRequired: true,
				Meta: &EntityFieldRef{Entity: "Request", Field: "id"},
			}},
		}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("valid query: %v", err)
	}
	legacy := base()
	legacy.Parameters = nil
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy literal query: %v", err)
	}
	unused := base()
	unused.Parameters = append(unused.Parameters, ParameterDef{ID: "region", Type: "string", IsRequired: true})
	err := unused.Validate()
	var te *HTTPTemplateError
	if !errors.As(err, &te) || te.Code != HTTPErrUnused || !strings.HasPrefix(err.Error(), "parameters: ") {
		t.Fatalf("expected unused, got %v", err)
	}
	withBody := base()
	withBody.HTTP = &QueryHTTPRequest{Method: "POST", Headers: []HTTPHeaderItem{{Name: "Content-Type", Value: "application/json"}}, Body: `{"n":{n}}`}
	withBody.Parameters = append(withBody.Parameters, ParameterDef{ID: "n", Type: "integer", IsRequired: true})
	if err = withBody.Validate(); err != nil {
		t.Fatalf("body query: %v", err)
	}
	if got := withBody.HTTPTemplate(); got.Method != "POST" || got.Body != `{"n":{n}}` || len(got.Headers) != 1 {
		t.Fatalf("template %+v", got)
	}
	credential := base()
	credential.Parameters = append(credential.Parameters, ParameterDef{ID: "token", Type: "string", IsRequired: true, IsCredential: true})
	if err = credential.Validate(); err != nil {
		t.Fatalf("credential query: %v", err)
	}
	data, _ := json.Marshal(credential.Parameters[1])
	if !strings.Contains(string(data), `"isCredential":true`) {
		t.Fatalf("isCredential not serialised: %s", data)
	}
}
