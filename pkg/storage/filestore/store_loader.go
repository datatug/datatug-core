package filestore

import (
	"context"
	"fmt"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/parallel"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/strongo/validation"
	"os"
	"path"
	"sync"
)

// fileSystemLoader implements storage.Loader interface
type fileSystemLoader struct {
	pathByID map[string]string
}

// NewSingleProjectLoader create new single project loader
func NewSingleProjectLoader(path string) (loader storage.ProjectStore, projectID string) {
	return newFsProjectStore(storage.SingleProjectID, path), storage.SingleProjectID
}

// LoadProject loads project
func (store fsProjectStore) LoadProject(ctx context.Context) (project *models.DatatugProject, err error) {
	project = new(models.DatatugProject)
	if err = loadProjectFile(store.projectPath, project); err != nil {
		return nil, err
	}
	if err = parallel.Run(
		func() error {
			return loadEnvironments(ctx, store.projectPath, project)
		},
		func() error {
			entities, err := loadEntities(ctx, store.projectPath)
			if err != nil {
				return err
			}
			project.Entities = entities
			return err
		},
		func() error {
			return loadBoards(ctx, store.projectPath, project)
		},
		func() error {
			return loadDbModels(ctx, store.projectPath, project)
		},
		func() error {
			projDbServers, err := loadDbDrivers(ctx, store.projectPath)
			if err != nil {
				return err
			}
			project.DbServers = projDbServers
			return nil
		},
	); err != nil {
		err = fmt.Errorf("failed to load project by ID=[%v]: %w", store.projectID, err)
		return
	}
	return project, err
}

func loadDbDrivers(_ context.Context, projPath string) (dbServers models.ProjDbServers, err error) {
	dbServersPath := path.Join(projPath, DatatugFolder, ServersFolder, DbFolder)
	if err = loadDir(nil, dbServersPath, processDirs, func(files []os.FileInfo) {
		dbServers = make(models.ProjDbServers, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) error {
		dbDriver, err := loadDbDriver(dbServersPath, f.Name())
		if err != nil {
			return err
		}
		mutex.Lock()
		dbServers = append(dbServers, dbDriver...)
		mutex.Unlock()
		return err
	}); err != nil {
		return dbServers, err
	}
	return dbServers, nil
}

func loadDbDriver(dbServersPath, driverName string) (dbServers models.ProjDbServers, err error) {
	driverDirPath := path.Join(dbServersPath, driverName)
	if err = loadDir(nil, driverDirPath, processDirs, func(files []os.FileInfo) {
		dbServers = make(models.ProjDbServers, 0, len(files))
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
		var dbServer *models.ProjDbServer
		dbServer, err = loadDbServer(driverDirPath, driverName, f.Name())
		if err != nil {
			return
		}
		mutex.Lock()
		dbServers = append(dbServers, dbServer)
		mutex.Unlock()
		return
	}); err != nil {
		return
	}
	return
}

func loadDbServer(driverDirPath, driver, serverName string) (dbServer *models.ProjDbServer, err error) {
	dbServer = new(models.ProjDbServer)
	if dbServer.Server, err = models.NewDbServer(driver, serverName, "@"); err != nil {
		return
	}
	dbServerDirPath := path.Join(driverDirPath, serverName)
	err = parallel.Run(
		func() error {
			jsonFileName := jsonFileName(fmt.Sprintf("%v.%v", driver, serverName), dbServerFileSuffix)
			jsonFilePath := path.Join(dbServerDirPath, jsonFileName)
			if err := readJSONFile(jsonFilePath, false, dbServer); err != nil {
				return fmt.Errorf("failed to load db server summary file: %w", err)
			}
			dbServer.Server.Driver = driver
			if dbServer.ID == "" {
				dbServer.ID = serverName
			} else if dbServer.ID != serverName {
				return fmt.Errorf("dbServer.ID != serverName: %v != %v", dbServer.ID, serverName)
			}
			return nil
		},
		func() error {
			dbCatalogsDir := path.Join(dbServerDirPath, DbCatalogsFolder)
			if err := loadDbCatalogs(dbCatalogsDir, dbServer); err != nil {
				return fmt.Errorf("failed to load DB catalogs: %w", err)
			}
			return nil
		},
	)
	return
}

// LoadProjectSummary loads project summary
func (store fsProjectStore) LoadProjectSummary(context.Context) (projectSummary models.ProjectSummary, err error) {
	projectSummary.ID = store.projectID
	if projectSummary.ProjectFile, err = LoadProjectFile(store.projectPath); err != nil {
		return projectSummary, fmt.Errorf("failed to load project file: %w", err)
	}
	return
}

// LoadProjectFile loads project file
func LoadProjectFile(projPath string) (v models.ProjectFile, err error) {
	fileName := path.Join(projPath, DatatugFolder, ProjectSummaryFileName)
	if err = readJSONFile(fileName, true, &v); os.IsNotExist(err) {
		err = fmt.Errorf("%w: %v", models.ErrProjectDoesNotExist, err)
	}
	return
}

func (loader fileSystemLoader) GetFolderPath(projectID string, folder ...string) (folderPath string, err error) {
	_, projectPath, err := loader.GetProjectPath(projectID)
	if err != nil {
		return "", err
	}
	return path.Join(projectPath, DatatugFolder, path.Join(folder...)), nil
}

// GetProjectPath returns project projDirPath by project ID
func (loader fileSystemLoader) GetProjectPath(projectID string) (projID string, projPath string, err error) {
	if projectID == "" && len(projectPaths) == 1 {
		projID = storage.SingleProjectID
	} else {
		projID = projectID
	}
	projPath, knownProjectID := loader.pathByID[projID]
	if !knownProjectID {
		err = validation.NewErrBadRequestFieldValue("projectID", fmt.Sprintf("unknown: [%v]ro", projectID))
		return
	}
	return
}
