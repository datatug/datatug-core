package filestore

import (
	"context"
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"github.com/datatug/datatug/packages/storage"
	"io"
	"log"
	"os"
	"path"
)

var _ storage.DbCatalogStore = (*fsDbCatalogStore)(nil)

type fsDbCatalogStore struct {
	catalogID string
	fsDbCatalogsStore
}

func (store fsDbCatalogStore) Server() storage.DbServerStore {
	return store.fsDbServerStore
}

func newFsDbCatalogStore(catalogID string, fsDbCatalogsStore fsDbCatalogsStore) fsDbCatalogStore {
	return fsDbCatalogStore{
		catalogID:         catalogID,
		fsDbCatalogsStore: fsDbCatalogsStore,
	}
}

func (store fsDbCatalogStore) LoadDbCatalogSummary(context.Context) (*models.DbCatalogSummary, error) {
	return loadDbCatalogSummary(store.catalogsDirPath, store.catalogID)
}

func (store fsDbCatalogStore) saveDbCatalogs(dbServer models.ProjDbServer, repository *models.ProjectRepository) (err error) {
	return saveItems("catalogs", len(dbServer.Catalogs), func(i int) func() error {
		return func() error {
			dbCatalog := dbServer.Catalogs[i]
			if err := store.saveDbCatalog(dbServer.Catalogs[i], repository); err != nil {
				if dbCatalog.ID == "" {
					return fmt.Errorf("failed to save db catalog at index %v: %w", i, err)
				}
				return fmt.Errorf("failed to save db catalog [%v] at index %v: %w", dbCatalog.ID, i, err)
			}
			return nil
		}
	})
}

func (store fsDbCatalogStore) saveDbCatalog(dbCatalog *models.DbCatalog, repository *models.ProjectRepository) (err error) {
	if dbCatalog == nil {
		return errors.New("dbCatalog is nil")
	}
	log.Printf("Saving DB catalog [%v]...", dbCatalog.ID)
	dbCatalog.Driver = store.dbServer.Driver
	if err = dbCatalog.Validate(); err != nil {
		return fmt.Errorf("invalid db catalog: %w", err)
	}
	//serverName := store.dbServer.FileName()
	saverCtx := saveDbServerObjContext{
		catalog: dbCatalog.ID,
		//dbServer:   store.dbServer,
		repository: repository,
		dirPath:    path.Join(store.catalogsDirPath, dbCatalog.ID),
	}
	if err := os.MkdirAll(saverCtx.dirPath, 0777); err != nil {
		return err
	}

	err = parallel.Run(
		func() error {
			if err := store.saveDbCatalogJSON(*dbCatalog, saverCtx); err != nil {
				return fmt.Errorf("failed to save db catalog JSON: %w", err)
			}
			return nil
		},
		func() (err error) {
			if err := store.saveDbCatalogObjects(*dbCatalog, saverCtx); err != nil {
				return fmt.Errorf("failed to save db catalog objects: %w", err)
			}
			return nil
		},
		func() (err error) {
			if err := store.saveDbCatalogRefs(*dbCatalog, saverCtx); err != nil {
				return fmt.Errorf("failed to save db catalog refs: %w", err)
			}
			return nil
		},
		func() error {
			if err = store.saveDbSchemas(dbCatalog.Schemas, saverCtx); err != nil {
				return fmt.Errorf("failed to save db catalog schemas: %w", err)
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	log.Printf("Saved DB catalog [%v].", dbCatalog.ID)
	return nil
}

func (store fsDbCatalogStore) saveDbCatalogJSON(dbCatalog models.DbCatalog, saverCtx saveDbServerObjContext) error {
	fileName := jsonFileName(dbCatalog.ID, dbCatalogFileSuffix)
	dbFile := DbCatalogFile{
		Driver:  saverCtx.dbServer.Server.Driver,
		DbModel: dbCatalog.DbModel,
		Path:    dbCatalog.Path,
	}
	if err := saveJSONFile(saverCtx.dirPath, fileName, dbFile); err != nil {
		return fmt.Errorf("failed to write dbCatalog json to file: %w", err)
	}
	return nil
}

func (store fsDbCatalogStore) saveDbCatalogReadme(dbCatalog models.DbCatalog, saverCtx saveDbServerObjContext) error {
	return saveReadme(saverCtx.dirPath, "DB catalog", func(w io.Writer) error {
		if err := store.readmeEncoder.DbCatalogToReadme(w, saverCtx.repository, saverCtx.dbServer, dbCatalog); err != nil {
			return fmt.Errorf("failed to write README.md for DB server: %w", err)
		}
		return nil
	})
}

func (store fsDbCatalogStore) saveDbCatalogObjects(dbCatalog models.DbCatalog, saverCtx saveDbServerObjContext) error {
	dbObjects := make(models.CatalogObjects, 0)
	for _, schema := range dbCatalog.Schemas {
		for _, t := range schema.Tables {
			dbObjects = append(dbObjects, models.CatalogObject{
				Type:         "table",
				Schema:       t.Schema,
				Name:         t.Name,
				DefaultAlias: "",
			})
		}
		for _, t := range schema.Views {
			dbObjects = append(dbObjects, models.CatalogObject{
				Type:         "view",
				Schema:       t.Schema,
				Name:         t.Name,
				DefaultAlias: "",
			})
		}
	}
	fileName := jsonFileName(dbCatalog.ID, dbCatalogObjectFileSuffix)
	if len(dbObjects) > 0 {
		if err := saveJSONFile(saverCtx.dirPath, fileName, dbObjects); err != nil {
			return fmt.Errorf("failed to write dbCatalog objects json to file: %w", err)
		}
	} else {
		// TODO: delete file if exists
	}
	return nil
}

func (store fsDbCatalogStore) saveDbCatalogRefs(dbCatalog models.DbCatalog, saverCtx saveDbServerObjContext) error {
	dbObjects := make(models.CatalogObjectsWithRefs, 0)
	for _, schema := range dbCatalog.Schemas {
		for _, t := range schema.Tables {
			if len(t.ForeignKeys) == 0 && len(t.ReferencedBy) == 0 {
				continue
			}
			dbObjects = append(dbObjects, models.CatalogObjectWithRefs{
				CatalogObject: models.CatalogObject{
					Type:         "table",
					Schema:       t.Schema,
					Name:         t.Name,
					DefaultAlias: "",
				},
				PrimaryKey:   t.PrimaryKey,
				ForeignKeys:  t.ForeignKeys,
				ReferencedBy: t.ReferencedBy,
			})
		}
		for _, t := range schema.Views {
			if len(t.ForeignKeys) == 0 && len(t.ReferencedBy) == 0 {
				continue
			}
			dbObjects = append(dbObjects, models.CatalogObjectWithRefs{
				CatalogObject: models.CatalogObject{
					Type:         "view",
					Schema:       t.Schema,
					Name:         t.Name,
					DefaultAlias: "",
				},
				PrimaryKey:   t.PrimaryKey,
				ForeignKeys:  t.ForeignKeys,
				ReferencedBy: t.ReferencedBy,
			})
		}
	}
	fileName := jsonFileName(dbCatalog.ID, dbCatalogRefsFileSuffix)
	if len(dbObjects) > 0 {
		if err := saveJSONFile(saverCtx.dirPath, fileName, dbObjects); err != nil {
			return fmt.Errorf("failed to write dbCatalog refs json to file: %w", err)
		}
	} else {
		// TODO: delete file if exists
	}
	return nil
}
