package walkthrough

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	validTargetJSON = `{"kind":"check","checkId":"check-1"}`
	validDoneJSON   = `{"kind":"acknowledged"}`
	validStepFields = `"target":` + validTargetJSON + `,"done":` + validDoneJSON
)

func validStepJSON() string {
	return `{"id":"one","title":"O","why":"W",` + validStepFields + `}`
}

func validChapterJSON() string {
	return `{"id":"start","title":"S","why":"W","steps":[` + validStepJSON() + `]}`
}

func declarationWithChapters(chaptersJSON string) string {
	return `{"schema":"datatug-walkthrough/1","id":"tour","version":1,"title":"T","chapters":[` + chaptersJSON + `]}`
}

func declarationWithChapterBody(body string) string {
	return declarationWithChapters(`{"id":"start","title":"S","why":"W",` + body + `}`)
}

func declarationWithStepBody(body string) string {
	return declarationWithChapterBody(`"steps":[{"id":"one","title":"O","why":"W",` + body + `}]`)
}

func declarationWithTitle(titleJSON string) string {
	return `{"schema":"datatug-walkthrough/1","id":"tour","version":1,"title":"` + titleJSON +
		`","chapters":[{"id":"start","title":"S","why":"W","steps":[{"id":"one","title":"O","why":"W",` +
		validStepFields + `}]}]}`
}

type wantIssue struct {
	code Code
	path string
}

func requireRefusedIssues(t *testing.T, result Result, issues ...wantIssue) {
	t.Helper()
	require.False(t, result.Accepted)
	require.Nil(t, result.Definition)
	require.Error(t, result.Err())
	require.Len(t, result.Issues, len(issues))
	for index, issue := range issues {
		require.Equal(t, issue.code, result.Issues[index].Code, "issue %d code", index)
		require.Equal(t, issue.path, result.Issues[index].Path, "issue %d path", index)
	}
}

