package execute

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/datatug/datatug-core/pkg/dbconnection"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/google/uuid"
	"github.com/mitchellh/go-homedir"
	"github.com/strongo/slice"
)

// Executor executes DataTug commands
type Executor struct {
	getDbByID         func(envID, dbID string) (*models.EnvDb, error)
	getCatalogSummary func(server models.ServerReference, catalogID string) (*models.DbCatalogSummary, error)
}

// NewExecutor creates new executor
func NewExecutor(
	getDbByID func(envID, dbID string) (*models.EnvDb, error),
	getCatalogSummary func(server models.ServerReference, catalogID string) (*models.DbCatalogSummary, error),
) Executor {
	return Executor{
		getDbByID:         getDbByID,
		getCatalogSummary: getCatalogSummary,
	}
}

// Execute executes DataTug commands
func (e Executor) Execute(request Request) (response Response, err error) {
	if len(request.Commands) == 1 {
		return e.ExecuteSingle(request.Commands[0])
	}
	return e.executeMulti(request)
}

// ExecuteSingle executes single DB command
func (e Executor) ExecuteSingle(command RequestCommand) (response Response, err error) {
	var recordset models.Recordset
	if recordset, err = e.executeCommand(command); err != nil {
		return
	}
	response.Commands = []*CommandResponse{
		{
			Items: []CommandResponseItem{
				{
					Type:  "recordset",
					Value: recordset,
				},
			},
		},
	}
	response.Duration = recordset.Duration
	return
}

func (e Executor) executeMulti(request Request) (response Response, err error) {
	started := time.Now()
	var wg sync.WaitGroup
	wg.Add(len(request.Commands))
	response.Commands = make([]*CommandResponse, 0, len(request.Commands))
	for _, command := range request.Commands {
		var commandResponse CommandResponse
		response.Commands = append(response.Commands, &commandResponse)
		go func(cmd RequestCommand) {
			var (
				recordset  models.Recordset
				commandErr error
			)
			if recordset, commandErr = e.executeCommand(cmd); commandErr != nil {
				err = commandErr
				wg.Done()
				return
			}
			commandResponse.Items = []CommandResponseItem{
				{
					Type:  "recordset",
					Value: recordset,
				},
			}
			wg.Done()
		}(command)
	}
	wg.Wait()
	response.Duration = time.Since(started)
	return
}

var reParameter = regexp.MustCompile(`@\w+`)

