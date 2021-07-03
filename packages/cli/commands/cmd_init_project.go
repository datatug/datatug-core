package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/datatug/datatug/packages/storage/filestore"
	"github.com/strongo/random"
	"log"
	"os"
	"os/user"
	"path"
	"time"
)

func init() {
	_, err := Parser.AddCommand("init",
		"Creates a new datatug project",
		"Creates a new datatug project in specified directory using a connection to some database",
		&initProjectCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

// initProjectCommand defines parameters for a command to init a new DataTug project
type initProjectCommand struct {
	projectBaseCommand
	//Driver      string `short:"d" long:"driver" required:"true"`
	//Host        string `short:"s" long:"server" required:"true" default:"localhost"`
	//User        string `short:"u" long:"user"`
	//Port        string `long:"port"`
	//Password    string `short:"p" long:"password"`
	//Database    string `long:"db"`
	//Environment string `long:"env" required:"true" description:"Specify environment the DB belongs to. E.g.: LOCAL, DEV, SIT, UAT, PERF, PROD"`
}

// Execute executes init project command
func (v *initProjectCommand) Execute(_ []string) (err error) {
	log.Println("Initiating project...")

	if err = os.MkdirAll(v.ProjectDir, 0777); err != nil {
		return err
	}
	dataTugDirPath := path.Join(v.ProjectDir, "datatug")
	var fileInfo os.FileInfo
	if fileInfo, err = os.Stat(dataTugDirPath); err != nil {
		if os.IsNotExist(err) {
			err = nil
		} else {
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
	//connString := execute.NewConnectionString(v.Host, v.User, v.Password, v.Database, port)
	//
	//var db *sql.DB
	//
	//if db, err = sql.Open(v.Driver, connString.String()); err != nil {
	//	log.Fatal("Error creating DB connection: " + err.Error())
	//}
	//
	//// Close the database connection pool after command executes
	//defer func() { _ = db.Close() }()
	//
	//server := models.ServerReference{Driver: v.Driver, Host: v.Host, Port: port}
	//informationSchema := schemer.NewInformationSchema(server, db)
	//
	//var database *models.Database
	//if database, err = informationSchema.GetDatabase(v.Database); err != nil {
	//	return fmt.Errorf("failed to get database metadata: %w", err)
	//}

	projectID := random.ID(9)

	storage.Current, projectID = filestore.NewSingleProjectStore(v.ProjectDir, projectID)
	datatugProject := models.DatatugProject{
		ID:     projectID,
		Access: "private",
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
	if err = dal.Project(projectID).SaveProject(datatugProject); err != nil {
		return err
	}
	return err
}
