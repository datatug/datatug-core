package dataaccess

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
)

type guardedSession struct {
	authority        *Authority
	read             dal.ReadSession
	write            dal.WriteSession
	bound            *lease
	lifetime         context.Context
	identityDB       dal.DB
	identityVerifier TableIdentityVerifier
}

func (s guardedSession) prepare(ctx context.Context) (context.Context, *lease, context.CancelFunc, error) {
	return s.prepareRequest(ctx, nil)
}

func (s guardedSession) prepareRequest(ctx context.Context, request *access.Request) (context.Context, *lease, context.CancelFunc, error) {
	if s.lifetime == nil {
		return s.authority.prepareRequest(ctx, s.bound, request)
	}
	if err := s.lifetime.Err(); err != nil {
		return nil, nil, nil, err
	}
	merged, cancelLifetime := context.WithCancel(ctx)
	stop := context.AfterFunc(s.lifetime, cancelLifetime)
	prepared, authorization, cancel, err := s.authority.prepareRequest(merged, s.bound, request)
	if err != nil {
		stop()
		cancelLifetime()
		return nil, nil, nil, err
	}
	return prepared, authorization, func() { stop(); cancel(); cancelLifetime() }, nil
}

func (s guardedSession) run(ctx context.Context, operation func(context.Context) error) error {
	ctx, authorization, cancel, err := s.prepare(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	if err := operation(ctx); err != nil {
		return err
	}
	return s.authority.recheck(ctx, authorization)
}

func (s guardedSession) Exists(ctx context.Context, key *record.Key) (exists bool, err error) {
	err = s.run(ctx, func(ctx context.Context) error {
		var readErr error
		exists, readErr = s.read.Exists(ctx, key)
		return readErr
	})
	if err != nil {
		exists = false
	}
	return
}

func (s guardedSession) Get(ctx context.Context, rec record.Record) error {
	return s.GetMulti(ctx, []record.Record{rec})
}

func (s guardedSession) GetMulti(ctx context.Context, records []record.Record) error {
	targets := make([]*readTarget, 0, len(records))
	staged := make([]record.Record, 0, len(records))
	for _, rec := range records {
		target, err := stageRead(rec)
		if err != nil {
			return s.authority.denial(DataRead, "ACL_ENFORCEMENT_UNSUPPORTED")
		}
		targets = append(targets, target)
		staged = append(staged, target.record)
	}
	err := s.run(ctx, func(ctx context.Context) error {
		if err := s.read.GetMulti(ctx, staged); err != nil {
			return err
		}
		return record.AnyRecordWithError(staged...)
	})
	for i, target := range targets {
		if err != nil {
			target.clear()
			records[i].SetError(err)
		} else {
			target.publish()
			if target.record.Exists() {
				records[i].SetError(nil)
			} else {
				records[i].SetError(record.ErrRecordNotFound)
			}
		}
	}
	return err
}

type readTarget struct {
	original reflect.Value
	staged   reflect.Value
	record   record.Record
}

// Point reads stage into fresh containers of the caller's data type. The
// adapter never receives caller-owned aliases; a partial error cannot populate
// them. Maps, non-nil pointers and fixed-length slices are supported. Key-only,
// nil, scalar or otherwise inaccessible targets are explicitly refused.
func stageRead(rec record.Record) (target *readTarget, err error) {
	defer func() {
		if recover() != nil {
			target, err = nil, fmt.Errorf("dataaccess: inaccessible read target")
		}
	}()
	original := reflect.ValueOf(rec.Data())
	var staged reflect.Value
	switch original.Kind() {
	case reflect.Map:
		staged = reflect.MakeMap(original.Type())
	case reflect.Pointer:
		staged = reflect.New(original.Type().Elem())
	case reflect.Slice:
		staged = reflect.MakeSlice(original.Type(), original.Len(), original.Len())
	default:
		return nil, fmt.Errorf("dataaccess: unsupported read target")
	}
	if original.IsNil() {
		return nil, fmt.Errorf("dataaccess: nil read target")
	}
	return &readTarget{original: original, staged: staged, record: record.NewRecordWithData(rec.Key(), staged.Interface())}, nil
}

func (t *readTarget) clear() {
	switch t.original.Kind() {
	case reflect.Map, reflect.Slice:
		t.original.Clear()
	case reflect.Pointer:
		t.original.Elem().SetZero()
	}
}

func (t *readTarget) publish() {
	switch t.original.Kind() {
	case reflect.Map:
		t.original.Clear()
		iterator := t.staged.MapRange()
		for iterator.Next() {
			t.original.SetMapIndex(iterator.Key(), iterator.Value())
		}
	case reflect.Pointer:
		t.original.Elem().Set(t.staged.Elem())
	case reflect.Slice:
		reflect.Copy(t.original, t.staged)
	}
}

var (
	keysFromType  = reflect.TypeOf(dal.From(dal.NewRootCollectionRef("dataaccess", "")))
	keysQueryType = reflect.TypeOf(dal.From(dal.NewRootCollectionRef("dataaccess", "")).NewQuery().SelectKeysOnly(reflect.String))
)

// freezeKeysQuery admits only a known, unconditional key-enumeration profile.
// Rebuilding it freezes source/shape before authorization and prevents mutable
// custom StructuredQuery implementations changing the dispatched operation.
func freezeKeysQuery(query dal.Query) (dal.Query, bool) {
	return freezeNamedKeysQuery(query, false)
}

func freezeNamedKeysQuery(query dal.Query, allowQualified bool) (dal.Query, bool) {
	if reflect.TypeOf(query) != keysQueryType {
		return nil, false
	}
	structured, ok := query.(dal.StructuredQuery)
	if !ok || structured.From() == nil || reflect.TypeOf(structured.From()) != keysFromType || reflect.ValueOf(structured.From()).IsNil() || structured.Where() != nil ||
		structured.Having() != nil || len(structured.GroupBy()) != 0 || len(structured.OrderBy()) != 0 ||
		len(structured.Columns()) != 0 || len(structured.From().Joins()) != 0 ||
		structured.IDKind() == reflect.Invalid {
		return nil, false
	}
	source, ok := structured.From().Base().(dal.CollectionRef)
	if !ok || source.Database() != "" || source.Parent() != nil ||
		(source.Schema() != "" && !allowQualified) ||
		(source.Schema() != "" && (!canonical(source.Schema()) || strings.ContainsAny(source.Schema(), "*?[]"))) ||
		!canonical(source.Name()) || strings.ContainsAny(source.Name(), "*?[]") ||
		(source.Schema() == "" && (strings.Contains(source.Name(), ".") || source.Alias() != "")) ||
		(source.Alias() != "" && !canonical(source.Alias())) ||
		source.ScanLimit() != 0 || len(source.ScanOrders()) != 0 {
		return nil, false
	}
	frozenSource := dal.NewRootCollectionRef(source.Name(), "")
	if source.Schema() != "" {
		frozenSource = dal.NewQualifiedRootCollectionRef(source.Schema(), source.Name(), source.Alias())
	}
	builder := dal.From(frozenSource).NewQuery().
		Limit(structured.Limit()).Offset(structured.Offset())
	if structured.StartFrom() != "" {
		builder.StartFrom(structured.StartFrom())
	} else {
		builder.StartAfter(structured.StartAfter())
	}
	return builder.SelectKeysOnly(structured.IDKind()), true
}

func (s guardedSession) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	query, supported := freezeNamedKeysQuery(query, s.identityVerifier != nil)
	if !supported {
		return nil, s.authority.denial(QueryTugQL, "ACL_ENFORCEMENT_UNSUPPORTED")
	}
	request := &access.Request{Operation: access.Query, Resources: access.ResourcesForQuery(query)}
	ctx, authorization, cancel, err := s.prepareRequest(ctx, request)
	if err != nil {
		return nil, err
	}
	// Permission candidacy and its original ceiling precede any fact-port I/O.
	if table, explicit := request.Resources[0].Table(); explicit {
		authorization.identity = &tableIdentityBinding{db: s.identityDB, verifier: s.identityVerifier, request: TableIdentityRequest{Connection: s.authority.request.Connection, Table: table}}
		if err := authorization.identity.verify(ctx); err != nil {
			cancel()
			return nil, s.authority.denial(DataRead, "TABLE_IDENTITY_UNVERIFIED")
		}
		if err := s.authority.check(ctx, authorization); err != nil {
			cancel()
			return nil, err
		}
	}
	reader, err := s.read.ExecuteQueryToRecordsReader(ctx, query)
	if err != nil {
		cancel()
		return nil, err
	}
	if err := s.authority.recheck(ctx, authorization); err != nil {
		cancel()
		_ = reader.Close()
		return nil, err
	}
	return newRecordsReader(s.authority, ctx, authorization, cancel, reader), nil
}

