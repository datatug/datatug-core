package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
	"os"
	"path"
	"sync"
)

// fileSystemLoader implements store.Loader interface
type fileSystemLoader struct {
	pathByID map[string]string
}

// GetEnvironmentDbSummary return DB summary for specific environment
func (loader fileSystemLoader) LoadEnvironmentDbSummary(projectID, environmentID, databaseID string) (models.DbCatalogSummary, error) {
	panic(fmt.Sprintf("implement me: %v, %v, %v", projectID, environmentID, databaseID))
}

var _ store.Loader = (*fileSystemLoader)(nil)

func newLoader(pathByID map[string]string) fileSystemLoader {
	return fileSystemLoader{
		pathByID: pathByID,
	}
}

// NewSingleProjectLoader create new single project loader
func NewSingleProjectLoader(path string) (loader store.Loader, projectID string) {
	return newLoader(map[string]string{store.SingleProjectID: path}), store.SingleProjectID
}

// LoadProject loads project
func (loader fileSystemLoader) LoadProject(projID string) (project *models.DatatugProject, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	project = new(models.DatatugProject)
	if err = loadProjectFile(projPath, project); err != nil {
		return nil, err
	}
	if err = parallel.Run(
		func() error {
			return loadEnvironments(projPath, project)
		},
		func() error {
			entities, err := loadEntities(projPath)
			if err != nil {
				return err
			}
			project.Entities = entities
			return err
		},
		func() error {
			return loadBoards(projPath, project)
		},
		func() error {
			return loadDbModels(projPath, project)
		},
		func() error {
			projDbServers, err := loadDbDrivers(projPath)
			if err != nil {
				return err
			}
			project.DbServers = projDbServers
			return nil
		},
	); err != nil {
		err = fmt.Errorf("failed to load project by ID=[%v]: %w", projID, err)
		return
	}
	return project, err
}

func loadDbDrivers(projPath string) (dbServers models.ProjDbServers, err error) {
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

// LoadBoard loads board
func (loader fileSystemLoader) LoadBoard(projID, boardID string) (board models.Board, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	fileName := path.Join(projPath, DatatugFolder, BoardsFolder, fmt.Sprintf("%v.json", boardID))
	if err = readJSONFile(fileName, true, &board); err != nil {
		err = fmt.Errorf("faile to load board [%v] from project [%v]: %w", boardID, projID, err)
		return
	}
	return
}

// LoadProjectSummary loads project summary
func (loader fileSystemLoader) LoadProjectSummary(projID string) (projectSummary models.ProjectSummary, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		err = fmt.Errorf("failed to get project path: %w", err)
		return
	}
	projectSummary.ProjectFile.ID = projID
	if projectSummary.ProjectFile, err = LoadProjectFile(projPath); err != nil {
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
		projID = store.SingleProjectID
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

// GetEnvironmentSummary loads environment summary
func (loader fileSystemLoader) LoadEnvironmentSummary(projID, envID string) (envSummary models.EnvironmentSummary, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	envDirPath := path.Join(projPath, DatatugFolder, EnvironmentsFolder, envID)
	if envSummary, err = loadEnvFile(envDirPath, envID); err != nil {
		err = fmt.Errorf("failed to load environment [%v] from project [%v]: %w", envID, projID, err)
		return
	}
	return
}

// GetEnvironmentDb return information about environment DB
func (loader fileSystemLoader) LoadEnvironmentCatalog(projID, environmentID, databaseID string) (envDb *models.EnvDb, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	filePath := path.Join(projPath, DatatugFolder, EnvironmentsFolder, environmentID, DbCatalogsFolder, databaseID, jsonFileName(databaseID, dbCatalogFileSuffix))
	envDb = new(models.EnvDb)
	if err = readJSONFile(filePath, true, envDb); err != nil {
		err = fmt.Errorf("failed to load environment DB catalog [%v] from env [%v] from project [%v]: %w", databaseID, environmentID, projID, err)
		return nil, err
	}
	envDb.ID = databaseID
	if err = envDb.Validate(); err != nil {
		return nil, fmt.Errorf("loaded environmend DB catalog file is invalid: %w", err)
	}
	return
}
