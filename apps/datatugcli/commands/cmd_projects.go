package commands

import (
	"fmt"
	"github.com/datatug/datatug/apps/datatugcli/config"
	"log"
	"strings"
)

func projectsCommandArgs(p Parser) {
	projectsCommand, err := p.AddCommand("projects",
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
	//Format []string `short:"f" long:"format" description:"Output format, default CSV"`
	All  []bool   `short:"a" long:"all" description:"Output all fields"`
	List []string `short:"f" long:"fields" description:"Comma separate list of fields to output, default is 'id'. Possible values: id, path, title"`
}

func getProjPathsByID(config config.Settings) (pathsByID map[string]string) {
	pathsByID = make(map[string]string, len(config.Projects))
	for _, p := range config.Projects {
		pathsByID[p.ID] = p.Url
	}
	return
}

func (v *projectsCommand) Execute(_ []string) error {
	settings, err := config.GetSettings()
	if err != nil {
		return fmt.Errorf("failed to get settings: %w", err)
	}
	if len(v.List) == 0 {
		if len(v.All) == 1 {
			v.List = []string{"id", "path", "title"}
		} else {
			v.List = []string{"id"}
		}
	}
	fields := make([]string, 0, len(v.List))
	for _, field := range v.List {
		fields = append(fields, strings.Split(field, ",")...)
	}

	for _, project := range settings.Projects {
		line := make([]string, 0, len(v.List))
		for _, field := range fields {
			switch field {
			case "id":
				line = append(line, project.ID)
			case "url":
				line = append(line, project.Url)
			case "title":
				line = append(line, project.Title)
			default:
				return fmt.Errorf("unsupported field: %v", field)
			}
		}
		fmt.Println(strings.Join(line, ","))
	}
	return nil
}
