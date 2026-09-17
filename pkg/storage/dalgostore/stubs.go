package dalgostore

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// This file holds every datatug.ProjectStore member this package does not
// implement yet. Each stub returns ErrNotImplemented (see notImplemented in
// project_store.go) so callers get a typed, self-explanatory error rather
// than a silent no-op or a panic. Later tasks of the dalgo-project-store
// plan replace these one collection at a time.

// LoadQueries is not implemented; see the package doc comment.
func (s *ProjectStore) LoadQueries(context.Context, string, ...datatug.StoreOption) (*datatug.QueriesFolder, error) {
	return nil, notImplemented("LoadQueries")
}

// LoadQuery is not implemented; see the package doc comment.
func (s *ProjectStore) LoadQuery(context.Context, string, ...datatug.StoreOption) (*datatug.QueryDef, error) {
	return nil, notImplemented("LoadQuery")
}

// SaveQuery is not implemented; see the package doc comment.
func (s *ProjectStore) SaveQuery(context.Context, *datatug.QueryDefWithFolderPath) error {
	return notImplemented("SaveQuery")
}

// DeleteQuery is not implemented; see the package doc comment.
func (s *ProjectStore) DeleteQuery(context.Context, string) error {
	return notImplemented("DeleteQuery")
}

// LoadBoards is not implemented; see the package doc comment.
func (s *ProjectStore) LoadBoards(context.Context, ...datatug.StoreOption) (datatug.Boards, error) {
	return nil, notImplemented("LoadBoards")
}

// LoadBoard is not implemented; see the package doc comment.
func (s *ProjectStore) LoadBoard(context.Context, string, ...datatug.StoreOption) (*datatug.Board, error) {
	return nil, notImplemented("LoadBoard")
}

// SaveBoard is not implemented; see the package doc comment.
func (s *ProjectStore) SaveBoard(context.Context, *datatug.Board) error {
	return notImplemented("SaveBoard")
}

// DeleteBoard is not implemented; see the package doc comment.
func (s *ProjectStore) DeleteBoard(context.Context, string) error {
	return notImplemented("DeleteBoard")
}

// LoadFolders is not implemented; see the package doc comment.
func (s *ProjectStore) LoadFolders(context.Context, ...datatug.StoreOption) (datatug.Folders, error) {
	return nil, notImplemented("LoadFolders")
}

// LoadFolder is not implemented; see the package doc comment.
func (s *ProjectStore) LoadFolder(context.Context, string, ...datatug.StoreOption) (*datatug.Folder, error) {
	return nil, notImplemented("LoadFolder")
}

// SaveFolder is not implemented; see the package doc comment.
func (s *ProjectStore) SaveFolder(context.Context, string, *datatug.Folder) error {
	return notImplemented("SaveFolder")
}

// SaveFolders is not implemented; see the package doc comment.
func (s *ProjectStore) SaveFolders(context.Context, string, datatug.Folders) error {
	return notImplemented("SaveFolders")
}

// DeleteFolder is not implemented; see the package doc comment.
func (s *ProjectStore) DeleteFolder(context.Context, string) error {
	return notImplemented("DeleteFolder")
}

// LoadEntities is not implemented; see the package doc comment.
func (s *ProjectStore) LoadEntities(context.Context, ...datatug.StoreOption) (datatug.Entities, error) {
	return nil, notImplemented("LoadEntities")
}

// LoadEntity is not implemented; see the package doc comment.
func (s *ProjectStore) LoadEntity(context.Context, string, ...datatug.StoreOption) (*datatug.Entity, error) {
	return nil, notImplemented("LoadEntity")
}

// SaveEntity is not implemented; see the package doc comment.
func (s *ProjectStore) SaveEntity(context.Context, *datatug.Entity) error {
	return notImplemented("SaveEntity")
}

// DeleteEntity is not implemented; see the package doc comment.
func (s *ProjectStore) DeleteEntity(context.Context, string) error {
	return notImplemented("DeleteEntity")
}

// LoadEnvironments is not implemented; see the package doc comment.
func (s *ProjectStore) LoadEnvironments(context.Context, ...datatug.StoreOption) (datatug.Environments, error) {
	return nil, notImplemented("LoadEnvironments")
}

