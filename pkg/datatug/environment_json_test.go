package datatug

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A new typed field must never remain classified as opaque metadata: otherwise
// clearing its omitempty value could reintroduce the old JSON value on save.
func TestEnvironmentJSONFieldsMatchSchema(t *testing.T) {
	count := 0
	for _, field := range reflect.VisibleFields(reflect.TypeFor[Environment]()) {
		if !field.IsExported() || field.Anonymous {
			continue
		}
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		count++
		require.Equal(t, name, environmentJSONField(name), "update the environment JSON field list for a new typed field")
	}
	require.Len(t, environmentJSONFields, count)
}
