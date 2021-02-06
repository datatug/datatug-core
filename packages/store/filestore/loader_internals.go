package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"log"
	"os"
	"path"
	"strings"
	"sync"
)

func loadProjectFile(projPath string, project *models.DataTugProject) (err error) {
	filePath := path.Join(projPath, DatatugFolder, ProjectSummaryFileName)
	return readJSONFile(filePath, true, project)
}

func loadEnvironments(projPath string, project *models.DataTugProject) (err error) {
	envsDirPath := path.Join(projPath, DatatugFolder, EnvironmentsFolder)
	err = loadDir(nil, envsDirPath, processDirs, func(files []os.FileInfo) {
		project.Environments = make(models.Environments, 0, len(files))
	}, func(f os.FileInfo, i int, _ *sync.Mutex) (err error) {
		env := &models.Environment{
			ProjectItem: models.ProjectItem{
				ID: f.Name(),
			},
		}
		project.Environments = append(project.Environments, env)
		if err = loadEnvironment(path.Join(envsDirPath, env.ID), env); err != nil {
			return err
		}
		return
	})
	if err != nil {
		return err
	}
	return err
}

type process uint8

const (
	processDirs process = 1 << iota
	processFiles
)

func loadDir(
	mutex *sync.Mutex, // pass null by default unless you want to use existing shared mutex
	dirPath string,
	filter process,
	init func(files []os.FileInfo),
	loader func(f os.FileInfo, i int, mutex *sync.Mutex) (err error),
) (err error) {
	//log.Println("Loading dir:", dirPath)
	var dir *os.File
	if dir, err = os.Open(dirPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer func() { _ = dir.Close() }()
	var files []os.FileInfo
	if files, err = dir.Readdir(0); err != nil {
		return err
	}
	workers := make([]func() error, 0, len(files))
	j := 0
	if mutex == nil {
		mutex = new(sync.Mutex)
	}
	for i := range files {
		file := files[i]
		isDir := file.IsDir()
		if isDir && filter&processDirs == processDirs || !isDir && filter&processFiles == processFiles {
			k := j
			workers = append(workers, func() error {
				return loader(file, k, mutex)
			})
			j++
		}
	}
	if err = dir.Close(); err != nil {
		return err
	}
	if init != nil {
		init(files)
	}
	return parallel.Run(workers...)
}

func loadBoards(projPath string, project *models.DataTugProject) (err error) {
	boardsDirPath := path.Join(projPath, DatatugFolder, "boards")
	if err = loadDir(nil, boardsDirPath, processFiles,
		func(files []os.FileInfo) {
			project.Boards = make(models.Boards, len(files))
		},
		func(f os.FileInfo, i int, _ *sync.Mutex) error {
			if f.IsDir() {
				return nil
			}
			board := &models.Board{
				ProjBoardBrief: models.ProjBoardBrief{
					ProjectItem: models.ProjectItem{
						ID: f.Name(),
					},
				},
			}
			project.Boards[i] = board
			fileName := path.Join(boardsDirPath, board.ID)
			if err = readJSONFile(fileName, true, board); err != nil {
				return err
			}
			return nil
		}); err != nil {
		return err
	}
	return err
}

func loadEntities(projPath string, project *models.DataTugProject) error {
	entitiesDirPath := path.Join(projPath, DatatugFolder, "entities")
	if err := loadDir(nil, entitiesDirPath, processDirs,
		func(files []os.FileInfo) {
			project.Entities = make(models.Entities, 0, len(files))
		},
		func(f os.FileInfo, i int, _ *sync.Mutex) error {
			if f.IsDir() {
				return nil
			}
			entityID := f.Name()
			entity := &models.Entity{
				ProjEntityBrief: models.ProjEntityBrief{
					ProjectItem: models.ProjectItem{
						ID: entityID,
					},
				},
			}
			entityFileName := projItemFileName(entity.ID, EntityPrefix)
			project.Entities = append(project.Entities, entity)
			entityFilePath := path.Join(entitiesDirPath, entityFileName)
			if err := readJSONFile(entityFilePath, true, entity); err != nil {
				return err
			}
			if entity.ID != entityID {
				entity.ID = entityID
			}
			return nil
		}); err != nil {
		return fmt.Errorf("failed to load entities: %w", err)
	}
	return nil
}

func loadDbModels(projPath string, project *models.DataTugProject) error {
	dbModelsDirPath := path.Join(projPath, DatatugFolder, "dbmodels")
	if err := loadDir(nil, dbModelsDirPath, processDirs,
		func(files []os.FileInfo) {
			project.DbModels = make(models.DbModels, 0, len(files))
		},
		func(f os.FileInfo, i int, _ *sync.Mutex) (err error) {
			if !f.IsDir() {
				return nil
			}
			var dbModel *models.DbModel
			if dbModel, err = loadDbModel(dbModelsDirPath, f.Name()); err != nil {
				return err
			}
			project.DbModels = append(project.DbModels, dbModel)
			return nil
		}); err != nil {
		return fmt.Errorf("failed to load DB models: %w", err)
	}
	return nil
}

func loadDbModel(dbModelsDirPath, id string) (dbModel *models.DbModel, err error) {
	dbModelDirPath := path.Join(dbModelsDirPath, id)
	dbModel = &models.DbModel{}
	return dbModel, parallel.Run(
		func() (err error) {
			fileName := path.Join(dbModelDirPath, id+".dbmodel.json")
			if err = readJSONFile(fileName, true, dbModel); err != nil {
				return err
			}
			if dbModel.ID == "" {
				dbModel.ID = id
			} else if dbModel.ID != id {
				return fmt.Errorf("dbmodel file has id not matching directory: expected=%v, actual=%v", id, dbModel.ID)
			}
			return err
		},
		func() (err error) {
			return loadDir(nil, dbModelDirPath, processDirs,
				func(files []os.FileInfo) {
					dbModel.Schemas = make([]*models.SchemaModel, 0, len(files))
				},
				func(f os.FileInfo, i int, _ *sync.Mutex) (err error) {
					var schemaModel *models.SchemaModel
					if schemaModel, err = loadSchemaModel(dbModelDirPath, f.Name()); err != nil {
						return err
					}
					dbModel.Schemas = append(dbModel.Schemas, schemaModel)
					return nil
				})
		},
	)
}

func loadSchemaModel(dbModelDirPath, schemaID string) (schemaModel *models.SchemaModel, err error) {
	schemaModel = &models.SchemaModel{
		ProjectItem: models.ProjectItem{ID: schemaID},
	}
	schemaDirPath := path.Join(dbModelDirPath, schemaID)

	loadTableModels := func(dir, dbType string) (tables models.TableModels, err error) {
		dirPath := path.Join(schemaDirPath, dir)

		err = loadDir(nil, dirPath, processDirs, func(files []os.FileInfo) {
			tables = make(models.TableModels, len(files))
		}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
			tables[i], err = loadTableModel(f.Name())
			tables[i].DbType = dbType
			return err
		})
		return
	}
	err = parallel.Run(
		func() (err error) {
			schemaModel.Tables, err = loadTableModels("tables", "BASE TABLE")
			return
		},
		func() (err error) {
			schemaModel.Tables, err = loadTableModels("views", "VIEW")
			return
		},
	)
	return
}