func (e Executor) executeCommand(command RequestCommand) (recordset models.Recordset, err error) {

	var dbServer models.ServerReference

	if e.getDbByID != nil {
		var envDb *models.EnvDb
		if envDb, err = e.getDbByID(command.Env, command.DB); err != nil {
			return
		}
		dbServer = envDb.Server
	} else {
		strings.Split(command.DB, ":")
		dbServer = models.ServerReference{Host: command.Host, Port: command.Port, Driver: command.Driver}
		if err = dbServer.Validate(); err != nil {
			return recordset, fmt.Errorf("execute command does not have valid server parameters: %w", err)
		}
	}
	var options []string
	if dbServer.Port != 0 {
		options = append(options, fmt.Sprintf("mode=%v", dbServer.Port))
	}
	var connParams dbconnection.Params

	switch dbServer.Driver {
	case "sqlite3":
		var catalogSummary *models.DbCatalogSummary
		if catalogSummary, err = e.getCatalogSummary(dbServer, command.DB); err != nil {
			return models.Recordset{}, err
		}
		fullPath, err := homedir.Expand(catalogSummary.Path)
		if err != nil {
			err = fmt.Errorf("failed to expand path for SQLite3 connection string: %w", err)
			return models.Recordset{}, err
		}
		connParams = dbconnection.NewSQLite3ConnectionParams(fullPath, command.DB, dbconnection.ModeReadOnly)
	default:
		connParams, err = dbconnection.NewConnectionString(
			dbServer.Driver,
			dbServer.Host,
			command.Username,
			command.Password,
			command.DB,
			options...,
		)
	}

	if err != nil {
		err = fmt.Errorf("invalid connection parameters: %w", err)
		return
	}

	fmt.Println(connParams)
	//fmt.Println(envDb.ServerReference.Driver, connParams.String())
	//fmt.Println(command.Text)
	var db *sql.DB
	if db, err = sql.Open(dbServer.Driver, connParams.ConnectionString()); err != nil {
		return
	}
	//defer func() {
	//	if err := db.Close(); err != nil {
	//		log.Printf("Failed to close DB: %v", err)
	//	}
	//}()

	//var stmt *sql.Stmt
	//if stmt, err = db.Prepare(command.Text); err != nil {
	//	return
	//}

	queryText := command.Text
	var args []interface{}
	for i, p := range command.Parameters {
		k := "@" + p.ID
		j := strings.Index(queryText, k)
		if j >= 0 {
			queryText = strings.ReplaceAll(queryText, k, ":"+strconv.Itoa(i+1))
		}
		args = append(args, p.Value)
	}

	if recordset, err = e.executeQuery(db, dbServer.Driver, queryText, args); err != nil {
		parameters := reParameter.FindAllString(queryText, -1)
		if strings.HasPrefix(err.Error(), "not enough args to execute query:") {
			if len(parameters) > 0 {
				for i, p := range parameters {
					if slice.Index(parameters[:i], p) >= 0 {
						continue
					}
					args = append(args, nil)
					queryText = strings.ReplaceAll(queryText, p, fmt.Sprintf(":%v", len(args)))
				}
				return e.executeQuery(db, dbServer.Driver, queryText, args)
			}
		}
		return
	}
	return
}

// TODO: Return slice of recordsets
func (e Executor) executeQuery(db *sql.DB, driver, text string, args []interface{}) (recordset models.Recordset, err error) {
	started := time.Now()
	var rows *sql.Rows
	if rows, err = db.Query(text, args...); err != nil {
		log.Printf("Failed to execute %v: %v", text, err)
		return
	}
	//defer func() {
	//	if err := rows.Close(); err != nil {
	//		log.Printf("Failed to close rows reader: %v", err)
	//	}
	//}()
	var columnTypes []*sql.ColumnType
	if columnTypes, err = rows.ColumnTypes(); err != nil {
		return
	}

	for _, col := range columnTypes {
		recordset.Columns = append(recordset.Columns, models.RecordsetColumn{
			Name:   col.Name(),
			DbType: col.DatabaseTypeName(),
		})
	}
	var rowNumber int
	for rows.Next() {
		rowNumber++
		row := make([]interface{}, len(recordset.Columns))
		valPointers := make([]interface{}, len(recordset.Columns))
		for i := range row {
			valPointers[i] = &row[i]
		}
		if err = rows.Scan(valPointers...); err != nil {
			err = fmt.Errorf("failed to scan values for row #%v: %w", rowNumber, err)
			return
		}
		for i, col := range recordset.Columns {
			switch col.DbType {
			case "UNIQUEIDENTIFIER":
				if row[i] != nil {
					v := row[i].([]byte)
					if driver == "sqlserver" {
						// Workaround for GUID - see inspiration here https://github.com/satori/go.uuid/issues/19
						binary.BigEndian.PutUint32(v[0:4], binary.LittleEndian.Uint32(v[0:4]))
						binary.BigEndian.PutUint16(v[4:6], binary.LittleEndian.Uint16(v[4:6]))
						binary.BigEndian.PutUint16(v[6:8], binary.LittleEndian.Uint16(v[6:8]))
					}
					if row[i], err = uuid.FromBytes(v); err != nil {
						return
					}
				}
			}
		}
		recordset.Rows = append(recordset.Rows, row)
	}
	recordset.Duration = time.Since(started)
	return
}
