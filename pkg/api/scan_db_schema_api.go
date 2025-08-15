package api

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug-core/pkg/dbconnection"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/parallel"
	"github.com/datatug/datatug-core/pkg/schemer"
	"github.com/datatug/datatug-core/pkg/schemer/mssql"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/strongo/random"
	"github.com/strongo/slice"
	"github.com/strongo/validation"
	"log"
	"strings"
	"time"
)

// ProjectLoader defines an interface to load project info
type ProjectLoader interface {
	// Loads project summary
	LoadProjectSummary(ctx context.Context) (projectSummary models.ProjectSummary, err error)
	// Loads the whole project
	LoadProject(ctx context.Context) (project *models.DatatugProject, err error)
}

var _ ProjectLoader = (storage.ProjectStore)(nil)

// UpdateDbSchema updates DB schema
func UpdateDbSchema(ctx context.Context, projectLoader ProjectLoader, projectID, environment, driver, dbModelID string, dbConnParams dbconnection.Params) (project *models.DatatugProject, err error) {
	log.Printf("Updating DB info for project=%v, env=%v, driver=%v, dbModelID=%v, dbCatalog=%v, connStr=%v",
		projectID, environment, driver, dbModelID, dbConnParams.Catalog(), dbConnParams.String())

	if dbConnParams.Catalog() == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("dbConnParams.Catalog")
	}

	if projectID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	if environment == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("environment")
	}
	if driver == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("driver")
	}
	if dbModelID == "" {
		return nil, validation.NewErrRequestIsMissingRequiredField("dbModelId")
	}
	var (
		dbCatalog *models.DbCatalog
	)
	var projSummaryErr error
	getProjectSummaryWorker := func() error {
		_, projSummaryErr = projectLoader.LoadProjectSummary(ctx)
		if err != nil {
			if models.ProjectDoesNotExist(projSummaryErr) {
				return nil
			}
			return fmt.Errorf("failed to load project summary: %w", err)
		}
		return nil
	}
	dbServer := models.ServerReference{
		Driver: driver,
		Host:   dbConnParams.Server(),
		Port:   dbConnParams.Port(),
	}
	scanDbWorker := func() error {
		var scanErr error
		if dbCatalog, scanErr = scanDbCatalog(dbServer, dbConnParams); err != nil {
			return scanErr
		}
		return scanErr
	}
	if err = parallel.Run(
		getProjectSummaryWorker,
		scanDbWorker,
	); err != nil {
		return project, err
	}

	if dbCatalog.Path == "" {
		if connWithPath, ok := dbConnParams.(interface{ Path() string }); ok {
			dbCatalog.Path = connWithPath.Path()
		}
	}

	if dbCatalog.DbModel != "" {
		dbCatalog.DbModel = dbModelID
	} else if dbCatalog.DbModel == "" {
		dbCatalog.DbModel = dbCatalog.ID
	}
	if models.ProjectDoesNotExist(projSummaryErr) {
		log.Println("Creating a new DataTug project...")
		if project, err = newProjectWithDatabase(environment, dbServer, dbCatalog); err != nil {
			return project, err
		}
	} else {
		log.Printf("Loading existing project...")
		if project, err = projectLoader.LoadProject(ctx); err != nil {
			err = fmt.Errorf("failed to load DataTug project: %w", err)
			return
		}
		log.Println("Updating project with latest database info...", environment)
		if err = updateProjectWithDbCatalog(project, environment, dbServer, dbCatalog); err != nil {
			return project, fmt.Errorf("failed in updateProjectWithDbCatalog(): %w", err)
		}
	}
	dbModel := project.DbModels.GetDbModelByID(dbCatalog.DbModel)
	if dbModel == nil {
		err = fmt.Errorf("db model not found by ID: %v. there is %v db models in the project: %v", dbCatalog.DbModel, len(project.DbModels), strings.Join(project.DbModels.IDs(), ", "))
		return
	}
	if err = updateDbModelWithDbCatalog(environment, dbModel, dbCatalog); err != nil {
		err = fmt.Errorf("failed to update dbModel with database: %w", err)
		return
	}
	return project, err
}

