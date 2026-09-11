package datatug

import "context"

// QueryRevision identifies one persisted state of a query's on-disk pair
// ("<id>.query.json" + its body sidecar, see pkg/datatug/doc.go). It is
// derived from the exact bytes a store persisted - metadata bytes, the body
// sidecar's type-derived extension and exact body bytes - so any change to
// either file, including one made outside the revisioned API (a hand edit,
// another process), produces a different revision and invalidates a writer
// holding the old one. It never depends on how a caller spelled the id to
// find the pair, so a record read through two spellings a file system
// resolves to the same files (e.g. letter case on macOS/Windows) has one
// revision. It carries no meaning beyond equality comparison: callers must
// not parse it or assume any ordering.
type QueryRevision string

// QueryWriteCondition is the optimistic-concurrency precondition a
// RevisionedQueriesStore.PutQuery call must supply: exactly one of
// IfNoneMatch (create - succeed only when no record currently exists) or
// IfMatch (update - succeed only when the current record's revision equals
// this value). Supplying both, or neither, is a caller error that PutQuery
// rejects before touching the file system.
type QueryWriteCondition struct {
	IfNoneMatch bool
	IfMatch     QueryRevision
}

// StoredQuery pairs a loaded/persisted query with the revision of the exact
// bytes it was loaded from or written as.
type StoredQuery struct {
	Query    QueryDefWithFolderPath
	Revision QueryRevision
}

// RevisionedQueriesStore extends QueriesStore with optimistic-concurrency
// operations for capturing a query as a project asset: create-only,
// update-if-unchanged and delete-if-unchanged. It is additive - every
// QueriesStore implementation may add it without breaking source
// compatibility with existing callers of the plain interface.
//
// Implementations must persist QueryDefWithFolderPath.QueryDef.Text and its
// JSON metadata as one recoverable logical transaction (see
// pkg/storage/filestore's query transaction helpers): participating readers
// never observe a mix of an old and a new revision's bytes, and an
// interrupted write leaves either the previous complete revision or the new
// complete revision on disk, never a torn pair.
type RevisionedQueriesStore interface {
	// LoadQueryRevision loads the query at id (a "<folder>/.../<id>" path,
	// like QueriesStore.LoadQuery) together with the revision of the exact
	// bytes it was read from. Unlike the tolerant QueriesStore.LoadQuery, it
	// rejects an incomplete committed pair (IncompleteQueryRecordError)
	// rather than silently treating a missing body sidecar as an empty body.
	LoadQueryRevision(ctx context.Context, id string, o ...StoreOption) (*StoredQuery, error)

	// PutQuery creates or updates a query under condition. condition must set
	// exactly one of IfNoneMatch or IfMatch; a request that sets neither or
	// both fails validation before any I/O. On success it returns the stored
	// query and its new revision. A failed precondition - the query already
	// exists for IfNoneMatch, or the current revision does not equal IfMatch
	// (including because the record does not exist) - returns a
	// QueryRevisionConflictError and leaves every persisted file unchanged.
	PutQuery(ctx context.Context, query *QueryDefWithFolderPath, condition QueryWriteCondition) (*StoredQuery, error)

	// DeleteQueryRevision deletes the query at id only when its current
	// revision equals expected. A stale or missing revision returns a
	// QueryRevisionConflictError and leaves every persisted file unchanged.
	DeleteQueryRevision(ctx context.Context, id string, expected QueryRevision) error
}
