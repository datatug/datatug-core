package commands

import (
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/sql2csv"
	"github.com/google/uuid"
	"github.com/gosuri/uitable"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

func init() {
	_, err := Parser.AddCommand("execute",
		"Runs SQL",
		"The `execute` command executes SQL command",
		&executeSQLCommand{})
	if err != nil {
		log.Fatal(err)
	}
}

// executeSQLCommand defines parameters for execute SQL command
type executeSQLCommand struct {
	Driver       string `long:"driver" required:"true"`
	Host         string `short:"t" long:"host" required:"true" default:"localhost"`
	User         string `short:"u" long:"user"`
	Port         string `long:"port"`
	Password     string `short:"p" long:"password"`
	Schema       string `short:"s" long:"schema"`
	CommandText  string `short:"q" long:"command-text" required:"true"`
	OutputPath   string `short:"o" long:"output-path"`
	OutputFormat string `short:"f" long:"output-format" choice:"csv" default:"csv"`
}

// Execute - executes SQL command
func (v *executeSQLCommand) Execute(args []string) error {
	fmt.Printf("Validating (%+v): %v\n", v, args)
	var err error

	var port int
	if v.Port != "" {
		if port, err = strconv.Atoi(v.Port); err != nil {
			return err
		}
	}

	connString := execute.NewConnectionString(v.Host, v.User, v.Password, v.Schema, port)

	var db *sql.DB

	log.Printf("Connecting to: %v\n", regexp.MustCompile("password=.+?(;|$)").ReplaceAllString(connString.String(), "password=******"))

	// Create connection pool

	if db, err = sql.Open(v.Driver, connString.String()); err != nil {
		log.Fatal("Error creating connection pool: " + err.Error())
	}
	// Close the database connection pool after command executes
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close DB: %v", err)
		}
	}()
	log.Println("Connected")

	if strings.HasPrefix(v.CommandText, "*=") {
		v.CommandText = "SELECT * FROM " + strings.TrimLeft(v.CommandText, "*=")
	}

	var rows *sql.Rows
	if rows, err = db.Query(v.CommandText); err != nil {
		log.Printf("Failed to execute %v: %v", v.CommandText, err)
		return err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows reader: %v", err)
		}
	}()

	var columnTypes []*sql.ColumnType
	if columnTypes, err = rows.ColumnTypes(); err != nil {
		return err
	}
	colNames := make([]interface{}, len(columnTypes))
	colSpec := make([]string, len(columnTypes))
	colTypeNames := make([]string, len(columnTypes))
	for i, colType := range columnTypes {
		colNames[i] = colType.Name()
		colTypeNames[i] = colType.DatabaseTypeName()
		colSpec[i] = fmt.Sprintf("%v: %v", colNames[i], colTypeNames[i])
	}
	log.Printf(`
----------
%v
----------
--	Columns:
--		%v
`, v.CommandText, strings.Join(colSpec, "\n--\t\t"))

	var handler rowsHandler
	if v.OutputPath != "" {
		handler = csvHandler{path: v.OutputPath}
	} else {
		handler = writerHandler{os.Stdout}
	}
	return handler.Process(columnTypes, rows)
}

type rowsHandler interface {
	Process(columnTypes []*sql.ColumnType, rows *sql.Rows) (err error)
}

type csvHandler struct {
	path string
}

func (handler csvHandler) Process(_ []*sql.ColumnType, rows *sql.Rows) (err error) {
	converter := sql2csv.NewConverter(rows)
	return converter.WriteFile(handler.path)
}

type writerHandler struct {
	w io.Writer
}

func (handler writerHandler) Process(columnTypes []*sql.ColumnType, rows *sql.Rows) (err error) {

	//if colNames, err = rows.Columns(); err != nil {
	//	return err
	//}

	//if rowsAffected, err := result.RowsAffected(); err != nil {
	//	return err
	//} else {
	//	log.Printf("%v rows affected", rowsAffected)
	//}

	// https://kylewbanks.com/blog/query-result-to-map-in-golang

	values := make([]interface{}, len(columnTypes))
	valPointers := make([]interface{}, len(values))
	for i := range columnTypes {
		valPointers[i] = &values[i]
	}

	row := make([]interface{}, len(values))

	table := uitable.New()
	table.MaxColWidth = 50

	colNames := make([]interface{}, len(columnTypes))
	for i, colType := range columnTypes {
		colNames[i] = colType.Name()
	}

	table.AddRow(colNames...)
	for i, colName := range colNames {
		colNames[i] = strings.Repeat("-", len(colName.(string)))
	}
	table.AddRow(colNames...)
	for rows.Next() {
		if err = rows.Scan(valPointers...); err != nil {
			return err
		}
		for i, val := range values {
			switch columnTypes[i].DatabaseTypeName() {
			case "UNIQUEIDENTIFIER":
				var v uuid.UUID
				{
				}
				if v, err = uuid.FromBytes(val.([]byte)); err != nil {
					return err
				}
				row[i] = v.String()
			default:
				row[i] = val
			}
		}
		table.AddRow(row...)
	}
	if _, err = fmt.Fprintf(handler.w, "%v records:\n%v\n", len(table.Rows), table); err != nil {
		return err
	}

	return err
}
