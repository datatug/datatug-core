package commands

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/gosuri/uitable"
	cliv3 "github.com/urfave/cli/v3"
	"os"
	"path"
	"strings"
)

func showCommandArgs() *cliv3.Command {
	return &cliv3.Command{
		Name:        "show",
		Usage:       "Displays project data",
		Description: "Outputs project data in human readable format",
		Action: func(ctx context.Context, c *cliv3.Command) error {
			v := &showProjectCommand{}
			return v.Execute(nil)
		},
	}
}

// showProjectCommand defines parameters for show project consoleCommand
type showProjectCommand struct {
	projectBaseCommand
}

// Execute executes show project consoleCommand
func (v *showProjectCommand) Execute(_ []string) error {
	if err := v.initProjectCommand(projectCommandOptions{projNameOrDirRequired: true}); err != nil {
		return err
	}
	project, err := v.store.GetProjectStore(v.projectID).LoadProject(context.Background())
	if err != nil {
		return fmt.Errorf("failed to load project from [%v]: %w", v.ProjectDir, err)
	}
	var wd string
	if wd, err = os.Getwd(); err != nil {
		return err
	}
	w := os.Stdout
	_, _ = fmt.Fprintln(w, "GetProjectStore: ", path.Join(wd, v.ProjectDir))
	for _, env := range project.Environments {
		_, _ = fmt.Fprintln(w, "\t🌎 Environment: ", env.ID)
		for _, dbServer := range env.DbServers {
			_, _ = fmt.Fprintln(w, "\t\t🛢️🛢️ DB server: ", dbServer.ID())
			for _, db := range dbServer.Catalogs {
				_, _ = fmt.Fprintln(w, "\t\t\t🛢️ DB: ", db)
			}
		}
	}
	_, _ = fmt.Fprintln(w, "DB servers: ", len(project.DbServers))
	for _, dbServer := range project.DbServers {
		_, _ = fmt.Fprintf(w, "\t🛢️🛢️ %v: %v\n", dbServer.Server.Driver, dbServer.Server.Address())
		for _, db := range dbServer.Catalogs {
			_, _ = fmt.Fprintln(w, "\t\t🛢️ Database: ", db.ID)
			for _, schema := range db.Schemas {
				_, _ = fmt.Fprintln(w, "\t\t\t Schema: ", db.ID)
				printCols := func(t *models.Table) {
					if len(t.Columns) > 0 {
						table := uitable.New()
						for _, c := range t.Columns {
							s := strings.ToUpper(c.DbType)
							if c.CharMaxLength != nil && c.DbType != "text" {
								s = fmt.Sprintf("%v(%v)", s, *c.CharMaxLength)
							}
							table.AddRow("\t\t\t\t\t\t"+c.Name, s)
						}
						_, _ = fmt.Fprintf(w, "\t\t\t\t\tColumns (%v):\n", len(t.Columns))
						_, _ = fmt.Fprintln(w, table.String())
					}
				}
				printTable := func(singular string, t *models.Table) {
					_, _ = fmt.Fprintf(w, "\t\t\t\t📄 %v: %v.%v\n", singular, t.Schema, t.Name)
					if t.PrimaryKey != nil {
						_, _ = fmt.Fprintf(w, "\t\t\t\t\t🔑 Primary key: %v (%v)\n", t.PrimaryKey.Name, strings.Join(t.PrimaryKey.Columns, ", "))
					}
					if len(t.ForeignKeys) > 0 {
						_, _ = fmt.Fprintf(w, "\t\t\t\t\t🔗 Foreign keys (%v):", len(t.ForeignKeys))
						for _, fk := range t.ForeignKeys {
							_, _ = fmt.Fprintf(w, "\t\t\t\t\t\t (%v) %v.%v @ %v\n", strings.Join(fk.Columns, ", "), fk.RefTable.Schema, fk.RefTable.Name, fk.Name)
						}
					}
					for _, refBy := range t.ReferencedBy {
						for _, fk := range refBy.ForeignKeys {
							_, _ = fmt.Fprintf(w, "\t\t\t\t\t📎 Referenced by: %v.%v (%v) @ %v\n", refBy.Schema, refBy.Name, strings.Join(fk.Columns, ", "), fk.Name)
						}
					}
					printCols(t)
				}
				for _, t := range schema.Tables {
					printTable("Table", t)
				}
				for _, t := range schema.Views {
					printTable("View", t)
				}
			}
		}
	}
	return err
}
