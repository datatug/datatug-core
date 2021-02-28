package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"github.com/datatug/datatug/packages/schemer"
	"github.com/datatug/datatug/packages/schemer/mssql"
	"github.com/datatug/datatug/packages/schemer/sqlite"
	"github.com/datatug/datatug/packages/slice"
	"github.com/strongo/random"
	"log"
	"strings"
	"time"
)

// ProjectLoader defines an interface to load project info
type ProjectLoader interface {
	// Loads project summary
	LoadProjectSummary(id string) (projectSummary models.ProjectSummary, err error)
	// Loads the whole project
	LoadProject(id string) (project *models.DataTugProject, err error)
}

// UpdateDbSchema updates DB schema
func UpdateDbSchema(ctx context.Context, loader ProjectLoader, projectID, environment, driver, dbModelID string, connectionString execute.ConnectionString) (project *models.DataTugProject, err error) {
	log.Printf("Updating DB info for project=%v, env=%v, driver=%v, dbModelId=%v, connStr=%v", projectID, environment, driver, dbModelID, connectionString.String())
	if projectID == "" {
		return nil, errors.New("argument projectID must be provided")
	}
	if environment == "" {
		return nil, errors.New("argument environment must be provided")
	}
	if driver == "" {
		return nil, errors.New("argument driver must be provided")
	}
	if dbModelID == "" {
		return nil, errors.New("argument dbModelID must be provided")
	}
	var (
		latestDb *models.DbCatalog
	)
	var projSummaryErr error
	getProjectSummaryWorker := func() error {
		_, projSummaryErr = loader.LoadProjectSummary(projectID)
		if err != nil {
			if models.ProjectDoesNotExist(projSummaryErr) {
				return nil
			}
			return fmt.Errorf("failed to load project sumary: %w", err)
		}
		return nil
	}
	dbServer := models.ServerReference{
		Driver: driver,
		Host:   connectionString.Server(),
		Port:   connectionString.Port(),
	}
	scanDbWorker := func() error {
		var scanErr error
		if latestDb, scanErr = scanDbSchema(dbServer, connectionString); err != nil {
			return scanErr
		}
		return scanErr
	}
	if err = parallel.Run(getProjectSummaryWorker, scanDbWorker); err != nil {
		return project, err
	}
	if dbModelID != "" {
		latestDb.DbModel = dbModelID
	} else if latestDb.DbModel == "" {
		latestDb.DbModel = latestDb.ID
	}
	if models.ProjectDoesNotExist(projSummaryErr) {
		log.Println("Creating a new DataTug project...")
		if project, err = newProjectWithDatabase(environment, dbServer, latestDb); err != nil {
			return project, err
		}
	} else {
		log.Printf("Loading existing project...")
		if project, err = loader.LoadProject(projectID); err != nil {
			err = fmt.Errorf("failed to load DataTug project: %w", err)
			return
		}
		log.Println("Updating project with latest database info...", environment)
		if err = updateProjectWithDbCatalog(project, environment, dbServer, latestDb); err != nil {
			return project, err
		}
	}
	dbModel := project.DbModels.GetDbModelByID(latestDb.DbModel)
	if dbModel == nil {
		err = fmt.Errorf("db model not found by ID: %v. there is %v db models in the project: %v", latestDb.DbModel, len(project.DbModels), strings.Join(project.DbModels.IDs(), ", "))
		return
	}
	if err = updateDbModelWithDatabase(environment, dbModel, latestDb); err != nil {
		err = fmt.Errorf("failed to update dbModel with database: %w", err)
		return
	}
	return project, err
}

