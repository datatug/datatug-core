package dalgostore

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/strongo/validation"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/datatug/datatug-core/pkg/storage"
)

// defaultProjectAccess is the access level CreateProject stamps on a new
// project record. dto.CreateProjectRequest carries no access field, while
// datatug.ProjectFile.Validate() rejects a record whose access is not one of
// "private", "protected" or "public" (pkg/datatug/project.go), so a value
// must be chosen here. "private" is the least-exposing of the three: a
// project that should be shared is widened by a later save, whereas a
// project created public cannot be un-shared retroactively.
const defaultProjectAccess = "private"

// timeNow is time.Now indirected so a test can pin the creation timestamp.
var timeNow = time.Now

// Store is a storage.Store backed by a dal.DB the caller constructs and
// hands in. It keeps many projects, each addressed at
// ext/datatug/projects/<project-id> (REQ:extension-namespace).
//
// Store works over the DALgo interfaces alone and touches no file system:
// it runs unchanged on any backend a dal.DB can front. Installing the
// static inGitDB schema (ingitdbschema.WriteSchema) is deliberately not
// part of it — that is a property of a git-backed store root, known only to
// whoever opens the driver over that directory, and it is done there,
// before a Store is built on top.
type Store struct {
	db dal.DB
	id string
}

var _ storage.Store = (*Store)(nil)

// NewStore returns a Store with the given store id backed by db. db is
// constructed by the caller — the edge (CLI or backend) — never by this
// package, so no driver is named here. Like filestore's own constructor
// (newStore, pkg/storage/filestore/store.go:57-62) and NewProjectStore
// above, NewStore assigns its arguments without validating them.
func NewStore(db dal.DB, id string) *Store {
	return &Store{db: db, id: id}
}

// ID returns the id of this store.
func (s *Store) ID() string {
	return s.id
}

// GetProjectStore returns the datatug.ProjectStore addressing projectID in
// this store, mirroring filestore's own GetProjectStore
// (pkg/storage/filestore/store.go:31-34): it builds the project store
// without checking that the project exists.
func (s *Store) GetProjectStore(projectID string) datatug.ProjectStore {
	return NewProjectStore(s.db, projectID)
}

// CreateProject creates a new DataTug project in this store: the request is
// validated (dto.CreateProjectRequest.Validate requires a store, an id and
// a title, and applies the project-id rules), its StoreID must name this
// store, and the project record is then inserted at
// ext/datatug/projects/<id>.
//
// The id is the caller's: it addresses the project for the rest of its life
// and is never derived from the title here. The record is written with
// Insert, not Set, so creating a project whose id is taken fails with
// record.ErrRecordExists — naming the id — instead of overwriting the
// project that holds it.
//
// Unlike filestore's saveProjectFile, CreateProject persists the title:
// filestore's own GetProjects reads a project's title back out of the
// project file (pkg/storage/filestore/store.go:49), so a created project
// that did not carry its title would list with no title at all.
//
// Failure states. CreateProject is a single record insert in one
// transaction, and it rolls nothing back:
//
//   - a refusal before the insert (an invalid request, a request for
//     another store, an invalid assembled record) writes nothing at all;
//   - a failed insert — a duplicate id, or a backend failure — leaves the
//     store exactly as it was, because the transaction that carries it is
//     the only write. Whatever the backend already held under that id (the
//     project that caused a duplicate) is untouched, and a retry is safe;
//   - it does NOT install the inGitDB schema, create a directory, or
//     initialise a store: a store whose schema was never installed fails
//     here in whatever way its driver reports an unknown collection. See
//     Store's own doc comment for where the schema install belongs.
func (s *Store) CreateProject(ctx context.Context, request dto.CreateProjectRequest) (*datatug.ProjectSummary, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.StoreID != s.id {
		return nil, validation.NewErrBadRequestFieldValue("store",
			fmt.Sprintf("request is for store %q but this store's id is %q", request.StoreID, s.id))
	}
	file := datatug.ProjectFile{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    request.ID,
				Title: request.Title,
			},
			Access: defaultProjectAccess,
		},
		Created: &datatug.ProjectCreated{At: timeNow().UTC()},
	}
	if err := file.Validate(); err != nil {
		return nil, fmt.Errorf("dalgostore: invalid project record: %w", err)
	}
	key := projectKey(request.ID)
	if err := s.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, record.NewRecordWithData(key, &file))
	}); err != nil {
		if record.IsAlreadyExists(err) {
			return nil, fmt.Errorf("dalgostore: project %q already exists in store %q (%s): %w", request.ID, s.id, key, err)
		}
		return nil, fmt.Errorf("dalgostore: failed to create project %s: %w", key, err)
	}
	return &datatug.ProjectSummary{ProjectFile: file}, nil
}

