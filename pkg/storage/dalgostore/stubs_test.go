package dalgostore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
)

// TestStubs_ReturnErrNotImplemented drives every datatug.ProjectStore member
// that Task 5 does not implement, asserting each wraps
// dalgostore.ErrNotImplemented rather than panicking or silently no-opping.
func TestStubs_ReturnErrNotImplemented(t *testing.T) {
	ctx := context.Background()
	store := dalgostore.NewProjectStore(newTestDB(), "p1")

	checks := []struct {
		name string
		call func() error
	}{
		{"LoadQueries", func() error { _, err := store.LoadQueries(ctx, ""); return err }},
		{"LoadQuery", func() error { _, err := store.LoadQuery(ctx, "q1"); return err }},
		{"SaveQuery", func() error { return store.SaveQuery(ctx, &datatug.QueryDefWithFolderPath{}) }},
		{"DeleteQuery", func() error { return store.DeleteQuery(ctx, "q1") }},

		{"LoadBoards", func() error { _, err := store.LoadBoards(ctx); return err }},
		{"LoadBoard", func() error { _, err := store.LoadBoard(ctx, "b1"); return err }},
		{"SaveBoard", func() error { return store.SaveBoard(ctx, &datatug.Board{}) }},
		{"DeleteBoard", func() error { return store.DeleteBoard(ctx, "b1") }},

		{"LoadFolders", func() error { _, err := store.LoadFolders(ctx); return err }},
		{"LoadFolder", func() error { _, err := store.LoadFolder(ctx, "f1"); return err }},
		{"SaveFolder", func() error { return store.SaveFolder(ctx, "", &datatug.Folder{}) }},
		{"SaveFolders", func() error { return store.SaveFolders(ctx, "", nil) }},
		{"DeleteFolder", func() error { return store.DeleteFolder(ctx, "f1") }},

		{"LoadEntities", func() error { _, err := store.LoadEntities(ctx); return err }},
		{"LoadEntity", func() error { _, err := store.LoadEntity(ctx, "e1"); return err }},
		{"SaveEntity", func() error { return store.SaveEntity(ctx, &datatug.Entity{}) }},
		{"DeleteEntity", func() error { return store.DeleteEntity(ctx, "e1") }},

		{"LoadEnvironments", func() error { _, err := store.LoadEnvironments(ctx); return err }},
		{"LoadEnvironment", func() error { _, err := store.LoadEnvironment(ctx, "env1"); return err }},
		{"LoadEnvironmentSummary", func() error { _, err := store.LoadEnvironmentSummary(ctx, "env1"); return err }},
		{"SaveEnvironment", func() error { return store.SaveEnvironment(ctx, &datatug.Environment{}) }},
		{"SaveEnvironments", func() error { return store.SaveEnvironments(ctx, nil) }},
		{"DeleteEnvironment", func() error { return store.DeleteEnvironment(ctx, "env1") }},

		{"LoadEnvDbServers", func() error { _, err := store.LoadEnvDbServers(ctx, "env1"); return err }},
		{"LoadEnvDbServer", func() error { _, err := store.LoadEnvDbServer(ctx, "env1", "s1"); return err }},
		{"SaveEnvDbServer", func() error { return store.SaveEnvDbServer(ctx, "env1", &datatug.EnvDbServer{}) }},
		{"SaveEnvServers", func() error { return store.SaveEnvServers(ctx, "env1", nil) }},
		{"DeleteEnvDbServer", func() error { return store.DeleteEnvDbServer(ctx, "env1", "s1") }},

		{"LoadEnvDbCatalogs", func() error { _, err := store.LoadEnvDbCatalogs(ctx, "env1"); return err }},
		{"LoadEnvDbCatalog", func() error { _, err := store.LoadEnvDbCatalog(ctx, "env1", "s1", "c1"); return err }},
		{"SaveEnvDbCatalog", func() error { return store.SaveEnvDbCatalog(ctx, "env1", "s1", "c1", &datatug.DbCatalog{}) }},
		{"SaveEnvDbCatalogs", func() error { return store.SaveEnvDbCatalogs(ctx, "env1", "s1", "c1", nil) }},
		{"DeleteEnvDbCatalog", func() error { return store.DeleteEnvDbCatalog(ctx, "env1", "s1", "c1") }},

		{"LoadProjDbDrivers", func() error { _, err := store.LoadProjDbDrivers(ctx); return err }},
		{"LoadProjDbDriver", func() error { _, err := store.LoadProjDbDriver(ctx, "d1"); return err }},
		{"SaveProjDbDriver", func() error { return store.SaveProjDbDriver(ctx, &datatug.ProjDbDriver{}) }},
		{"DeleteProjDbDriver", func() error { return store.DeleteProjDbDriver(ctx, "d1") }},

		{"LoadRecordsetDefinitions", func() error { _, err := store.LoadRecordsetDefinitions(ctx); return err }},
		{"LoadRecordsetDefinition", func() error { _, err := store.LoadRecordsetDefinition(ctx, "rs1"); return err }},
		{"LoadRecordsetData", func() error { _, err := store.LoadRecordsetData(ctx, "rs1"); return err }},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			err := c.call()
			assert.True(t, errors.Is(err, dalgostore.ErrNotImplemented), "%s: expected ErrNotImplemented, got: %v", c.name, err)
		})
	}
}

// TestDbServersStore_StubsReturnErrNotImplemented exercises the nested
// ProjDbServersStore/DbCatalogsStore stubs DbServersStore returns.
func TestDbServersStore_StubsReturnErrNotImplemented(t *testing.T) {
	ctx := context.Background()
	store := dalgostore.NewProjectStore(newTestDB(), "p1")

	dbServersStore := store.DbServersStore("d1")
	assert.Equal(t, "d1", dbServersStore.DriverID())

	serverRef := datatug.ServerRef{}
	catalogsStore := dbServersStore.CatalogsStore(serverRef)
	assert.Equal(t, serverRef, catalogsStore.Server())

	checks := []struct {
		name string
		call func() error
	}{
		{"ProjDbServersStore.LoadProjDbServers", func() error { _, err := dbServersStore.LoadProjDbServers(ctx); return err }},
		{"ProjDbServersStore.LoadProjDbServer", func() error { _, err := dbServersStore.LoadProjDbServer(ctx, "s1"); return err }},
		{"ProjDbServersStore.SaveProjDbServer", func() error { return dbServersStore.SaveProjDbServer(ctx, &datatug.ProjDbServer{}) }},
		{"ProjDbServersStore.DeleteProjDbServer", func() error { return dbServersStore.DeleteProjDbServer(ctx, "s1") }},

		{"DbCatalogsStore.LoadDbCatalogs", func() error { _, err := catalogsStore.LoadDbCatalogs(ctx); return err }},
		{"DbCatalogsStore.SaveDbCatalog", func() error { return catalogsStore.SaveDbCatalog(ctx, &datatug.DbCatalog{}) }},
		{"DbCatalogsStore.DeleteDbCatalog", func() error { return catalogsStore.DeleteDbCatalog(ctx, "c1") }},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			err := c.call()
			assert.True(t, errors.Is(err, dalgostore.ErrNotImplemented), "%s: expected ErrNotImplemented, got: %v", c.name, err)
		})
	}
}
