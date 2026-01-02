package datatug

import (
	"context"
	"fmt"
)

type Step struct {
	Name   string
	Status string
}

type StatusReporter func(step string, status string)

type ProjectVisibility int

const (
	PublicProject ProjectVisibility = iota
	PrivateProject
)

func (v ProjectVisibility) Validate() error {
	switch v {
	case PublicProject, PrivateProject:
		return nil
	default:
		return fmt.Errorf("invalid project visibility: %v", v)
	}
}

type ProjectsStore interface {
	CreateNewProject(ctx context.Context, id, title string, visibility ProjectVisibility, report StatusReporter) (project *Project, err error)
}

type ProjectStore interface {
	ProjectID() string
	LoadProjectSummary(ctx context.Context) (ProjectSummary, error)
	LoadProject(ctx context.Context, o ...StoreOption) (p *Project, err error)
	SaveProject(ctx context.Context, p *Project) error

	QueriesStore
	BoardsStore
	FoldersStore
	EntitiesStore
	EnvironmentsStore
	EnvDbServersStore
	EnvDbCatalogStore
	ProjDbDriversStore
	RecordsetDefinitionsStore
}

type EnvironmentsStore interface {
	LoadEnvironments(ctx context.Context, o ...StoreOption) (Environments, error)
	LoadEnvironment(ctx context.Context, id string, o ...StoreOption) (*Environment, error)
	LoadEnvironmentSummary(ctx context.Context, id string) (*EnvironmentSummary, error)
	SaveEnvironment(ctx context.Context, env *Environment) error
	SaveEnvironments(ctx context.Context, envs Environments) error
	DeleteEnvironment(ctx context.Context, id string) error
}

type EnvDbServersStore interface {
	LoadEnvDbServers(ctx context.Context, envID string, o ...StoreOption) (EnvDbServers, error)
	LoadEnvDbServer(ctx context.Context, envID, serverID string, o ...StoreOption) (*EnvDbServer, error)
	SaveEnvDbServer(ctx context.Context, envID string, server *EnvDbServer) error
	SaveEnvServers(ctx context.Context, envID string, servers EnvDbServers) error
	DeleteEnvDbServer(ctx context.Context, envID, serverID string) error
}

type EnvDbCatalogStore interface {
	LoadEnvDbCatalogs(ctx context.Context, envID string, o ...StoreOption) (DbCatalogs, error)
	LoadEnvDbCatalog(ctx context.Context, envID, serverID, catalogID string, o ...StoreOption) (DbCatalog, error)
	SaveEnvDbCatalog(ctx context.Context, envID, serverID, catalogID string, catalogs *DbCatalog) error
	SaveEnvDbCatalogs(ctx context.Context, envID, serverID, catalogID string, catalogs DbCatalogs) error
	DeleteEnvDbCatalog(ctx context.Context, envID, serverID, catalogID string) error
}

type DbModelsStore interface {
	LoadDbModels(ctx context.Context, o ...StoreOption) (DbModels, error)
	LoadDbModel(ctx context.Context, id string, o ...StoreOption) (*DbModel, error)
	SaveDbModel(ctx context.Context, dbModel *DbModel) error
	SaveDbModels(ctx context.Context, dbModels DbModels) error
	DeleteDbModel(ctx context.Context, id string) error
}

type BoardsStore interface {
	LoadBoards(ctx context.Context, o ...StoreOption) (Boards, error)
	LoadBoard(ctx context.Context, id string, o ...StoreOption) (*Board, error)
	SaveBoard(ctx context.Context, board *Board) error
	DeleteBoard(ctx context.Context, id string) error
}

type DbSchemasStore interface {
	LoadDbSchemas(ctx context.Context, o ...StoreOption) (DbSchemas, error)
	LoadDbSchema(ctx context.Context, id string, o ...StoreOption) (*DbSchema, error)
	SaveDbSchema(ctx context.Context, board *DbSchema) error
	DeleteDbSchema(ctx context.Context, id string) error
}

type ProjDbDriversStore interface {
	LoadProjDbDrivers(ctx context.Context, o ...StoreOption) (ProjDbDrivers, error)
	LoadProjDbDriver(ctx context.Context, id string, o ...StoreOption) (*ProjDbDriver, error)
	SaveProjDbDriver(ctx context.Context, dbDriver *ProjDbDriver, o ...StoreOption) error
	DeleteProjDbDriver(ctx context.Context, id string) error
	DbServersStore(dbDriver string) ProjDbServersStore
}

type ProjDbServersStore interface {
	DriverID() string
	CatalogsStore(serverRef ServerRef) DbCatalogsStore
	LoadProjDbServers(ctx context.Context, o ...StoreOption) (ProjDbServers, error)
	LoadProjDbServer(ctx context.Context, serverID string, o ...StoreOption) (*ProjDbServer, error)
	SaveProjDbServer(ctx context.Context, server *ProjDbServer, o ...StoreOption) error
	DeleteProjDbServer(ctx context.Context, serverID string) error
}

type DbCatalogsStore interface {
	Server() ServerRef
	LoadDbCatalogs(ctx context.Context, o ...StoreOption) (DbCatalogs, error)
	SaveDbCatalog(ctx context.Context, dbCatalog *DbCatalog) error
	DeleteDbCatalog(ctx context.Context, id string) error
}

type EntitiesStore interface {
	LoadEntities(ctx context.Context, o ...StoreOption) (Entities, error)
	LoadEntity(ctx context.Context, id string, o ...StoreOption) (*Entity, error)
	SaveEntity(ctx context.Context, entity *Entity) error
	DeleteEntity(ctx context.Context, id string) error
}

type QueriesStore interface {
	LoadQueries(ctx context.Context, folderPath string, o ...StoreOption) (folder *QueriesFolder, err error)
	LoadQuery(ctx context.Context, id string, o ...StoreOption) (*QueryDef, error)
	SaveQuery(ctx context.Context, query *QueryDefWithFolderPath) error
	DeleteQuery(ctx context.Context, id string) error
}

type FoldersStore interface {
	LoadFolders(ctx context.Context, o ...StoreOption) (Folders, error)
	LoadFolder(ctx context.Context, id string, o ...StoreOption) (*Folder, error)
	SaveFolder(ctx context.Context, path string, folder *Folder) error
	SaveFolders(ctx context.Context, path string, folders Folders) error
	DeleteFolder(ctx context.Context, id string) error
}

type RecordsetDefinitionsStore interface {
	LoadRecordsetDefinitions(ctx context.Context, o ...StoreOption) ([]*RecordsetDefinition, error)
	LoadRecordsetDefinition(ctx context.Context, id string, o ...StoreOption) (*RecordsetDefinition, error)
	LoadRecordsetData(ctx context.Context, id string) (Recordset, error)
}