func updateProjectWithDbCatalog(project *models.DataTugProject, envID string, dbServer models.ServerReference, latestDbCatalog *models.DbCatalog) (err error) {
	// Update environment
	{
		if environment := project.Environments.GetEnvByID(envID); environment == nil {
			environment = &models.Environment{
				ProjectItem: models.ProjectItem{
					ID: envID,
				},
				DbServers: models.EnvDbServers{
					{
						ServerReference: dbServer,
						Databases:       []string{latestDbCatalog.ID},
					},
				},
			}
			project.Environments = append(project.Environments, environment)
		} else if envDbServer := environment.DbServers.GetByID(dbServer.ID()); envDbServer == nil {
			environment.DbServers = append(environment.DbServers, &models.EnvDbServer{
				ServerReference: dbServer,
				Databases:       []string{latestDbCatalog.ID},
			})
		} else if i := slice.IndexOfString(envDbServer.Databases, latestDbCatalog.ID); i < 0 {
			envDbServer.Databases = append(envDbServer.Databases, latestDbCatalog.ID)
		}
	}
	// Update DB server
	{
		for i, projDbServer := range project.DbServers {
			if projDbServer.ProjectItem.ID == dbServer.ID() {
				for j, db := range project.DbServers[i].Catalogs {
					if db.ID == latestDbCatalog.ID {
						project.DbServers[i].Catalogs[j] = latestDbCatalog
						goto ProjectDbServerDatabaseUpdated
					}
				}
				project.DbServers[i].Catalogs = append(project.DbServers[i].Catalogs, latestDbCatalog)
			ProjectDbServerDatabaseUpdated:
				goto ProjDbServerUpdate
			}
		}
		project.DbServers = append(project.DbServers, &models.ProjDbServer{
			ProjectItem: models.ProjectItem{ID: dbServer.ID()},
			Server:      dbServer,
			Catalogs:    models.DbCatalogs{latestDbCatalog},
		})
	ProjDbServerUpdate:
	}
	return nil
}

func newProjectWithDatabase(environment string, dbServer models.ServerReference, database *models.DbCatalog) (project *models.DataTugProject, err error) {
	//var currentUser *user.User
	//if currentUser, err = user.Current(); err != nil {
	//	err = fmt.Errorf("failed to get current OS user")
	//	return
	//}
	project = &models.DataTugProject{
		Access: "private",
		Created: &models.ProjectCreated{
			//ByUsername: currentUser.Username,
			//ByName:     currentUser.Name,
			At: time.Now(),
		},
		DbModels: models.DbModels{
			&models.DbModel{
				ProjectItem: models.ProjectItem{ID: database.DbModel},
			},
		},
		Environments: models.Environments{
			{
				ProjectItem: models.ProjectItem{ID: environment},
				DbServers: []*models.EnvDbServer{
					{
						ServerReference: dbServer,
						Databases:       []string{database.ID},
					},
				},
			},
		},
		DbServers: models.ProjDbServers{
			{
				ProjectItem: models.ProjectItem{ID: dbServer.ID()},
				Server:      dbServer,
				Catalogs: models.DbCatalogs{
					database,
				},
			},
		},
	}
	project.ID = random.ID(9)
	log.Println("project.ID:", project.ID)
	return project, err
}

func scanDbSchema(server models.ServerReference, connectionString execute.ConnectionString) (database *models.DbCatalog, err error) {
	var db *sql.DB

	if db, err = sql.Open(server.Driver, connectionString.String()); err != nil {
		return nil, fmt.Errorf("failed to open SQL db: %w", err)
	}

	// Close the database connection pool after command executes
	defer func() { _ = db.Close() }()

	//informationSchema := schemer.NewInformationSchema(server, db)

	var scanner schemer.Scanner
	switch server.Driver {
	case "sqlserver":
		scanner = schemer.NewScanner(mssql.NewSchemaProvider())
	case "sqlite3":
		scanner = schemer.NewScanner(sqlite.NewSchemaProvider())
	default:
		return nil, fmt.Errorf("unsupported DB driver: %v", err)
	}

	database, err = scanner.ScanCatalog(context.Background(), db, connectionString.Database())
	if err != nil {
		return database, fmt.Errorf("failed to get database metadata: %w", err)
	}
	//if database, err = informationSchema.GetDatabase(connectionString.Database()); err != nil {
	//	return nil, fmt.Errorf("failed to get database metadata: %w", err)
	//}
	return
}

func updateDbModelWithDatabase(envID string, dbModel *models.DbModel, database *models.DbCatalog) (err error) {
	{ // Update dbmodel environments
		environment := dbModel.Environments.GetByID(envID)
		if environment == nil {
			environment = &models.DbModelEnv{ID: envID}
			dbModel.Environments = append(dbModel.Environments, environment)
		}
		dbModelDb := environment.Databases.GetByID(database.ID)
		if dbModelDb == nil {
			environment.Databases = append(environment.Databases, &models.DbModelDb{
				ID: database.ID,
			})
		}
	}

	for _, schema := range database.Schemas {
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
