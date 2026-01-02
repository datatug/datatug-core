package filestore

import (
	"fmt"
	"log"
	"os"
	"path"
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
	"github.com/datatug/datatug-core/pkg/storage"
)

func loadProjectFile(projPath string, project *datatug.Project) (err error) {
	filePath := path.Join(projPath, storage.ProjectSummaryFileName)
	if err = readJSONFile(filePath, true, project); err != nil {
		err = fmt.Errorf("failed to load project file %s: %w", filePath, err)
	}
	return
}

type process uint8

const (
	processDirs process = 1 << iota
	processFiles
)

func loadDir(
	mutex *sync.Mutex, // pass null by default unless you want to use existing shared mutex
	dirPath string,
	fileMask string,
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
		//log.Printf("failed to readdir [%v]: %v", dirPath, err)
		return err
	}
	workers := make([]func() error, 0, len(files))
	j := 0
	if mutex == nil {
		mutex = new(sync.Mutex)
	}
	if _, err = path.Match(fileMask, ""); err != nil {
		return
	}
	var errs []storage.FileLoadError
	for i := range files {
		file := files[i]
		isDir := file.IsDir()
		if isDir && filter&processDirs == processDirs || !isDir && filter&processFiles == processFiles {
			if !isDir && fileMask != "" {
				if matched, _ := path.Match(fileMask, file.Name()); !matched {
					continue
				}
			}
			f := file
			k := j
			workers = append(workers, func() error {
				if loaderErr := loader(f, k, mutex); loaderErr != nil {
					mutex.Lock()
					errs = append(errs, storage.NewFileLoadError(f.Name(), loaderErr))
					mutex.Unlock()
				}
				return nil
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
	//log.Printf("loadDir: %v, workers: %v", dirPath, len(workers))
	if err = parallel.Run(workers...); err != nil {
		return fmt.Errorf("parallel.Run failed for [%v]: %w", dirPath, err)
	}
	if len(errs) > 0 {
		return storage.NewFilesLoadError(errs)
	}
	return
}

func loadDbModel(dbModelsDirPath, id string) (dbModel *datatug.DbModel, err error) {
	dbModelDirPath := path.Join(dbModelsDirPath, id)
	dbModel = &datatug.DbModel{}
	return dbModel, parallel.Run(
		func() (err error) {
			fileName := path.Join(dbModelDirPath, storage.JsonFileName(id, storage.DbModelFileSuffix))
			if err = readJSONFile(fileName, true, dbModel); err != nil {
				//log.Printf("failed to load db model from [%v]: %v", fileName, err)
				return err
			}
			if dbModel.ID == "" {
				dbModel.ID = id
			} else if dbModel.ID != id {
				return fmt.Errorf("dbModel file has id not matching directory: expected=%v, actual=%v", id, dbModel.ID)
			}
			return err
		},
		func() (err error) {
			return loadDir(nil, dbModelDirPath, "", processDirs,
				func(files []os.FileInfo) {
					dbModel.Schemas = make([]*datatug.Schema, 0, len(files))
				},
				func(f os.FileInfo, i int, _ *sync.Mutex) (err error) {
					var schemaModel *datatug.Schema
					if schemaModel, err = loadSchemaModel(dbModelDirPath, f.Name()); err != nil {
						return err
					}
					dbModel.Schemas = append(dbModel.Schemas, schemaModel)
					return nil
				})
		},
	)
}

func loadSchemaModel(dbModelDirPath, schemaID string) (schemaModel *datatug.Schema, err error) {
	schemaModel = &datatug.Schema{}
	schemaModel.ID = schemaID
	schemaDirPath := path.Join(dbModelDirPath, schemaID)

	loadTableModels := func(dir, dbType string) (tables datatug.TableModels, err error) {
		dirPath := path.Join(schemaDirPath, dir)

		err = loadDir(nil, dirPath, "", processDirs, func(files []os.FileInfo) {
			tables = make(datatug.TableModels, len(files))
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

func loadEnvFile(envDirPath, envID string) (envSummary *datatug.EnvironmentSummary, err error) {
	filePath := path.Join(envDirPath, envID, storage.EnvironmentSummaryFileName)
	envSummary = new(datatug.EnvironmentSummary)
	if err = readJSONFile(filePath, true, envSummary); err != nil {
		return
	}
	if envSummary.ID == "" {
		envSummary.ID = envID
	} else if envSummary.ID != envID {
		err = fmt.Errorf("env file has id not matching directory: expected=%v, actual=%v", envID, envSummary.ID)
	}
	return
}

func loadDbCatalogs(dirPath string, dbServer *datatug.ProjDbServer) (err error) {
	return loadDir(nil, dirPath, "", processDirs, func(files []os.FileInfo) {
		dbServer.Catalogs = make(datatug.DbCatalogs, 0, len(files))
	}, func(f os.FileInfo, i int, _ *sync.Mutex) error {
		dbCatalog := new(datatug.DbCatalog)
		dbCatalog.ID = f.Name()
		catalogPath := path.Join(dirPath, dbCatalog.ID)
		if err = loadDbCatalog(catalogPath, dbCatalog); err != nil {
			return err
		}
		dbServer.Catalogs = append(dbServer.Catalogs, dbCatalog)
		return nil
	})
}

func loadDbCatalog(dirPath string, dbCatalog *datatug.DbCatalog) (err error) {
	//log.Printf("Loading DB catalog: %v from %v...\n", dbCatalog.ID, dirPath)
	filePath := path.Join(dirPath, storage.JsonFileName(dbCatalog.ID, storage.DbCatalogFileSuffix))
	if err = readJSONFile(filePath, false, dbCatalog); err != nil {
		log.Printf("failed to read DB catalog file [%v]: %v", filePath, err)
		return err
	}
	if err := dbCatalog.Validate(); err != nil {
		return fmt.Errorf("db catalog loaded from JSON file is invalid: %w", err)
	}

	schemasDirPath := path.Join(dirPath, storage.SchemasFolder)
	return loadDir(nil, schemasDirPath, "", processDirs, func(files []os.FileInfo) {
		dbCatalog.Schemas = make(datatug.DbSchemas, len(files))
	}, func(f os.FileInfo, i int, _ *sync.Mutex) error {
		dbSchema, err := loadSchema(schemasDirPath, f.Name())
		if err != nil {
			log.Printf("failed to load schema [%v] from [%v]: %v", f.Name(), schemasDirPath, err)
			return err
		}
		dbCatalog.Schemas[i] = dbSchema
		return nil
	})
}

func loadSchema(schemasDirPath string, id string) (dbSchema *datatug.DbSchema, err error) {
	//log.Printf("Loading schema: %v from %v...", id, schemasDirPath)
	dbSchema = &datatug.DbSchema{}
	dbSchema.ID = id
	err = parallel.Run(
		func() (err error) {
			dbSchema.Tables, err = loadTables(schemasDirPath, dbSchema.ID, "tables")
			if err != nil {
				log.Printf("loadTables(tables) failed for schema [%v]: %v", id, err)
			}
			return
		},
		func() (err error) {
			dbSchema.Views, err = loadTables(schemasDirPath, dbSchema.ID, "views")
			if err != nil {
				log.Printf("loadTables(views) failed for schema [%v]: %v", id, err)
			}
			return
		},
	)
	if err != nil {
		return
	}
	if err = dbSchema.Validate(); err != nil {
		return nil, fmt.Errorf("loaded db schema is invalid: %w", err)
	}
	//log.Println("Successfully loaded schema:", dbSchema.ID, "; tables:", len(dbSchema.Tables), "; views:", len(dbSchema.Views))
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

func loadTables(schemasDirPath, schema, folder string) (tables datatug.Tables, err error) {
	dirPath := path.Join(schemasDirPath, schema, folder)
	//if dirs, err = getSortedSubDirNames(dirPath); err != nil {
	//	return err
	//}
	err = loadDir(nil, dirPath, "", processDirs,
		func(files []os.FileInfo) {
			tables = make(datatug.Tables, 0, len(files))
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

func loadTable(dirPath, schema, tableName string) (table *datatug.CollectionInfo, err error) {
	tableDirPath := path.Join(dirPath, tableName)
	//log.Printf("loadTable: schema=%v, table=%v, dirPath=%v", schema, tableName, tableDirPath)

	prefix := fmt.Sprintf("%v.%v.", schema, tableName)
	//log.Printf("loadTable: prefix=%v", prefix)

	table = &datatug.CollectionInfo{}
	table.DBCollectionKey = datatug.NewCollectionKey(datatug.CollectionTypeTable, tableName, schema, "", nil)
	loadTableFile := func(suffix string, required bool) (err error) {
		filePath := path.Join(tableDirPath, prefix+suffix)
		//log.Printf("loadTableFile: path=%v, required=%v", filePath, required)
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

func loadTableModel(name string) (tableModel *datatug.TableModel, err error) {
	tableModel = &datatug.TableModel{
		DBCollectionKey: datatug.NewCollectionKey(datatug.CollectionTypeTable, name, "", "", nil),
	}
	return
}