func TestDeclarationInputRefusals(t *testing.T) {
	base := declarationWithChapters(validChapterJSON())
	tests := []struct {
		name   string
		data   string
		issues []wantIssue
	}{
		{
			name:   "missing schema",
			data:   `{"id":"tour","version":1,"title":"T","chapters":[` + validChapterJSON() + `]}`,
			issues: []wantIssue{{CodeMissingField, "/schema"}},
		},
		{
			name:   "non-string schema",
			data:   `{"schema":1,"id":"tour","version":1,"title":"T","chapters":[` + validChapterJSON() + `]}`,
			issues: []wantIssue{{CodeInvalidType, "/schema"}},
		},
		{
			name:   "missing version",
			data:   `{"schema":"datatug-walkthrough/1","id":"tour","title":"T","chapters":[` + validChapterJSON() + `]}`,
			issues: []wantIssue{{CodeMissingField, "/version"}},
		},
		{
			name:   "chapters is not an array",
			data:   `{"schema":"datatug-walkthrough/1","id":"tour","version":1,"title":"T","chapters":{}}`,
			issues: []wantIssue{{CodeInvalidType, "/chapters"}},
		},
		{
			name:   "chapters is empty",
			data:   `{"schema":"datatug-walkthrough/1","id":"tour","version":1,"title":"T","chapters":[]}`,
			issues: []wantIssue{{CodeTooMany, "/chapters"}},
		},
		{
			name:   "finish is not an object",
			data:   base[:len(base)-1] + `,"finish":"index"}`,
			issues: []wantIssue{{CodeInvalidType, "/finish"}},
		},
		{
			name:   "finish has no variant",
			data:   base[:len(base)-1] + `,"finish":{}}`,
			issues: []wantIssue{{CodeInvalidValue, "/finish"}},
		},
		{
			name:   "chapter is not an object",
			data:   declarationWithChapters(`1`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0"}},
		},
		{
			name:   "chapter is missing id",
			data:   declarationWithChapters(`{"title":"S","why":"W","steps":[` + validStepJSON() + `]}`),
			issues: []wantIssue{{CodeMissingField, "/chapters/0/id"}},
		},
		{
			name:   "formerIds is not an array",
			data:   declarationWithChapterBody(`"formerIds":{},"steps":[` + validStepJSON() + `]`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/formerIds"}},
		},
		{
			name:   "formerIds entry is not a string",
			data:   declarationWithChapterBody(`"formerIds":[1],"steps":[` + validStepJSON() + `]`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/formerIds/0"}},
		},
		{
			name:   "requires is not a string",
			data:   declarationWithChapterBody(`"requires":1,"steps":[` + validStepJSON() + `]`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/requires"}},
		},
		{
			name:   "steps is not an array",
			data:   declarationWithChapterBody(`"steps":{}`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps"}},
		},
		{
			name:   "steps is empty",
			data:   declarationWithChapterBody(`"steps":[]`),
			issues: []wantIssue{{CodeTooMany, "/chapters/0/steps"}},
		},
		{
			name:   "step is not an object",
			data:   declarationWithChapterBody(`"steps":[1]`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0"}},
		},
		{
			name:   "modes is not an array",
			data:   declarationWithStepBody(`"modes":{},` + validStepFields),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/modes"}},
		},
		{
			name:   "mode entry is not a string",
			data:   declarationWithStepBody(`"modes":[7,"explore"],` + validStepFields),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/modes/0"}},
		},
		{
			name:   "needs is not a string",
			data:   declarationWithStepBody(`"needs":1,` + validStepFields),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/needs"}},
		},
		{
			name:   "target is not an object",
			data:   declarationWithStepBody(`"target":"check","done":` + validDoneJSON),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/target"}},
		},
		{
			name:   "done is not an object",
			data:   declarationWithStepBody(`"target":` + validTargetJSON + `,"done":"ack"`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/done"}},
		},
		{
			name:   "target kind is not a string",
			data:   declarationWithStepBody(`"target":{"kind":1},"done":` + validDoneJSON),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/target/kind"}},
		},
		{
			name:   "done kind is not a string",
			data:   declarationWithStepBody(`"target":` + validTargetJSON + `,"done":{"kind":1}`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/done/kind"}},
		},
		{
			name:   "table-record asset field is missing",
			data:   declarationWithStepBody(`"target":{"kind":"table-record","source":"s"},"done":` + validDoneJSON),
			issues: []wantIssue{{CodeMissingField, "/chapters/0/steps/0/target/table"}},
		},
		{
			name:   "table-record asset field is empty",
			data:   declarationWithStepBody(`"target":{"kind":"table-record","source":"","table":"t"},"done":` + validDoneJSON),
			issues: []wantIssue{{CodeInvalidValue, "/chapters/0/steps/0/target/source"}},
		},
		{
			name:   "panel-open required enum is missing",
			data:   declarationWithStepBody(`"target":` + validTargetJSON + `,"done":{"kind":"panel-open"}`),
			issues: []wantIssue{{CodeMissingField, "/chapters/0/steps/0/done/panel"}},
		},
		{
			name:   "panel-open enum is not a string",
			data:   declarationWithStepBody(`"target":` + validTargetJSON + `,"done":{"kind":"panel-open","panel":1}`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/done/panel"}},
		},
		{
			name:   "related key is missing",
			data:   declarationWithStepBody(`"target":{"kind":"related","source":"s","table":"t"},"done":` + validDoneJSON),
			issues: []wantIssue{{CodeMissingField, "/chapters/0/steps/0/target/key"}},
		},
		{
			name:   "parameter value is a container",
			data:   declarationWithStepBody(`"target":{"kind":"card","cardId":"c","parameters":{"p":[1]}},"done":` + validDoneJSON),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/target/parameters/p"}},
		},
		{
			name:   "parameter value object has neither captured nor default",
			data:   declarationWithStepBody(`"target":{"kind":"card","cardId":"c","parameters":{"p":{}}},"done":` + validDoneJSON),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/target/parameters/p"}},
		},
		{
			name:   "parameter value object has only a default",
			data:   declarationWithStepBody(`"target":{"kind":"card","cardId":"c","parameters":{"p":{"default":1}}},"done":` + validDoneJSON),
			issues: []wantIssue{{CodeMissingField, "/chapters/0/steps/0/target/parameters/p/captured"}},
		},
		{
			name:   "parameter value captured is not a string",
			data:   declarationWithStepBody(`"target":{"kind":"card","cardId":"c","parameters":{"p":{"captured":1,"default":1}}},"done":` + validDoneJSON),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/target/parameters/p/captured"}},
		},
		{
			name:   "continuesFrom is not a string",
			data:   declarationWithChapterBody(`"continuesFrom":1,"steps":[` + validStepJSON() + `]`),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/continuesFrom"}},
		},
		{
			name:   "capture is not a string",
			data:   declarationWithStepBody(`"capture":1,` + validStepFields),
			issues: []wantIssue{{CodeInvalidType, "/chapters/0/steps/0/capture"}},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			requireRefusedIssues(t, ParseDeclaration("tour", []byte(test.data)), test.issues...)
		})
	}
}

func TestVersionExponentSignIsParsedThenRefusedAsNonCanonical(t *testing.T) {
	result := ParseDeclaration("tour", []byte(
		`{"schema":"datatug-walkthrough/1","id":"tour","version":1e+2,"title":"T","chapters":[`+validChapterJSON()+`]}`,
	))
	requireRefusedIssues(t, result, wantIssue{CodeInvalidValue, "/version"})
}

func TestParserRefusalBranches(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		path string
	}{
		{"object key expected", []byte(`{"schema":"datatug-walkthrough/1",1:2}`), ""},
		{"object key string escape", []byte(`{"a\q":1}`), ""},
		{"colon expected after key", []byte(`{"schema" 1}`), "/schema"},
		{"value string escape", []byte(`{"schema":"datatug-walkthrough/1","id":"a\q"}`), "/id"},
		{"true literal", []byte(`{"x":tru}`), "/x"},
		{"false literal", []byte(`{"x":fals}`), "/x"},
		{"null literal", []byte(`{"x":nul}`), "/x"},
		{"number ends after minus", []byte(`-`), ""},
		{"number starts with invalid digit", []byte(`{"x":-x}`), "/x"},
		{"number fraction has no digits", []byte(`{"x":1.}`), "/x"},
		{"number exponent has no digits", []byte(`{"x":1e}`), "/x"},
		{"array separator expected", []byte(`{"x":[1 2]}`), "/x/0"},
		{"object separator expected", []byte(`{"x":1 2}`), "/x"},
		{"unterminated escape", []byte(`{"x":"abc\`), "/x"},
		{"short unicode escape", []byte(`{"x":"\u12`), "/x"},
		{"invalid unicode escape hex", []byte(`{"x":"\uZZZZ"}`), "/x"},
		{"invalid escape", []byte(`{"x":"\q"}`), "/x"},
		{"unescaped control", []byte("{\"x\":\"a\nb\"}"), "/x"},
		{"unterminated string", []byte(`{"x":"abc`), "/x"},
		{"unexpected end after open object", []byte(`{`), ""},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			requireRefusedIssues(t, ParseDeclaration("tour", test.data), wantIssue{CodeInvalidJSON, test.path})
		})
	}
}

func TestPeekHex4RejectsShortInput(t *testing.T) {
	value, ok := peekHex4([]byte("0a"))
	require.False(t, ok)
	require.Zero(t, value)
}

func TestDeclarationAcceptsAllEscapeForms(t *testing.T) {
	result := ParseDeclaration("tour", []byte(declarationWithTitle(`a\"b\\c\/d\be\ff\ng\rh\ti`)))
	require.True(t, result.Accepted, "%+v", result.Issues)
	require.Equal(t, "a\"b\\c/d\be\ff\ng\rh\ti", result.Definition.Title)
}

func TestDeclarationAcceptsLoneLowSurrogateAsReplacement(t *testing.T) {
	result := ParseDeclaration("tour", []byte(declarationWithTitle(`\uDC00`)))
	require.True(t, result.Accepted, "%+v", result.Issues)
	require.Equal(t, "\uFFFD", result.Definition.Title)
}

func TestDeclarationAcceptsUppercaseHexSurrogatePair(t *testing.T) {
	result := ParseDeclaration("tour", []byte(declarationWithTitle(`\uD83D\uDE00`)))
	require.True(t, result.Accepted, "%+v", result.Issues)
	require.Equal(t, "\U0001F600", result.Definition.Title)
}

func TestOptionalBooleanDefaultsFalse(t *testing.T) {
	result := ParseDeclaration("tour", []byte(declarationWithStepBody(
		`"target":{"kind":"board","boardId":"board-1"},"done":`+validDoneJSON,
	)))
	require.True(t, result.Accepted, "%+v", result.Issues)
	board := result.Definition.Chapters[0].Steps[0].Target.Board
	require.NotNil(t, board)
	require.False(t, board.Tutorial)
}