func updateProjectWithDbCatalog(project *models.DatatugProject, envID string, dbServerRef models.ServerReference, dbCatalog *models.DbCatalog) (err error) {
	if envID == "" {
		return validation.NewErrRequestIsMissingRequiredField("envID")
	}
	if dbServerRef.Host == "" {
		return validation.NewErrRequestIsMissingRequiredField("dbServerRef.Host")
	}
	if dbCatalog == nil {
		return validation.NewErrRequestIsMissingRequiredField("dbCatalog")
	}
	if dbCatalog.ID == "" {
		return validation.NewErrRequestIsMissingRequiredField("dbCatalog.ID")
	}
	if dbCatalog.Driver == "" {
		dbCatalog.Driver = dbServerRef.Driver
	} else if dbCatalog.Driver != dbServerRef.Driver {
		return validation.NewErrBadRecordFieldValue("driver", "dbCatalog.Driver != dbServerRef.Driver")
	}
	if err = dbServerRef.Validate(); err != nil {
		return fmt.Errorf("db server ref is invalid: %w", err)
	}
	if err = dbCatalog.Validate(); err != nil {
		return fmt.Errorf("new db catalog is invalid: %w", err)
	}
	// Update environment
	{
		if environment := project.Environments.GetEnvByID(envID); environment == nil {
			environment = &models.Environment{
				ProjectItem: models.ProjectItem{
					ProjItemBrief: models.ProjItemBrief{ID: envID},
				},
				DbServers: models.EnvDbServers{
					{
						ServerReference: dbServerRef,
						Catalogs:        []string{dbCatalog.ID},
					},
				},
			}
			project.Environments = append(project.Environments, environment)
		} else if envDbServer := environment.DbServers.GetByServerRef(dbServerRef); envDbServer == nil {
			environment.DbServers = append(environment.DbServers, &models.EnvDbServer{
				ServerReference: dbServerRef,
				Catalogs:        []string{dbCatalog.ID},
			})
		} else if i := slice.Index(envDbServer.Catalogs, dbCatalog.ID); i < 0 {
			envDbServer.Catalogs = append(envDbServer.Catalogs, dbCatalog.ID)
		}
	}
	// Update DB server
	{
		projDbServer := project.DbServers.GetProjDbServer(dbServerRef)
		if projDbServer == nil {
			projDbServer = &models.ProjDbServer{
				ProjectItem: models.ProjectItem{ProjItemBrief: models.ProjItemBrief{ID: dbServerRef.ID()}},
				Server:      dbServerRef,
				Catalogs:    models.DbCatalogs{dbCatalog},
			}
			project.DbServers = append(project.DbServers, projDbServer)
		}
		updated := false
		for j, db := range projDbServer.Catalogs {
			if db.ID == dbCatalog.ID {
				// restore path as path might be normalized if pointing to user's directory
				dbCatalog.Path = projDbServer.Catalogs[j].Path
				// TODO: currently replaces catalog with new info, should merge to preserver comments
				projDbServer.Catalogs[j] = dbCatalog
				updated = true
				break
			}
		}
		if !updated {
			projDbServer.Catalogs = append(projDbServer.Catalogs, dbCatalog)
		}
	}
	return nil
}

func newProjectWithDatabase(environment string, dbServer models.ServerReference, dbCatalog *models.DbCatalog) (project *models.DatatugProject, err error) {
	//var currentUser *user.User
	//if currentUser, err = user.Current(); err != nil {
	//	err = fmt.Errorf("failed to get current OS user")
	//	return
	//}
	project = &models.DatatugProject{
		ProjectItem: models.ProjectItem{
			ProjItemBrief: models.ProjItemBrief{ID: random.ID(9)},
			Access:        "private",
		},
		Created: &models.ProjectCreated{
			//ByUsername: currentUser.Username,
			//ByName:     currentUser.Name,
			At: time.Now(),
		},
		DbModels: models.DbModels{
			&models.DbModel{
				ProjectItem: models.ProjectItem{ProjItemBrief: models.ProjItemBrief{ID: dbCatalog.DbModel}},
			},
		},
		Environments: models.Environments{
			{
				ProjectItem: models.ProjectItem{ProjItemBrief: models.ProjItemBrief{ID: environment}},
				DbServers: []*models.EnvDbServer{
					{
						ServerReference: dbServer,
						Catalogs:        []string{dbCatalog.ID},
					},
				},
			},
		},
		DbServers: models.ProjDbServers{
			{
				ProjectItem: models.ProjectItem{ProjItemBrief: models.ProjItemBrief{ID: dbServer.ID()}},
				Server:      dbServer,
				Catalogs: models.DbCatalogs{
					dbCatalog,
				},
			},
		},
	}
	log.Println("project.ID:", project.ID)
	return project, err
}