// LoadEnvironment is not implemented; see the package doc comment.
func (s *ProjectStore) LoadEnvironment(context.Context, string, ...datatug.StoreOption) (*datatug.Environment, error) {
	return nil, notImplemented("LoadEnvironment")
}

// LoadEnvironmentSummary is not implemented; see the package doc comment.
func (s *ProjectStore) LoadEnvironmentSummary(context.Context, string) (*datatug.EnvironmentSummary, error) {
	return nil, notImplemented("LoadEnvironmentSummary")
}

// SaveEnvironment is not implemented; see the package doc comment.
func (s *ProjectStore) SaveEnvironment(context.Context, *datatug.Environment) error {
	return notImplemented("SaveEnvironment")
}

// SaveEnvironments is not implemented; see the package doc comment.
func (s *ProjectStore) SaveEnvironments(context.Context, datatug.Environments) error {
	return notImplemented("SaveEnvironments")
}

// DeleteEnvironment is not implemented; see the package doc comment.
func (s *ProjectStore) DeleteEnvironment(context.Context, string) error {
	return notImplemented("DeleteEnvironment")
}

// LoadEnvDbServers is not implemented; see the package doc comment.
func (s *ProjectStore) LoadEnvDbServers(context.Context, string, ...datatug.StoreOption) (datatug.EnvDbServers, error) {
	return nil, notImplemented("LoadEnvDbServers")
}

// LoadEnvDbServer is not implemented; see the package doc comment.
func (s *ProjectStore) LoadEnvDbServer(context.Context, string, string, ...datatug.StoreOption) (*datatug.EnvDbServer, error) {
	return nil, notImplemented("LoadEnvDbServer")
}

// SaveEnvDbServer is not implemented; see the package doc comment.
func (s *ProjectStore) SaveEnvDbServer(context.Context, string, *datatug.EnvDbServer) error {
	return notImplemented("SaveEnvDbServer")
}

// SaveEnvServers is not implemented; see the package doc comment.
func (s *ProjectStore) SaveEnvServers(context.Context, string, datatug.EnvDbServers) error {
	return notImplemented("SaveEnvServers")
}

// DeleteEnvDbServer is not implemented; see the package doc comment.
func (s *ProjectStore) DeleteEnvDbServer(context.Context, string, string) error {
	return notImplemented("DeleteEnvDbServer")
}

// LoadEnvDbCatalogs is not implemented; see the package doc comment.
func (s *ProjectStore) LoadEnvDbCatalogs(context.Context, string, ...datatug.StoreOption) (datatug.DbCatalogs, error) {
	return nil, notImplemented("LoadEnvDbCatalogs")
}

// LoadEnvDbCatalog is not implemented; see the package doc comment.
func (s *ProjectStore) LoadEnvDbCatalog(context.Context, string, string, string, ...datatug.StoreOption) (datatug.DbCatalog, error) {
	return datatug.DbCatalog{}, notImplemented("LoadEnvDbCatalog")
}

// SaveEnvDbCatalog is not implemented; see the package doc comment.
func (s *ProjectStore) SaveEnvDbCatalog(context.Context, string, string, string, *datatug.DbCatalog) error {
	return notImplemented("SaveEnvDbCatalog")
}

// SaveEnvDbCatalogs is not implemented; see the package doc comment.
func (s *ProjectStore) SaveEnvDbCatalogs(context.Context, string, string, string, datatug.DbCatalogs) error {
	return notImplemented("SaveEnvDbCatalogs")
}

// DeleteEnvDbCatalog is not implemented; see the package doc comment.
func (s *ProjectStore) DeleteEnvDbCatalog(context.Context, string, string, string) error {
	return notImplemented("DeleteEnvDbCatalog")
}

// LoadProjDbDrivers is not implemented; see the package doc comment.
func (s *ProjectStore) LoadProjDbDrivers(context.Context, ...datatug.StoreOption) (datatug.ProjDbDrivers, error) {
	return nil, notImplemented("LoadProjDbDrivers")
}

