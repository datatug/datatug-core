package api

import (
	"github.com/datatug/datatug/packages/execute"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
	"github.com/strongo/validation"
)

// GetServerDatabases returns list of databases hosted at a server
func GetServerDatabases(request GetServerDatabasesRequest) (databases []dto.DbCatalog, err error) {
	if err = request.Validate(); err != nil {
		return nil, validation.NewBadRequestError(err)
	}

	executor := execute.NewExecutor(nil)

	command := execute.RequestCommand{
		ServerReference: request.ServerReference,
		Text:            "select name from sys.databases where owner_sid > 0x01",
	}
	var response execute.Response
	if response, err = executor.ExecuteSingle(command); err != nil {
		return nil, err
	}
	recordset := response.Commands[0].Items[0].Value.(models.Recordset)
	databases = make([]dto.DbCatalog, len(recordset.Rows))
	for i, row := range recordset.Rows {
		databases[i] = dto.DbCatalog{
			Name: row[0].(string),
		}
	}
	return
}
