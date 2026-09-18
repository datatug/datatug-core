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
	"github.com/datatug/datatug-core/pkg/storage/ingitdbschema"
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
// The store's own dir is the inGitDB store root the static schema is
// installed into by CreateProject. dal.DB deliberately hides which backend
// it runs on — it exposes only ID(), Adapter(), Schema() and the read/write
// sessions, none of which yields a filesystem path — so the directory
// cannot be recovered from db and is supplied by whoever constructs both,
// exactly as filestore's own NewStore is handed its projects' paths
// (pkg/storage/filestore/store.go:12). A store opened over a backend with
// no local directory (a remote driver, or an in-memory double) is
// constructed with an empty dir; CreateProject then refuses rather than
// silently skipping the schema install.
type Store struct {
	db  dal.DB
	id  string
	dir string
}

var _ storage.Store = (*Store)(nil)

// NewStore returns a Store with the given store id backed by db, whose
// inGitDB store root is dir. db is constructed by the caller — the edge
// (CLI or backend) — never by this package, so no driver is named here.
// Like filestore's own constructor (newStore,
// pkg/storage/filestore/store.go:57-62) and NewProjectStore above,
// NewStore assigns its arguments without validating them.
func NewStore(db dal.DB, id, dir string) *Store {
	return &Store{db: db, id: id, dir: dir}
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

// CreateProject creates a new DataTug project in this store:
//
//  1. the request is validated (dto.CreateProjectRequest.Validate requires
//     both store and title), and its StoreID must name this store;
//  2. the static inGitDB schema is installed into the store root with
//     ingitdbschema.WriteSchema, which is idempotent and safe to repeat, so
//     a store whose schema is already installed is left as it is;
//  3. the project record is inserted at ext/datatug/projects/<project-id>.
//
// The project id is derived from the title (see projectIDFromTitle): it
// becomes a key segment, and under the local driver a directory name, so a
// human-readable, git-friendly id is produced rather than an opaque random
// one. The record is written with Insert, not Set, so creating a project
// whose id is taken fails with record.ErrRecordExists instead of
// overwriting the existing project.
//
// Unlike SaveProject, CreateProject persists the title: filestore's own
// GetProjects reads a project's title back out of the project file
// (pkg/storage/filestore/store.go:49), so a created project that did not
// carry its title would list with no title at all.
func (s *Store) CreateProject(ctx context.Context, request dto.CreateProjectRequest) (*datatug.ProjectSummary, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.StoreID != s.id {
		return nil, validation.NewErrBadRequestFieldValue("store",
			fmt.Sprintf("request is for store %q but this store's id is %q", request.StoreID, s.id))
	}
	projectID, err := projectIDFromTitle(request.Title)
	if err != nil {
		return nil, err
	}
	if s.dir == "" {
		return nil, fmt.Errorf("dalgostore: store %q has no store root directory, so the inGitDB schema can not be installed: pass the store root to NewStore", s.id)
	}
	if err = ingitdbschema.WriteSchema(s.dir); err != nil {
		return nil, fmt.Errorf("dalgostore: failed to install inGitDB schema into %s: %w", s.dir, err)
	}
	file := datatug.ProjectFile{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    projectID,
				Title: request.Title,
			},
			Access: defaultProjectAccess,
		},
		Created: &datatug.ProjectCreated{At: timeNow().UTC()},
	}
	if err = file.Validate(); err != nil {
		return nil, fmt.Errorf("dalgostore: invalid project record: %w", err)
	}
	key := projectKey(projectID)
	if err = s.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, record.NewRecordWithData(key, &file))
	}); err != nil {
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
// key rather than from the stored data.
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

// projectIDFromTitle derives a project id from a project title, because
// dto.CreateProjectRequest carries no id and the id becomes a key segment —
// a directory name on the local driver (REQ:canonical-project-layout). Each
// run of characters that is neither an ASCII letter nor a digit becomes a
// single "-", letters are lower-cased, and leading/trailing "-" are
// trimmed: "My First Project!" becomes "my-first-project".
//
// The mapping is deterministic rather than random, so the id a title
// produces is predictable and readable in Git. Two projects whose titles
// slug the same collide, and CreateProject's Insert then fails with
// record.ErrRecordExists rather than overwriting the first.
func projectIDFromTitle(title string) (string, error) {
	var b strings.Builder
	lastWasDash := false
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastWasDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r - 'A' + 'a')
			lastWasDash = false
		default:
			if b.Len() > 0 && !lastWasDash {
				b.WriteByte('-')
				lastWasDash = true
			}
		}
	}
	id := strings.TrimSuffix(b.String(), "-")
	if id == "" {
		return "", validation.NewErrBadRequestFieldValue("title",
			fmt.Sprintf("a project id can not be derived from title %q: it has no ASCII letters or digits", title))
	}
	return id, nil
}
