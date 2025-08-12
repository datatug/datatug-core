package commands

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/appconfig"
	cliv3 "github.com/urfave/cli/v3"
	"strings"
)

func projectsCommandAction(_ context.Context, _ *cliv3.Command) error {
	v := &projectsCommand{}
	settings, err := appconfig.GetSettings()
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

func projectsCommandArgs() *cliv3.Command {
	return &cliv3.Command{
		Name:        "projects",
		Usage:       "List registered projects",
		Description: "",
		Action:      projectsCommandAction,
	}
}

type projectsCommand struct {
	//Format []string `short:"f" long:"format" description:"Output format, default CSV"`
	All  []bool   `short:"a" long:"all" description:"Output all fields"`
	List []string `short:"f" long:"fields" description:"Comma separate list of fields to output, default is 'id'. Possible values: id, path, title"`
}

func getProjPathsByID(config appconfig.Settings) (pathsByID map[string]string) {
	pathsByID = make(map[string]string, len(config.Projects))
	for _, p := range config.Projects {
		pathsByID[p.ID] = p.Url
	}
	return
}
