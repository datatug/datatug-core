package walkthrough

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeepOpaqueNestingIsIterative(t *testing.T) {
	for _, depth := range []int{300, 5000, 20000} {
		declaration := deepOpaqueDeclaration(depth)
		require.LessOrEqual(t, len(declaration), MaxDeclarationBytes, "depth %d", depth)
		result := ParseDeclaration("tour", declaration)
		require.True(t, result.Accepted, "depth %d: %+v", depth, result.Issues)
		require.NotNil(t, result.Definition)
		chapter := result.Definition.Chapters[0]
		require.Equal(t, "one", chapter.EntryStepID)
		require.Equal(t, SupportNeedsNewer, chapter.Steps[0].Support)
		require.Equal(t, "future-deep-target", chapter.Steps[0].Target.Kind)
		require.False(t, chapter.Steps[0].Target.Supported)
	}
}

func TestSchemaMajorRecognition(t *testing.T) {
	for _, schema := range []string{
		"datatug-walkthrough/2",
		"datatug-walkthrough/10",
		"datatug-walkthrough/11",
		"datatug-walkthrough/99",
		"datatug-walkthrough/100",
		"datatug-walkthrough/100000000000000000000",
	} {
		result := ParseDeclaration("tour", []byte(fmt.Sprintf(`{"schema":%q,"id":"tour"}`, schema)))
		require.True(t, result.NewerClient, schema)
		require.False(t, result.Accepted, schema)
		require.Nil(t, result.Definition, schema)
	}
	for _, schema := range []string{
		"datatug-walkthrough/01",
		"datatug-walkthrough/1.0",
		"datatug-walkthrough/",
		"walkthrough/1",
	} {
		result := ParseDeclaration("tour", []byte(fmt.Sprintf(`{"schema":%q,"id":"tour"}`, schema)))
		require.False(t, result.NewerClient, schema)
		require.False(t, result.Accepted, schema)
		require.Nil(t, result.Definition, schema)
		require.Equal(t, CodeInvalidValue, result.Issues[0].Code, schema)
	}
}

func deepOpaqueDeclaration(depth int) []byte {
	var builder strings.Builder
	builder.WriteString(`{"schema":"datatug-walkthrough/1","id":"tour","version":1,"title":"T","chapters":[{"id":"start","title":"S","why":"W","steps":[{"id":"one","title":"O","why":"W","target":{"kind":"future-deep-target","opaque":`)
	for index := 0; index < depth; index++ {
		builder.WriteByte('[')
	}
	builder.WriteByte('0')
	for index := 0; index < depth; index++ {
		builder.WriteByte(']')
	}
	builder.WriteString(`},"done":{"kind":"acknowledged"}}]}]}`)
	return []byte(builder.String())
}
