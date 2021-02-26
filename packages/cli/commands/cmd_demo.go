package commands

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
)

const chinookSQLiteFileName = "chinook.sqlite"
const chinookDbDir = "chinook/db"
const datatugUserDir = "datatug"
const demoProjectDir = "demo"
const demoProjectAlias = "demo"

func init() {
	_, err := Parser.AddCommand("demo",
		"Installs & runs demo",
		"Adds demo DB & creates or update demo DataTug project",
		&demoDbCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

type demoDbCommand struct {
	Driver string `short:"D" long:"driver" required:"true"`
	Name   string `short:"n" long:"name" required:"true"`
}

func (c demoDbCommand) Execute(args []string) error {
	switch c.Driver {
	case "sqlite3": // OK
		break
	default:
		return fmt.Errorf("unknown DB driver: %v", c.Driver)
	}
	datatugUserDir, err := c.getDatatugUserDir()
	if err != nil {
		return fmt.Errorf("failed to get datatug user dir: %w", err)
	}
	filePath, err := c.downloadChinookSQLiteFile(datatugUserDir)
	if err != nil {
		return fmt.Errorf("failed to download Chinook db file: %w", err)
	}
	if err = c.VerifyChinookDb(filePath); err != nil {
		return fmt.Errorf("failed to verify demo db: %w", err)
	}
	demoProjectPath := path.Join(datatugUserDir, demoProjectDir)
	if err = c.createOrUpdateDemoProject(demoProjectPath, filePath); err != nil {
		return fmt.Errorf("faield to create or update demo project: %w", err)
	}
	if err = c.addDemoProjectToDatatugConfig(datatugUserDir, demoProjectPath); err != nil {
		return fmt.Errorf("failed to update datatug config: %w", err)
	}
	return nil
}

func (c demoDbCommand) VerifyChinookDb(filePath string) error {
	db, err := sql.Open(c.Driver, filePath)
	if err != nil {
		return fmt.Errorf("failed to open demo SQLite db: %w", err)
	}
	rows, err := db.Query("SELECT COUNT(1) FROM Album")
	if err != nil {
		return fmt.Errorf("failed to query count of records in Album table: %w", err)
	}
	if rows.Next(); err != nil {
		return fmt.Errorf("failed to retrieve 1st row: %w", err)
	}
	var count int
	if err = rows.Scan(&count); err != nil {
		return fmt.Errorf("failed to retrieve count value: %w", err)
	}
	if count <= 0 {
		return fmt.Errorf("expected some records on Almub table got: %v", count)
	}
	log.Println("Albums count:", count)
	return nil
}

func (c demoDbCommand) getDatatugUserDir() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home dir: %w", err)
	}
	return path.Join(dir, datatugUserDir), nil
}

func (c demoDbCommand) downloadChinookSQLiteFile(datatugUserDir string) (filePath string, err error) {
	dirPath := path.Join(datatugUserDir, demoProjectDir, chinookDbDir)
	if err = os.MkdirAll(dirPath, 0777); err != nil {
		return "", fmt.Errorf("failed  to create directory for db file(s): %w", err)
	}
	log.Println("Downloading SQLite version of Chinook database...")
	const url = "https://github.com/datatug/chinook-database/blob/master/ChinookDatabase/DataSources/Chinook_Sqlite.sqlite?raw=true"
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("get request failed: %w", err)
	}
	defer resp.Body.Close()

	// Create the file
	filePath = path.Join(dirPath, chinookSQLiteFileName)
	out, err := os.Create(filePath)
	if err != nil {
		return filePath, fmt.Errorf("failed to create db file: %v", err)
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return filePath, fmt.Errorf("failed to write reponse body into file: %w", err)
	}
	return filePath, err
}

func (c demoDbCommand) createOrUpdateDemoProject(demoProjectPath, filePath string) error {
	fileInfo, err := os.Stat(demoProjectPath)
	if os.IsNotExist(err) {
		if err := c.creatDemoProject(demoProjectPath); err != nil {
			return fmt.Errorf("failed to create demo project: %v", err)
		}
	}
	if !fileInfo.IsDir() {
		return fmt.Errorf("expected to have a directory at path %v", demoProjectPath)
	}
	if err = c.updateDemoProject(demoProjectPath); err != nil {
		return fmt.Errorf("failed to update demo project: %w", err)
	}
	return nil
}

func (c demoDbCommand) addDemoProjectToDatatugConfig(datatugUserDir, demoProjectPath string) error {
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

func (c demoDbCommand) creatDemoProject(demoProjectPath string) error {
	if err := os.MkdirAll(demoProjectPath, 0666); err != nil {
		return fmt.Errorf("failed to create a directory for demo project: %w", err)
	}
	return nil
}

func (c demoDbCommand) updateDemoProject(demoProjectPath string) error {
	if err := os.MkdirAll(demoProjectPath, 0666); err != nil {
		return fmt.Errorf("failed to create a directory for demo project: %w", err)
	}
	return nil
}
