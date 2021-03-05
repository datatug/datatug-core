package execute

import (
	"database/sql"
	"encoding/binary"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
	"github.com/google/uuid"
	"log"
	"strings"
	"sync"
	"time"
)

// Executor executes DataTug commands
type Executor struct {
	getDbByID func(envID, dbID string) (*dto.EnvDb, error)
}

// NewExecutor creates new executor
func NewExecutor(getDbByID func(envID, dbID string) (*dto.EnvDb, error)) Executor {
	return Executor{getDbByID}
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
	if recordset, err = executeCommand(command, e.getDbByID); err != nil {
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
			if recordset, commandErr = executeCommand(cmd, e.getDbByID); commandErr != nil {
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

func executeCommand(command RequestCommand, getDbByID func(envID, dbID string) (*dto.EnvDb, error)) (recordset models.Recordset, err error) {

	var dbServer models.ServerReference

	if getDbByID != nil {
		var envDb *dto.EnvDb
		if envDb, err = getDbByID(command.Env, command.DB); err != nil {
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
	var connStr ConnectionString
	connStr, err = NewConnectionString(
		dbServer.Driver,
		dbServer.Host,
		command.Credentials.Username,
		command.Credentials.Password,
		command.DB,
		options...,
	)

	if err != nil {
		err = fmt.Errorf("invalid connection parameters: %w", err)
		return
	}

	fmt.Println(connStr)
	//fmt.Println(envDb.ServerReference.Driver, connStr.String())
	//fmt.Println(command.Text)
	var db *sql.DB
	if db, err = sql.Open(dbServer.Driver, connStr.String()); err != nil {
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

	started := time.Now()

	var rows *sql.Rows
	if rows, err = db.Query(command.Text); err != nil {
		log.Printf("Failed to execute %v: %v", command.Text, err)
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
					if dbServer.Driver == "sqlserver" {
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
