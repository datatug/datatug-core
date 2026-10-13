package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type readTestSession struct {
	dal.ReadSession
	getErr     error
	getMissing bool
	getData    map[string]any
	queryErr   error
	reader     dal.RecordsReader
}

func (s readTestSession) Get(ctx context.Context, rec record.Record) error {
	if s.getErr != nil {
		return s.getErr
	}
	if s.getMissing {
		rec.SetError(record.ErrRecordNotFound)
		return nil
	}
	if s.getData != nil {
		rec.SetError(record.ErrNoError)
		if target, ok := rec.Data().(*map[string]any); ok {
			*target = s.getData
			return nil
		}
		return errors.New("unexpected record data destination")
	}
	return s.ReadSession.Get(ctx, rec)
}

func (s readTestSession) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	if s.queryErr != nil {
		return nil, s.queryErr
	}
	if s.reader != nil {
		return s.reader, nil
	}
	return s.ReadSession.ExecuteQueryToRecordsReader(ctx, query)
}

type sliceRecordsReader struct {
	records  []record.Record
	index    int
	nextErr  error
	closeErr error
}

func (r *sliceRecordsReader) Cursor() (string, error) { return "", nil }
func (r *sliceRecordsReader) Close() error            { return r.closeErr }
func (r *sliceRecordsReader) Next() (record.Record, error) {
	if r.nextErr != nil {
		err := r.nextErr
		r.nextErr = nil
		return nil, err
	}
	if r.index >= len(r.records) {
		return nil, dal.ErrNoMoreRecords
	}
	rec := r.records[r.index]
	r.index++
	return rec, nil
}

type failingSetDB struct {
	dal.DB
	setErr error
}

func (d failingSetDB) RunReadwriteTransaction(ctx context.Context, worker dal.RWTxWorker, options ...dal.TransactionOption) error {
	return d.DB.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return worker(ctx, failingSetTransaction{ReadwriteTransaction: tx, setErr: d.setErr})
	}, options...)
}

type failingSetTransaction struct {
	dal.ReadwriteTransaction
	setErr error
}

func (tx failingSetTransaction) Set(context.Context, record.Record) error { return tx.setErr }