func (s guardedSession) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, s.authority.denial(QueryTugQL, "ACL_ENFORCEMENT_UNSUPPORTED")
}

func (s guardedSession) Set(ctx context.Context, rec record.Record) error {
	return s.run(ctx, func(ctx context.Context) error { return s.write.Set(ctx, rec) })
}

func (s guardedSession) SetMulti(ctx context.Context, records []record.Record) error {
	return s.run(ctx, func(ctx context.Context) error { return s.write.SetMulti(ctx, records) })
}

func (s guardedSession) Insert(ctx context.Context, rec record.Record, options ...dal.InsertOption) error {
	return s.run(ctx, func(ctx context.Context) error { return s.write.Insert(ctx, rec, options...) })
}

func (s guardedSession) InsertMulti(ctx context.Context, records []record.Record, options ...dal.InsertOption) error {
	return s.run(ctx, func(ctx context.Context) error { return s.write.InsertMulti(ctx, records, options...) })
}

func (s guardedSession) Update(ctx context.Context, key *record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	return s.run(ctx, func(ctx context.Context) error { return s.write.Update(ctx, key, updates, preconditions...) })
}

func (s guardedSession) UpdateRecord(ctx context.Context, rec record.Record, updates []update.Update, preconditions ...dal.Precondition) error {
	return s.run(ctx, func(ctx context.Context) error { return s.write.UpdateRecord(ctx, rec, updates, preconditions...) })
}