func scanDbCatalog(server models.ServerReference, connectionParams dbconnection.Params) (dbCatalog *models.DbCatalog, err error) {
	var db *sql.DB

	if db, err = sql.Open(server.Driver, connectionParams.ConnectionString()); err != nil {
		return nil, fmt.Errorf("failed to open SQL db: %w", err)
	}

	// Close the database connection pool after command executes
	defer func() { _ = db.Close() }()

	//informationSchema := schemer.NewInformationSchema(server, db)

	var scanner schemer.Scanner
	switch server.Driver {
	case "sqlserver":
		scanner = schemer.NewScanner(mssql.NewSchemaProvider())
	// TODO: Disabled until figured out how to exclude it from Google Appengine Build
	//case "sqlite3":
	//	scanner = schemer.NewScanner(sqlite.NewSchemaProvider())
	default:
		return nil, fmt.Errorf("unsupported DB driver: %v", err)
	}

	dbCatalog, err = scanner.ScanCatalog(context.Background(), db, connectionParams.Catalog())
	dbCatalog.ID = connectionParams.Catalog()
	if err != nil {
		return dbCatalog, fmt.Errorf("failed to get dbCatalog metadata: %w", err)
	}
	//if database, err = informationSchema.GetDatabase(connectionParams.Database()); err != nil {
	//	return nil, fmt.Errorf("failed to get database metadata: %w", err)
	//}
	return
}

func updateDbModelWithDbCatalog(envID string, dbModel *models.DbModel, dbCatalog *models.DbCatalog) (err error) {
	{ // Update dbmodel environments
		environment := dbModel.Environments.GetByID(envID)
		if environment == nil {
			environment = &models.DbModelEnv{ID: envID}
			dbModel.Environments = append(dbModel.Environments, environment)
		}
		dbModelDb := environment.DbCatalogs.GetByID(dbCatalog.ID)
		if dbModelDb == nil {
			environment.DbCatalogs = append(environment.DbCatalogs, &models.DbModelDbCatalog{
				ID: dbCatalog.ID,
			})
		}
	}

	for _, schema := range dbCatalog.Schemas {
		var schemaModel *models.SchemaModel
		for _, sm := range dbModel.Schemas {
			if sm.ID == schema.ID {
				schemaModel = sm
				goto UpdateSchemaModel
			}
		}
		schemaModel = &models.SchemaModel{
			ProjectItem: schema.ProjectItem,
		}
		dbModel.Schemas = append(dbModel.Schemas, schemaModel)
	UpdateSchemaModel:
		if err = updateSchemaModel(envID, schemaModel, schema); err != nil {
			return fmt.Errorf("faild to update DB schema model: %w", err)
		}
	}
	return nil
}

func updateSchemaModel(envID string, schemaModel *models.SchemaModel, dbSchema *models.DbSchema) (err error) {
	updateTables := func(tables []*models.Table) (result models.TableModels) {
		for _, table := range tables {
			tableModel := schemaModel.Tables.GetByKey(table.TableKey)
			if tableModel == nil {
				tableModel = &models.TableModel{
					TableKey: table.TableKey,
					ByEnv:    make(models.StateByEnv),
				}
				tableModel.ByEnv[envID] = &models.EnvState{
					Status: "exists",
				}
				tableModel.Columns = make(models.ColumnModels, len(table.Columns))
				for i, c := range table.Columns {
					tableModel.Columns[i] = &models.ColumnModel{
						TableColumn: *c,
						ByEnv:       make(models.StateByEnv),
					}
					tableModel.Columns[i].ByEnv[envID] = &models.EnvState{
						Status: "exists",
					}
				}
				result = append(result, tableModel)
			} else {
				panic(errNotImplementedYet)
			}
		}
		return
	}
	schemaModel.Tables = updateTables(dbSchema.Tables)
	schemaModel.Views = updateTables(dbSchema.Views)
	return nil
}
