package dataaccess

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
)

// TableReadProfile admits only a Core-constructed, qualified, unconditional
// all-column recordset preview. It does not enable arbitrary structured queries.
const TableReadProfile = "table-recordset/1"

// TableRead is an immutable operation description, not SQL or authorization.
// MaxRows is 1..100; MaxBytes is 1..262144 and includes protected preview metadata.
type TableRead struct {
	Object   ObjectRef `json:"object"`
	MaxRows  uint32    `json:"maxRows"`
	MaxBytes uint32    `json:"maxBytes"`
}

// TableReadSource is a trusted host opener. It independently binds the exact
// source, table, compiler/options and native allocation limits before opening
// data rows. No grants or payload callback enter this port. An unproved native
// profile must refuse; implementing this interface does not certify an opener.
type TableReadSource interface {
	OpenTableRead(context.Context, TableIdentityRequest) (TableReadSpan, error)
}

// TableReadSpan pins one actual read-only native execution span. Session must
// remain bound to it; Verify is SQL-free immutable-proof/liveness validation,
// including while a cursor is active. No pooled sampling, lazy metadata SQL,
// mutable routing, compiler options, views or fallback are permitted. Close must
// cancel/invalidate and roll back safely during reads; never commit. The host
// owns redacted cleanup-error reporting, including after output was handed off.
type TableReadSpan interface {
	Session() dal.ReadSession
	Verify(context.Context, TableIdentityRequest) (TableIdentityFacts, error)
	Close() error
}

func (op TableRead) valid() bool {
	return canonical(op.Object.Namespace) && canonical(op.Object.Name) &&
		!strings.ContainsAny(op.Object.Namespace+op.Object.Name, "*?[]") &&
		op.MaxRows > 0 && op.MaxRows <= 100 && op.MaxBytes > 0 && op.MaxBytes <= 262144
}

func (op TableRead) query() (dal.Query, *access.Request, error) {
	if !op.valid() {
		return nil, nil, fmt.Errorf("dataaccess: unsupported table preview")
	}
	query := dal.From(dal.NewQualifiedRootCollectionRef(op.Object.Namespace, op.Object.Name, "")).NewQuery().
		Limit(int(op.MaxRows) + 1).SelectIntoRecordset()
	request := &access.Request{Operation: access.Query, Resources: access.ResourcesForQuery(query)}
	return query, request, nil
}

// ReadTableResult constructs and executes its own frozen operation, secures the
// supplied native session with DALgo, serializes actual cells into private bytes
// and returns a version2 retained result. Existing v1 capture stays connection-
// wide. The authenticated host computes ResultRequest from the actual complete
// command/scope; the client computes its expected digest independently.
func (a *Authority) ReadTableResult(ctx context.Context, request ResultRequest, operation TableRead, source TableReadSource) (*Result, error) {
	if a == nil || a.resolve == nil || ctx == nil || !request.valid() || isNilValue(source) {
		return nil, fmt.Errorf("dataaccess: table source, authority and request required")
	}
	query, readRequest, err := operation.query()
	if err != nil {
		return nil, err
	}
	prepared, bound, cancel, err := a.prepareRequest(ctx, nil, readRequest)
	if err != nil {
		return nil, err
	}
	// This cancellation remains owned by the Result, rather than the call stack.
	clampContext(ctx, bound)
	// One fixed backing allocation: growing with append must not strand older
	// serialized record copies outside the expiry/purge owner.
	result := a.newTableResult(ctx, request, operation, bound, make([]byte, 0, int(operation.MaxBytes)))
	result.ownCleanup(cancel)
	factsRequest := TableIdentityRequest{Connection: a.request.Connection, Table: access.TableName{Schema: operation.Object.Namespace, Name: operation.Object.Name}}
	span, err := source.OpenTableRead(prepared, factsRequest)
	if !isNilValue(span) {
		result.ownCleanup(func() { _ = span.Close() })
	}
	if err != nil {
		err = a.denial(DataRead, "TABLE_SOURCE_UNAVAILABLE")
	}
	if err == nil && isNilValue(span) {
		err = a.denial(DataRead, "TABLE_IDENTITY_UNVERIFIED")
	}
	if err == nil {
		bound.identity = &tableIdentityBinding{span: span, request: factsRequest}
		err = result.readTable(prepared, span, query, operation)
	}
	if err != nil {
		result.Close()
		return nil, err
	}
	return result, nil
}

func (r *Result) readTable(ctx context.Context, span TableReadSpan, query dal.Query, operation TableRead) error {
	if err := r.guard(ctx); err != nil {
		return err
	}
	session := span.Session()
	if isNilValue(session) {
		return r.authority.denial(DataRead, "ACL_ENFORCEMENT_UNSUPPORTED")
	}
	// DALgo may enforce a field/record residual by rewriting a query. This
	// profile cannot silently become a projection or filtered investigation.
	// Only the exact Core-built builtin snapshot reaches the native session.
	secured := access.SecureReadSession(frozenTableSession{ReadSession: session, query: query}, r.authorization.policy)
	reader, err := secured.ExecuteQueryToRecordsetReader(ctx, query)
	var once sync.Once
	var closeErr error
	closeReader := func() { once.Do(func() { closeErr = reader.Close() }) }
	if !isNilValue(reader) {
		r.ownCleanup(closeReader)
	}
	if err != nil {
		return r.authority.denial(DataRead, "TABLE_READ_FAILED")
	}
	if isNilValue(reader) {
		return r.authority.denial(DataRead, "ACL_ENFORCEMENT_UNSUPPORTED")
	}
	if err := r.guard(ctx); err != nil {
		return err
	}
	if err := r.readTableRows(ctx, reader, operation); err != nil {
		return err
	}
	closeReader()
	reader = nil // the cleanup closure must not retain closed provider rows across fresh resolve
	if closeErr != nil {
		return r.authority.denial(DataRead, "TABLE_READER_CLOSE_FAILED")
	}
	if err := r.guard(ctx); err != nil {
		return err
	}
	return r.sealTable()
}

type frozenTableSession struct {
	dal.ReadSession
	query dal.Query
}

func (s frozenTableSession) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	// DALgo adds its private requested-query carrier even with no residual.
	// Compare the effective public shape, then dispatch our original builtin
	// snapshot so no policy wrapper/custom execution method reaches the driver.
	effective, ok := query.(dal.StructuredQuery)
	if len(options) != 0 || !ok || !reflect.DeepEqual(effective.From(), s.query.(dal.StructuredQuery).From()) ||
		effective.Where() != nil || effective.Having() != nil || len(effective.Columns()) != 0 ||
		len(effective.GroupBy()) != 0 || len(effective.OrderBy()) != 0 || effective.IDKind() != reflect.Invalid ||
		effective.Offset() != 0 || effective.Limit() != s.query.Limit() || effective.StartFrom() != "" || effective.StartAfter() != "" {
		return nil, fmt.Errorf("dataaccess: table preview query rewrite is unsupported")
	}
	return s.ReadSession.ExecuteQueryToRecordsetReader(ctx, s.query)
}

// A concurrent expiry can win after the last authority check and before seal.
// Never turn purged bytes into a successful newly published retained result.
func (r *Result) sealTable() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.payload == nil {
		return errors.New("dataaccess: result is closed")
	}
	r.metadata.PayloadSHA256 = hashBytes(r.payload)
	return nil
}
