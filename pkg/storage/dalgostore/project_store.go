// Package dalgostore implements storage.Store and datatug.ProjectStore over
// a caller-supplied dal.DB (dalgo-project-store plan): Store (store.go)
// creates, lists, opens and deletes projects in one store, and ProjectStore
// (this file) addresses one project in it. It depends on the github.com/dal-go/dalgo
// and github.com/dal-go/record interfaces only: no DALgo driver
// (dalgo2ingitdb, dalgo2ingitdb4github, dalgo2openvaultdb) is imported here,
// and no code in this package branches on which concrete driver backs db
// (REQ:project-store-is-a-dalgo-database). The driver is selected at the
// edge — whichever code constructs the dal.DB this package is handed (a
// later task in the plan).
//
// Every DataTug record lives under the extension namespace ext/datatug/
// (REQ:extension-namespace): a project's key path is
// ext/datatug/projects/<project-id>, and every project item nests below it
// (REQ:hierarchy-is-a-key-path).
//
// Only the project record itself is implemented so far: Store's four
// members and ProjectStore's LoadProjectFile, LoadProject and SaveProject
// create, list, delete, read and write the record at that key path.
// Every other project-item collection (queries, boards, folders, entities,
// environments, env db servers/catalogs, project db drivers/servers,
// recordset definitions) is a stub that returns ErrNotImplemented; later
// tasks in the dalgo-project-store plan implement them one collection at a
// time. SaveProject and LoadProject also return ErrNotImplemented — writing
// or loading nothing — rather than silently dropping or omitting a
// collection they do not yet persist or load; see their doc comments.
package dalgostore

import (
	"context"
	"errors"
	"fmt"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/strongo/validation"

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
// does not implement yet, and by SaveProject/LoadProject when the caller
// asks for more than the project record (see the package doc comment).
var ErrNotImplemented = errors.New("dalgostore: not implemented")

// notImplemented reports that member is not implemented yet, naming it so a
// caller that hits a stub knows exactly what is missing and why.
func notImplemented(member string) error {
	return fmt.Errorf("%w: %s", ErrNotImplemented, member)
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
// package, so no driver is named here. Like filestore's own constructor
// (newFsProjectStore, pkg/storage/filestore/project_store.go:10-26),
// NewProjectStore assigns its arguments without validating them.
func NewProjectStore(db dal.DB, projectID string) *ProjectStore {
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
// Depth follows filestore's own convention
// (pkg/storage/filestore/store_loader.go:27): Depth()==0 — the default, when
// the caller passes no options — or a depth greater than 1 asks for the full
// project graph (sub-item collections included), which this package does
// not load yet. Rather than return a Project with those collections
// silently left empty, LoadProject returns ErrNotImplemented naming what it
// cannot load. Only an explicit datatug.Depth(1) — "just this record" — is
// served.
func (s *ProjectStore) LoadProject(ctx context.Context, o ...datatug.StoreOption) (*datatug.Project, error) {
	opts := datatug.GetStoreOptions(o...)
	if opts.Depth() == 0 || opts.Depth() > 1 {
		return nil, notImplemented("LoadProject: full project graph (queries, boards, entities, environments, db models, db drivers); pass datatug.Depth(1) to load only the project record")
	}
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

// SaveProject writes the project record at ext/datatug/projects/<project-id>,
// matching filestore's own SaveProject semantics for the part this package
// implements:
//   - the whole project is validated first, exactly as filestore does
//     (pkg/storage/filestore/store_project_saver.go:23, project.Validate());
//   - the fields persisted are exactly the ones filestore's saveProjectFile
//     builds — ID, Title, Access, Repository and Created
//     (pkg/storage/filestore/store_project_saver.go:133-149). Folder, tags
//     and UserIDs are not part of the project record there, and are not
//     carried here either;
//   - the assembled ProjectFile is validated again before being written,
//     exactly as filestore's putProjectFile does
//     (pkg/storage/filestore/store_project_saver.go:113-115).
//
// Before any of that, SaveProject refuses a project whose ID does not match
// this store's own project ID with a typed validation.ErrBadFieldValue, and
// a project carrying data in a collection this package does not persist yet
// (queries, boards, entities, environments, db models or db drivers) with a
// wrapped ErrNotImplemented naming the collection — so nothing is silently
// dropped and nothing is written. Sub-item persistence is later work (see
// the package doc comment).
func (s *ProjectStore) SaveProject(ctx context.Context, p *datatug.Project) error {
	if p.ID != s.projectID {
		return validation.NewErrBadRecordFieldValue("id",
			fmt.Sprintf("project id %q does not match this store's project id %q", p.ID, s.projectID))
	}
	if unsupported := firstUnsupportedProjectData(p); unsupported != "" {
		return notImplemented("SaveProject: project carries " + unsupported + " data, which this store does not persist yet")
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("dalgostore: project validation failed: %w", err)
	}
	file := datatug.ProjectFile{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{ID: p.ID, Title: p.Title},
			Access:        p.Access,
		},
		Repository: p.Repository,
		Created:    p.Created,
	}
	if err := file.Validate(); err != nil {
		return fmt.Errorf("dalgostore: invalid project record: %w", err)
	}
	key := projectKey(s.projectID)
	return s.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(key, &file))
	})
}

// firstUnsupportedProjectData returns the name of the first project
// collection SaveProject does not yet persist that p carries data in, or ""
// if p holds only its own record data. It is checked before writing so a
// project SaveProject cannot fully persist is rejected outright rather than
// having that data silently dropped.
func firstUnsupportedProjectData(p *datatug.Project) string {
	switch {
	case p.Queries != nil && (len(p.Queries.Folders) > 0 || len(p.Queries.Items) > 0):
		return "queries"
	case len(p.Boards) > 0:
		return "boards"
	case len(p.Entities) > 0:
		return "entities"
	case len(p.Environments) > 0:
		return "environments"
	case len(p.DbModels) > 0:
		return "db models"
	case len(p.DbDrivers) > 0:
		return "db drivers"
	default:
		return ""
	}
}
