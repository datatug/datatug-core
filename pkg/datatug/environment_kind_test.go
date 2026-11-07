package datatug

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentKind(t *testing.T) {
	for _, kind := range append([]EnvironmentKind{""}, EnvironmentKinds...) {
		assert.NoError(t, kind.Validate(), kind)
	}
	assert.Error(t, EnvironmentKind("prod").Validate())

	var env Environment
	require.NoError(t, json.Unmarshal([]byte(`{"id":"factory-a-prod","kind":"production","dbServers":[]}`), &env))
	assert.Equal(t, EnvironmentKindProduction, env.Kind)
	assert.NoError(t, env.Validate())
	out, err := json.Marshal(env)
	require.NoError(t, err)
	assert.Contains(t, string(out), `"kind":"production"`)

	env.Kind = "prod"
	assert.Error(t, env.Validate())
}

func TestDbCatalogTenants(t *testing.T) {
	catalog := DbCatalogBase{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "factory"}}, Driver: "sqlite3", Path: "f.db", DbModel: "factory", Tenants: []string{"factory-a"}}
	assert.NoError(t, catalog.Validate())
	catalog.Tenants = []string{"a", "a"}
	assert.Error(t, catalog.Validate())
	catalog.Tenants = []string{""}
	assert.Error(t, catalog.Validate())
}
