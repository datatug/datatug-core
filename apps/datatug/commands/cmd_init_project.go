package commands

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/datatug/datatug/packages/storage/filestore"
	cliv3 "github.com/urfave/cli/v3"
	"log"
	"os"
	"os/user"
	"path"
	"time"
)

var projectIdArg = &cliv3.StringArg{Name: "project"}
var projectPathArg = &cliv3.StringArg{Name: "projectPath"}

func initCommand() *cliv3.Command {
	return &cliv3.Command{
		Name:        "init",
		Usage:       "Creates a new datatug project",
		Description: "Creates a new datatug project in specified directory using a connection to some database",
		Action:      initCommandAction,
		Arguments: []cliv3.Argument{
			projectIdArg,
			projectPathArg,
		},
	}
}

func initCommandAction(ctx context.Context, c *cliv3.Command) (err error) {
	log.Println("Initiating project...")

	projectDir := c.Arguments[1].Get().(string)
	if err = os.MkdirAll(projectDir, 0777); err != nil {
		return err
	}
	dataTugDirPath := path.Join(projectDir, "datatug")
	var fileInfo os.FileInfo
	if fileInfo, err = os.Stat(dataTugDirPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to get info about %v: %w", dataTugDirPath, err)
		}
	} else if fileInfo.IsDir() {
		return fmt.Errorf("the folder already contains datatug project: %v", dataTugDirPath)
	} else {
		return fmt.Errorf("the folder  contains a `datatug` file, this name is reserver for DataTug directory: %v", dataTugDirPath)
	}

	//var port int
	//if v.Port != "" {
	//	if port, err = strconv.Atoi(v.Port); err != nil {
	//		return err
	//	}
	//}
	//
	//connString := updateUrlConfig.NewConnectionString(v.Host, v.User, v.Password, v.Database, port)
	//
	//var db *sql.DB
	//
	//if db, err = sql.Open(v.Driver, connString.String()); err != nil {
	//	log.Fatal("Error creating DB connection: " + err.Error())
	//}
	//
	//// Close the database connection pool after consoleCommand executes
	//defer func() { _ = db.Close() }()
	//
	//server := models.ServerReference{Driver: v.Driver, Host: v.Host, Port: port}
	//informationSchema := schemer.NewInformationSchema(server, db)
	//
	//var database *models.Database
	//if database, err = informationSchema.GetDatabase(v.Database); err != nil {
	//	return fmt.Errorf("failed to get database metadata: %w", err)
	//}

	projectID := projectIdArg.Get().(string)

	storage.Current, projectID = filestore.NewSingleProjectStore(projectDir, projectID)
	datatugProject := models.DatatugProject{
		ProjectItem: models.ProjectItem{
			ProjItemBrief: models.ProjItemBrief{ID: projectID},
			Access:        "private",
		},
		//Environments: []*models.Environment{
		//	{
		//		ProjectItem: models.ProjectItem{ID: v.Environment},
		//		DbServers: []*models.EnvDbServer{
		//			{
		//				Driver:    v.Driver,
		//				Host:      server.Host,
		//				Port:      server.Port,
		//				DatabaseDiffs: []string{database.ID},
		//			},
		//		},
		//		DatabaseDiffs: []*models.Database{
		//			database,
		//		},
		//	},
		//},
	}
	var currentUser *user.User
	if currentUser, err = user.Current(); err != nil {
		return err
	}
	if currentUser != nil {
		datatugProject.Created = &models.ProjectCreated{
			//ByName:     currentUser.Name,
			//ByUsername: currentUser.Username,
			At: time.Now(),
		}
	}

	var dal storage.Store
	if dal, err = storage.NewDatatugStore(""); err != nil {
		return err
	}
	if err = dal.GetProjectStore(projectID).SaveProject(context.Background(), datatugProject); err != nil {
		return err
	}
	return err
}
