package filestore

import (
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"io"
	"log"
	"os"
	"path"
)

func (s fileSystemSaver) saveDbCatalogs(dbServer models.ProjDbServer, repository *models.ProjectRepository) (err error) {
	return s.saveItems("catalogs", len(dbServer.Catalogs), func(i int) func() error {
		return func() error {
			return s.saveDbCatalog(dbServer, dbServer.Catalogs[i], repository)
		}
	})
}

func (s fileSystemSaver) saveDbCatalog(dbServer models.ProjDbServer, dbCatalog *models.DbCatalog, repository *models.ProjectRepository) (err error) {
	if dbCatalog == nil {
		return errors.New("dbCatalog is nil")
	}
	log.Printf("Saving DB catalog [%v]...", dbCatalog.ID)
	serverName := dbServer.Server.FileName()
	saverCtx := saveDbServerObjContext{
		catalog:    dbCatalog.ID,
		dbServer:   dbServer,
		repository: repository,
		dirPath:    path.Join(s.projDirPath, DatatugFolder, ServersFolder, DbFolder, dbServer.Server.Driver, serverName, DbCatalogsFolder, dbCatalog.ID),
	}
	if err := os.MkdirAll(saverCtx.dirPath, os.ModeDir); err != nil {
		return err
	}

	err = parallel.Run(
		func() error {
			return s.saveDbCatalogJSON(*dbCatalog, saverCtx)
		},
		func() (err error) {
			return s.saveDbCatalogObjects(*dbCatalog, saverCtx)
		},
		func() (err error) {
			return s.saveDbCatalogRefs(*dbCatalog, saverCtx)
		},
		func() error {
			if err = s.saveDbSchemas(dbCatalog.Schemas, saverCtx); err != nil {
				return err
			}
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to save DB catalog [%v]: %w", dbCatalog.ID, err)
	}
	log.Printf("Saved DB catalog [%v].", dbCatalog.ID)
	return nil
}

func (s fileSystemSaver) saveDbCatalogJSON(dbCatalog models.DbCatalog, saverCtx saveDbServerObjContext) error {
	fileName := jsonFileName(dbCatalog.ID, dbCatalogFileSuffix)
	dbFile := DbCatalogFile{
		DbModel: dbCatalog.DbModel,
		Path:    dbCatalog.Path,
	}
	if err := s.saveJSONFile(saverCtx.dirPath, fileName, dbFile); err != nil {
		return fmt.Errorf("failed to write dbCatalog json to file: %w", err)
	}
	return nil
}

func (s fileSystemSaver) saveDbCatalogReadme(dbCatalog models.DbCatalog, saverCtx saveDbServerObjContext) error {
	return saveReadme(saverCtx.dirPath, "DB catalog", func(w io.Writer) error {
		if err := s.readmeEncoder.DbCatalogToReadme(w, saverCtx.repository, saverCtx.dbServer, dbCatalog); err != nil {
			return fmt.Errorf("failed to write README.md for DB server: %w", err)
		}
		return nil
	})
}

func (s fileSystemSaver) saveDbCatalogObjects(dbCatalog models.DbCatalog, saverCtx saveDbServerObjContext) error {
	dbObjects := make([]models.CatalogObject, 0)
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
		if err := s.saveJSONFile(saverCtx.dirPath, fileName, dbObjects); err != nil {
			return fmt.Errorf("failed to write dbCatalog objects json to file: %w", err)
		}
	} else {
		// TODO: delete file if exists
	}
	return nil
}

func (s fileSystemSaver) saveDbCatalogRefs(dbCatalog models.DbCatalog, saverCtx saveDbServerObjContext) error {
	dbObjects := make([]models.CatalogObjectWithRefs, 0)
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
		if err := s.saveJSONFile(saverCtx.dirPath, fileName, dbObjects); err != nil {
			return fmt.Errorf("failed to write dbCatalog refs json to file: %w", err)
		}
	} else {
		// TODO: delete file if exists
	}
	return nil
}
