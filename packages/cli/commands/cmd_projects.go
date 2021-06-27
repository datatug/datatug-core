package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/store"
	"github.com/datatug/datatug/packages/store/filestore"
	"log"
	"os"
)

func init() {
	projectsCommand, err := Parser.AddCommand("projects",
		"List registered projects",
		"",
		&projectsCommand{})
	if err != nil {
		log.Fatal(err)
	}
	projectsCommand.SubcommandsOptional = true
	_, err = projectsCommand.AddCommand("add",
		"Adds a <name>=<path> to list of known projects",
		"",
		&addProjectCommand{},
	)
	if err != nil {
		log.Fatal(err)
	}
}

type projectsCommand struct {
}

func getProjPathsByID(config ConfigFile) (pathsByID map[string]string) {
	pathsByID = make(map[string]string, len(config.Projects))
	for id, p := range config.Projects {
		pathsByID[id] = p.Path
	}
	return
}

func (v *projectsCommand) Execute(_ []string) error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if err = printConfig(config, os.Stdout); err != nil {
		return err
	}
	pathsByID := getProjPathsByID(config)
	if store.Current, err = filestore.NewStore(pathsByID); err != nil {
		return err
	}
	var dal store.Interface
	if dal, err = store.NewDatatugStore(""); err != nil {
		return err
	}
	projects, err := dal.GetProjects()
	if err != nil {
		fmt.Println("Failed to load projects: ", err)
	}
	for _, p := range projects {
		fmt.Printf("ID=%v, Title: %v\n", p.ID, p.Title)
	}
	//for _, p := range config.Stores {
	//	if p.Title == "" {
	//		fmt.Println(p.Path)
	//	} else {
	//		fmt.Println(p.Path, ":", p.Title)
	//	}
	//}
	return nil
}
