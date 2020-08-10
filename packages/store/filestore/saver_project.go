package filestore

import (
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"github.com/datatug/datatug/packages/slice"
	"log"
	"os"
	"path"
	"strings"
	"sync"
)

// Save saves project
func (s FileSystemSaver) Save(project models.DataTugProject) (err error) {
	if err = project.Validate(); err != nil {
		return fmt.Errorf("project validation failed: %w", err)
	}
	if err = os.MkdirAll(path.Join(s.path, DatatugFolder), os.ModeDir); err != nil {
		return err
	}
	if err = s.saveProjectFile(project); err != nil {
		return err
	}
	if err = s.saveEntities(project.Entities); err != nil {
		return fmt.Errorf("failed to save environments: %w", err)
	}
	if err = s.saveEnvironments(project); err != nil {
		return fmt.Errorf("failed to save environments: %w", err)
	}
	if err = s.saveDbModels(project.DbModels); err != nil {
		return fmt.Errorf("failed to save DB models: %w", err)
	}
	if err = s.saveBoards(project.Boards); err != nil {
		return fmt.Errorf("failed to save boards: %w", err)
	}
	if err = s.saveDbServers(project.DbServers); err != nil {
		return fmt.Errorf("failed to save boards: %w", err)
	}
	return nil
}

// SaveBoard saves board
func (s FileSystemSaver) SaveBoard(board models.Board) (err error) {
	if err = s.updateProjectFileWithBoard(board); err != nil {
		return fmt.Errorf("failed to update project file with board: %w", err)
	}
	fileName := projItemFileName(board.ID, BoardPrefix)
	board.ID = ""
	if err = s.saveJSONFile(
		s.boardsDirPath(),
		fileName,
		board,
	); err != nil {
		return fmt.Errorf("failed to save board file: %w", err)
	}
	return err
}

func (s FileSystemSaver) putProjectFile(projFile models.ProjectFile) error {
	if err := projFile.Validate(); err != nil {
		return fmt.Errorf("invalid project file: %w", err)
	}
	return s.saveJSONFile(path.Join(s.path, DatatugFolder), ProjectSummaryFileName, projFile)
}

func (s FileSystemSaver) boardsDirPath() string {
	return path.Join(s.path, DatatugFolder, BoardsFolder)
}

func (s FileSystemSaver) entitiesDirPath() string {
	return path.Join(s.path, DatatugFolder, EntitiesFolder)
}

func projItemFileName(id, prefix string) string {
	id = strings.ToLower(id)
	if prefix == "" {
		return fmt.Sprintf("%v.json", id)
	}
	return fmt.Sprintf("%v-%v.json", prefix, id)
}

// DeleteBoard deletes board
func (s FileSystemSaver) DeleteBoard(boardID string) error {
	filePath := path.Join(s.boardsDirPath(), projItemFileName(boardID, BoardPrefix))
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Remove(filePath)
}

// DeleteEntity deletes entity
func (s FileSystemSaver) DeleteEntity(entityID string) error {
	deleteFile := func() (err error) {
		filePath := path.Join(s.entitiesDirPath(), projItemFileName(entityID, EntityPrefix))
		if _, err := os.Stat(filePath); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		return os.Remove(filePath)
	}
	deleteFromProjectSummary := func() error {
		projectSummary, err := s.loadProjectFile()
		if err != nil {
			return err
		}

		var entityIds []string
		if err := loadDir(s.entitiesDirPath(), processFiles, func(files []os.FileInfo) {
			entityIds = make([]string, 0, len(files))
		}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
			fileName := f.Name()
			if strings.HasSuffix(fileName, ".json") {
				entityIds = append(entityIds, strings.Replace(fileName, ".json", "", 1))
			}
			return nil
		}); err != nil {
			return fmt.Errorf("failed to load names of entity files: %w", err)
		}
		shift := 0
		for i, entity := range projectSummary.Entities {
			if entity.ID == entityID || slice.IndexOfString(entityIds, entity.ID) < 0 {
				shift++
				continue
			}
			projectSummary.Entities[i-shift] = entity
		}
		projectSummary.Entities = projectSummary.Entities[0 : len(projectSummary.Entities)-shift]
		if err := s.putProjectFile(projectSummary); err != nil {
			return fmt.Errorf("failed to save project file: %w", err)
		}
		return nil
	}
	if err := deleteFile(); err != nil {
		return fmt.Errorf("failed to delete entity file: %w", err)
	}
	if err := deleteFromProjectSummary(); err != nil {
		fmt.Printf("Failed to remove entity record from project summary: %v\n", err) // TODO: Log as an error
	}
	return nil
}

