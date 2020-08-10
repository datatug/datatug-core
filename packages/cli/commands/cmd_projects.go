package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/store"
	"github.com/datatug/datatug/packages/store/filestore"
	"log"
	"os"
)

func init() {
	_, err := Parser.AddCommand("projects",
		"List registered projects",
		"",
		&projectsCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

type projectsCommand struct {
}

func (v *projectsCommand) Execute(_ []string) error {
	config, err := getConfig()
	if err != nil {
		return fmt.Errorf("failed to get config: %w", err)
	}
	if err = printConfig(config, os.Stdout); err != nil {
		return err
	}
	projectPaths := make([]string, len(config.Projects))
	for i, p := range config.Projects {
		projectPaths[i] = p.Path
	}
	if store.Current, err = filestore.NewStore(projectPaths); err != nil {
		return err
	}
	projects, err := store.Current.GetProjects()
	if err != nil {
		fmt.Println("Failed to load projects: ", err)
	}
	for _, p := range projects {
		fmt.Printf("ID=%v, Title: %v\n", p.ID, p.Title)
	}
	//for _, p := range config.Projects {
	//	if p.Title == "" {
	//		fmt.Println(p.Path)
	//	} else {
	//		fmt.Println(p.Path, ":", p.Title)
	//	}
	//}
	return nil
}