func (s guardedSession) UpdateMulti(ctx context.Context, keys []*record.Key, updates []update.Update, preconditions ...dal.Precondition) error {
	return s.run(ctx, func(ctx context.Context) error { return s.write.UpdateMulti(ctx, keys, updates, preconditions...) })
}

func (s guardedSession) Delete(ctx context.Context, key *record.Key) error {
	return s.run(ctx, func(ctx context.Context) error { return s.write.Delete(ctx, key) })
}

func (s guardedSession) DeleteMulti(ctx context.Context, keys []*record.Key) error {
	return s.run(ctx, func(ctx context.Context) error { return s.write.DeleteMulti(ctx, keys) })
}

type database struct {
	guardedSession
	secured dal.DB
}

func (db *database) RunReadonlyTransaction(ctx context.Context, worker dal.ROTxWorker, options ...dal.TransactionOption) error {
	ctx, authorization, cancel, err := db.authority.prepare(ctx, nil)
	if err != nil {
		return err
	}
	defer cancel()
	return db.secured.RunReadonlyTransaction(ctx, func(workerCtx context.Context, tx dal.ReadTransaction) error {
		guarded := &readTransaction{ReadSession: guardedSession{authority: db.authority, read: tx, bound: authorization, lifetime: ctx}, options: tx.Options()}
		workerCtx = dal.NewContextWithTransaction(workerCtx, guarded)
		if err := worker(workerCtx, guarded); err != nil {
			return err
		}
		return db.authority.recheck(workerCtx, authorization)
	}, options...)
}

type readTransaction struct {
	dal.ReadSession
	options dal.TransactionOptions
}

func (tx *readTransaction) Options() dal.TransactionOptions { return tx.options }
