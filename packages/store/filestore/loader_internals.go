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

func loadProjectFile(projPath string, project *models.DatatugProject) (err error) {
	filePath := path.Join(projPath, DatatugFolder, ProjectSummaryFileName)
	return readJSONFile(filePath, true, project)
}

func loadEnvironments(projPath string, project *models.DatatugProject) (err error) {
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

func loadBoards(projPath string, project *models.DatatugProject) (err error) {
	boardsDirPath := path.Join(projPath, DatatugFolder, "boards")
	if err = loadDir(nil, boardsDirPath, processFiles,
		func(files []os.FileInfo) {
			project.Boards = make(models.Boards, 0, len(files))
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
			var suffix string
			board.ID, suffix = getProjItemIdFromFileName(f.Name())
			if strings.ToLower(suffix) != boardFileSuffix {
				return nil
			}
			fullFileName := path.Join(boardsDirPath, f.Name())
			if err = readJSONFile(fullFileName, true, board); err != nil {
				return err
			}
			project.Boards = append(project.Boards, board)
			return nil
		}); err != nil {
		return err
	}
	return err
}

func loadDbModels(projPath string, project *models.DatatugProject) error {
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
			fileName := path.Join(dbModelDirPath, jsonFileName(id, dbModelFileSuffix))
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
	filePath := path.Join(envDirPath, jsonFileName(environment.ID, environmentFileSuffix))
	return readJSONFile(filePath, true, environment)
}

func loadEnvironment(dirPath string, env *models.Environment) (err error) {
	return parallel.Run(
		func() error {
			return loadEnvFile(dirPath, env)
		},
		func() error {
			return loadEnvServers(path.Join(dirPath, ServersFolder), env)
		},
	)
}

func loadDbCatalogs(dirPath string, dbServer *models.ProjDbServer) (err error) {
	return loadDir(nil, dirPath, processDirs, func(files []os.FileInfo) {
		dbServer.Catalogs = make(models.DbCatalogs, 0, len(files))
	}, func(f os.FileInfo, i int, _ *sync.Mutex) error {
		dbCatalog := new(models.DbCatalog)
		dbCatalog.ID = f.Name()
		catalogPath := path.Join(dirPath, dbCatalog.ID)
		if err = loadDbCatalog(catalogPath, dbCatalog); err != nil {
			return err
		}
		dbServer.Catalogs = append(dbServer.Catalogs, dbCatalog)
		return nil
	})
}

func loadDbCatalog(dirPath string, dbCatalog *models.DbCatalog) (err error) {
	log.Printf("Loading DB catalog: %v...\n", dbCatalog.ID)
	filePath := path.Join(dirPath, jsonFileName(dbCatalog.ID, dbCatalogFileSuffix))
	if err = readJSONFile(filePath, false, dbCatalog); err != nil {
		return err
	}
	if err := dbCatalog.Validate(); err != nil {
		return fmt.Errorf("db catalog loaded from JSON file is invalid: %w", err)
	}

	schemasDirPath := path.Join(dirPath, SchemasFolder)
	return loadDir(nil, schemasDirPath, processDirs, func(files []os.FileInfo) {
		dbCatalog.Schemas = make(models.DbSchemas, len(files))
	}, func(f os.FileInfo, i int, _ *sync.Mutex) error {
		dbCatalog.Schemas[i], err = loadSchema(schemasDirPath, f.Name())
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
	if err = dbSchema.Validate(); err != nil {
		return nil, fmt.Errorf("loaded db schema is invalid: %w", err)
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
			tables = make(models.Tables, 0, len(files))
		}, func(f os.FileInfo, i int, _ *sync.Mutex) error {
			if !f.IsDir() {
				return nil
			}
			name := f.Name()
			table, err := loadTable(dirPath, schema, name)
			if err != nil {
				return fmt.Errorf("failed to load table [%v].[%v]: %w", schema, name, err)
			}
			tables = append(tables, table)
			return nil
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
	table.Name = tableName
	table.Schema = schema
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
			err = fmt.Errorf("failed to load table file [%v]: %w", prefix+suffix, err)
			return
		}
	}
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
