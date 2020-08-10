package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
	"github.com/datatug/datatug/packages/store/filestore"
	"log"
	"os"
)

func init() {
	_, err := Parser.AddCommand("scan",
		"Adds or updates DB metadata",
		"Adds or updates DB metadata from a specific server in a specific environment",
		&scanDb)
	if err != nil {
		log.Fatal(err)
	}
}

// scanDbCommand defines parameters for scan command
type scanDbCommand struct {
	ProjectDir  string `short:"t" long:"directory"  required:"true" description:"Path to DataTug project directory."`
	Driver      string `short:"d" long:"driver" required:"true" description:"Supported values: sqlserver."`
	Host        string `short:"s" long:"server" required:"true" default:"localhost" description:"Network server name."`
	Port        int    `long:"port" description:"DbServer network port, if not specified default is used."`
	User        string `short:"u" long:"user" description:"User name to login to DB."`
	Password    string `short:"p" long:"password" description:"Password to login to DB."`
	Database    string `long:"db" required:"true" description:"Name of database to be scanned."`
	DbModel     string `long:"dbmodel" required:"false" description:"Name of DB model, is required for newly scanned databases."`
	Environment string `long:"env" required:"true" description:"Specify environment the DB belongs to. E.g.: LOCAL, DEV, SIT, UAT, PERF, PROD."`
}

var scanDb scanDbCommand

// Execute executes scan command
func (v *scanDbCommand) Execute(_ []string) (err error) {
	log.Println("Initiating project...")
	if _, err := os.Stat(v.ProjectDir); os.IsNotExist(err) {
		return err
	}

	connString := execute.NewConnectionString(v.Host, v.User, v.Password, v.Database, v.Port)

	loader, projectID := filestore.NewSingleProjectLoader(v.ProjectDir)

	var dataTugProject *models.DataTugProject
	if dataTugProject, err = api.UpdateDbSchema(loader, projectID, v.Environment, v.Driver, v.DbModel, connString); err != nil {
		return err
	}

	log.Println("Saving project", dataTugProject.ID, "...")
	store.Current, projectID = filestore.NewSingleProjectStore(v.ProjectDir, projectID)
	if err = store.Current.Save(*dataTugProject); err != nil {
		err = fmt.Errorf("failed to save datatug project [%v]: %w", projectID, err)
		return err
	}

	return err
}
