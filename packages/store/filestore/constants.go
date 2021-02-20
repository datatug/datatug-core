package filestore

import "fmt"

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

const (
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

const (
	// EntityPrefix = "entity"
	EntityPrefix = "entity"
	// BoardPrefix  = "board"
	BoardPrefix = "board"
)
