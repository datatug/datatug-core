package datatug

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestClickHouseServerDescriptorAndCatalogRoundTrip(t *testing.T) {
	const serverJSON = `{"driver":"clickhouse","catalogs":["analytics"]}`
	var server EnvDbServer
	if err := json.Unmarshal([]byte(serverJSON), &server); err != nil {
		t.Fatal(err)
	}
	if err := server.Validate(); err != nil {
		t.Fatal("descriptor-backed ClickHouse server refused", err)
	}
	if server.GetID() != "clickhouse" || server.Host != "" || server.Port != 0 {
		t.Fatal("descriptor gained connection details or lost its driver identity")
	}
	encoded, err := json.Marshal(server)
	if err != nil || string(encoded) != serverJSON {
		t.Fatal("descriptor server round trip changed", err)
	}
	servers := EnvDbServers{&server}
	if err := servers.Validate(); err != nil || servers.GetByServerRef(ServerRef{Driver: "clickhouse"}) != &server {
		t.Fatal("descriptor server cannot be resolved", err)
	}
	if err := (EnvDbServers{&server, &server}).Validate(); err == nil {
		t.Fatal("duplicate descriptor server IDs admitted")
	}
	catalog := DbCatalog{DbCatalogBase: DbCatalogBase{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "analytics"}},
		Driver:      "clickhouse", Path: "connections/local/analytics.json", DbModel: "analytics",
	}}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	var restored DbCatalog
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil || !reflect.DeepEqual(restored, catalog) {
		t.Fatal("saved ClickHouse catalog descriptor reference changed", err)
	}
}

func TestClickHouseHostOptionalValidationRetainsRefusals(t *testing.T) {
	for _, ref := range []ServerRef{{Driver: "clickhouse"}, {Driver: "clickhouse", Host: "example.invalid", Port: 9440}} {
		if err := ref.Validate(); err != nil {
			t.Fatal("supported ClickHouse server form refused", err)
		}
	}
	for _, ref := range []ServerRef{
		{Driver: "clickhouse", Port: -1}, {Driver: "ClickHouse"}, {Driver: "clickhouse+http"}, {Driver: "unknown"},
	} {
		if err := ref.Validate(); err == nil {
			t.Fatal("invalid server driver or port admitted")
		}
	}
	if err := (ServerRef{Driver: "unknown"}).Validate(); err == nil || !strings.Contains(err.Error(), "clickhouse") {
		t.Fatal("accepted-driver guidance omits ClickHouse")
	}
}