func (s FileSystemSaver) saveProjectFile(project models.DataTugProject) error {

	//var existingProject models.ProjectFile
	//if err := loadFile(path.Join(s.path, DatatugFolder, ProjectSummaryFileName), false, &existingProject); err != nil {
	//	return err
	//}
	projFile := models.ProjectFile{
		ProjectEntity: models.ProjectEntity{
			ID: project.ID,
		},
		Access: project.Access,
		//UUID:    project.UUID,
		Created: project.Created,
	}
	//if existingProject.UUID == uuid.Nil {
	//	projFile.UUID = project.UUID
	//} else {
	//	projFile.UUID = existingProject.UUID
	//}
	//if existingProject.Access == "" {
	//	projFile.Access = project.Access
	//} else {
	//	projFile.Access = existingProject.Access
	//}
	//if existingProject.ID == "" {
	//	projFile.ID = project.ID
	//} else {
	//	projFile.ID = existingProject.ID
	//}
	for _, env := range project.Environments {
		envBrief := models.ProjEnvBrief{
			ProjectEntity: env.ProjectEntity,
			NumberOf: models.ProjEnvNumbers{
				DbServers: len(env.DbServers),
			},
		}
		for _, dbServer := range env.DbServers {
			envBrief.NumberOf.Databases += len(dbServer.Databases)
		}
		projFile.Environments = append(projFile.Environments, &envBrief)
	}
	for _, board := range project.Boards {
		projFile.Boards = append(projFile.Boards,
			&models.ProjBoardBrief{
				ProjectEntity: board.ProjectEntity,
				Parameters:    board.Parameters,
			},
		)
	}
	for _, dbModel := range project.DbModels {
		brief := models.ProjDbModelBrief{
			ProjectEntity: dbModel.ProjectEntity,
			NumberOf: models.ProjDbModelNumbers{
				Schemas: len(dbModel.Schemas),
			},
		}
		for _, schema := range dbModel.Schemas {
			brief.NumberOf.Tables = len(schema.Tables)
			brief.NumberOf.Views = len(schema.Views)
		}
		projFile.DbModels = append(projFile.DbModels,
			&brief,
		)
	}
	if err := s.writeProjectReadme(project); err != nil {
		return fmt.Errorf("failed to write project README.md: %w", err)
	}
	if err := s.putProjectFile(projFile); err != nil {
		return fmt.Errorf("failed to save project file: %w", err)
	}
	return nil
}

func (s FileSystemSaver) saveEnvironments(project models.DataTugProject) (err error) {
	return s.saveItems("environments", len(project.Environments), func(i int) func() error {
		return func() error {
			return s.saveEnvironment(*project.Environments[i])
		}
	})
}

func (s FileSystemSaver) saveDbModels(dbModels models.DbModels) (err error) {
	return s.saveItems(DbModelsFolder, len(dbModels), func(i int) func() error {
		return func() error {
			return s.saveDbModel(dbModels[i])
		}
	})
}

func (s FileSystemSaver) saveDbModel(dbModel *models.DbModel) (err error) {
	if err = dbModel.Validate(); err != nil {
		return err
	}
	dirPath := path.Join(s.path, DatatugFolder, DbModelsFolder, dbModel.ID)
	if err = os.MkdirAll(dirPath, os.ModeDir); err != nil {
		return fmt.Errorf("failed to create db model folder: %w", err)
	}
	return parallel.Run(
		func() error {
			return s.saveJSONFile(dirPath, dbModel.ID+".dbmodel.json", DbModelFile{
				Environments: dbModel.Environments,
			})
		},
		func() error {
			return s.saveSchemaModels(dirPath, dbModel.Schemas)
		},
	)
}

func (s FileSystemSaver) saveBoards(boards models.Boards) (err error) {
	return s.saveItems(BoardsFolder, len(boards), func(i int) func() error {
		return func() error {
			return s.SaveBoard(*boards[i])
		}
	})
}

