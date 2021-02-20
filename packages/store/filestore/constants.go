package filestore

import (
	"fmt"
	"strings"
)

// DatatugFolder defines a folder name in a repo where to store DataTug project
const (
	ProjectSummaryFileName = "datatug-project.json"
	DatatugFolder          = "datatug"
	BoardsFolder           = "boards"
	DataFolder             = "data"
	QueriesFolder          = "queries"
	RecordsetsFolder       = "recordsets"
	DbCatalogsFolder       = "dbcatalogs"
	DbFolder               = "db"
	DbModelsFolder         = "dbmodels"
	EntitiesFolder         = "entities"
	EnvironmentsFolder     = "environments"
	ServersFolder          = "servers"
	SchemasFolder          = "tables"
	TablesFolder           = "tables"
	ViewsFolder            = "tables"
)

func jsonFileName(id, suffix string) string {
	switch suffix {
	case
		boardFileSuffix,
		dbCatalogFileSuffix,
		dbModelFileSuffix,
		dbServerFileSuffix,
		recordsetFileSuffix,
		environmentFileSuffix,
		entityFileSuffix,
		serverFileSuffix,
		columnsFileSuffix,
		queryFileSuffix:
		// OK
	default:
		panic("unknown JSON file suffix")

	}
	return fmt.Sprintf("%v.%v.json", id, suffix)
}

func getProjItemIdFromFileName(fileName string) (id string, suffix string) {
	parts := strings.Split(fileName, ".")
	if len(parts) < 3 {
		return "", ""
	}
	suffixIndex := len(parts) - 2
	suffix = parts[suffixIndex]
	id = strings.Join(parts[:suffixIndex], ".")
	return
}

const (
	boardFileSuffix       = "board"
	dbCatalogFileSuffix   = "db"
	dbModelFileSuffix     = "dbmodel"
	dbServerFileSuffix    = "dbserver"
	recordsetFileSuffix   = "recordset"
	environmentFileSuffix = "env"
	entityFileSuffix      = "entity"
	serverFileSuffix      = "server"
	columnsFileSuffix     = "columns"
	queryFileSuffix       = "q"
)
