package commands

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dbconnection"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/datatug/datatug/packages/storage/filestore"
	"github.com/mitchellh/go-homedir"
	"log"
	"os"
	"strconv"
)

func init() {
	_, err := Parser.AddCommand("scan",
		"Adds or updates DB metadata",
		"Adds or updates DB metadata from a specific server in a specific environment",
		&scanDbCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

// scanDbCommand defines parameters for scan command
type scanDbCommand struct {
	projectBaseCommand
	Driver      string `short:"D" long:"driver" description:"Supported values: sqlserver."`
	Host        string `short:"s" long:"server" description:"Network server name."`
	Port        int    `long:"port" description:"ServerReference network port, if not specified default is used."`
	User        string `short:"U" long:"user" description:"User name to login to DB."`
	Password    string `short:"P" long:"password" description:"Password to login to DB."`
	Database    string `long:"db" required:"true" description:"Title of database to be scanned."`
	DbModel     string `long:"dbmodel" required:"false" description:"Title of DB model, is required for newly scanned databases."`
	Environment string `long:"env" required:"true" description:"Specify environment the DB belongs to. E.g.: LOCAL, DEV, SIT, UAT, PERF, PROD."`
}

// Execute executes scan command
func (v *scanDbCommand) Execute(_ []string) (err error) {
	if err = v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	log.Println("Initiating project...")
	if _, err := os.Stat(v.ProjectDir); os.IsNotExist(err) {
		return fmt.Errorf("ProjectDir=[%v] not found: %w", v.ProjectDir, err)
	}

	if v.Host == "" {
		envDb, err := v.store.Project(v.projectID).Environments().Environment(v.Environment).Servers().Server(v.Host).Catalogs().Catalog(v.Database).LoadEnvironmentCatalog()
		if err != nil {
			return err
		}
		if v.Driver == "" {
			if envDb.Server.Driver == "" {
				return fmt.Errorf("env DB has no driver specified: %v @ %v", v.Database, v.Host)
			}
			v.Driver = envDb.Server.Driver
		} else if envDb.Server.Driver != v.Driver {
			return fmt.Errorf("requested driver %v is different from one used by DB [%v]: %v", v.Driver, v.Database, envDb.Server.Driver)
		}
		v.Host = envDb.Server.Host
		if v.DbModel == "" {
			v.DbModel = envDb.DbModel
		}
	}

	options := []string{"mode=" + dbconnection.ModeReadOnly}
	if v.Port != 0 {
		options = append(options, "port="+strconv.Itoa(v.Port))
	}

	var connParams dbconnection.Params

	ctx := context.Background()

	switch v.Driver {
	case "sqlite3":
		serverRef := models.ServerReference{Driver: v.Driver, Host: "localhost"}
		dbCatalog, err := v.store.Project(v.projectID).DbServers().DbServer(serverRef).Catalogs().DbCatalog(v.Database).LoadDbCatalogSummary(ctx)
		if err != nil {
			return fmt.Errorf("failed to load DB catalog: %w", err)
		}
		if dbCatalog == nil {
			return fmt.Errorf("db catalog not found for server=%+v, catalog=%v", serverRef, v.Database)
		}
		fullPath, err := homedir.Expand(dbCatalog.Path)
		if err != nil {
			return fmt.Errorf("failed to expand path for SQLite3 connection string: %w", err)
		}
		connParams = dbconnection.NewSQLite3ConnectionParams(fullPath, v.Database, dbconnection.ModeReadOnly)
	default:
		if connParams, err = dbconnection.NewConnectionString(v.Driver, v.Host, v.User, v.Password, v.Database, options...); err != nil {
			return fmt.Errorf("invalid connection string: %v", err)
		}
	}

	if v.DbModel == "" {
		v.DbModel = v.Database
	}

	var datatugProject *models.DatatugProject
	projectStore := v.store.Project(v.projectID)
	if datatugProject, err = api.UpdateDbSchema(context.Background(), projectStore, v.projectID, v.Environment, v.Driver, v.DbModel, connParams); err != nil {
		return err
	}

	log.Println("Saving project", datatugProject.ID, "...")
	storage.Current, _ = filestore.NewSingleProjectStore(v.ProjectDir, v.projectID)
	var dal storage.Store
	if dal, err = storage.NewDatatugStore(""); err != nil {
		return err
	}
	if err = dal.Project(datatugProject.ID).SaveProject(context.Background(), *datatugProject); err != nil {
		err = fmt.Errorf("failed to save datatug project [%v]: %w", v.projectID, err)
		return err
	}

	return err
}