func (s FileSystemSaver) saveDbServers(dbServers models.ProjDbServers) (err error) {
	return s.saveItems("dbservers", len(dbServers), func(i int) func() error {
		return func() error {
			return s.SaveDbServer(dbServers[i])
		}
	})
}

func (s FileSystemSaver) saveEnvironment(env models.Environment) (err error) {
	log.Printf("Saving environment: %v", env.ID)
	dirPath := path.Join(s.path, DatatugFolder, EnvironmentsFolder, env.ID)
	if err = os.MkdirAll(dirPath, os.ModeDir); err != nil {
		return fmt.Errorf("failed to create environemtn folder: %w", err)
	}
	return parallel.Run(
		func() error {
			if err = s.saveJSONFile(dirPath, fmt.Sprintf("%v.environment.json", env.ID), models.EnvironmentFile{ID: env.ID}); err != nil {
				return fmt.Errorf("failed to write environment json to file: %w", err)
			}
			return nil
		},
		func() error {
			if err = s.saveEnvServers(env.ID, env.DbServers); err != nil {
				return fmt.Errorf("failed to save environment servers: %w", err)
			}
			return nil
		},
	)
}

func (s FileSystemSaver) saveEnvServers(env string, servers []*models.EnvDbServer) (err error) {
	dirPath := path.Join(s.path, DatatugFolder, EnvironmentsFolder, env, ServersFolder, DbFolder)
	if err = os.MkdirAll(dirPath, os.ModeDir); err != nil {
		return fmt.Errorf("failed to create environment servers folder: %w", err)
	}
	return s.saveItems("servers", len(servers), func(i int) func() error {
		return func() error {
			server := servers[i]
			if err = s.saveJSONFile(dirPath, fmt.Sprintf("%v.%v.server.json", server.Driver, server.FileName()), server); err != nil {
				return fmt.Errorf("failed to write server json to file: %w", err)
			}
			return nil
		}
	})
}

func (s FileSystemSaver) saveDatabases(dbServer models.DbServer, databases []*models.Database) (err error) {
	return s.saveItems("databases", len(databases), func(i int) func() error {
		return func() error {
			return s.saveDatabase(dbServer, databases[i])
		}
	})
}

func (s FileSystemSaver) saveDatabase(dbServer models.DbServer, database *models.Database) (err error) {
	if database == nil {
		return errors.New("database is nil")
	}
	serverName := dbServer.FileName()
	dbDirPath := path.Join(s.path, DatatugFolder, ServersFolder, DbFolder, dbServer.Driver, serverName, DatabasesFolder, database.ID)
	if err := os.MkdirAll(dbDirPath, os.ModeDir); err != nil {
		return err
	}

	fileName := fmt.Sprintf("%v.db.json", database.ID)
	dbFile := DatabaseFile{
		DbModel: database.DbModel,
	}
	return parallel.Run(
		func() error {
			if err = s.saveJSONFile(dbDirPath, fileName, dbFile); err != nil {
				return fmt.Errorf("failed to write database json to file: %w", err)
			}
			return nil
		},
		func() error {
			if err = s.saveDbSchemas(dbDirPath, database.Schemas); err != nil {
				return err
			}
			return nil
		},
	)
}

//func (s FileSystemSaver) createStrFile() io.StringWriter {
//
//}
//
//func (s FileSystemSaver) getDatabasesReadme(project DataTugProject) (content bytes.Buffer, err error) {
//
//	_, err = w.WriteString("# DatabaseDiffs\n\n")
//	l, err := f.WriteString("Hello World")
//	if err != nil {
//		fmt.Println(err)
//		f.Close()
//		return
//	}
//	return err
//}
//
//func (s FileSystemSaver) writeDatabaseReadme(database *schemer.Database, dbDirPath string) (err error) {
//
//	return err
//}

func (s FileSystemSaver) saveSchemaModels(dirPath string, schemas []*models.SchemaModel) error {
	return s.saveItems("schemaModel", len(schemas), func(i int) func() error {
		return func() error {
			schema := schemas[i]
			schemaDirPath := path.Join(dirPath, schema.ID)
			if err := os.MkdirAll(schemaDirPath, os.ModeDir); err != nil {
				return err
			}
			return s.saveSchemaModel(schemaDirPath, *schemas[i])
		}
	})
}

