package dalgostore

import (
	"context"
	"testing"
	"time"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datatug/datatug-core/pkg/dto"
)

// TestCreateProject_RefusesAnInvalidProjectRecord pins the guard that
// validates the assembled record before it is written: with the clock
// pinned to the zero time, datatug.ProjectFile.Validate() rejects the
// record for a missing created.at, and nothing must be written.
func TestCreateProject_RefusesAnInvalidProjectRecord(t *testing.T) {
	restore := timeNow
	timeNow = func() time.Time { return time.Time{} }
	defer func() { timeNow = restore }()

	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	store := NewStore(db, "s1", t.TempDir())

	summary, err := store.CreateProject(context.Background(), dto.CreateProjectRequest{StoreID: "s1", Title: "T"})
	require.Error(t, err)
	assert.Nil(t, summary)
	assert.Contains(t, err.Error(), "invalid project record")

	exists, err := db.Exists(context.Background(), projectKey("t"))
	require.NoError(t, err)
	assert.False(t, exists, "nothing must be written when the record is refused")
}
