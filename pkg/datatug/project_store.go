package datatug

import "context"

type ProjectStore interface {
	ProjectID() string
	LoadProjectSummary(ctx context.Context) (ProjectSummary, error)
	LoadProject(ctx context.Context, o ...StoreOption) (p *Project, err error)
	SaveProject(ctx context.Context, p *Project) error

	//SaveProjectSummary(ctx context.Context, summary *ProjectSummary) error

	//LoadQueries(ctx context.Context, folder *QueryFolder, o ...StoreOption) error
	//SaveQuery(ctx context.Context, folder *QueryFolder, query *QueryDef) error
	//DeleteQuery(ctx context.Context, id string) error

	//LoadEntities(ctx context.Context, o ...StoreOption) (Entities, error)
	//SaveEntity(ctx context.Context, entity *Entity) error
	//DeleteEntity(ctx context.Context, id string) error

	queryStore
	BoardsStore
	foldersStore
	entitiesStore
	environmentsStore
	projDbServersStore
	recordsetDefinitionsStore
}

type environmentsStore interface {
	LoadEnvironments(ctx context.Context, o ...StoreOption) (Environments, error)
	LoadEnvironmentSummary(ctx context.Context, id string) (*EnvironmentSummary, error)
	//SaveEnvironment(ctx context.Context, env *Environment) error
	//DeleteEnvironment(ctx context.Context, id string) error
}

type BoardsStore interface {
	LoadBoards(ctx context.Context, o ...StoreOption) (Boards, error)
	LoadBoard(ctx context.Context, id string, o ...StoreOption) (*Board, error)
	SaveBoard(ctx context.Context, board *Board) error
	DeleteBoard(ctx context.Context, id string) error
}

type projDbServersStore interface {
	LoadProjDbServers(ctx context.Context, o ...StoreOption) (ProjDbServers, error)
	LoadProjDbServerSummary(ctx context.Context, id string) (*ProjDbServerSummary, error)
	SaveProjDbServer(ctx context.Context, server *ProjDbServer) error
	DeleteProjDbServer(ctx context.Context, id string) error
}

type entitiesStore interface {
	LoadEntities(ctx context.Context, o ...StoreOption) (Entities, error)
	LoadEntity(ctx context.Context, id string, o ...StoreOption) (*Entity, error)
	SaveEntity(ctx context.Context, entity *Entity) error
	DeleteEntity(ctx context.Context, id string) error
}

type queryStore interface {
	LoadQuery(ctx context.Context, id string) (*QueryDefWithFolderPath, error)
	SaveQuery(ctx context.Context, query *QueryDefWithFolderPath) error
	DeleteQuery(ctx context.Context, id string) error
}

type foldersStore interface {
	LoadFolders(ctx context.Context, o ...StoreOption) (*Folder, error)
	SaveFolder(ctx context.Context, path string, folder *Folder) error
	DeleteFolder(ctx context.Context, id string) error
}

type recordsetDefinitionsStore interface {
	LoadRecordsetDefinitions(ctx context.Context, o ...StoreOption) ([]*RecordsetDefinition, error)
	LoadRecordsetDefinition(ctx context.Context, id string) (*RecordsetDefinition, error)
	LoadRecordsetData(ctx context.Context, id string) (Recordset, error)
}
