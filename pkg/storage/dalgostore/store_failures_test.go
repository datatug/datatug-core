package dalgostore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
)

// errBackend is what the failing test double below returns.
var errBackend = errors.New("backend is down")

// failingDB decorates a real dal.DB and fails the one operation each test
// needs to fail. Decorating by embedding dal.DB is the documented way to
// wrap a database (see dal.DB's own doc comment): the sealing marker method
// is promoted along with everything not overridden, so no library change is
// needed to build a double that reports backend failures.
type failingDB struct {
	dal.DB
	failWrite bool
	failQuery bool
}

func (db failingDB) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, options ...dal.TransactionOption) error {
	if db.failWrite {
		return errBackend
	}
	return db.DB.RunReadwriteTransaction(ctx, f, options...)
}

func (db failingDB) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	if db.failQuery {
		return nil, errBackend
	}
	return db.DB.ExecuteQueryToRecordsReader(ctx, query)
}

func TestStore_DeleteProject_ReportsABackendFailure(t *testing.T) {
	store := dalgostore.NewStore(failingDB{DB: newTestDB(), failWrite: true}, testStoreID, t.TempDir())
	err := store.DeleteProject(context.Background(), "p1")
	require.Error(t, err)
	assert.ErrorIs(t, err, errBackend)
	assert.Contains(t, err.Error(), "ext/datatug/projects/p1")
}

func TestStore_GetProjects_ReportsABackendFailure(t *testing.T) {
	store := dalgostore.NewStore(failingDB{DB: newTestDB(), failQuery: true}, testStoreID, t.TempDir())
	projects, err := store.GetProjects(context.Background())
	require.Error(t, err)
	assert.Nil(t, projects)
	assert.ErrorIs(t, err, errBackend)
}

// recordsReader yields a fixed slice of records, so a test can hand
// GetProjects a result set the real backend can not produce and pin the
// guards that reject it.
type recordsReader struct {
	records []record.Record
	i       int
}

func (r *recordsReader) Cursor() (string, error) { return "", nil }

func (r *recordsReader) Close() error { return nil }

func (r *recordsReader) Next() (record.Record, error) {
	if r.i >= len(r.records) {
		return nil, dal.ErrNoMoreRecords
	}
	rec := r.records[r.i]
	r.i++
	return rec, nil
}

// readerDB decorates a real dal.DB and answers every query with a fixed
// result set.
type readerDB struct {
	dal.DB
	records []record.Record
}

func (db readerDB) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return &recordsReader{records: db.records}, nil
}

func TestStore_GetProjects_RefusesARecordOfTheWrongType(t *testing.T) {
	key := record.NewKeyWithParentAndID(record.NewKeyWithID("ext", "datatug"), "projects", "p1")
	type notAProjectFile struct{}
	store := dalgostore.NewStore(
		readerDB{DB: newTestDB(), records: []record.Record{record.NewRecordWithData(key, &notAProjectFile{})}},
		testStoreID, t.TempDir())

	projects, err := store.GetProjects(context.Background())
	require.Error(t, err)
	assert.Nil(t, projects)
	assert.Contains(t, err.Error(), "expected *datatug.ProjectFile")
}

func TestStore_GetProjects_RefusesANonStringProjectID(t *testing.T) {
	key := record.NewKeyWithParentAndID(record.NewKeyWithID("ext", "datatug"), "projects", 42)
	store := dalgostore.NewStore(
		readerDB{DB: newTestDB(), records: []record.Record{record.NewRecordWithData(key, &datatug.ProjectFile{})}},
		testStoreID, t.TempDir())

	projects, err := store.GetProjects(context.Background())
	require.Error(t, err)
	assert.Nil(t, projects)
	assert.Contains(t, err.Error(), "non-string ID")
}
