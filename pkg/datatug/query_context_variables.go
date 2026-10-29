package datatug

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/tugql"
	"github.com/strongo/validation"
	"gopkg.in/yaml.v3"
)

// Context variables are values DataTug supplies from the signed-in user's
// session, never typed by the user (backstage spec/features/my-data-project,
// REQ context-variables). A query uses one as "{name}" in a FROM/JOIN path
// or as "@name" in a value position without declaring it.
const (
	// ContextVariableSneatUserID is the Firebase Auth uid of the signed-in user.
	ContextVariableSneatUserID = "SneatUserID"
	// ContextVariableSneatUserPersonalSpaceID is the key of the signed-in
	// user's personal Sneat space.
	ContextVariableSneatUserPersonalSpaceID = "SneatUserPersonalSpaceID"
)

// ErrContextVariableNotBound is matched by the error
// QueryDef.RefuseContextVariables returns: only the DataTug web app binds context
// variables; DataTug Core and the CLI refuse to run a query that uses one.
var ErrContextVariableNotBound = errors.New("context variable is not bound")

// IsContextVariableName reports whether name is a reserved context variable
// name, which no query may declare as a parameter.
func IsContextVariableName(name string) bool {
	return name == ContextVariableSneatUserID || name == ContextVariableSneatUserPersonalSpaceID
}

func reservedParameterNameMessage(name string) string {
	return name + " is a reserved context variable name: DataTug supplies its value at run time, so a query must not declare a parameter with that name"
}

// validateReservedParameterNames fails when the query declares a parameter
// named after a context variable, either in its parameter list or in the
// PARAMETERS block of its TugQL text. Text that does not parse declares
// nothing here; running it reports the syntax error.
func (v QueryDef) validateReservedParameterNames() error {
	for i, p := range v.Parameters {
		if IsContextVariableName(p.ID) {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("parameters[%d].id", i), reservedParameterNameMessage(p.ID))
		}
	}
	if v.Type != QueryTypeTugQL || v.StructuredBody {
		return nil
	}
	document, diagnostics := tugql.ParseTugQL(v.Text)
	if len(diagnostics) != 0 {
		return nil
	}
	for _, p := range document.Tree.Parameters {
		if IsContextVariableName(p.Name) {
			return validation.NewErrBadRecordFieldValue("text", "TugQL parameter @"+p.Name+": "+reservedParameterNameMessage(p.Name))
		}
	}
	return nil
}

// substitutedSegment matches a whole "{name}" path segment.
var substitutedSegment = regexp.MustCompile(`\{([^{}/]+)\}`)

// ContextVariablesUsed returns, sorted and once each, the context variables a
// TugQL query uses as a "{name}" path segment or a "@name" bound value. It
// returns nil for other query types and for a body that does not parse.
func (v QueryDef) ContextVariablesUsed() []string {
	if v.Type != QueryTypeTugQL {
		return nil
	}
	var tree any
	if v.StructuredBody {
		if err := yaml.Unmarshal([]byte(v.Text), &tree); err != nil {
			return nil
		}
	} else {
		document, diagnostics := tugql.ParseTugQL(v.Text)
		if len(diagnostics) != 0 {
			return nil
		}
		// ParseTugQL validates the tree against the JSON-compatible
		// profile, so this round trip cannot fail.
		data, _ := json.Marshal(document.Tree)
		_ = json.Unmarshal(data, &tree)
	}
	found := map[string]bool{}
	collectContextVariables(tree, found)
	if len(found) == 0 {
		return nil
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// collectContextVariables walks a TugQL tree: a {param: name} node is a bound
// value, and a "{name}" segment of a source's schema or name is a
// substitution. String literals are never inspected.
func collectContextVariables(node any, found map[string]bool) {
	switch n := node.(type) {
	case map[string]any:
		for key, value := range n {
			if s, ok := value.(string); ok {
				switch key {
				case "param":
					if IsContextVariableName(s) {
						found[s] = true
					}
				case "schema", "name":
					for _, m := range substitutedSegment.FindAllStringSubmatch(s, -1) {
						if IsContextVariableName(m[1]) {
							found[m[1]] = true
						}
					}
				}
				continue
			}
			collectContextVariables(value, found)
		}
	case []any:
		for _, item := range n {
			collectContextVariables(item, found)
		}
	}
}

// ContextVariableNotBoundError names the context variables a query uses. It
// matches ErrContextVariableNotBound with errors.Is.
type ContextVariableNotBoundError struct {
	Names []string
}

func (e ContextVariableNotBoundError) Error() string {
	noun := "context variable"
	if len(e.Names) > 1 {
		noun += "s"
	}
	return fmt.Sprintf("query uses %s %s, which only the DataTug web app binds for a signed-in user: DataTug Core and the CLI refuse to run it rather than run it with an empty value",
		noun, strings.Join(e.Names, ", "))
}

// Is makes errors.Is(err, ErrContextVariableNotBound) hold.
func (e ContextVariableNotBoundError) Is(target error) bool {
	return target == ErrContextVariableNotBound
}

// RefuseContextVariables returns a ContextVariableNotBoundError naming every
// context variable the query uses. The Go runtime does not bind context
// variables, so DataTug Core and the CLI call it before opening any
// connection rather than run the query with an empty value.
func (v QueryDef) RefuseContextVariables() error {
	names := v.ContextVariablesUsed()
	if len(names) == 0 {
		return nil
	}
	return ContextVariableNotBoundError{Names: names}
}
