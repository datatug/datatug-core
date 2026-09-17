// Package dalgostore implements datatug.ProjectStore over a caller-supplied
// dal.DB (dalgo-project-store plan, task 5). It depends on the
// github.com/dal-go/dalgo and github.com/dal-go/record interfaces only: no
// DALgo driver (dalgo2ingitdb, dalgo2ingitdb4github, dalgo2openvaultdb) is
// imported here, and no code in this package branches on which concrete
// driver backs db (REQ:project-store-is-a-dalgo-database). The driver is
// selected at the edge — whichever code constructs the dal.DB this package
// is handed (a later task in the plan).
//
// Every DataTug record lives under the extension namespace ext/datatug/
// (REQ:extension-namespace): a project's key path is
// ext/datatug/projects/<project-id>, and every project item nests below it
// (REQ:hierarchy-is-a-key-path).
//
// Task 5 implements only the project record itself: LoadProjectFile,
// LoadProject and SaveProject read and write the record at that key path.
// Every other project-item collection (queries, boards, folders, entities,
// environments, env db servers/catalogs, project db drivers/servers,
// recordset definitions) is a stub that returns ErrNotImplemented; later
// tasks in the dalgo-project-store plan implement them one collection at a
// time.
package dalgostore

import (
	"context"
	"errors"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Collection names and the extension-namespace record id every DataTug
// record is addressed under (REQ:extension-namespace).
const (
	extensionCollection = "ext"
	extensionRecordID   = "datatug"
	projectsCollection  = "projects"
)

// ErrNotImplemented is returned by every ProjectStore member this package
// does not implement yet. Task 5 of the dalgo-project-store plan covers only
// the project record; every other project-item collection is later work
// (see the package doc comment).
var ErrNotImplemented = errors.New("dalgostore: not implemented")

// notImplemented reports that member is not implemented yet, naming it so a
// caller that hits a stub knows exactly what is missing and why.
func notImplemented(member string) error {
	return fmt.Errorf("%w: %s (dalgo-project-store plan, task 5 covers only the project record)", ErrNotImplemented, member)
}

// extensionKey returns the ext/datatug scoping-parent key every DataTug
// record nests under. It is never written as a record of its own
// (REQ:extension-namespace): like a Firestore parent document, it need not
// exist for its children's key paths to resolve.
func extensionKey() *record.Key {
	return record.NewKeyWithID(extensionCollection, extensionRecordID)
}

// projectKey returns the key path of a project's own record:
// ext/datatug/projects/<projectID>.
func projectKey(projectID string) *record.Key {
	return record.NewKeyWithParentAndID(extensionKey(), projectsCollection, projectID)
}

// ProjectStore is a datatug.ProjectStore backed by a dal.DB the caller
// constructs and hands in.
type ProjectStore struct {
	db        dal.DB
	projectID string
}

var _ datatug.ProjectStore = (*ProjectStore)(nil)

// NewProjectStore returns a ProjectStore for projectID backed by db. db is
// constructed by the caller — the edge (CLI or backend) — never by this
// package, so no driver is named here.
func NewProjectStore(db dal.DB, projectID string) *ProjectStore {
	if db == nil {
		panic("dalgostore.NewProjectStore: db is required")
	}
	if projectID == "" {
		panic("dalgostore.NewProjectStore: projectID is required")
	}
	return &ProjectStore{db: db, projectID: projectID}
}

// ProjectID returns the id of the project this store addresses.
func (s *ProjectStore) ProjectID() string {
	return s.projectID
}

// LoadProjectFile reads the project record at
// ext/datatug/projects/<project-id>.
func (s *ProjectStore) LoadProjectFile(ctx context.Context) (datatug.ProjectFile, error) {
	var file datatug.ProjectFile
	rec := record.NewRecordWithData(projectKey(s.projectID), &file)
	if err := s.db.Get(ctx, rec); err != nil {
		if record.IsNotFound(err) {
			return datatug.ProjectFile{}, fmt.Errorf("%w: %v", datatug.ErrProjectDoesNotExist, err)
		}
		return datatug.ProjectFile{}, fmt.Errorf("dalgostore: failed to load project record: %w", err)
	}
	file.ID = s.projectID
	return file, nil
}

// LoadProject reads the project record and returns a Project carrying it.
// Task 5 covers only the project record: sub-item collections (queries,
// boards, entities, environments, ...) are not loaded here and are left
// nil, since their stores are stubs (see the package doc comment).
func (s *ProjectStore) LoadProject(ctx context.Context, _ ...datatug.StoreOption) (*datatug.Project, error) {
	file, err := s.LoadProjectFile(ctx)
	if err != nil {
		return nil, err
	}
	project := datatug.NewProjectWithStore(s.projectID, s)
	project.ProjectItem = file.ProjectItem
	project.Created = file.Created
	project.Repository = file.Repository
	return project, nil
}

// SaveProject validates and writes the project record at
// ext/datatug/projects/<project-id>. Task 5 covers only the project record:
// p's sub-item collections (queries, boards, entities, environments, ...)
// are not persisted here, since their stores are stubs (see the package doc
// comment).
func (s *ProjectStore) SaveProject(ctx context.Context, p *datatug.Project) error {
	file := datatug.ProjectFile{
		ProjectItem: p.ProjectItem,
		Created:     p.Created,
		Repository:  p.Repository,
	}
	if err := file.Validate(); err != nil {
		return fmt.Errorf("dalgostore: invalid project record: %w", err)
	}
	key := projectKey(s.projectID)
	return s.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(key, &file))
	})
}