func (s FileSystemSaver) saveSchemaModel(schemaDirPath string, schema models.SchemaModel) error {
	saveTables := func(plural string, tables []*models.TableModel) func() error {
		dirPath := path.Join(schemaDirPath, plural)
		return func() error {
			return s.saveItems(fmt.Sprintf("models of %v for schema [%v]", plural, schema.ID), len(tables), func(i int) func() error {
				return func() error {
					return s.saveTableModel(dirPath, *tables[i])
				}
			})
		}
	}
	return parallel.Run(
		saveTables(TablesFolder, schema.Tables),
		saveTables(ViewsFolder, schema.Views),
	)
}

func (s FileSystemSaver) saveDbSchemas(dirPath string, schemas []*models.DbSchema) error {
	return s.saveItems("schemas", len(schemas), func(i int) func() error {
		return func() error {
			schema := schemas[i]
			return s.saveDbSchema(path.Join(dirPath, SchemasFolder, schema.ID), schema)
		}
	})
}

func (s FileSystemSaver) saveDbSchema(schemaDirPath string, dbSchema *models.DbSchema) error {
	return parallel.Run(
		func() error {
			return s.saveTables(schemaDirPath, TablesFolder, dbSchema.Tables)
		},
		func() error {
			return s.saveTables(schemaDirPath, ViewsFolder, dbSchema.Views)
		},
	)
}

func (s FileSystemSaver) saveTables(schemaDirPath, plural string, tables []*models.Table) error {
	dirPath := path.Join(schemaDirPath, plural)
	if len(tables) > 0 {
		if err := os.MkdirAll(dirPath, os.ModeDir); err != nil {
			return err
		}
	}
	// TODO: Remove tables that does not exist anymore
	return s.saveItems("tables", len(tables), func(i int) func() error {
		return func() error {
			return s.saveTable(dirPath, tables[i])
		}
	})
}

func (s FileSystemSaver) saveTableModel(dirPath string, table models.TableModel) error {
	tableDirPath := path.Join(dirPath, table.Name)
	if err := os.MkdirAll(tableDirPath, os.ModeDir); err != nil {
		return err
	}

	var filePrefix string
	if table.Schema == "" {
		filePrefix = table.Name
	} else {
		filePrefix = fmt.Sprintf("%v.%v", table.Schema, table.Name)
	}

	tableKeyWithoutCatalog := table.TableKey
	tableKeyWithoutCatalog.Catalog = ""
	tableKeyWithoutCatalog.Schema = ""

	workers := make([]func() error, 0, 9)
	if len(table.Columns) > 0 { // Saving TABLE_NAME.columns.json
		workers = append(workers, s.saveToFile(tableDirPath, fmt.Sprintf("%v.columns.json", filePrefix), TableModelColumnsFile{
			Columns: table.Columns,
		}))
	}
	return parallel.Run(workers...)

}

func (s FileSystemSaver) saveToFile(tableDirPath, fileName string, data interface{}) func() error {
	return func() (err error) {
		if err = s.saveJSONFile(tableDirPath, fileName, data); err != nil {
			return fmt.Errorf("failed to write json to file %v: %w", fileName, err)
		}
		return nil
	}
}

func (s FileSystemSaver) saveTable(dirPath string, table *models.Table) (err error) {
	tableDirPath := path.Join(dirPath, table.Name)
	if err = os.MkdirAll(tableDirPath, os.ModeDir); err != nil {
		return err
	}

	var filePrefix string
	if table.Schema == "" {
		filePrefix = table.Name
	} else {
		filePrefix = fmt.Sprintf("%v.%v", table.Schema, table.Name)
	}

	workers := make([]func() error, 0, 9)

	tableKeyWithoutCatalog := table.TableKey
	tableKeyWithoutCatalog.Catalog = ""
	tableKeyWithoutCatalog.Schema = ""

	tableFile := TableFile{
		TableProps:   table.TableProps,
		PrimaryKey:   table.PrimaryKey,
		ForeignKeys:  table.ForeignKeys,
		ReferencedBy: table.ReferencedBy,
		Columns:      table.Columns,
	}

	workers = append(workers, s.saveToFile(tableDirPath, fmt.Sprintf("%v.json", filePrefix), tableFile))
	workers = append(workers, s.writeTableReadme(tableDirPath, table))

	return parallel.Run(workers...)
}
