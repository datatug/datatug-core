package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"github.com/datatug/datatug/packages/server/dto"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
	"os"
	"path"
	"strings"
	"sync"
)

// fileSystemLoader implements store.Loader interface
type fileSystemLoader struct {
	pathByID map[string]string
}

// GetEnvironmentDbSummary return DB summary for specific environment
func (loader fileSystemLoader) LoadEnvironmentDbSummary(projectID, environmentID, databaseID string) (dto.DatabaseSummary, error) {
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
func (loader fileSystemLoader) LoadProject(projID string) (project *models.DataTugProject, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	project = new(models.DataTugProject)
	if err = loadProjectFile(projPath, project); err != nil {
		return nil, err
	}
	if err = parallel.Run(
		func() error {
			return loadEnvironments(projPath, project)
		},
		func() error {
			return loadEntities(projPath, project)
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
	if dbServer.DbServer, err = models.NewDbServer(driver, serverName, "@"); err != nil {
		return
	}
	dbServerDirPath := path.Join(driverDirPath, serverName)
	err = parallel.Run(
		func() (err error) {
			err = readJSONFile(path.Join(dbServerDirPath, jsonFileName(fmt.Sprintf("%v.%v", driver, serverName), dbServerFileSuffix)), true, dbServer)
			if err != nil {
				err = fmt.Errorf("failed to load db server summary file: %w", err)
			}
			if dbServer.ID == "" {
				dbServer.ID = serverName
			} else if dbServer.ID != serverName {
				return fmt.Errorf("dbServer.ID != serverName: %v != %v", dbServer.ID, serverName)
			}
			return err
		},
		func() (err error) {
			return loadDatabases(path.Join(dbServerDirPath, DbCatalogsFolder), dbServer)
		},
	)
	return
}

func (loader fileSystemLoader) LoadEntity(projID, entityID string) (entity models.Entity, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	fileName := path.Join(projPath, DatatugFolder, EntitiesFolder, jsonFileName(entityID, entityFileSuffix))
	if err = readJSONFile(fileName, true, &entity); err != nil {
		err = fmt.Errorf("faile to load entity [%v] from project [%v]: %w", entityID, projID, err)
		return
	}
	return
}

func (loader fileSystemLoader) LoadEntities(projID string) (entities []models.Entity, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	entitiesPath := path.Join(projPath, DatatugFolder, EntitiesFolder)
	isEntityFile := func(fileName string) bool {
		return strings.HasSuffix(fileName, ".json")
	}
	err = loadDir(nil, entitiesPath, processFiles, func(files []os.FileInfo) {
		count := 0
		for _, f := range files {
			if isEntityFile(f.Name()) {
				count++
			}
		}
		entities = make([]models.Entity, count)
	}, func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
		if !isEntityFile(f.Name()) {
			return nil
		}
		if err = readJSONFile(path.Join(entitiesPath, f.Name()), true, &entities[i]); err != nil {
			err = fmt.Errorf("faile to load entity from file [%v] from project [%v]: %w", f.Name(), projID, err)
			return
		}
		return nil
	})
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
func (loader fileSystemLoader) LoadEnvironmentSummary(projID, envID string) (envSummary dto.EnvironmentSummary, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	env := models.Environment{ProjectItem: models.ProjectItem{ID: envID}}
	if err = loadEnvironment(path.Join(projPath, DatatugFolder, EnvironmentsFolder, envID), &env); err != nil {
		err = fmt.Errorf("failed to load environment [%v] from project [%v]: %w", envID, projID, err)
		return
	}
	envSummary.ProjectItem = env.ProjectItem
	for _, dbServer := range env.DbServers {
		envSummary.Servers = append(envSummary.Servers, *dbServer)
	}
	return
}

// GetEnvironmentDb return information about environment DB
func (loader fileSystemLoader) LoadEnvironmentDb(projID, environmentID, databaseID string) (envDb *dto.EnvDb, err error) {
	var projPath string
	if projID, projPath, err = loader.GetProjectPath(projID); err != nil {
		return
	}
	filePath := path.Join(projPath, DatatugFolder, EnvironmentsFolder, environmentID, DbCatalogsFolder, databaseID, jsonFileName(databaseID, dbCatalogFileSuffix))
	envDb = new(dto.EnvDb)
	if err = readJSONFile(filePath, true, envDb); err != nil {
		err = fmt.Errorf("failed to load DB [%v] from env [%v] from project [%v]: %w", databaseID, environmentID, projID, err)
		return nil, err
	}
	envDb.ID = databaseID
	return
}