func TestDALRepositoryConstructorAndTransactionFailures(t *testing.T) {
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	for name, mounted := range map[string]MountedDatabase{
		"nil mount":           nil,
		"invalid database id": testMountedDatabase{id: "bad id", db: db},
		"nil database":        testMountedDatabase{id: "db1"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewDALRepository(mounted, nil); !errors.Is(err, ErrInvalid) {
				t.Fatalf("NewDALRepository error = %v", err)
			}
		})
	}
	base, err := NewDALRepository(testMountedDatabase{id: "db1", db: db}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = base.Transact(context.Background(), func(Transaction) error { return ErrConflict }); !errors.Is(err, ErrConflict) {
		t.Fatalf("transaction callback failure = %v", err)
	}
	failing := *base
	failing.db = failingSetDB{DB: db, setErr: errors.New("write failed")}
	err = failing.Transact(context.Background(), func(tx Transaction) error {
		return tx.Save(context.Background(), Sessions, "s1", Session{ID: "s1"}, 0, "alice", "s1")
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("DAL write failure = %v, want unavailable", err)
	}
}

func TestDALLoadAndSaveValidationAndReadFailures(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name string
		col  string
		id   string
	}{
		{"unknown collection", "unknown", "id"},
		{"unsafe id", Sessions, "bad id"},
	} {
		t.Run(test.name, func(t *testing.T) {
			unit := &dalTransaction{loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}
			if _, err := unit.Load(ctx, test.col, test.id, &Session{}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Load error = %v", err)
			}
		})
	}
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	err := db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		unit := &dalTransaction{reader: readTestSession{ReadSession: tx, getErr: errors.New("database unavailable")}, loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}
		if _, err := unit.Load(ctx, Sessions, "s1", &Session{}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("unclassified Get failure = %v", err)
		}
		absent := &dalTransaction{reader: readTestSession{ReadSession: tx, getMissing: true}, loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}
		if _, err := absent.Load(ctx, Sessions, "s2", &Session{}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("nonexistent DAL record = %v", err)
		}
		badJSON := &dalTransaction{pending: map[string]storedRecord{
			"sessionContextSessions/s3": {Version: 1, Payload: json.RawMessage(`{"ID":"s3"}`)},
			"sessionContextSessions/s4": {},
		}}
		if _, err := badJSON.Load(ctx, Sessions, "s3", make(chan int)); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("invalid destination = %v", err)
		}
		if _, err := badJSON.Load(ctx, Sessions, "s4", &Session{}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("missing pending record = %v", err)
		}
		invalidStored := map[string]any{"recordId": "s5", "owner": "alice", "sessionId": "s5", "version": 1, "payload": "not-json"}
		if _, err := (&dalTransaction{reader: readTestSession{ReadSession: tx, getData: invalidStored}, loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}).Load(ctx, Sessions, "s5", &Session{}); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("invalid stored record = %v", err)
		}
		invalidSave := &dalTransaction{reader: readTestSession{ReadSession: tx, getData: invalidStored}, loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}
		if err := invalidSave.Save(ctx, Sessions, "s5", Session{ID: "s5"}, 0, "alice", "s5"); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Save did not propagate read failure = %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string]struct {
		collection, id, owner, session string
		expected                       int64
		value                          any
	}{
		"negative version": {collection: Sessions, id: "s1", owner: "alice", session: "s1", expected: -1, value: Session{}},
		"empty owner":      {collection: Sessions, id: "s1", owner: "", session: "s1", expected: 0, value: Session{}},
		"unsafe session":   {collection: Sessions, id: "s1", owner: "alice", session: "bad session", expected: 0, value: Session{}},
	} {
		t.Run(name, func(t *testing.T) {
			unit := &dalTransaction{loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}
			if err := unit.Save(ctx, args.collection, args.id, args.value, args.expected, args.owner, args.session); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Save error = %v", err)
			}
		})
	}
	marshalFailure := &dalTransaction{loaded: map[string]storedRecord{}, pending: map[string]storedRecord{"sessionContextSessions/s1": {}}}
	if err := marshalFailure.Save(ctx, Sessions, "s1", func() {}, 0, "alice", "s1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unencodable Save value = %v", err)
	}
	largeValue := struct{ Value string }{Value: strings.Repeat("x", 800001)}
	if err := marshalFailure.Save(ctx, Sessions, "s1", largeValue, 0, "alice", "s1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized Save value = %v", err)
	}
	conflict := &dalTransaction{loaded: map[string]storedRecord{"sessionContextSessions/s1": {Version: 2, Payload: json.RawMessage(`{}`)}}, pending: map[string]storedRecord{}}
	if err := conflict.Save(ctx, Sessions, "s1", Session{ID: "s1"}, 0, "alice", "s1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale Save = %v", err)
	}
}

