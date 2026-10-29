package datatug

import (
	"errors"
	"strings"
	"testing"
)

// The "My DataTug Data" queries (backstage spec/features/my-data-project).
const (
	myProjectsTugQL   = "FROM spaces/{SneatUserPersonalSpaceID}/ext/datatug/projects\nORDER BY title"
	myUserRecordTugQL = "FROM users\nWHERE __key__ = @SneatUserID"
)

func TestIsContextVariableName(t *testing.T) {
	for name, want := range map[string]bool{
		"SneatUserID":              true,
		"SneatUserPersonalSpaceID": true,
		"sneatUserID":              false,
		"projectId":                false,
		"":                         false,
	} {
		if got := IsContextVariableName(name); got != want {
			t.Errorf("IsContextVariableName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestQueryDef_ContextVariablesUsed(t *testing.T) {
	for _, tc := range []struct {
		name       string
		queryType  QueryType
		structured bool
		text       string
		want       []string
	}{
		{name: "substituted FROM segment", queryType: QueryTypeTugQL, text: myProjectsTugQL, want: []string{"SneatUserPersonalSpaceID"}},
		{name: "bound value", queryType: QueryTypeTugQL, text: myUserRecordTugQL, want: []string{"SneatUserID"}},
		{name: "both, sorted and once each", queryType: QueryTypeTugQL,
			text: "FROM spaces/{SneatUserPersonalSpaceID}/ext/datatug/projects\nWHERE owner = @SneatUserID AND space = @SneatUserPersonalSpaceID",
			want: []string{"SneatUserID", "SneatUserPersonalSpaceID"}},
		{name: "declared parameter segment is not a context variable", queryType: QueryTypeTugQL,
			text: "FROM spaces/{projectId}/queries\nWHERE a = @other", want: nil},
		{name: "context variable in a JOIN source", queryType: QueryTypeTugQL,
			text: "FROM a\nJOIN users/{SneatUserID}/ext AS e\n  ON a.x = e.y", want: []string{"SneatUserID"}},
		{name: "structured body", queryType: QueryTypeTugQL, structured: true,
			text: "from:\n  schema: spaces/{SneatUserPersonalSpaceID}/ext/datatug\n  name: projects\nwhere:\n  left: {field: __key__}\n  op: \"==\"\n  right: {param: SneatUserID}\n",
			want: []string{"SneatUserID", "SneatUserPersonalSpaceID"}},
		{name: "unparseable text", queryType: QueryTypeTugQL, text: "FROM spaces/{SneatUserID}/x ORDER BY title", want: nil},
		{name: "unparseable structured body", queryType: QueryTypeTugQL, structured: true, text: "from: [", want: nil},
		{name: "not TugQL", queryType: QueryTypeSQL, text: "SELECT * FROM t WHERE id = @SneatUserID", want: nil},
		{name: "a string literal is not a use", queryType: QueryTypeTugQL,
			text: "FROM users\nWHERE note = '{SneatUserID}'", want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := QueryDef{Type: tc.queryType, StructuredBody: tc.structured, Text: tc.text}
			got := q.ContextVariablesUsed()
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("ContextVariablesUsed() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQueryDef_RefuseContextVariables(t *testing.T) {
	t.Run("my projects", func(t *testing.T) {
		err := QueryDef{Type: QueryTypeTugQL, Text: myProjectsTugQL}.RefuseContextVariables()
		if !errors.Is(err, ErrContextVariableNotBound) {
			t.Fatalf("expected ErrContextVariableNotBound, got %v", err)
		}
		const want = "query uses context variable SneatUserPersonalSpaceID, which only the DataTug web app binds for a signed-in user: DataTug Core and the CLI refuse to run it rather than run it with an empty value"
		if err.Error() != want {
			t.Errorf("got %q\nwant %q", err.Error(), want)
		}
	})
	t.Run("two variables", func(t *testing.T) {
		err := QueryDef{Type: QueryTypeTugQL,
			Text: "FROM spaces/{SneatUserPersonalSpaceID}/ext\nWHERE owner = @SneatUserID"}.RefuseContextVariables()
		if err == nil || !strings.HasPrefix(err.Error(), "query uses context variables SneatUserID, SneatUserPersonalSpaceID, which only") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("no context variables", func(t *testing.T) {
		if err := (QueryDef{Type: QueryTypeTugQL, Text: "FROM users\nORDER BY title"}).RefuseContextVariables(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestQueryDef_Validate_ReservedParameterNames(t *testing.T) {
	t.Run("declared in parameters", func(t *testing.T) {
		q := newQueryDef(QueryTypeTugQL, myUserRecordTugQL)
		q.Parameters = Parameters{{ID: "limit", Type: "integer"}, {ID: "SneatUserID", Type: "string"}}
		err := q.Validate()
		if err == nil {
			t.Fatal("expected a validation error")
		}
		const want = "bad value for field [parameters[1].id]: SneatUserID is a reserved context variable name: DataTug supplies its value at run time, so a query must not declare a parameter with that name"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got %q\nwant it to contain %q", err.Error(), want)
		}
	})
	t.Run("declared in TugQL text", func(t *testing.T) {
		q := newQueryDef(QueryTypeTugQL, "parameters (\n  @SneatUserPersonalSpaceID string required\n)\nfrom spaces/{SneatUserPersonalSpaceID}/ext\nselect title")
		err := q.Validate()
		if err == nil {
			t.Fatal("expected a validation error")
		}
		const want = "bad value for field [text]: TugQL parameter @SneatUserPersonalSpaceID: SneatUserPersonalSpaceID is a reserved context variable name"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got %q\nwant it to contain %q", err.Error(), want)
		}
	})
	t.Run("using context variables is valid", func(t *testing.T) {
		for _, text := range []string{myProjectsTugQL, myUserRecordTugQL, "parameters (\n  @projectId string required\n)\nfrom spaces/{SneatUserPersonalSpaceID}/ext/datatug/projects/{projectId}/queries\nselect title"} {
			if err := newQueryDef(QueryTypeTugQL, text).Validate(); err != nil {
				t.Errorf("%q: unexpected error: %v", text, err)
			}
		}
	})
	t.Run("structured and unparseable bodies are not parsed for declarations", func(t *testing.T) {
		q := newQueryDef(QueryTypeTugQL, "from: {name: users}\n")
		q.StructuredBody = true
		if err := q.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := newQueryDef(QueryTypeTugQL, "FROM users ORDER BY title").Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