func loadEnvFile(envDirPath string, environment *models.Environment) (err error) {
	filePath := path.Join(envDirPath, fmt.Sprintf("%v.environment.json", environment.ID))
	return readJSONFile(filePath, true, environment)
}

func loadEnvironment(dirPath string, env *models.Environment) (err error) {
	return parallel.Run(
		func() error {
			return loadEnvFile(dirPath, env)
		},
		func() error {
			return loadEnvServers(path.Join(dirPath, "servers"), env)
		},
	)
}

func loadEnvServers(dirPath string, env *models.Environment) error {
	return loadDir(nil, dirPath, processFiles, func(files []os.FileInfo) {
		env.DbServers = make([]*models.EnvDbServer, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		fileName := f.Name()
		serverName := fileName[0:strings.Index(fileName, ".")]
		server := models.EnvDbServer{DbServer: models.DbServer{Host: serverName}}
		if err := readJSONFile(path.Join(dirPath, fileName), false, &server); err != nil {
			return err
		}
		env.DbServers = append(env.DbServers, &server)
		return nil
	})
}

func loadDatabases(dirPath string, dbServer *models.ProjDbServer) (err error) {
	return loadDir(nil, dirPath, processDirs, func(files []os.FileInfo) {
		dbServer.Databases = make(models.Databases, len(files))
	}, func(f os.FileInfo, i int, _ *sync.Mutex) error {
		id := f.Name()
		dbServer.Databases[i] = &models.Database{
			ProjectItem: models.ProjectItem{ID: id},
		}
		if err = loadDatabase(path.Join(dirPath, id), dbServer.Databases[i]); err != nil {
			return err
		}
		return nil
	})
}

func loadDatabase(dirPath string, db *models.Database) (err error) {
	log.Println("Loading database", db.ID)
	filePath := path.Join(dirPath, fmt.Sprintf("%v.db.json", db.ID))
	if err = readJSONFile(filePath, false, db); err != nil {
		return err
	}

	schemasDirPath := path.Join(dirPath, "schemas")
	return loadDir(nil, schemasDirPath, processDirs, func(files []os.FileInfo) {
		db.Schemas = make(models.DbSchemas, len(files))
	}, func(f os.FileInfo, i int, _ *sync.Mutex) error {
		db.Schemas[i], err = loadSchema(schemasDirPath, f.Name())
		return err
	})
}

func loadSchema(schemasDirPath string, id string) (dbSchema *models.DbSchema, err error) {
	log.Printf("Loading schema: %v...", id)
	dbSchema = &models.DbSchema{
		ProjectItem: models.ProjectItem{ID: id},
	}
	err = parallel.Run(
		func() (err error) {
			dbSchema.Tables, err = loadTables(schemasDirPath, dbSchema.ID, "tables")
			return
		},
		func() (err error) {
			dbSchema.Views, err = loadTables(schemasDirPath, dbSchema.ID, "views")
			return
		},
	)
	if err != nil {
		return
	}
	log.Println("Successfully loaded schema:", dbSchema.ID, "; tables:", len(dbSchema.Tables), "; views:", len(dbSchema.Views))
	return
}

//func getSortedSubDirNames(dirPath string) (dirNames []string, err error) {
//	var dir *os.File
//	if dir, err = os.Open(dirPath); err != nil {
//		return
//	}
//	var files []os.FileInfo
//	if files, err = dir.Readdir(0); err != nil {
//		return
//	}
//	dirNames = make([]string, 0, len(files))
//	for _, f := range files {
//		if f.IsDir() {
//			dirNames = append(dirNames, f.Name())
//		}
//	}
//	sort.Slice(dirNames, func(i, j int) bool {
//		return strings.ToLower(dirNames[i]) < strings.ToLower(dirNames[j])
//	})
//	return
//}

func loadTables(schemasDirPath, schema, folder string) (tables models.Tables, err error) {
	dirPath := path.Join(schemasDirPath, schema, folder)
	//if dirs, err = getSortedSubDirNames(dirPath); err != nil {
	//	return err
	//}
	err = loadDir(nil, dirPath, processDirs,
		func(files []os.FileInfo) {
			tables = make(models.Tables, len(files))
		}, func(f os.FileInfo, i int, _ *sync.Mutex) (err error) {
			if !f.IsDir() {
				return nil
			}
			if tables[i], err = loadTable(dirPath, schema, f.Name()); err != nil {
				return
			}
			return err
		})
	if err != nil {
		err = fmt.Errorf("failed to load tables: %w", err)
		return
	}
	return
}

func loadTable(dirPath, schema, tableName string) (table *models.Table, err error) {
	tableDirPath := path.Join(dirPath, tableName)

	prefix := fmt.Sprintf("%v.%v.", schema, tableName)

	table = new(models.Table)
	loadTableFile := func(suffix string, required bool) (err error) {
		filePath := path.Join(tableDirPath, prefix+suffix)
		return readJSONFile(filePath, required, table)
	}
	suffixes := []string{
		"json",
		//"properties.json",
		//"columns.json",
		//"primary_key.json",
		//"foreign_keys.json",
		//"referenced_by.json",
	}
	for _, suffix := range suffixes {
		if err = loadTableFile(suffix, true /*suffix == "properties.json" || suffix == "columns.json"*/); err != nil {
			return
		}
	}
	table.Name = tableName
	table.Schema = schema
	// TODO: For some reason parallel loading is not working here (too tired to think about it now, not critical)
	//workers := make([]func() error, len(suffixes))
	//for i, suffix := range suffixes {
	//	workers[i] = func() error {
	//		return loadTableFile(suffix)
	//	}
	//}
	//if err = parallel.Run(workers...); err != nil {
	//	err = fmt.Errorf("failed to load table files: %w", err)
	//	return
	//}
	return
}

func loadTableModel(name string) (tableModel *models.TableModel, err error) {
	tableModel = &models.TableModel{
		TableKey: models.TableKey{
			Name: name,
		},
	}

	return
}
