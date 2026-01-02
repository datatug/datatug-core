package storage

import (
	"fmt"
	"strings"
)

const (
	RepoRootDataTugFileName = ".datatug.yaml"
	BoardsFolder            = "boards"
	ProjectSummaryFileName  = "datatug-project.json"
	DataFolder              = "data"
	DbsFolder               = "dbs"
	EnvDbCatalogsFolder     = "catalogs"
	DbModelsFolder          = "dbmodels"
	EntitiesFolder          = "entities"
	EnvironmentsFolder      = "environments"
	QueriesFolder           = "queries"
	RecordsetsFolder        = "recordsets"
	ServersFolder           = "servers"
	SchemasFolder           = "schemas"
	//TablesFolder            = "tables"
	//ViewsFolder             = "views"
	//DatatugFolder          = "datatug"
)

func JsonFileName(id, suffix string) string {
	if suffix == "" {
		return id + ".json"
	}
	switch suffix {
	case
		BoardFileSuffix,
		DbCatalogFileSuffix,
		DbCatalogObjectFileSuffix,
		DbCatalogRefsFileSuffix,
		DbModelFileSuffix,
		DbServerFileSuffix,
		RecordsetFileSuffix,
		EntityFileSuffix,
		ServerFileSuffix,
		ColumnsFileSuffix,
		QueryFileSuffix:
		// OK
	default:
		panic(fmt.Sprintf("unknown JSON file suffix=[%v], id=[%v]", suffix, id))

	}
	return fmt.Sprintf("%v.%v.json", id, suffix)
}

func GetProjItemIDFromFileName(fileName string) (id string, suffix string) {
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
	BoardFileSuffix           = "board"
	DbCatalogFileSuffix       = "db"
	DbCatalogObjectFileSuffix = "objects"
	DbCatalogRefsFileSuffix   = "refs"
	DbModelFileSuffix         = "dbmodel"
	//DbSchemaFileSuffix        = "schema"
	DbServerFileSuffix  = "dbserver"
	RecordsetFileSuffix = "recordset"
	EntityFileSuffix    = "entity"
	ServerFileSuffix    = "server"
	ColumnsFileSuffix   = "columns"
	QueryFileSuffix     = "query"
)

const (
	EnvironmentSummaryFileName = "environment-summary.json"
)
