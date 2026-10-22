package datatug

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func httpQuery(h *QueryHTTPRequest) QueryDef {
	q := QueryDef{Type: QueryTypeHTTP, Text: "https://api.example.com/x", HTTP: h}
	q.ID, q.Title = "q1", "Q1"
	return q
}

func TestQueryDef_HTTPOmittedWhenNil(t *testing.T) {
	q := QueryDef{Type: QueryTypeSQL, Text: "select 1"}
	q.ID, q.Title = "q1", "Q1"
	b, err := json.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"http"`) {
		t.Fatalf("http key must be omitted: %s", b)
	}
	y, err := yaml.Marshal(q)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(y), "http") {
		t.Fatalf("http key must be omitted: %s", y)
	}
}

func TestQueryDef_HTTPRoundTrip(t *testing.T) {
	q := httpQuery(&QueryHTTPRequest{Method: "POST", Headers: []HTTPHeaderItem{{Name: "Accept", Value: "application/json"}}, Body: `{"q":1}`})
	b, _ := json.Marshal(q)
	var back QueryDef
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.HTTP == nil || back.HTTP.Method != "POST" || back.HTTP.Body != `{"q":1}` || back.HTTP.Headers[0].Name != "Accept" {
		t.Fatalf("round trip lost data: %+v", back.HTTP)
	}
	y, _ := yaml.Marshal(q)
	var backY QueryDef
	if err := yaml.Unmarshal(y, &backY); err != nil || backY.HTTP == nil || backY.HTTP.Method != "POST" {
		t.Fatalf("yaml round trip: %v %+v", err, backY.HTTP)
	}
}

func TestQueryDef_HTTPValidate(t *testing.T) {
	hdr := func(n, v string) []HTTPHeaderItem { return []HTTPHeaderItem{{Name: n, Value: v}} }
	many := make([]HTTPHeaderItem, MaxQueryHTTPHeaders+1)
	for i := range many {
		many[i] = HTTPHeaderItem{Name: "X-A", Value: "1"}
	}
	cases := []struct {
		name string
		q    QueryDef
		ok   bool
	}{
		{"nil", httpQuery(nil), true},
		{"empty block", httpQuery(&QueryHTTPRequest{}), true},
		{"full", httpQuery(&QueryHTTPRequest{Method: "PUT", Headers: hdr("X-Trace", "1"), Body: "b"}), true},
		{"head", httpQuery(&QueryHTTPRequest{Method: "HEAD"}), true},
		{"lower method", httpQuery(&QueryHTTPRequest{Method: "get"}), false},
		{"bad method", httpQuery(&QueryHTTPRequest{Method: "TRACE"}), false},
		{"empty header name", httpQuery(&QueryHTTPRequest{Headers: hdr("", "v")}), false},
		{"space in name", httpQuery(&QueryHTTPRequest{Headers: hdr("a b", "v")}), false},
		{"crlf value", httpQuery(&QueryHTTPRequest{Headers: hdr("X-A", "a\r\nb")}), false},
		{"authorization", httpQuery(&QueryHTTPRequest{Headers: hdr("authorization", "v")}), false},
		{"too many", httpQuery(&QueryHTTPRequest{Headers: many}), false},
		{"body on GET", httpQuery(&QueryHTTPRequest{Body: "b"}), false},
		{"body too big", httpQuery(&QueryHTTPRequest{Method: "POST", Body: strings.Repeat("a", MaxQueryHTTPBodyBytes+1)}), false},
	}
	for _, c := range cases {
		err := c.q.Validate()
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v want ok=%v", c.name, err, c.ok)
		}
	}
	sql := QueryDef{Type: QueryTypeSQL, HTTP: &QueryHTTPRequest{}}
	sql.ID, sql.Title = "q1", "Q1"
	if sql.Validate() == nil {
		t.Error("http on SQL must be rejected")
	}
	folder := QueryDef{Type: "folder", HTTP: &QueryHTTPRequest{}}
	folder.ID, folder.Title = "q1", "Q1"
	if folder.Validate() == nil {
		t.Error("http on folder must be rejected")
	}
}

func TestIsCredentialHTTPHeader(t *testing.T) {
	for _, n := range []string{"Authorization", "PROXY-AUTHORIZATION", "Cookie", "Set-Cookie", "X-API-Key", "x-auth-token",
		"X-Access-Token", "X-Amz-Security-Token", "X-My-Secret", "X-Password", "X-ApiKey", "Csrf-Token", " authorization "} {
		if !IsCredentialHTTPHeader(n) {
			t.Errorf("%q should be credential", n)
		}
	}
	for _, n := range []string{"Accept", "Content-Type", "X-Trace-Id", ""} {
		if IsCredentialHTTPHeader(n) {
			t.Errorf("%q should not be credential", n)
		}
	}
}

func TestIsHTTPToken(t *testing.T) {
	if !IsHTTPToken("X-Custom_1.a") || IsHTTPToken("") || IsHTTPToken("a:b") {
		t.Fatal("token check wrong")
	}
	if !HTTPMethodAllowsBody("POST") || HTTPMethodAllowsBody("GET") {
		t.Fatal("body method check wrong")
	}
}

func TestQueryDef_HTTPRejectsEmbeddedSecrets(t *testing.T) {
	secret := storageCredentialValue
	q := httpQuery(&QueryHTTPRequest{Headers: []HTTPHeaderItem{{Name: "X-Dsn", Value: secret}}})
	if q.Validate() == nil {
		t.Error("secret in header value must be rejected")
	}
	q = httpQuery(&QueryHTTPRequest{Method: "POST", Body: secret})
	if q.Validate() == nil {
		t.Error("secret in body must be rejected")
	}
}

func TestQueryHTTPRequest_Validate_HeaderAndBodyScreening(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		header  string
		body    string
		wantErr string
	}{
		{name: "header value user:pass@host is free text", header: "a:b@c"},
		{name: "json email in body", method: "POST", body: `{"email":"bob@x.com"}`},
		{name: "graphql email argument", method: "POST", body: `mutation { login(email:"a@b.c") }`},
		{name: "json token member refused", method: "POST", body: `{"token":"abc"}`, wantErr: "secret JSON member"},
		{name: "key=value refresh_token refused", method: "POST", body: `refresh_token=abc`, wantErr: "key/value pair"},
		{name: "header url password refused", header: "https://u:p@host/x", wantErr: "password in a URL"},
		{name: "header key=value secret refused", header: "api_key=abc", wantErr: "key/value pair"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &QueryHTTPRequest{Method: tc.method, Body: tc.body}
			if tc.header != "" {
				req.Headers = []HTTPHeaderItem{{Name: "X-Note", Value: tc.header}}
			}
			err := req.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestEmbeddedCredentialReason_StillScreensDSN(t *testing.T) {
	if _, found := EmbeddedCredentialReason("user:pass@host"); !found {
		t.Fatal("text screening must still refuse a DSN password")
	}
	if _, found := embeddedCredentialReasonWithoutDSN("user:pass@host"); found {
		t.Fatal("DSN check must be skipped")
	}
}
