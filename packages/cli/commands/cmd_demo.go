package commands

import (
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"github.com/datatug/datatug/packages/store"
	"github.com/datatug/datatug/packages/store/filestore"
	"github.com/go-git/go-git/v5"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
)

const (
	demoDriver            = "sqlite3"
	localhost             = "localhost"
	chinookSQLiteFileName = "chinook.sqlite"
	demoDbsDirName        = "dbs"
	datatugUserDir        = "datatug"
	demoProjectAlias      = "datatug-demo-project"
	chinookCatalog        = "chinook"
	demoProjectID         = demoProjectAlias // + "@datatug"
	demoProjectDir        = "demo-project"
)

func init() {
	_, err := Parser.AddCommand("demo",
		"Installs & runs demo",
		"Adds demo DB & creates or update demo DataTug project",
		&demoCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

type demoCommand struct {
	ResetDB      bool `long:"reset-db" required:"false" description:"Re-downloads demo DB file from internet"`
	ResetProject bool `long:"reset-project" required:"false" description:"Recreates demo project"`
}

func (c demoCommand) Execute(_ []string) error {
	datatugUserDirPath, err := c.getDatatugUserDirPath()
	demoProjectPath := path.Join(datatugUserDirPath, demoProjectDir)
	if err != nil {
		return fmt.Errorf("failed to get datatug user dir: %w", err)
	}

	dbFilePath := path.Join(datatugUserDirPath, demoDbsDirName, chinookSQLiteFileName)
	if c.ResetDB {
		if err = c.reDownloadChinookDb(dbFilePath); err != nil {
			return err
		}
	} else if _, err := os.Stat(demoProjectPath); os.IsNotExist(err) {
		if err = c.downloadChinookSQLiteFile(dbFilePath); err != nil {
			return fmt.Errorf("failed to download Chinook db file: %w", err)
		}
		if err = c.VerifyChinookDb(dbFilePath); err != nil {
			return fmt.Errorf("failed to verify downloaded demo db: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check if demo db file exists: %w", err)
	} else {
		log.Println("Demo DB file already exists.")
		if err = c.VerifyChinookDb(dbFilePath); err != nil {
			log.Println("Failed to verify demo db:", err)
			if err = c.reDownloadChinookDb(dbFilePath); err != nil {
				return err
			}
		}
	}

	if c.ResetProject {
		if err := os.RemoveAll(demoProjectPath); err != nil {
			return fmt.Errorf("failed to remove existig demo project: %w", err)
		}
	}
	if err = c.createOrUpdateDemoProject(demoProjectPath, dbFilePath); err != nil {
		return fmt.Errorf("failed to create or update demo project: %w", err)
	}
	if err = c.addDemoProjectToDatatugConfig(datatugUserDirPath, demoProjectPath); err != nil {
		return fmt.Errorf("failed to update datatug config: %w", err)
	}
	log.Println("DataTug demo project is ready!")
	log.Println("Run `./datatug serve` to see the demo project.")
	return nil
}

func (c demoCommand) reDownloadChinookDb(dbFilePath string) error {
	log.Println("Will re-download the demo DB file.")
	if err := c.downloadChinookSQLiteFile(dbFilePath); err != nil {
		return fmt.Errorf("failed to re-download Chinook db file: %w", err)
	}
	if err := c.VerifyChinookDb(dbFilePath); err != nil {
		return fmt.Errorf("failed to verify re-downloaded demo db: %w", err)
	}
	return nil
}
func (c demoCommand) VerifyChinookDb(filePath string) error {
	log.Println("Verifying demo db...")
	tables := []string{
		"Employee",
		"Customer",
		"Invoice",
		"InvoiceLine",
		"Artist",
		"Album",
		"MediaType",
		"Genre",
		"Track",
		"Playlist",
		"PlaylistTrack",
	}
	workers := make([]func() error, len(tables))

	results := make([]int, len(tables))
	for i, t := range tables {
		workers[i] = c.getRecordsCountWorker(t, filePath, i, results)
	}
	if err := parallel.Run(workers...); err != nil {
		return err
	}
	for i, t := range tables {
		log.Println(fmt.Sprintf("- %v count:", t), results[i])
	}
	return nil
}

func (c demoCommand) getRecordsCountWorker(objName, filePath string, i int, results []int) func() error {
	return func() error {
		db, err := sql.Open(demoDriver, filePath)
		if err != nil {
			return fmt.Errorf("failed to open demo SQLite db: %w", err)
		}
		rows, err := db.Query("SELECT COUNT(1) FROM " + objName)
		if err != nil {
			return fmt.Errorf("failed to retrieve how many records in [%v]: %w", objName, err)
		}
		if rows.Next() {

		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("failed to retrieve 1st row: %w", err)
		}
		var count int
		if err = rows.Scan(&count); err != nil {
			return fmt.Errorf("failed to retrieve count value: %w", err)
		}
		if count <= 0 {
			return fmt.Errorf("expected some records on Almub table got: %v", count)
		}
		results[i] = count
		return nil
	}
}

func (c demoCommand) getDatatugUserDirPath() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home dir: %w", err)
	}
	return path.Join(dir, datatugUserDir), nil
}

func (c demoCommand) downloadChinookSQLiteFile(dbFilePath string) error {
	dirPath := path.Dir(dbFilePath)
	if err := os.MkdirAll(dirPath, 0777); err != nil {
		return fmt.Errorf("failed  to create directory for db file(s): %w", err)
	}
	log.Println("Downloading SQLite version of Chinook database...")
	const url = "https://github.com/datatug/chinook-database/blob/master/ChinookDatabase/DataSources/Chinook_Sqlite.sqlite?raw=true"
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("get request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Create the file
	out, err := os.Create(dbFilePath)
	if err != nil {
		return fmt.Errorf("failed to create db file: %v", err)
	}
	defer func() {
		_ = out.Close()
	}()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write reponse body into file: %w", err)
	}
	return err
}

func (c demoCommand) isDemoProjectExists(demoProjectPath string) (bool, error) {
	if _, err := os.Stat(demoProjectPath); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func (c demoCommand) createOrUpdateDemoProject(demoProjectPath, filePath string) error {
	fileInfo, err := os.Stat(demoProjectPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to check if demo project dir exists: %w", err)
		}
		if err := c.creatDemoProject(demoProjectPath); err != nil {
			return fmt.Errorf("failed to create demo project: %v", err)
		}
	}
	if fileInfo != nil && !fileInfo.IsDir() {
		return fmt.Errorf("expected to have a directory at path %v", demoProjectPath)
	}
	if err = c.updateDemoProject(demoProjectPath, filePath); err != nil {
		return fmt.Errorf("failed to update demo project: %w", err)
	}
	return nil
}

func (c demoCommand) addDemoProjectToDatatugConfig(datatugUserDir, demoProjectPath string) error {
	log.Printf("Adding demo project to DataTug config into %v...", datatugUserDir)
	config, err := getConfig()
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read datatug config: %w", err)
		}
	}
	demoProjConfig, ok := config.Projects[demoProjectAlias]
	if ok && demoProjConfig.Path != demoProjectPath {
		return fmt.Errorf("demo project expected to be located at %v but is pointing to unexpected path: %v",
			demoProjectPath, demoProjConfig.Path)
	}
	if !ok {
		demoProjConfig.Path = demoProjectPath
		if config.Projects == nil {
			config.Projects = make(map[string]ProjectConfig, 1)
		}
		config.Projects[demoProjectAlias] = demoProjConfig
		if err = saveConfig(config); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
	}
	return nil
}

