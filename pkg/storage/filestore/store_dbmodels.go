package filestore

//import (
//	"fmt"
//	"os"
//	"path"
//
//	"github.com/datatug/datatug-core/pkg/datatug"
//	"github.com/datatug/datatug-core/pkg/parallel"
//)

//var _ storage.DbModelsStore = (*fsDbModelsStore)(nil)

//type fsDbModelsStore struct {
//	dbModelsPath string
//	fsProjectStoreRef
//}
//
//func newFsDbModelsStore(fsProjectStore fsProjectStore) fsDbModelsStore {
//	return fsDbModelsStore{
//		dbModelsPath:      path.Join(fsProjectStore.projectPath, DbModelsFolder),
//		fsProjectStoreRef: fsProjectStoreRef{fsProjectStore},
//	}
//}

//func (store fsDbModelsStore) DbModel(id string) storage.DbModelStore {
//	return store.dbModel(id)
//}
//
//func (store fsDbModelsStore) dbModel(id string) storage.DbModelStore {
//	return newFsDbModelStore(id, store)
//}
//
//var _ storage.DbModelStore = (*fsDbModelStore)(nil)

//type fsDbModelStore struct {
//	dbModelID string
//	fsDbModelsStore
//}
//
//func (store fsDbModelStore) GetID() string {
//	return store.dbModelID
//}
//
//func newFsDbModelStore(dbModelID string, fsDbModelsStore fsDbModelsStore) fsDbModelStore {
//	return fsDbModelStore{
//		dbModelID:       dbModelID,
//		fsDbModelsStore: fsDbModelsStore,
//	}
//}
//
////func (store fsDbModelStore) DbModels() datatug.DbModelsStore {
////	return store.fsDbModelsStore
////}
//
//func (store fsDbModelsStore) saveDbModels(dbModels datatug.DbModels) (err error) {
//	return saveItems(DbModelsFolder, len(dbModels), func(i int) func() error {
//		return func() error {
//			dbModel := dbModels[i]
//			err := store.saveDbModel(dbModel)
//			if err != nil {
//				if dbModel.GetID == "" {
//					return fmt.Errorf("failed to save db model at index %v: %w", i, err)
//				}
//				return fmt.Errorf("failed to save db model [%v] at index %v: %w", dbModel.GetID, i, err)
//			}
//			return nil
//		}
//	})
//}
//
//func (store fsDbModelsStore) saveDbModel(dbModel *datatug.DbModel) (err error) {
//	if err = dbModel.ValidateWithOptions(); err != nil {
//		return fmt.Errorf("db models is invalid: %w", err)
//	}
//	dirPath := path.Join(store.projectPath, DatatugFolder, DbModelsFolder, dbModel.GetID)
//	if err = os.MkdirAll(dirPath, 0777); err != nil {
//		return fmt.Errorf("failed to create db model folder: %w", err)
//	}
//	return parallel.Run(
//		func() error {
//			dbModelFile := DbModelFile{
//				ProjectItem:  dbModel.ProjectItem,
//				Environments: dbModel.Environments,
//			}
//			return saveJSONFile(dirPath, JsonFileName(dbModel.GetID, DbModelFileSuffix), dbModelFile)
//		},
//		func() error {
//			return store.saveSchemaModels(dirPath, dbModel.Schemas)
//		},
//	)
//}
//
//func (store fsDbModelsStore) saveSchemaModels(dirPath string, schemas []*datatug.SchemaModel) error {
//	return saveItems("schemaModel", len(schemas), func(i int) func() error {
//		return func() error {
//			schema := schemas[i]
//			schemaDirPath := path.Join(dirPath, schema.GetID)
//			if err := os.MkdirAll(schemaDirPath, 0777); err != nil {
//				return err
//			}
//			return store.saveSchemaModel(schemaDirPath, *schemas[i])
//		}
//	})
//}
//
//func (store fsDbModelsStore) saveSchemaModel(schemaDirPath string, schema datatug.SchemaModel) error {
//	saveTables := func(plural string, tables []*datatug.TableModel) func() error {
//		dirPath := path.Join(schemaDirPath, plural)
//		return func() error {
//			return saveItems(fmt.Sprintf("models of %v for schema [%v]", plural, schema.GetID), len(tables), func(i int) func() error {
//				return func() error {
//					return store.saveTableModel(dirPath, *tables[i])
//				}
//			})
//		}
//	}
//	return parallel.Run(
//		saveTables(TablesFolder, schema.Tables),
//		saveTables(ViewsFolder, schema.Views),
//	)
//}
//
//func (store fsDbModelsStore) saveTableModel(dirPath string, table datatug.TableModel) error {
//	tableDirPath := path.Join(dirPath, table.Name())
//	if err := os.MkdirAll(tableDirPath, 0777); err != nil {
//		return err
//	}
//
//	var filePrefix string
//	if table.Schema() == "" {
//		filePrefix = table.Name()
//	} else {
//		filePrefix = fmt.Sprintf("%s.%s", table.Schema(), table.Name())
//	}
//
//	workers := make([]func() error, 0, 9)
//	if len(table.Columns) > 0 { // Saving TABLE_NAME.columns.json
//		workers = append(workers, saveToFile(tableDirPath, JsonFileName(filePrefix, ColumnsFileSuffix), TableModelColumnsFile{
//			Columns: table.Columns,
//		}))
//	}
//	return parallel.Run(workers...)
//
//}