func TestDALListValidatesRowsAndMapsFailures(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name, collection, owner, cursor string
		limit                           int
	}{
		{"unknown collection", "unknown", "alice", "", 10},
		{"missing owner", Sessions, "", "", 10},
		{"low limit", Sessions, "alice", "", 0},
		{"high limit", Sessions, "alice", "", 101},
		{"unsafe cursor", Sessions, "alice", "bad cursor", 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			unit := &dalTransaction{loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}
			if _, _, err := unit.List(ctx, test.collection, test.owner, test.cursor, test.limit); !errors.Is(err, ErrInvalid) {
				t.Fatalf("List error = %v", err)
			}
		})
	}
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	err := db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		unit := func(session readTestSession) *dalTransaction {
			return &dalTransaction{reader: session, loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}
		}
		if _, _, err := unit(readTestSession{queryErr: errors.New("query failed")}).List(ctx, Sessions, "alice", "", 1); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("query error = %v", err)
		}
		if _, _, err := unit(readTestSession{reader: &sliceRecordsReader{nextErr: errors.New("reader failed")}}).List(ctx, Sessions, "alice", "", 1); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("read error = %v", err)
		}
		badData := []any{nil, "not a map", map[string]any{"recordId": "s1"}, map[string]any{"recordId": "s1", "owner": "alice", "sessionId": "s1", "version": 1, "payload": "not json"}, map[string]any{"recordId": "other", "owner": "alice", "sessionId": "s1", "version": 1, "payload": `{"id":"other"}`}}
		for i, data := range badData {
			rec := record.NewRecordWithData(record.NewKeyWithParentAndID(nil, Sessions, fmt.Sprintf("s%d", i)), data)
			if _, _, err := unit(readTestSession{reader: &sliceRecordsReader{records: []record.Record{rec}}}).List(ctx, Sessions, "alice", "", 10); !errors.Is(err, ErrUnavailable) {
				t.Errorf("malformed row %d = %v", i, err)
			}
		}
		wrongOwner := record.NewRecordWithData(record.NewKeyWithParentAndID(nil, Sessions, "s1"), map[string]any{"recordId": "s1", "owner": "bob", "sessionId": "s1", "version": 1, "payload": `{"id":"s1"}`})
		if _, _, err := unit(readTestSession{reader: &sliceRecordsReader{records: []record.Record{wrongOwner}}}).List(ctx, Sessions, "alice", "", 10); !errors.Is(err, ErrForbidden) {
			t.Fatalf("owner mismatch = %v", err)
		}
		good := func(id string) record.Record {
			return record.NewRecordWithData(record.NewKeyWithParentAndID(nil, Sessions, id), map[string]any{"recordId": id, "owner": "alice", "sessionId": id, "version": 1, "payload": fmt.Sprintf(`{"id":%q}`, id)})
		}
		page, cursor, err := unit(readTestSession{reader: &sliceRecordsReader{records: []record.Record{good("s1"), good("s2")}}}).List(ctx, Sessions, "alice", "", 1)
		if err != nil || len(page) != 1 || cursor != "s1" {
			t.Fatalf("bounded List = %s %s %v", page, cursor, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDALRecordAndSchemaHelpers(t *testing.T) {
	valid := map[string]any{"payload": `{"id":"s1"}`, "owner": "alice", "recordId": "s1", "sessionId": "s1", "version": 1}
	if _, err := decodeRecord(Sessions, "s1", valid); err != nil {
		t.Fatal(err)
	}
	invalid := []any{
		"not a map",
		map[string]any{"payload": 1, "owner": "alice", "recordId": "s1", "sessionId": "s1", "version": 1},
		map[string]any{"payload": "not json", "owner": "alice", "recordId": "s1", "sessionId": "s1", "version": 1},
		map[string]any{"payload": `{"id":"s1"}`, "owner": "", "recordId": "s1", "sessionId": "s1", "version": 1},
		map[string]any{"payload": `{"id":"s1"}`, "owner": "alice", "recordId": "other", "sessionId": "s1", "version": 1},
		map[string]any{"payload": `{"id":"s1"}`, "owner": "alice", "recordId": "s1", "version": 1},
		map[string]any{"payload": `{"id":"s1"}`, "owner": "alice", "recordId": "s1", "sessionId": "s1", "version": "bad"},
		map[string]any{"payload": `{"id":"s1"}`, "owner": "alice", "recordId": "s1", "sessionId": "s1", "version": 0},
	}
	for i, data := range invalid {
		if _, err := decodeRecord(Sessions, "s1", data); !errors.Is(err, ErrUnavailable) {
			t.Errorf("invalid row %d = %v", i, err)
		}
	}
	if !knownCollection(Sessions) || knownCollection("other") {
		t.Fatal("collection allowlist is inconsistent")
	}
	statements := SQLiteSchemaStatements()
	if len(statements) != len(Collections())*2 {
		t.Fatalf("schema statement count %d", len(statements))
	}
	manifest := ManifestCollectionsYAML()
	if !strings.Contains(manifest, "collections:") || !strings.Contains(manifest, Sessions) || !strings.Contains(manifest, "payload: {type: string}") {
		t.Fatalf("manifest omits declared collection fields: %s", manifest)
	}
	for _, err := range []error{nil, ErrConflict, ErrForbidden, ErrReadOnly, ErrInvalid, ErrNotFound, ErrUnavailable, context.Canceled, context.DeadlineExceeded} {
		if got := storageError(err); err == nil && got != nil || err != nil && !errors.Is(got, err) {
			t.Errorf("storageError(%v) = %v", err, got)
		}
	}
	if !errors.Is(storageError(errors.New("SQLITE_BUSY: locked")), ErrConflict) || !errors.Is(storageError(errors.New("unknown")), ErrUnavailable) {
		t.Fatal("storage error mapping lost conflict or unavailable classification")
	}
}