func (c demoCommand) creatDemoProject(demoProjectPath string) error {
	const demoProjectWebURL = "https://github.com/datatug/datatug-demo-project"
	const demoProjectGitURL = "https://github.com/datatug/datatug-demo-project.git"
	log.Println("Cloning demo project from", demoProjectWebURL, "...")
	repository, err := git.PlainClone(demoProjectPath, false, &git.CloneOptions{
		URL:  demoProjectGitURL,
		Tags: git.NoTags,
	})
	if err != nil {
		return fmt.Errorf("failed to `git clone` demo project from %v: %w", demoProjectGitURL, err)
	}
	ref, err := repository.Head()
	if err != nil {
		return fmt.Errorf("failed to get git head info: %w", err)
	}
	log.Println("Cloned demo project, head @ ", ref.String())
	return nil
}

func (c demoCommand) updateDemoProject(demoProjectPath, demoDbPath string) error {
	log.Println("Updating demo project...")
	store.Current, _ = filestore.NewSingleProjectStore(demoProjectPath, demoProjectID)
	project, err := api.GetProjectFull(demoProjectID)
	if err != nil {
		return fmt.Errorf("failed to load demo project: %w", err)
	}
	projDbServer := project.DbServers.GetProjDbServer(demoDriver, localhost, 0)
	if projDbServer == nil {
		projDbServer = &models.ProjDbServer{
			ProjectItem: models.ProjectItem{
				ID: localhost,
			},
			Server: models.ServerReference{
				Driver: demoDriver,
				Host:   localhost,
			},
		}
		project.DbServers = append(project.DbServers, projDbServer)
		log.Printf("Added new DB server: %v", projDbServer.ID)
	}

	catalog := projDbServer.Catalogs.GetDbByID(chinookCatalog)
	if catalog == nil {
		catalog = &models.DbCatalog{
			ProjectItem: models.ProjectItem{
				ID: chinookCatalog,
			},
			Path: demoDbPath,
		}
		if dir, err := os.UserHomeDir(); err == nil {
			if strings.HasPrefix(catalog.Path, dir) {
				catalog.Path = strings.Replace(catalog.Path, dir, "~", 1)
			}
		}

		projDbServer.Catalogs = append(projDbServer.Catalogs, catalog)
		log.Printf("Added new catalog [%v] to DB server [%v]", catalog.ID, projDbServer.ID)
	}

	if dbModel := project.DbModels.GetDbModelByID(chinookCatalog); dbModel == nil {
		dbModel = &models.DbModel{
			ProjectItem: models.ProjectItem{ID: chinookCatalog},
		}
		project.DbModels = append(project.DbModels, dbModel)
	}
	if err := store.Current.Save(*project); err != nil {
		return fmt.Errorf("faield to save project: %w", err)
	}
	return nil
}