// LoadProjDbDriver is not implemented; see the package doc comment.
func (s *ProjectStore) LoadProjDbDriver(context.Context, string, ...datatug.StoreOption) (*datatug.ProjDbDriver, error) {
	return nil, notImplemented("LoadProjDbDriver")
}

// SaveProjDbDriver is not implemented; see the package doc comment.
func (s *ProjectStore) SaveProjDbDriver(context.Context, *datatug.ProjDbDriver, ...datatug.StoreOption) error {
	return notImplemented("SaveProjDbDriver")
}

// DeleteProjDbDriver is not implemented; see the package doc comment.
func (s *ProjectStore) DeleteProjDbDriver(context.Context, string) error {
	return notImplemented("DeleteProjDbDriver")
}

// DbServersStore returns a stub ProjDbServersStore for dbDriver; every
// member of it is not implemented (see the package doc comment).
func (s *ProjectStore) DbServersStore(dbDriver string) datatug.ProjDbServersStore {
	return notImplementedDbServersStore{driverID: dbDriver}
}

// notImplementedDbServersStore stubs datatug.ProjDbServersStore. See the
// package doc comment.
type notImplementedDbServersStore struct {
	driverID string
}

var _ datatug.ProjDbServersStore = notImplementedDbServersStore{}

func (s notImplementedDbServersStore) DriverID() string {
	return s.driverID
}

func (s notImplementedDbServersStore) CatalogsStore(serverRef datatug.ServerRef) datatug.DbCatalogsStore {
	return notImplementedDbCatalogsStore{server: serverRef}
}

func (s notImplementedDbServersStore) LoadProjDbServers(context.Context, ...datatug.StoreOption) (datatug.ProjDbServers, error) {
	return nil, notImplemented("ProjDbServersStore.LoadProjDbServers")
}

func (s notImplementedDbServersStore) LoadProjDbServer(context.Context, string, ...datatug.StoreOption) (*datatug.ProjDbServer, error) {
	return nil, notImplemented("ProjDbServersStore.LoadProjDbServer")
}

func (s notImplementedDbServersStore) SaveProjDbServer(context.Context, *datatug.ProjDbServer, ...datatug.StoreOption) error {
	return notImplemented("ProjDbServersStore.SaveProjDbServer")
}

func (s notImplementedDbServersStore) DeleteProjDbServer(context.Context, string) error {
	return notImplemented("ProjDbServersStore.DeleteProjDbServer")
}

// notImplementedDbCatalogsStore stubs datatug.DbCatalogsStore. See the
// package doc comment.
type notImplementedDbCatalogsStore struct {
	server datatug.ServerRef
}

var _ datatug.DbCatalogsStore = notImplementedDbCatalogsStore{}

func (s notImplementedDbCatalogsStore) Server() datatug.ServerRef {
	return s.server
}

func (s notImplementedDbCatalogsStore) LoadDbCatalogs(context.Context, ...datatug.StoreOption) (datatug.DbCatalogs, error) {
	return nil, notImplemented("DbCatalogsStore.LoadDbCatalogs")
}

func (s notImplementedDbCatalogsStore) SaveDbCatalog(context.Context, *datatug.DbCatalog) error {
	return notImplemented("DbCatalogsStore.SaveDbCatalog")
}

func (s notImplementedDbCatalogsStore) DeleteDbCatalog(context.Context, string) error {
	return notImplemented("DbCatalogsStore.DeleteDbCatalog")
}

// LoadRecordsetDefinitions is not implemented; see the package doc comment.
func (s *ProjectStore) LoadRecordsetDefinitions(context.Context, ...datatug.StoreOption) ([]*datatug.RecordsetDefinition, error) {
	return nil, notImplemented("LoadRecordsetDefinitions")
}

// LoadRecordsetDefinition is not implemented; see the package doc comment.
func (s *ProjectStore) LoadRecordsetDefinition(context.Context, string, ...datatug.StoreOption) (*datatug.RecordsetDefinition, error) {
	return nil, notImplemented("LoadRecordsetDefinition")
}

// LoadRecordsetData is not implemented; see the package doc comment.
func (s *ProjectStore) LoadRecordsetData(context.Context, string) (datatug.Recordset, error) {
	return datatug.Recordset{}, notImplemented("LoadRecordsetData")
}