// DeleteProject deletes the project record at
// ext/datatug/projects/<id>. Deleting a project that does not exist is not
// an error: DALgo's Deleter contract has no not-found signal, so this
// mirrors the delete-is-idempotent semantics every adapter implements.
//
// Only the project record is deleted. This store persists no project-item
// collection yet (see the package doc comment), so a project it created has
// no sub-items to cascade to; once a collection is implemented, its records
// must be deleted here too.
func (s *Store) DeleteProject(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return validation.NewErrRequestIsMissingRequiredField("id")
	}
	key := projectKey(id)
	if err := s.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Delete(ctx, key)
	}); err != nil {
		return fmt.Errorf("dalgostore: failed to delete project %s: %w", key, err)
	}
	return nil
}

// GetProjects lists the ext/datatug/projects collection and returns a brief
// per project record. Each brief carries what filestore's own GetProjects
// carries (id, title, repository; pkg/storage/filestore/store.go:40-55)
// plus the access level, which the project record holds and
// datatug.ProjectBrief.Validate() requires. The id comes from the record's
// key, the authority for it, exactly as LoadProjectFile takes it from the
// key rather than from the stored data: a record whose stored id disagrees
// with its key lists under the key's id.
//
// TODO: GetProjects has no pagination — it reads every project record in
// the store into memory in one call, and the dal.IQueryBuilder Limit/Offset
// and cursor support it would need (StartFrom/StartAfter, Reader.Cursor) is
// not plumbed through storage.Store's signature, which returns a plain
// slice and takes no page. A store with many projects needs that signature
// widened before this can be paged.
func (s *Store) GetProjects(ctx context.Context) ([]datatug.ProjectBrief, error) {
	query := dal.From(projectsCollectionRef()).NewQuery().
		SelectIntoRecord(func() record.Record {
			return record.NewRecordWithData(
				record.NewIncompleteKey(projectsCollection, reflect.String, extensionKey()),
				&datatug.ProjectFile{},
			)
		})
	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, query, s.db)
	if err != nil {
		return nil, fmt.Errorf("dalgostore: failed to list projects: %w", err)
	}
	projectBriefs := make([]datatug.ProjectBrief, len(records))
	for i, rec := range records {
		file, ok := rec.Data().(*datatug.ProjectFile)
		if !ok {
			return nil, fmt.Errorf("dalgostore: project record %s came back as %T, expected *datatug.ProjectFile", rec.Key(), rec.Data())
		}
		id, ok := rec.Key().ID.(string)
		if !ok {
			return nil, fmt.Errorf("dalgostore: project record key %s has a non-string ID of type %T", rec.Key(), rec.Key().ID)
		}
		projectBriefs[i] = datatug.ProjectBrief{
			Access:     file.Access,
			Repository: file.Repository,
		}
		projectBriefs[i].ID = id
		projectBriefs[i].Title = file.Title
	}
	return projectBriefs, nil
}

// projectsCollectionRef returns a reference to the ext/datatug/projects
// collection every project record lives in.
func projectsCollectionRef() dal.CollectionRef {
	return dal.NewCollectionRef(projectsCollection, "", extensionKey())
}
