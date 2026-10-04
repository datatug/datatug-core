package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/gofrs/flock"
)

type badValidate struct{}

func (badValidate) Validate() error { return nil }
func (badValidate) MarshalJSON() ([]byte, error) {
	return nil, errors.New("simulated marshal error")
}

func TestCoverage_Utils(t *testing.T) {
	// readJSONFile with required=false on nonexistent file
	var m map[string]any
	if err := readJSONFile(filepath.Join(t.TempDir(), "nonexistent.json"), false, &m); err != nil {
		t.Fatalf("expected nil error on nonexistent optional file, got %v", err)
	}

	// readFile where newDecoder closes file early so defer file.Close() encounters an error
	tmp := filepath.Join(t.TempDir(), "f.json")
	if err := os.WriteFile(tmp, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = readFile(tmp, true, &m, func(r io.Reader) Decoder {
		if f, ok := r.(*os.File); ok {
			_ = f.Close()
		}
		return json.NewDecoder(r)
	})

	// DirExists with ENOTDIR
	notDir := filepath.Join(tmp, "sub")
	exists, err := DirExists(notDir)
	if exists || err == nil {
		t.Fatalf("expected error on DirExists(file/sub), got exists=%v err=%v", exists, err)
	}

	// ExpandHome
	if ExpandHome("") != "" {
		t.Fatal("expected empty string")
	}
	_ = ExpandHome("~")
	_ = ExpandHome("~/test")
	if ExpandHome("normal/path") != "normal/path" {
		t.Fatal("expected unchanged normal path")
	}
}

func TestCoverage_SaverFilesystem(t *testing.T) {
	dir := t.TempDir()
	// saveJSONFile with unmarshalable object
	err := saveJSONFile(dir, "bad.json", badValidate{})
	if err == nil {
		t.Fatal("expected error encoding badValidate")
	}
}

func TestCoverage_FsStore_GetProjects(t *testing.T) {
	s := FsStore{
		pathByID: map[string]string{
			"p1": filepath.Join(t.TempDir(), "missing_p1"),
		},
	}
	_, err := s.GetProjects(context.Background())
	if err == nil {
		t.Fatal("expected error for nonexistent project file")
	}

	if err := s.DeleteProject(context.Background(), "p1"); err == nil {
		t.Fatal("expected DeleteProject error")
	}
}

func TestCoverage_DbCatalogsStore(t *testing.T) {
	dir := t.TempDir()
	serverRef := datatug.ServerRef{Host: "localhost", Driver: "sqlite3"}
	cs := newFsDbCatalogsStore(dir, serverRef)
	if cs.Server().Host != "localhost" {
		t.Fatalf("expected host localhost, got %s", cs.Server().Host)
	}
	ctx := context.Background()
	cats, err := cs.LoadDbCatalogs(ctx)
	if err != nil {
		t.Fatalf("unexpected error loading db catalogs: %v", err)
	}
	if len(cats) != 0 {
		t.Fatalf("expected 0 catalogs, got %d", len(cats))
	}

	cat := new(datatug.DbCatalog)
	cat.ID = "cat1"
	cat.Driver = "sqlite3"
	cat.Path = "test.db"
	if err := cs.SaveDbCatalog(ctx, cat); err != nil {
		t.Fatalf("unexpected error saving db catalog: %v", err)
	}
	cats, err = cs.LoadDbCatalogs(ctx)
	if err != nil || len(cats) != 1 {
		t.Fatalf("expected 1 catalog, got %d, err: %v", len(cats), err)
	}
	if err := cs.DeleteDbCatalog(ctx, "cat1"); err != nil {
		t.Fatalf("unexpected error deleting catalog: %v", err)
	}
}

func TestCoverage_ProjDbDriversStore_And_ServersStore(t *testing.T) {
	dir := t.TempDir()
	dstore := newFsProjDbDriversStore(dir)
	ctx := context.Background()

	// Empty LoadProjDbDrivers
	drivers, err := dstore.LoadProjDbDrivers(ctx)
	if err != nil || len(drivers) != 0 {
		t.Fatalf("expected empty drivers, got %d, err %v", len(drivers), err)
	}

	// SaveProjDbDriver
	driver := new(datatug.ProjDbDriver)
	driver.ID = "sqlite3"
	driver.Title = "SQLite"
	if err := dstore.SaveProjDbDriver(ctx, driver); err != nil {
		t.Fatalf("failed to save driver: %v", err)
	}

	// LoadProjDbDriver
	loaded, err := dstore.LoadProjDbDriver(ctx, "sqlite3")
	if err != nil || loaded.ID != "sqlite3" {
		t.Fatalf("failed to load driver: %v", err)
	}

	// DbServersStore
	sstore := dstore.DbServersStore("sqlite3")
	if sstore.DriverID() != "sqlite3" {
		t.Fatalf("expected driver sqlite3, got %s", sstore.DriverID())
	}
	cstore := sstore.CatalogsStore(datatug.ServerRef{Host: "srv1"})
	if cstore.Server().Host != "srv1" {
		t.Fatalf("expected server ref host srv1, got %s", cstore.Server().Host)
	}

	// SaveProjDbServer
	server := &datatug.ProjDbServer{Server: datatug.ServerRef{Driver: "mysql", Host: "srv1"}}
	server.ID = server.Server.GetID()
	server.Title = "MySQL Server"
	if err := sstore.SaveProjDbServer(ctx, server); err != nil {
		t.Fatalf("failed to save server: %v", err)
	}

	// LoadProjDbServer
	loadedSrv, err := sstore.LoadProjDbServer(ctx, server.ID)
	if err != nil || loadedSrv.Server.Host != "srv1" {
		t.Fatalf("failed to load server: %v", err)
	}

	// LoadProjDbServers
	srvs, err := sstore.LoadProjDbServers(ctx)
	if err != nil || len(srvs) != 1 {
		t.Fatalf("failed to load servers: %v", err)
	}

	// LoadProjDbDrivers with drivers > 0
	drivers, err = dstore.LoadProjDbDrivers(ctx)
	if err != nil || len(drivers) != 1 {
		t.Fatalf("failed to load drivers: %v", err)
	}

	// DeleteProjDbServer
	if err := sstore.DeleteProjDbServer(ctx, server.ID); err != nil {
		t.Fatalf("failed to delete server: %v", err)
	}

	// DeleteProjDbDriver
	if err := dstore.DeleteProjDbDriver(ctx, "sqlite3"); err != nil {
		t.Fatalf("failed to delete driver: %v", err)
	}
}

func TestCoverage_ProjectItemsStore(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	// ProjItemStoredAsFile with empty suffix
	store := newFileProjectItemsStore[datatug.DbCatalogs, *datatug.DbCatalog, datatug.DbCatalog](dir, "")
	if err := store.saveProjectItem(ctx, dir, nil); err == nil {
		t.Fatal("expected error saving nil item")
	}

	cat := new(datatug.DbCatalog)
	cat.ID = "cat1"
	cat.Driver = "sqlite3"
	cat.Path = "test.db"
	if err := store.saveProjectItem(ctx, dir, cat); err != nil {
		t.Fatalf("failed to save item: %v", err)
	}

	// loadProjectItem with empty suffix
	loaded, err := store.loadProjectItem(ctx, dir, "cat1", "")
	if err != nil || loaded.ID != "cat1" {
		t.Fatalf("failed to load item: %v", err)
	}

	// Create a subdirectory inside dir to hit f.IsDir() in loadProjectItems
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create a file with different suffix to hit suffix mismatch
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	items, err := store.loadProjectItems(ctx, dir)
	if err != nil || len(items) != 1 {
		t.Fatalf("failed to load items: %v, len=%d", err, len(items))
	}

	// deleteProjectItem on ENOTDIR
	dummyFile := filepath.Join(t.TempDir(), "dummy")
	if err := os.WriteFile(dummyFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.deleteProjectItem(ctx, dummyFile, "sub"); err == nil {
		t.Fatal("expected error on ENOTDIR in deleteProjectItem")
	}

	// ProjItemStoredAsDir: test !f.IsDir() and corrupt datatug json
	dirStore := newDirProjectItemsStore[datatug.ProjDbDrivers, *datatug.ProjDbDriver, datatug.ProjDbDriver](dir, "driver")
	dirStore.itemFileSuffix = "driver"
	// write a file directly in dir so f.IsDir() is false
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// write a subdir with corrupt .datatug-driver.json
	badDir := filepath.Join(dir, "baddrv")
	if err := os.Mkdir(badDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(badDir, ".datatug-driver.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err = dirStore.loadProjectItems(ctx, dir)
	if err == nil {
		t.Fatal("expected error on corrupt .datatug json in loadProjectItems")
	}
}

func TestCoverage_DualLayout_Boards(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := newFsBoardsStore(dir)

	// listBoardIDs on ENOTDIR
	fileAsDir := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	badStore := newFsBoardsStore(fileAsDir)
	if _, err := badStore.listBoardIDs(); err == nil {
		t.Fatal("expected error from listBoardIDs on file")
	}
	if _, err := badStore.LoadBoards(ctx); err == nil {
		t.Fatal("expected error from LoadBoards on bad dir")
	}

	boardsDir := store.dirPath
	// Corrupt nested file
	b1Dir := filepath.Join(boardsDir, "b1")
	if err := os.MkdirAll(b1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b1Dir, "board.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadBoard(ctx, "b1"); err == nil {
		t.Fatal("expected error on corrupt nested board")
	}
	if _, err := store.LoadBoards(ctx); err == nil {
		t.Fatal("expected error in LoadBoards with corrupt nested board")
	}

	// Corrupt flat file
	if err := os.RemoveAll(b1Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(boardsDir, "b1.board.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadBoard(ctx, "b1"); err == nil {
		t.Fatal("expected error on corrupt flat board")
	}

	// Board in both nested and flat layouts with identical content (dedupe in listBoardIDs)
	board := &datatug.Board{}
	board.ID = "b1"
	board.Title = "B1"
	raw, _ := json.Marshal(board)
	if err := os.WriteFile(filepath.Join(boardsDir, "b1.board.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b1Dir, "board.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}

	boards, err := store.LoadBoards(ctx)
	if err != nil || len(boards) != 1 {
		t.Fatalf("expected 1 deduped board, got len=%d err=%v", len(boards), err)
	}

	// SaveBoard error (e.g. read-only dir path)
	roStore := newFsBoardsStore(fileAsDir)
	if err := roStore.SaveBoard(ctx, board); err == nil {
		t.Fatal("expected error saving board to read-only target")
	}

	// DeleteBoard error on ENOTDIR
	if err := roStore.DeleteBoard(ctx, "b1"); err == nil {
		t.Fatal("expected error deleting board on ENOTDIR")
	}

	// DeleteBoard when remove fails (e.g. flat path is a non-empty directory)
	dirAsFile := filepath.Join(boardsDir, "b2.board.json")
	if err := os.Mkdir(dirAsFile, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirAsFile, "inner"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteBoard(ctx, "b2"); err == nil {
		t.Fatal("expected error deleting board when file is non-empty dir")
	}
}

func TestCoverage_DualLayout_DbModels(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := newFsDbModelsStore(dir)

	fileAsDir := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	badStore := newFsDbModelsStore(fileAsDir)
	if _, err := badStore.listDbModelIDs(); err == nil {
		t.Fatal("expected error from listDbModelIDs on file")
	}
	if _, err := badStore.LoadDbModels(ctx); err == nil {
		t.Fatal("expected error from LoadDbModels on bad dir")
	}

	modelsDir := store.dirPath
	// Corrupt nested
	m1Dir := filepath.Join(modelsDir, "m1")
	if err := os.MkdirAll(m1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m1Dir, "m1.dbmodel.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadDbModel(ctx, "m1"); err == nil {
		t.Fatal("expected error on corrupt nested model")
	}
	if _, err := store.LoadDbModels(ctx); err == nil {
		t.Fatal("expected error in LoadDbModels with corrupt nested model")
	}

	// Corrupt flat
	if err := os.RemoveAll(m1Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "m1.dbmodel.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadDbModel(ctx, "m1"); err == nil {
		t.Fatal("expected error on corrupt flat model")
	}

	// Dedupe in listDbModelIDs
	m := new(datatug.DbModel)
	m.ID = "m1"
	raw, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(modelsDir, "m1.dbmodel.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(m1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m1Dir, "m1.dbmodel.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	models, err := store.LoadDbModels(ctx)
	if err != nil || len(models) != 1 {
		t.Fatalf("expected 1 deduped model, got len=%d err=%v", len(models), err)
	}

	// SaveDbModel error
	roStore := newFsDbModelsStore(fileAsDir)
	if err := roStore.SaveDbModel(ctx, m); err == nil {
		t.Fatal("expected error saving model to bad store")
	}
	// DeleteDbModel error on ENOTDIR
	if err := roStore.DeleteDbModel(ctx, "m1"); err == nil {
		t.Fatal("expected error deleting model on ENOTDIR")
	}
	// DeleteDbModel remove error
	dirAsFile := filepath.Join(modelsDir, "m2.dbmodel.json")
	if err := os.Mkdir(dirAsFile, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirAsFile, "inner"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteDbModel(ctx, "m2"); err == nil {
		t.Fatal("expected error deleting model when file is non-empty dir")
	}
}

func TestCoverage_DualLayout_Entities(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := newFsEntitiesStore(dir)

	fileAsDir := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	badStore := newFsEntitiesStore(fileAsDir)
	if _, err := badStore.listEntityIDs(); err == nil {
		t.Fatal("expected error from listEntityIDs on file")
	}
	if _, err := badStore.LoadEntities(ctx); err == nil {
		t.Fatal("expected error from LoadEntities on bad dir")
	}

	entitiesDir := store.dirPath
	// Corrupt nested
	e1Dir := filepath.Join(entitiesDir, "e1")
	if err := os.MkdirAll(e1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e1Dir, "e1.entity.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadEntity(ctx, "e1"); err == nil {
		t.Fatal("expected error on corrupt nested entity")
	}
	if _, err := store.LoadEntities(ctx); err == nil {
		t.Fatal("expected error in LoadEntities with corrupt nested entity")
	}

	// Corrupt flat
	if err := os.RemoveAll(e1Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entitiesDir, "e1.entity.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadEntity(ctx, "e1"); err == nil {
		t.Fatal("expected error on corrupt flat entity")
	}

	// Dedupe in listEntityIDs
	ent := new(datatug.Entity)
	ent.ID = "e1"
	raw, _ := json.Marshal(ent)
	if err := os.WriteFile(filepath.Join(entitiesDir, "e1.entity.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(e1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e1Dir, "e1.entity.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	entities, err := store.LoadEntities(ctx)
	if err != nil || len(entities) != 1 {
		t.Fatalf("expected 1 deduped entity, got len=%d err=%v", len(entities), err)
	}

	// SaveEntity error
	roStore := newFsEntitiesStore(fileAsDir)
	if err := roStore.SaveEntity(ctx, ent); err == nil {
		t.Fatal("expected error saving entity to bad store")
	}
	// DeleteEntity error on ENOTDIR
	if err := roStore.DeleteEntity(ctx, "e1"); err == nil {
		t.Fatal("expected error deleting entity on ENOTDIR")
	}
	// DeleteEntity remove error
	dirAsFile := filepath.Join(entitiesDir, "e2.entity.json")
	if err := os.Mkdir(dirAsFile, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirAsFile, "inner"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteEntity(ctx, "e2"); err == nil {
		t.Fatal("expected error deleting entity when file is non-empty dir")
	}
}

func TestCoverage_DualLayout_EnvDbCatalogs(t *testing.T) {
	ctx := context.Background()
	projDir := t.TempDir()
	store := newFsEnvCatalogsStore(projDir)

	fileAsDir := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	badStore := newFsEnvCatalogsStore(fileAsDir)
	if _, err := badStore.listEnvDbCatalogIDs("env1"); err == nil {
		t.Fatal("expected error from listEnvDbCatalogIDs on file")
	}
	if _, err := badStore.LoadEnvDbCatalogs(ctx, "env1"); err == nil {
		t.Fatal("expected error from LoadEnvDbCatalogs on bad dir")
	}

	catalogsDir := store.getDirPath("env1")
	// Corrupt nested
	c1Dir := filepath.Join(catalogsDir, "c1")
	if err := os.MkdirAll(c1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c1Dir, "c1.db.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadEnvDbCatalog(ctx, "env1", "srv", "c1"); err == nil {
		t.Fatal("expected error on corrupt nested env db catalog")
	}
	if _, err := store.LoadEnvDbCatalogs(ctx, "env1"); err == nil {
		t.Fatal("expected error in LoadEnvDbCatalogs with corrupt nested catalog")
	}

	// Corrupt flat
	if err := os.RemoveAll(c1Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catalogsDir, "c1.db.json"), []byte("{corrupt"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadEnvDbCatalog(ctx, "env1", "srv", "c1"); err == nil {
		t.Fatal("expected error on corrupt flat catalog")
	}

	// Dedupe in listEnvDbCatalogIDs
	cat := new(datatug.DbCatalog)
	cat.ID = "c1"
	raw, _ := json.Marshal(cat)
	if err := os.WriteFile(filepath.Join(catalogsDir, "c1.db.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(c1Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c1Dir, "c1.db.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	catalogs, err := store.LoadEnvDbCatalogs(ctx, "env1")
	if err != nil || len(catalogs) != 1 {
		t.Fatalf("expected 1 deduped catalog, got len=%d err=%v", len(catalogs), err)
	}

	// SaveEnvDbCatalog error
	roStore := newFsEnvCatalogsStore(fileAsDir)
	if err := roStore.SaveEnvDbCatalog(ctx, "env1", "srv", "c1", cat); err == nil {
		t.Fatal("expected error saving catalog to bad store")
	}
	// DeleteEnvDbCatalog error on ENOTDIR
	if err := roStore.DeleteEnvDbCatalog(ctx, "env1", "srv", "c1"); err == nil {
		t.Fatal("expected error deleting catalog on ENOTDIR")
	}
	// DeleteEnvDbCatalog remove error
	dirAsFile := filepath.Join(catalogsDir, "c2.db.json")
	if err := os.Mkdir(dirAsFile, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirAsFile, "inner"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteEnvDbCatalog(ctx, "env1", "srv", "c2"); err == nil {
		t.Fatal("expected error deleting catalog when file is non-empty dir")
	}
}

func TestCoverage_EnvironmentsStore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := newFsEnvironmentsStore(dir)
	if err := os.MkdirAll(store.dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	// File in env dir (!de.IsDir())
	if err := os.WriteFile(filepath.Join(store.dirPath, "file.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	// Dir with neither environment file (env == nil continue)
	if err := os.Mkdir(filepath.Join(store.dirPath, "emptyenv"), 0755); err != nil {
		t.Fatal(err)
	}

	envs, err := store.LoadEnvironments(ctx)
	if err != nil || len(envs) != 0 {
		t.Fatalf("expected 0 envs, got %d, err %v", len(envs), err)
	}

	// LoadEnvironments on ENOTDIR
	badStore := newFsEnvironmentsStore(dir)
	badStore.dirPath = filepath.Join(store.dirPath, "file.txt")
	if _, err := badStore.LoadEnvironments(ctx); err == nil {
		t.Fatal("expected error on ENOTDIR")
	}

	// DeleteEnvironment on ENOTDIR
	if err := badStore.DeleteEnvironment(ctx, "sub"); err == nil {
		t.Fatal("expected error deleting env on ENOTDIR")
	}
}

func TestCoverage_LoaderEnvServers(t *testing.T) {
	dir := t.TempDir()
	env := &datatug.Environment{}

	// File suffix != ServerFileSuffix
	if err := os.WriteFile(filepath.Join(dir, "srv1.other.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	// Host empty defaults to serverName
	if err := os.WriteFile(filepath.Join(dir, "srv1.server.json"), []byte(`[{"host":""}]`), 0644); err != nil {
		t.Fatal(err)
	}
	// Bad json
	if err := os.WriteFile(filepath.Join(dir, "srv2.server.json"), []byte(`[corrupt`), 0644); err != nil {
		t.Fatal(err)
	}

	err := loadEnvServers(dir, env)
	if err == nil {
		t.Fatal("expected error from corrupt json")
	}

	// Clean corrupt file and test host default
	_ = os.Remove(filepath.Join(dir, "srv2.server.json"))
	env2 := &datatug.Environment{}
	if err := loadEnvServers(dir, env2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env2.DbServers) != 1 || env2.DbServers[0].Host != "srv1" {
		t.Fatalf("expected host srv1, got %+v", env2.DbServers)
	}
}

func TestCoverage_Cat1_Remaining(t *testing.T) {
	ctx := context.Background()

	// DirExists: nonexistent and existing
	if exists, err := DirExists(filepath.Join(t.TempDir(), "nonexistent")); exists || err != nil {
		t.Fatalf("expected false, nil for nonexistent dir, got %v, %v", exists, err)
	}
	if exists, err := DirExists(t.TempDir()); !exists || err != nil {
		t.Fatalf("expected true, nil for existing dir, got %v, %v", exists, err)
	}

	// saveItems count=0 and count > 1 with error
	if err := saveItems("items", 0, nil); err != nil {
		t.Fatalf("expected nil on count=0, got %v", err)
	}
	errFail := errors.New("worker fail")
	if err := saveItems("items", 2, func(i int) func() error {
		return func() error { return errFail }
	}); err == nil {
		t.Fatal("expected error from saveItems")
	}

	// NewProjectStore and NewSingleProjectStore
	pStore := NewProjectStore("p1", t.TempDir())
	if pStore == nil {
		t.Fatal("expected non-nil pStore")
	}
	singleStore, projID := NewSingleProjectStore(t.TempDir(), "")
	if singleStore == nil || projID == "" {
		t.Fatal("expected non-empty singleStore and projID")
	}
	singleStore2, projID2 := NewSingleProjectStore(t.TempDir(), "custom_id")
	if singleStore2 == nil || projID2 != "custom_id" {
		t.Fatal("expected custom_id projID")
	}

	// Ghost symlinks for Boards, DbModels, Entities, EnvDbCatalogs
	// 1. Boards
	boardsDir := t.TempDir()
	bStore := newFsBoardsStore(boardsDir)
	if err := os.MkdirAll(bStore.dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(filepath.Join(bStore.dirPath, "nonexistent"), filepath.Join(bStore.dirPath, "ghost.board.json"))
	boards, err := bStore.LoadBoards(ctx)
	if err != nil || len(boards) != 0 {
		t.Fatalf("expected 0 boards with ghost symlink, got len=%d, err=%v", len(boards), err)
	}

	// 2. DbModels
	modelsDir := t.TempDir()
	mStore := newFsDbModelsStore(modelsDir)
	if err := os.MkdirAll(mStore.dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(filepath.Join(mStore.dirPath, "nonexistent"), filepath.Join(mStore.dirPath, "ghost.dbmodel.json"))
	models, err := mStore.LoadDbModels(ctx)
	if err != nil || len(models) != 0 {
		t.Fatalf("expected 0 models with ghost symlink, got len=%d, err=%v", len(models), err)
	}
	// SaveDbModels
	dm := new(datatug.DbModel)
	dm.ID = "m1"
	if err := mStore.SaveDbModels(ctx, datatug.DbModels{dm}); err != nil {
		t.Fatalf("failed to SaveDbModels: %v", err)
	}

	// 3. Entities
	entitiesDir := t.TempDir()
	eStore := newFsEntitiesStore(entitiesDir)
	if err := os.MkdirAll(eStore.dirPath, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(filepath.Join(eStore.dirPath, "nonexistent"), filepath.Join(eStore.dirPath, "ghost.entity.json"))
	entities, err := eStore.LoadEntities(ctx)
	if err != nil || len(entities) != 0 {
		t.Fatalf("expected 0 entities with ghost symlink, got len=%d, err=%v", len(entities), err)
	}
	// SaveEntity with non-nil empty Fields
	ent := new(datatug.Entity)
	ent.ID = "e1"
	ent.Fields = []*datatug.EntityField{}
	if err := eStore.SaveEntity(ctx, ent); err != nil {
		t.Fatalf("failed to SaveEntity with empty fields: %v", err)
	}

	// 4. EnvDbCatalogs
	envProjDir := t.TempDir()
	cStore := newFsEnvCatalogsStore(envProjDir)
	// nonexistent env ID in LoadEnvDbCatalogs
	emptyCats, err := cStore.LoadEnvDbCatalogs(ctx, "nonexistent_env")
	if err != nil || len(emptyCats) != 0 {
		t.Fatalf("expected 0 catalogs for nonexistent env, got len=%d, err=%v", len(emptyCats), err)
	}
	catsDir := cStore.getDirPath("env1")
	if err := os.MkdirAll(catsDir, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(filepath.Join(catsDir, "nonexistent"), filepath.Join(catsDir, "ghost.db.json"))
	envCats, err := cStore.LoadEnvDbCatalogs(ctx, "env1")
	if err != nil || len(envCats) != 0 {
		t.Fatalf("expected 0 catalogs with ghost symlink, got len=%d, err=%v", len(envCats), err)
	}
	// SaveEnvDbCatalog when flat file already exists
	catFlat := new(datatug.DbCatalog)
	catFlat.ID = "flatcat"
	catFlat.Driver = "sqlite3"
	catFlat.Path = "test.db"
	if err := os.WriteFile(filepath.Join(catsDir, "flatcat.db.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := cStore.SaveEnvDbCatalog(ctx, "env1", "srv", "flatcat", catFlat); err != nil {
		t.Fatalf("failed to SaveEnvDbCatalog on flat layout: %v", err)
	}
	// SaveEnvDbCatalogs
	if err := cStore.SaveEnvDbCatalogs(ctx, "env1", "srv", "", datatug.DbCatalogs{catFlat}); err != nil {
		t.Fatalf("failed to SaveEnvDbCatalogs: %v", err)
	}

	// ProjectItemsStore: suffix mismatch in loadProjectItems and saveProjectItem error
	pitemsDir := t.TempDir()
	piStore := newFileProjectItemsStore[datatug.DbCatalogs, *datatug.DbCatalog, datatug.DbCatalog](pitemsDir, "")
	// foo.other.json has suffix "other" != ""
	if err := os.WriteFile(filepath.Join(pitemsDir, "foo.other.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	res, err := piStore.loadProjectItems(ctx, pitemsDir)
	if err != nil || len(res) != 0 {
		t.Fatalf("expected 0 items with suffix mismatch, got %d, err %v", len(res), err)
	}

	// saveProjectItem error
	roDir := filepath.Join(t.TempDir(), "ro_file")
	if err := os.WriteFile(roDir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := piStore.saveProjectItem(ctx, roDir, catFlat); err == nil {
		t.Fatal("expected error saving project item to file path")
	}
}

func TestCoverage_Cat2_StoreProjectSaver(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	store := newFsProjectStore("p1", tmpDir)

	// SaveProject with invalid project
	badProj := &datatug.Project{} // missing ID, title, access
	if err := store.SaveProject(ctx, badProj); err == nil {
		t.Fatal("expected error on invalid project")
	}

	// SaveProject with uncreatable path
	roFile := filepath.Join(tmpDir, "ro_file")
	if err := os.WriteFile(roFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	badPathStore := newFsProjectStore("p1", filepath.Join(roFile, "sub"))
	validProj := new(datatug.Project)
	validProj.ID = "p1"
	validProj.Title = "Project 1"
	validProj.Access = "public"
	if err := badPathStore.SaveProject(ctx, validProj); err == nil {
		t.Fatal("expected error creating project dir")
	}

	// putProjectFile with invalid file
	if err := store.putProjectFile(datatug.ProjectFile{}); err == nil {
		t.Fatal("expected error for invalid project file")
	}

	// SaveProject with full valid project: Entities, Environments, DbModels, Boards, Queries
	fullProj := new(datatug.Project)
	fullProj.ID = "p1"
	fullProj.Title = "Project 1"
	fullProj.Access = "public"
	fullProj.Created = &datatug.ProjectCreated{At: time.Now()}
	e1 := new(datatug.Entity)
	e1.ID = "e1"
	e1.Title = "Entity 1"
	fullProj.Entities = datatug.Entities{e1}

	env1 := new(datatug.Environment)
	env1.ID = "local"
	env1.Title = "Local"
	fullProj.Environments = datatug.Environments{env1}

	m1 := new(datatug.DbModel)
	m1.ID = "m1"
	m1.Title = "Model 1"
	fullProj.DbModels = datatug.DbModels{m1}

	b1 := new(datatug.Board)
	b1.ID = "b1"
	b1.Title = "Board 1"
	fullProj.Boards = datatug.Boards{b1}

	qRoot := new(datatug.QueriesFolder)
	qRoot.ID = "root"
	qRoot.Title = "Root"
	fullProj.Queries = qRoot

	if err := store.SaveProject(ctx, fullProj); err != nil {
		t.Fatalf("unexpected error saving full project: %v", err)
	}

	// Error saving sub-components in SaveProject:
	badEntitiesProj := new(datatug.Project)
	badEntitiesProj.ID = "p1"
	badEntitiesProj.Title = "Project 1"
	badEntitiesProj.Access = "public"
	badEntitiesProj.Entities = datatug.Entities{new(datatug.Entity)}
	if err := store.SaveProject(ctx, badEntitiesProj); err == nil {
		t.Fatal("expected error saving bad entities")
	}

	// saveProjectFile error on bad path
	if err := badPathStore.saveProjectFile(validProj); err == nil {
		t.Fatal("expected error saving project file")
	}
}

func TestCoverage_Cat2_StoreLoader(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	store := newFsProjectStore("p1", tmpDir)

	// LoadProject on missing project file
	if _, err := store.LoadProject(ctx); err == nil {
		t.Fatal("expected error loading missing project")
	}

	// GetProjectPath
	loader := fileSystemLoader{
		pathByID: map[string]string{
			"known": tmpDir,
		},
	}
	// unknown project ID
	if _, _, err := loader.GetProjectPath("unknown"); err == nil {
		t.Fatal("expected error for unknown project")
	}
	// single project ID default when len(projectPaths) == 1
	oldPaths := projectPaths
	defer func() { projectPaths = oldPaths }()
	projectPaths = map[string]string{storage.SingleProjectID: tmpDir}
	loaderSingle := fileSystemLoader{pathByID: projectPaths}
	projID, projPath, err := loaderSingle.GetProjectPath("")
	if err != nil || projID != storage.SingleProjectID || projPath != tmpDir {
		t.Fatalf("unexpected GetProjectPath: %v, %v, %v", projID, projPath, err)
	}

	// GetFolderPath on unknown project
	if _, err := loader.GetFolderPath("unknown", "folder"); err == nil {
		t.Fatal("expected error on GetFolderPath for unknown project")
	}
	if folderPath, err := loader.GetFolderPath("known", "data", "sub"); err != nil || folderPath == "" {
		t.Fatalf("unexpected GetFolderPath: %v, %v", folderPath, err)
	}

	// LoadProject with depth=1 (does not load subcomponents)
	projFile := datatug.ProjectFile{}
	projFile.ID = "p1"
	projFile.Title = "P1"
	projFile.Access = "public"
	projFile.Created = &datatug.ProjectCreated{At: time.Now()}
	_ = store.putProjectFile(projFile)
	loadedDepth1, err := store.LoadProject(ctx, datatug.Depth(1))
	if err != nil || loadedDepth1 == nil {
		t.Fatalf("failed to LoadProject with depth 1: %v", err)
	}

	// LoadProject with depth=0 or 2, but subcomponents fail:
	// Corrupt environment
	envDir := filepath.Join(tmpDir, storage.EnvironmentsFolder)
	_ = os.MkdirAll(filepath.Join(envDir, "e1"), 0755)
	_ = os.WriteFile(filepath.Join(envDir, "e1", "environment-summary.json"), []byte("{corrupt"), 0644)
	if _, err := store.LoadProject(ctx); err == nil {
		t.Fatal("expected error loading project with corrupt env")
	}
	_ = os.RemoveAll(envDir)

	// Corrupt entity
	entDir := filepath.Join(tmpDir, storage.EntitiesFolder)
	_ = os.MkdirAll(entDir, 0755)
	_ = os.WriteFile(filepath.Join(entDir, "e1.entity.json"), []byte("{corrupt"), 0644)
	if _, err := store.LoadProject(ctx); err == nil {
		t.Fatal("expected error loading project with corrupt entity")
	}
	_ = os.RemoveAll(entDir)

	// Corrupt board
	boardDir := filepath.Join(tmpDir, storage.BoardsFolder)
	_ = os.MkdirAll(boardDir, 0755)
	_ = os.WriteFile(filepath.Join(boardDir, "b1.board.json"), []byte("{corrupt"), 0644)
	if _, err := store.LoadProject(ctx); err == nil {
		t.Fatal("expected error loading project with corrupt board")
	}
	_ = os.RemoveAll(boardDir)

	// Corrupt dbModel
	modelDir := filepath.Join(tmpDir, storage.DbModelsFolder)
	_ = os.MkdirAll(modelDir, 0755)
	_ = os.WriteFile(filepath.Join(modelDir, "m1.dbmodel.json"), []byte("{corrupt"), 0644)
	if _, err := store.LoadProject(ctx); err == nil {
		t.Fatal("expected error loading project with corrupt model")
	}
	_ = os.RemoveAll(modelDir)

	// Corrupt dbDriver
	dbsDir := filepath.Join(tmpDir, storage.DbsFolder)
	_ = os.MkdirAll(filepath.Join(dbsDir, "drv1"), 0755)
	_ = os.WriteFile(filepath.Join(dbsDir, "drv1", ".datatug-.json"), []byte("{corrupt"), 0644)
	if _, err := store.LoadProject(ctx); err == nil {
		t.Fatal("expected error loading project with corrupt driver")
	}
	_ = os.RemoveAll(dbsDir)
}

func TestCoverage_Cat2_LoaderInternals(t *testing.T) {
	tmpDir := t.TempDir()

	// loadProjectFile with corrupt file
	corruptFile := filepath.Join(tmpDir, storage.ProjectSummaryFileName)
	_ = os.WriteFile(corruptFile, []byte("{corrupt"), 0644)
	proj := &datatug.Project{}
	if err := loadProjectFile(tmpDir, proj); err == nil {
		t.Fatal("expected error from loadProjectFile on corrupt file")
	}

	// loadDir with pattern "[-]"
	if err := loadDir(nil, tmpDir, "[-]", processFiles, nil, nil); err == nil {
		t.Fatal("expected error on invalid fileMask")
	}
	// loadDir with dirPath as file
	dummyFile := filepath.Join(tmpDir, "file")
	_ = os.WriteFile(dummyFile, []byte("x"), 0644)
	if err := loadDir(nil, filepath.Join(dummyFile, "sub"), "", processFiles, nil, nil); err == nil {
		t.Fatal("expected error on ENOTDIR")
	}

	// loadDbModel with empty ID defaulting to id
	mDir := filepath.Join(tmpDir, "m_test")
	_ = os.MkdirAll(mDir, 0755)
	_ = os.WriteFile(filepath.Join(mDir, "m_test.dbmodel.json"), []byte(`{}`), 0644)
	model, err := loadDbModel(tmpDir, "m_test")
	if err != nil || model.ID != "m_test" {
		t.Fatalf("expected model ID m_test, got %v, err=%v", model, err)
	}

	// loadDbModel with ID mismatch
	_ = os.WriteFile(filepath.Join(mDir, "m_test.dbmodel.json"), []byte(`{"id":"mismatch"}`), 0644)
	if _, err := loadDbModel(tmpDir, "m_test"); err == nil {
		t.Fatal("expected error on id mismatch in loadDbModel")
	}

	// loadSchemaModel error (corrupt table/view)
	tablesFile := filepath.Join(mDir, "s1", "tables")
	_ = os.MkdirAll(filepath.Dir(tablesFile), 0755)
	_ = os.WriteFile(tablesFile, []byte("x"), 0644)
	if _, err := loadSchemaModel(mDir, "s1"); err == nil {
		t.Fatal("expected error in loadSchemaModel")
	}

	// loadDbCatalog with corrupt json
	catDir := filepath.Join(tmpDir, "cat1")
	_ = os.MkdirAll(catDir, 0755)
	_ = os.WriteFile(filepath.Join(catDir, "cat1.db.json"), []byte("{corrupt"), 0644)
	cat := new(datatug.DbCatalog)
	cat.ID = "cat1"
	if err := loadDbCatalog(catDir, cat); err == nil {
		t.Fatal("expected error loading corrupt db catalog")
	}

	// loadDbCatalog validation error
	_ = os.WriteFile(filepath.Join(catDir, "cat1.db.json"), []byte(`{"driver":""}`), 0644)
	if err := loadDbCatalog(catDir, cat); err == nil {
		t.Fatal("expected validation error in loadDbCatalog")
	}

	// loadDbCatalogs error propagation
	srv := &datatug.ProjDbServer{Server: datatug.ServerRef{Driver: "sqlite3"}}
	if err := loadDbCatalogs(tmpDir, srv); err == nil {
		t.Fatal("expected error from loadDbCatalogs with corrupt catalog")
	}

	// loadSchema with corrupt table
	sDir := filepath.Join(tmpDir, "schemas")
	_ = os.MkdirAll(filepath.Join(sDir, "s1", "tables", "t1"), 0755)
	_ = os.WriteFile(filepath.Join(sDir, "s1", "tables", "t1", "s1.t1.json"), []byte("{corrupt"), 0644)
	if _, err := loadSchema(sDir, "s1"); err == nil {
		t.Fatal("expected error from loadSchema")
	}

	// loadTables with non-dir file in tables directory (!f.IsDir())
	tDir := filepath.Join(tmpDir, "schemas2", "s1", "tables")
	_ = os.MkdirAll(tDir, 0755)
	_ = os.WriteFile(filepath.Join(tDir, "ignored_file.txt"), []byte("x"), 0644)
	tables, err := loadTables(filepath.Join(tmpDir, "schemas2"), "s1", "tables")
	if err != nil || len(tables) != 0 {
		t.Fatalf("expected 0 tables, got %d, err %v", len(tables), err)
	}

	// loadTables on ENOTDIR
	if _, err := loadTables(filepath.Join(tDir, "ignored_file.txt"), "s1", "tables"); err == nil {
		t.Fatal("expected error on ENOTDIR in loadTables")
	}
}

func TestCoverage_Cat2_LoaderRecordsets(t *testing.T) {
	loader := fileSystemLoader{
		pathByID: map[string]string{},
	}
	// LoadRecordsetDefinitions with empty projectID
	if _, err := loader.LoadRecordsetDefinitions(""); err == nil {
		t.Fatal("expected error on empty projectID")
	}
	// LoadRecordsetDefinitions with unknown projectID
	if _, err := loader.LoadRecordsetDefinitions("unknown"); err == nil {
		t.Fatal("expected error on unknown projectID")
	}

	tmpDir := t.TempDir()
	loader.pathByID["p1"] = tmpDir

	// loadRecordsetsDir with corrupt recordset json
	rsDir := filepath.Join(tmpDir, storage.DataFolder, storage.RecordsetsFolder, "rs1")
	_ = os.MkdirAll(rsDir, 0755)
	_ = os.WriteFile(filepath.Join(rsDir, "rs1.recordset.json"), []byte("{corrupt"), 0644)
	if _, err := loader.loadRecordsetsDir("p1", "", filepath.Join(tmpDir, storage.DataFolder, storage.RecordsetsFolder)); err == nil {
		t.Fatal("expected error on corrupt recordset json")
	}

	// LoadRecordsetData on unknown projectID
	if _, err := loader.LoadRecordsetData("unknown", "ds", "f.json"); err == nil {
		t.Fatal("expected error on unknown projectID in LoadRecordsetData")
	}

	// LoadRecordsetData on nonexistent recordset
	if _, err := loader.LoadRecordsetData("p1", "ds", "f.json"); err == nil {
		t.Fatal("expected error on nonexistent recordset in LoadRecordsetData")
	}

	// Valid recordset definition + data file with missing column in row
	_ = os.RemoveAll(rsDir)
	_ = os.MkdirAll(rsDir, 0755)
	rsDef := new(datatug.RecordsetDefinition)
	rsDef.ID = "rs1"
	col1 := datatug.RecordsetColumnDef{}
	col1.Name = "col1"
	col2 := datatug.RecordsetColumnDef{}
	col2.Name = "col2"
	rsDef.Columns = datatug.RecordsetColumnDefs{col1, col2}
	defBytes, _ := json.Marshal(rsDef)
	_ = os.WriteFile(filepath.Join(rsDir, "rs1.recordset.json"), defBytes, 0644)

	// write data file where row is missing col2
	dataDir := filepath.Join(tmpDir, storage.DataFolder, "rs1")
	_ = os.MkdirAll(dataDir, 0755)
	// rows: [{"col1": "val1"}] -> col2 missing
	_ = os.WriteFile(filepath.Join(dataDir, "rows.json"), []byte(`[{"col1":"val1"}]`), 0644)

	rsData, err := loader.LoadRecordsetData("p1", "rs1", "rows.json")
	if err != nil {
		t.Fatalf("unexpected error loading recordset data: %v", err)
	}
	if len(rsData.Rows) != 1 || rsData.Rows[0][0] != "val1" || rsData.Rows[0][1] != nil {
		t.Fatalf("unexpected rows: %+v", rsData.Rows)
	}

	// LoadRecordsetData with invalid row type
	_ = os.WriteFile(filepath.Join(dataDir, "rows.json"), []byte(`["not an object"]`), 0644)
	if _, err := loader.LoadRecordsetData("p1", "rs1", "rows.json"); err == nil {
		t.Fatal("expected error on invalid row type")
	}
}

type mockFileInfo struct {
	os.FileInfo
	name  string
	size  int64
	mode  os.FileMode
	isDir bool
	sys   any
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return m.size }
func (m mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) Sys() any           { return m.sys }

type mockSafeReadFile struct {
	statFn  func() (os.FileInfo, error)
	readFn  func(p []byte) (int, error)
	closeFn func() error
}

func (m *mockSafeReadFile) Stat() (os.FileInfo, error) {
	if m.statFn != nil {
		return m.statFn()
	}
	return mockFileInfo{}, nil
}

func (m *mockSafeReadFile) Read(p []byte) (int, error) {
	if m.readFn != nil {
		return m.readFn(p)
	}
	return 0, io.EOF
}

func (m *mockSafeReadFile) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockChmodDirFile struct {
	statFn  func() (os.FileInfo, error)
	chmodFn func(perm os.FileMode) error
	closeFn func() error
}

func (m *mockChmodDirFile) Stat() (os.FileInfo, error) {
	if m.statFn != nil {
		return m.statFn()
	}
	return mockFileInfo{isDir: true}, nil
}

func (m *mockChmodDirFile) Chmod(perm os.FileMode) error {
	if m.chmodFn != nil {
		return m.chmodFn(perm)
	}
	return nil
}

func (m *mockChmodDirFile) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockExclusiveFile struct {
	writeFn func(p []byte) (int, error)
	syncFn  func() error
	closeFn func() error
}

func (m *mockExclusiveFile) Write(p []byte) (int, error) {
	if m.writeFn != nil {
		return m.writeFn(p)
	}
	return len(p), nil
}

func (m *mockExclusiveFile) Sync() error {
	if m.syncFn != nil {
		return m.syncFn()
	}
	return nil
}

func (m *mockExclusiveFile) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockSyncableDir struct {
	syncFn  func() error
	closeFn func() error
}

func (m *mockSyncableDir) Sync() error {
	if m.syncFn != nil {
		return m.syncFn()
	}
	return nil
}

func (m *mockSyncableDir) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockWriteCloser struct {
	writeFn func(p []byte) (int, error)
	closeFn func() error
}

func (m *mockWriteCloser) Write(p []byte) (int, error) {
	if m.writeFn != nil {
		return m.writeFn(p)
	}
	return len(p), nil
}

func (m *mockWriteCloser) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func testFsQueriesStore(dir string) fsQueriesStore {
	var qs fsQueriesStore
	qs.dirPath = dir
	qs.listingBudget = maxQueryListingBytes
	return qs
}

func testProjectItem(id string) datatug.ProjectItem {
	return datatug.ProjectItem{
		ProjItemBrief: datatug.ProjItemBrief{ID: id, Title: id},
		Access:        "public",
	}
}

func TestCoverage_Cat3_LoaderInternals(t *testing.T) {
	tmpDir := t.TempDir()

	// loader_internals.go:120 - loadSchemaModel fails inside loadDbModel
	mDir := filepath.Join(tmpDir, "m1")
	_ = os.MkdirAll(filepath.Join(mDir, "s1"), 0755)
	_ = os.WriteFile(filepath.Join(mDir, "s1", "tables"), []byte("not a dir"), 0644)
	_ = os.WriteFile(filepath.Join(mDir, "m1.db-model.json"), []byte(`{"id":"m1"}`), 0644)
	if _, err := loadDbModel(tmpDir, "m1"); err == nil {
		t.Fatal("expected error from loadDbModel when schemaModel fails")
	}

	// loader_internals.go:217 - loadSchema fails inside loadDbCatalog
	catDir := filepath.Join(tmpDir, "c1")
	_ = os.MkdirAll(filepath.Join(catDir, storage.SchemasFolder, "s1", "tables", "t1"), 0755)
	_ = os.WriteFile(filepath.Join(catDir, storage.SchemasFolder, "s1", "tables", "t1", "s1.t1.json"), []byte("{corrupt"), 0644)
	_ = os.WriteFile(filepath.Join(catDir, "c1.db.json"), []byte(`{"id":"c1","driver":"dummy"}`), 0644)
	cat := new(datatug.DbCatalog)
	cat.ID = "c1"
	if err := loadDbCatalog(catDir, cat); err == nil {
		t.Fatal("expected error from loadDbCatalog on corrupt table")
	}

	// loader_internals.go:240 - loadTables(views) fails inside loadSchema
	sDir := filepath.Join(tmpDir, "schemas_test")
	_ = os.MkdirAll(filepath.Join(sDir, "s1", "views", "v1"), 0755)
	_ = os.WriteFile(filepath.Join(sDir, "s1", "views", "v1", "s1.v1.json"), []byte("{corrupt"), 0644)
	if _, err := loadSchema(sDir, "s1"); err == nil {
		t.Fatal("expected error from loadSchema on corrupt view")
	}

	// loader_internals.go:249 - dbSchema.Validate() fails inside loadSchema
	if _, err := loadSchema(t.TempDir(), ""); err == nil {
		t.Fatal("expected error from loadSchema on empty id")
	}
}

func TestCoverage_Cat3_LoaderRecordsets(t *testing.T) {
	loader := fileSystemLoader{pathByID: map[string]string{}}
	tmpDir := t.TempDir()
	loader.pathByID["p1"] = tmpDir

	// loader_recordsets.go:39 - loadRecordsetsDir subfolder has corrupt recordset.json
	rsDir := filepath.Join(tmpDir, storage.DataFolder, storage.RecordsetsFolder, "folder1", "rs1")
	_ = os.MkdirAll(rsDir, 0755)
	_ = os.WriteFile(filepath.Join(rsDir, "rs1.recordset.json"), []byte("{corrupt"), 0644)
	if _, err := loader.loadRecordsetsDir("p1", "", filepath.Join(tmpDir, storage.DataFolder, storage.RecordsetsFolder)); err == nil {
		t.Fatal("expected error from nested loadRecordsetsDir")
	}
}

func TestCoverage_Cat3_QueriesTree(t *testing.T) {
	ctx := context.Background()
	// queries_tree.go:49 - walkQueryDir on a file root in loadQueriesTreeLocked
	tmpFile := filepath.Join(t.TempDir(), "not_a_dir")
	_ = os.WriteFile(tmpFile, []byte("x"), 0644)
	qs := testFsQueriesStore(tmpFile)
	if _, err := qs.loadQueriesTreeLocked(ctx, queryLockGuard{}, "", nil); err == nil {
		t.Fatal("expected error from loadQueriesTreeLocked on file root")
	}

	// queries_tree.go:58 - os.ReadDir error in loadQueriesTreeLocked
	tmpDir := t.TempDir()
	qs2 := testFsQueriesStore(tmpDir)
	subFile := filepath.Join(tmpDir, "subfile")
	_ = os.WriteFile(subFile, []byte("x"), 0644)
	if _, err := qs2.loadQueriesTreeLocked(ctx, queryLockGuard{}, "subfile/deep", nil); err == nil {
		t.Fatal("expected ENOTDIR error from loadQueriesTreeLocked")
	}

	// queries_tree.go:133 - resolveQueryLocation error in saveQueriesTreeLocked
	dir := t.TempDir()
	qs3 := testFsQueriesStore(dir)
	invalidItem := &datatug.QueryDef{
		ProjectItem: testProjectItem("COM1"),
		Type:        datatug.QueryTypeSQL,
	}
	folder := &datatug.QueriesFolder{
		Items: []*datatug.QueryDef{invalidItem},
	}
	if err := qs3.saveQueriesTreeLocked(ctx, queryLockGuard{}, "", folder); err == nil {
		t.Fatal("expected error on Windows reserved query id")
	}

	// queries_tree.go:137 - readQueryPair error in saveQueriesTreeLocked
	_ = os.MkdirAll(filepath.Join(dir, "queries"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "queries", "q_bad.query.json"), []byte("{corrupt"), 0644)
	validItem := &datatug.QueryDef{
		ProjectItem: testProjectItem("q_bad"),
		Type:        datatug.QueryTypeSQL,
	}
	folder2 := &datatug.QueriesFolder{
		Items: []*datatug.QueryDef{validItem},
	}
	qs3.dirPath = filepath.Join(dir, "queries")
	if err := qs3.saveQueriesTreeLocked(ctx, queryLockGuard{}, "", folder2); err == nil {
		t.Fatal("expected error reading corrupt existing query")
	}

	// queries_tree.go:140 - stageAndInstallQueryPair error in saveQueriesTreeLocked
	validItem2 := &datatug.QueryDef{
		ProjectItem: testProjectItem("q_valid"),
		Type:        datatug.QueryTypeSQL,
	}
	folder3 := &datatug.QueriesFolder{
		Items: []*datatug.QueryDef{validItem2},
	}
	badGuard := queryLockGuard{txnDir: filepath.Join(t.TempDir(), "not_exist", "sub")}
	if err := qs3.saveQueriesTreeLocked(ctx, badGuard, "", folder3); err == nil {
		t.Fatal("expected error staging query when stagingDir fails")
	}
}

func TestCoverage_Cat3_QueryContent(t *testing.T) {
	q := datatug.QueryDef{
		Parameters: datatug.Parameters{
			{
				ID:           "p1",
				Type:         "string",
				DefaultValue: make(chan int),
			},
		},
	}
	if _, err := queryJSONBytes(q); err == nil {
		t.Fatal("expected error from queryJSONBytes with unencodable chan")
	}
}

func TestCoverage_Cat3_QueryFsFlagsBSD(t *testing.T) {
	m := mockFileInfo{name: "f"}
	if reason, bad := fileFlagsIssue("", m); bad || reason != "" {
		t.Fatalf("expected false, got %v, %v", bad, reason)
	}
}

func TestCoverage_Cat3_QueryFsSafety(t *testing.T) {
	tmpDir := t.TempDir()
	fPath := filepath.Join(tmpDir, "file")
	_ = os.WriteFile(fPath, []byte("hello"), 0644)
	subOfFile := filepath.Join(fPath, "sub")

	// 115 - lstatRegularFile on ENOTDIR
	if _, _, err := lstatRegularFile(subOfFile); err == nil {
		t.Fatal("expected error on ENOTDIR in lstatRegularFile")
	}

	// 246 - checkRemovableQueryFile on ENOTDIR
	if err := checkRemovableQueryFile(subOfFile); err == nil {
		t.Fatal("expected error on ENOTDIR in checkRemovableQueryFile")
	}

	// 278 - checkQueryTargetReplaceable on ENOTDIR
	dirInfo, _ := os.Lstat(tmpDir)
	if err := checkQueryTargetReplaceable(dirInfo, subOfFile); err == nil {
		t.Fatal("expected error on ENOTDIR in checkQueryTargetReplaceable")
	}

	// 300 - checkQueryDirAcceptsChanges when dirLstat returns error
	origDirLstat := dirLstat
	dirLstat = func(name string) (os.FileInfo, error) {
		return nil, errors.New("simulated dirLstat error")
	}
	if err := checkQueryDirAcceptsChanges(tmpDir); err == nil {
		t.Fatal("expected error from checkQueryDirAcceptsChanges")
	}
	dirLstat = origDirLstat

	// 338 - chmodDirNoFollow open error
	origOpenDirForChmod := openDirForChmod
	openDirForChmod = func(dir string) (chmodDirFile, error) {
		return nil, errors.New("simulated open error")
	}
	if err := chmodDirNoFollow(tmpDir, dirInfo, 0700); err == nil {
		t.Fatal("expected error from chmodDirNoFollow open failure")
	}

	// 343 - chmodDirNoFollow Stat error
	openDirForChmod = func(dir string) (chmodDirFile, error) {
		return &mockChmodDirFile{
			statFn: func() (os.FileInfo, error) {
				return nil, errors.New("simulated stat error")
			},
		}, nil
	}
	if err := chmodDirNoFollow(tmpDir, dirInfo, 0700); err == nil {
		t.Fatal("expected error from chmodDirNoFollow Stat failure")
	}

	// 346 - chmodDirNoFollow !IsDir
	openDirForChmod = func(dir string) (chmodDirFile, error) {
		return &mockChmodDirFile{
			statFn: func() (os.FileInfo, error) {
				return mockFileInfo{isDir: false}, nil
			},
		}, nil
	}
	if err := chmodDirNoFollow(tmpDir, dirInfo, 0700); err == nil {
		t.Fatal("expected error from chmodDirNoFollow !IsDir")
	}
	openDirForChmod = origOpenDirForChmod

	// readRegularFileBudgeted tests:
	origOpenRegular := openRegularFileForRead
	defer func() { openRegularFileForRead = origOpenRegular }()

	// 190 - openRegularFileForRead returns os.ErrNotExist
	openRegularFileForRead = func(filePath string) (safeReadFile, error) {
		return nil, os.ErrNotExist
	}
	if data, exists, err := readRegularFileBudgeted(fPath, 100, nil); err != nil || exists || data != nil {
		t.Fatalf("expected exists=false, err=nil; got %v, %v, %v", exists, err, data)
	}

	// 197 - f.Stat() error
	openRegularFileForRead = func(filePath string) (safeReadFile, error) {
		return &mockSafeReadFile{
			statFn: func() (os.FileInfo, error) {
				return nil, errors.New("simulated stat error")
			},
		}, nil
	}
	if _, _, err := readRegularFileBudgeted(fPath, 100, nil); err == nil {
		t.Fatal("expected error on f.Stat failure")
	}

	// 200 - !os.SameFile
	openRegularFileForRead = func(filePath string) (safeReadFile, error) {
		return &mockSafeReadFile{
			statFn: func() (os.FileInfo, error) {
				return mockFileInfo{isDir: false, mode: 0644}, nil
			},
		}, nil
	}
	if _, _, err := readRegularFileBudgeted(fPath, 100, nil); err == nil {
		t.Fatal("expected error on !os.SameFile")
	}

	// 204 - f.Read() error
	info, _ := os.Lstat(fPath)
	openRegularFileForRead = func(filePath string) (safeReadFile, error) {
		return &mockSafeReadFile{
			statFn: func() (os.FileInfo, error) {
				return info, nil
			},
			readFn: func(p []byte) (int, error) {
				return 0, errors.New("simulated read error")
			},
		}, nil
	}
	if _, _, err := readRegularFileBudgeted(fPath, 100, nil); err == nil {
		t.Fatal("expected error on f.Read failure")
	}

	// 207 - len(data) > maxSize
	openRegularFileForRead = func(filePath string) (safeReadFile, error) {
		return &mockSafeReadFile{
			statFn: func() (os.FileInfo, error) {
				return info, nil
			},
			readFn: func(p []byte) (int, error) {
				copy(p, "12345678901234567890")
				return 20, io.EOF
			},
		}, nil
	}
	if _, _, err := readRegularFileBudgeted(fPath, 10, nil); err == nil {
		t.Fatal("expected error on len(data) > maxSize")
	}

	// 182-184 - len(data) > info.Size() with budget failure
	bgt := &queryReadBudget{limit: 5, remaining: 5}
	if data, _, err := readRegularFileBudgeted(fPath, 100, bgt); err == nil || data != nil {
		t.Fatal("expected budget error when file grew and budget is full")
	}

	// 182 - len(data) > info.Size() with budget success
	bgt2 := &queryReadBudget{limit: 100, remaining: 100}
	if data, _, err := readRegularFileBudgeted(fPath, 100, bgt2); err != nil || len(data) != 20 {
		t.Fatalf("expected success when budget has space, got data=%d err=%v", len(data), err)
	}
}

func TestCoverage_Cat3_QueryLocation(t *testing.T) {
	// 141 - segment > maxQuerySegmentLength
	if _, ok := validateQuerySegmentReason(strings.Repeat("a", 256)); ok {
		t.Fatal("expected failure on long segment")
	}

	// 195 - segment containing NUL byte
	if _, ok := validateQueryReadSegmentReason("abc\x00def"); ok {
		t.Fatal("expected failure on NUL byte")
	}

	// 290 - walkQueryDir create=true where parent is read-only
	p := t.TempDir()
	_ = os.Chmod(p, 0500)
	t.Cleanup(func() { _ = os.Chmod(p, 0700) })
	if _, err := walkQueryDir(p, "sub", "q1", true); err == nil {
		t.Fatal("expected error from walkQueryDir create=true in read-only parent")
	}

	// 295 - walkQueryDir create=false where Lstat returns ENOTDIR
	tmpFile := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(tmpFile, []byte("x"), 0644)
	if _, err := walkQueryDir(tmpFile, "sub", "q1", false); err == nil {
		t.Fatal("expected error from walkQueryDir create=false on ENOTDIR")
	}
}

func TestCoverage_Cat3_QueryLock(t *testing.T) {
	// 52 - solaris flags
	if flags := queryLockOpenFlagsForOS("solaris"); flags&os.O_RDWR == 0 {
		t.Fatalf("expected O_RDWR for solaris, got %d", flags)
	}

	tmpDir := t.TempDir()
	qs := testFsQueriesStore(tmpDir)

	// 100 - withQueryLock with canceled ctx
	cancCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := qs.withQueryLock(cancCtx, func(g queryLockGuard) error { return nil }); err == nil {
		t.Fatal("expected error with canceled context")
	}

	// 169 - withQueryReadLock with canceled ctx
	if err := qs.withQueryReadLock(cancCtx, func(g queryLockGuard) error { return nil }); err == nil {
		t.Fatal("expected error with canceled context")
	}

	// 116 - withQueryLock flock timeout
	txnDir, err := ensureQueryTxnDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	lockFile := filepath.Join(txnDir, queryTxnLockFile)
	fl := flock.New(lockFile)
	locked, err := fl.TryLock()
	if err != nil || !locked {
		t.Fatalf("failed to hold flock: %v, %v", locked, err)
	}
	timeoutCtx, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	if err := qs.withQueryLock(timeoutCtx, func(g queryLockGuard) error { return nil }); err == nil {
		t.Fatal("expected lock timeout error")
	}
	_ = fl.Unlock()

	// 226 - ensureQueryTxnDir walkQueryDir error
	tmpFile := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(tmpFile, []byte("x"), 0644)
	if _, err := ensureQueryTxnDir(tmpFile); err == nil {
		t.Fatal("expected error on file root in ensureQueryTxnDir")
	}

	// 254 - ensureQueryTxnDir default Lstat error (ENOTDIR)
	if _, err := ensureQueryTxnDir(filepath.Join(tmpFile, "sub")); err == nil {
		t.Fatal("expected ENOTDIR error in ensureQueryTxnDir")
	}

	// 234, 237, 239, 241 - mkdirTxnDir hooks
	origMkdir := mkdirTxnDir
	defer func() { mkdirTxnDir = origMkdir }()

	// 241 - mkdirTxnDir returns permission error
	mkdirTxnDir = func(name string, perm os.FileMode) error {
		return os.ErrPermission
	}
	freshDir := t.TempDir()
	if _, err := ensureQueryTxnDir(freshDir); err == nil {
		t.Fatal("expected error on mkdirTxnDir permission failure")
	}

	// 234 - mkdirTxnDir returns os.ErrExist and Lstat fails
	mkdirTxnDir = func(name string, perm os.FileMode) error {
		return os.ErrExist
	}
	if _, err := ensureQueryTxnDir(freshDir); err == nil {
		t.Fatal("expected error when txnDir does not actually exist on ErrExist")
	}

	// 239 - mkdirTxnDir returns os.ErrExist and directory exists and vets
	realTxn := filepath.Join(freshDir, reservedQueryTxnDirName)
	_ = os.Mkdir(realTxn, 0700)
	firstLstat := true
	origTxnDirLstat := txnDirLstat
	txnDirLstat = func(name string) (os.FileInfo, error) {
		if firstLstat {
			firstLstat = false
			return nil, os.ErrNotExist
		}
		return os.Lstat(name)
	}
	if _, err := ensureQueryTxnDir(freshDir); err != nil {
		t.Fatalf("unexpected error on valid existing dir: %v", err)
	}

	// 237 - mkdirTxnDir returns os.ErrExist and vet fails (e.g. symlink)
	_ = os.Remove(realTxn)
	targetDir := t.TempDir()
	_ = os.Symlink(targetDir, realTxn)
	firstLstat = true
	if _, err := ensureQueryTxnDir(freshDir); err == nil {
		t.Fatal("expected error vetting symlink txnDir")
	}
	txnDirLstat = origTxnDirLstat
	_ = os.Remove(realTxn)

	// 291 - vetExistingQueryTxnDir chmod failure
	broadDir := filepath.Join(t.TempDir(), "broad")
	_ = os.Mkdir(broadDir, 0777)
	bInfo, _ := os.Lstat(broadDir)
	origOpenDirForChmod := openDirForChmod
	openDirForChmod = func(dir string) (chmodDirFile, error) {
		return nil, errors.New("simulated chmod open error")
	}
	if err := vetExistingQueryTxnDir(broadDir, bInfo); err == nil {
		t.Fatal("expected error when chmod fails")
	}
	openDirForChmod = origOpenDirForChmod

	// 295 - txnDirLstat re-inspect error
	origTxnDirLstat = txnDirLstat
	txnDirLstat = func(name string) (os.FileInfo, error) {
		return nil, errors.New("simulated re-inspect error")
	}
	if err := vetExistingQueryTxnDir(broadDir, bInfo); err == nil {
		t.Fatal("expected error when re-inspect fails")
	}

	// 298 - txnDirLstat returns broad permissions
	txnDirLstat = func(name string) (os.FileInfo, error) {
		return mockFileInfo{isDir: true, mode: 0777}, nil
	}
	if err := vetExistingQueryTxnDir(broadDir, bInfo); err == nil {
		t.Fatal("expected error when re-inspect shows changed permissions")
	}
	txnDirLstat = origTxnDirLstat

	// 330 - queryTxnDirHasRecoveryContent ENOTDIR error
	if _, err := queryTxnDirHasRecoveryContent(tmpFile); err == nil {
		t.Fatal("expected ENOTDIR error from queryTxnDirHasRecoveryContent")
	}

	// 338 - queryTxnDirHasRecoveryContent slot dir error
	slotFailDir := t.TempDir()
	origFileOwned := fileOwnedByCurrentUser
	fileOwnedByCurrentUser = func(os.FileInfo) bool { return false }
	_ = os.Mkdir(queryTxnSlotDir(slotFailDir, 1), 0700)
	// Now queryTxnDirHasRecoveryContent on slotFailDir checks slot 1 with Lstat which succeeds
	if has, err := queryTxnDirHasRecoveryContent(slotFailDir); err != nil || !has {
		t.Fatalf("expected has=true for slot 1, got %v, %v", has, err)
	}
	fileOwnedByCurrentUser = origFileOwned

	// 121 - withQueryLock when tryLockContext returns (false, nil)
	origTryLock := tryLockContext
	tryLockContext = func(fl *flock.Flock, ctx context.Context, retryDelay time.Duration) (bool, error) {
		return false, nil
	}
	if err := qs.withQueryLock(context.Background(), func(g queryLockGuard) error { return nil }); err == nil {
		t.Fatal("expected error when tryLockContext returns false, nil")
	}
	tryLockContext = origTryLock

	// 231 - ensureQueryTxnDir walkQueryDir error (root cannot be created)
	roParent := t.TempDir()
	_ = os.Chmod(roParent, 0500)
	t.Cleanup(func() { _ = os.Chmod(roParent, 0700) })
	if _, err := ensureQueryTxnDir(filepath.Join(roParent, "missing_root")); err == nil {
		t.Fatal("expected error from ensureQueryTxnDir when root cannot be created")
	}

	// 259 - ensureQueryTxnDir switch default (txnDirLstat returns ErrPermission)
	origTxnDirLstat2 := txnDirLstat
	txnDirLstat = func(name string) (os.FileInfo, error) {
		return nil, os.ErrPermission
	}
	if _, err := ensureQueryTxnDir(t.TempDir()); err == nil {
		t.Fatal("expected error from ensureQueryTxnDir on ErrPermission")
	}
	txnDirLstat = origTxnDirLstat2

	// 307, 343 - queryTxnDirHasRecoveryContent slot dir error
	txnDirLstat = func(name string) (os.FileInfo, error) {
		if strings.Contains(name, queryTxnSlotPrefix) {
			return nil, errors.New("simulated slot lstat error")
		}
		return os.Lstat(name)
	}
	if _, err := queryTxnDirHasRecoveryContent(t.TempDir()); err == nil {
		t.Fatal("expected error from queryTxnDirHasRecoveryContent on slot error")
	}
	if err := vetExistingQueryTxnDir(broadDir, bInfo); err == nil {
		t.Fatal("expected error from vetExistingQueryTxnDir when queryTxnDirHasRecoveryContent fails")
	}
	txnDirLstat = origTxnDirLstat2
}

func TestCoverage_Cat3_QueryTxn(t *testing.T) {
	tmpDir := t.TempDir()

	// 320 - writeTxnFileExclusive close error
	origOpenExclusive := openExclusiveTxnFile
	openExclusiveTxnFile = func(filePath string) (exclusiveFile, error) {
		return &mockExclusiveFile{
			closeFn: func() error {
				return errors.New("simulated close error")
			},
		}, nil
	}
	if err := writeTxnFileExclusive(tmpDir, "test.txt", []byte("data")); err == nil {
		t.Fatal("expected close error from writeTxnFileExclusive")
	}

	// 323, 327 - writeTxnFileExclusive write error
	openExclusiveTxnFile = func(filePath string) (exclusiveFile, error) {
		return &mockExclusiveFile{
			writeFn: func(p []byte) (int, error) {
				return 0, errors.New("simulated write error")
			},
		}, nil
	}
	if err := writeTxnFileExclusive(tmpDir, "test.txt", []byte("data")); err == nil {
		t.Fatal("expected write error from writeTxnFileExclusive")
	}

	// 330 - writeTxnFileExclusive flush error
	openExclusiveTxnFile = func(filePath string) (exclusiveFile, error) {
		return &mockExclusiveFile{
			syncFn: func() error {
				return errors.New("simulated sync error")
			},
		}, nil
	}
	if err := writeTxnFileExclusive(tmpDir, "test.txt", []byte("data")); err == nil {
		t.Fatal("expected sync error from writeTxnFileExclusive")
	}
	openExclusiveTxnFile = origOpenExclusive

	// 355 - writeJournal marshalJournal error
	origMarshalJournal := marshalJournal
	marshalJournal = func(v any) ([]byte, error) {
		return nil, errors.New("simulated marshal error")
	}
	validJ := queryTxnJournal{
		Operation:    queryTxnOpPut,
		ID:           "q1",
		JSONFileName: "q1.query.json",
		JSONHash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		BodyFileName: "q1.query.sql",
		BodyHash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
	if err := writeJournal(tmpDir, validJ); err == nil {
		t.Fatal("expected marshal error from writeJournal")
	}
	marshalJournal = origMarshalJournal

	// 358 - writeJournal size > maxQueryTxnJournalSize
	bigPath := strings.Repeat("a/", 35000)
	bigJ := validJ
	bigJ.FolderPath = strings.TrimSuffix(bigPath, "/")
	if err := writeJournal(tmpDir, bigJ); err == nil {
		t.Fatal("expected journal size limit error from writeJournal")
	}

	// 364 - writeJournal journalPath Lstat ENOTDIR
	tmpFile := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(tmpFile, []byte("x"), 0644)
	if err := writeJournal(tmpFile, validJ); err == nil {
		t.Fatal("expected ENOTDIR error from writeJournal")
	}

	// 371 - writeJournal os.Rename error (journalPath is non-empty dir)
	txnDir := t.TempDir()
	jDir := filepath.Join(txnDir, queryTxnJournalFile)
	_ = os.Mkdir(jDir, 0755)
	_ = os.WriteFile(filepath.Join(jDir, "child"), []byte("x"), 0644)
	if err := writeJournal(txnDir, validJ); err == nil {
		t.Fatal("expected rename error from writeJournal")
	}

	// 395 - fsyncDirBestEffort sync error
	origOpenDirForSync := openDirForSync
	openDirForSync = func(dir string) (syncableDir, error) {
		return &mockSyncableDir{
			syncFn: func() error {
				return errors.New("simulated dir sync error")
			},
		}, nil
	}
	fsyncDirBestEffort(tmpDir)
	openDirForSync = origOpenDirForSync

	// 409 - cleanupTxnArtifacts remove error (journal is non-empty dir)
	cleanupDir := t.TempDir()
	jNonEmpty := filepath.Join(cleanupDir, queryTxnJournalFile)
	_ = os.Mkdir(jNonEmpty, 0755)
	_ = os.WriteFile(filepath.Join(jNonEmpty, "child"), []byte("x"), 0644)
	if err := cleanupTxnArtifacts(cleanupDir); err == nil {
		t.Fatal("expected error from cleanupTxnArtifacts on non-empty journal dir")
	}

	// 448 - sweepUncommittedTxnArtifacts removeIfExists failure
	sweepDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(sweepDir, queryTxnStagedJSON), []byte("{}"), 0600)
	_ = os.Chmod(sweepDir, 0500)
	_ = sweepUncommittedTxnArtifacts(sweepDir)
	_ = os.Chmod(sweepDir, 0700)

	// 510 - ensureInstalled rename error
	_ = os.WriteFile(filepath.Join(sweepDir, queryTxnStagedJSON), []byte("x"), 0600)
	h := hashBytes([]byte("x"))
	targetDir := t.TempDir()
	_ = os.Mkdir(filepath.Join(targetDir, "final.json"), 0755)
	if err := ensureInstalled(sweepDir, queryTxnStagedJSON, targetDir, "final.json", h); err == nil {
		t.Fatal("expected rename error from ensureInstalled")
	}

	// 547 - readJournal when readJournalFile returns exists=false
	origReadJournalFile := readJournalFile
	readJournalFile = func(filePath string, maxSize int64) ([]byte, bool, error) {
		return nil, false, nil
	}
	_ = os.WriteFile(filepath.Join(sweepDir, queryTxnJournalFile), []byte("{}"), 0600)
	if _, present, err := readJournal(sweepDir); err != nil || present {
		t.Fatalf("expected present=false, err=nil; got %v, %v", present, err)
	}
	readJournalFile = origReadJournalFile

	// 356 - writeJournal marshalJournal error
	origMarshal := marshalJournal
	marshalJournal = func(v any) ([]byte, error) {
		return nil, errors.New("simulated marshal error")
	}
	if err := writeJournal(tmpDir, validJ); err == nil {
		t.Fatal("expected error from writeJournal when marshal fails")
	}

	// 359 - writeJournal journal exceeds maxQueryTxnJournalSize
	marshalJournal = func(v any) ([]byte, error) {
		return make([]byte, maxQueryTxnJournalSize+1), nil
	}
	if err := writeJournal(tmpDir, validJ); err == nil {
		t.Fatal("expected error from writeJournal when journal exceeds size")
	}
	marshalJournal = origMarshal

	// 365 - writeJournal journalLstat error
	origJournalLstat := journalLstat
	journalLstat = func(name string) (os.FileInfo, error) {
		return nil, errors.New("simulated journal lstat error")
	}
	if err := writeJournal(tmpDir, validJ); err == nil {
		t.Fatal("expected error from writeJournal on journalLstat error")
	}
	journalLstat = origJournalLstat

	// 372 - writeJournal journalRename error
	origJournalRename := journalRename
	journalRename = func(oldpath, newpath string) error {
		return errors.New("simulated journal rename error")
	}
	renameDir := t.TempDir()
	if err := writeJournal(renameDir, validJ); err == nil {
		t.Fatal("expected rename error from writeJournal on journalRename error")
	}
	journalRename = origJournalRename

	// 540 - ensureInstalled staged read failure (over maxQueryFileSize)
	stagedBig := filepath.Join(sweepDir, queryTxnStagedJSON)
	fTrunc, _ := os.OpenFile(stagedBig, os.O_CREATE|os.O_RDWR, 0600)
	_ = fTrunc.Truncate(maxQueryFileSize + 1)
	_ = fTrunc.Close()
	emptyTargetDir := t.TempDir()
	if err := ensureInstalled(sweepDir, queryTxnStagedJSON, emptyTargetDir, "final.json", "hash"); err == nil {
		t.Fatal("expected error when staged file exceeds maxQueryFileSize")
	}

	// 676 - finishQueryTransaction journalRemove error
	origJournalRemove := journalRemove
	journalRemove = func(name string) error {
		return errors.New("simulated journal remove error")
	}
	finishDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(finishDir, queryTxnJournalFile), []byte(`{"operation":"put","id":"q1","jsonFileName":"q1.query.json","jsonHash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","bodyFileName":"q1.query.sql","bodyHash":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`), 0600)
	_ = os.WriteFile(filepath.Join(finishDir, queryTxnStagedJSON), []byte("{}"), 0600)
	_ = os.WriteFile(filepath.Join(finishDir, queryTxnStagedBody), []byte("body"), 0600)
	restoreFinish := failQueryTargetStep(t, "rename", "q1.query.json")
	_ = finishQueryTransaction(t.TempDir(), finishDir, validJ)
	restoreFinish()
	journalRemove = origJournalRemove

	// 633 - queryTxnIncompleteError Unwrap
	innerErr := errors.New("inner")
	incErr := &queryTxnIncompleteError{Err: innerErr}
	if !errors.Is(incErr, innerErr) {
		t.Fatalf("expected Unwrap to yield innerErr")
	}

	// 653 - finishQueryTransaction remove journal fails (chmod 0500)
	finishDir = t.TempDir()
	_ = os.WriteFile(filepath.Join(finishDir, queryTxnJournalFile), []byte("{}"), 0600)
	_ = os.WriteFile(filepath.Join(finishDir, queryTxnStagedJSON), []byte("{}"), 0600)
	_ = os.WriteFile(filepath.Join(finishDir, queryTxnStagedBody), []byte("body"), 0600)
	_ = os.Chmod(finishDir, 0500)
	_ = finishQueryTransaction(t.TempDir(), finishDir, validJ)
	_ = os.Chmod(finishDir, 0700)

	// 694 - queryTxnUntouched delete targetDir fails
	delJ := queryTxnJournal{
		Operation:  queryTxnOpDelete,
		FolderPath: "invalid/..path",
		ID:         "q1",
	}
	if queryTxnUntouched(tmpFile, t.TempDir(), delJ) {
		t.Fatal("expected untouched=false for invalid delete folder")
	}

	// 728-732 - queryTxnNothingHalfDone
	delJValid := queryTxnJournal{
		Operation:    queryTxnOpDelete,
		ID:           "q1",
		JSONFileName: "q1.query.json",
		BodyFileName: "q1.sql",
	}
	dirWithOneTarget := t.TempDir()
	_ = os.WriteFile(filepath.Join(dirWithOneTarget, "q1.query.json"), []byte("{}"), 0644)
	if queryTxnNothingHalfDone(t.TempDir(), t.TempDir(), delJValid, dirWithOneTarget) {
		t.Fatal("expected false when one target is still present")
	}
	_ = os.Remove(filepath.Join(dirWithOneTarget, "q1.query.json"))
	if !queryTxnNothingHalfDone(t.TempDir(), t.TempDir(), delJValid, dirWithOneTarget) {
		t.Fatal("expected true when all targets are removed")
	}

	// 1000 - readQueryPair corrupt JSON
	pairDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(pairDir, "q1.query.json"), []byte("{corrupt"), 0644)
	g := queryLockGuard{}
	if _, err := g.readQueryPair("", pairDir, "q1"); err == nil {
		t.Fatal("expected error on corrupt query json")
	}

	// 1003 - readQueryPair empty Type
	_ = os.WriteFile(filepath.Join(pairDir, "q1.query.json"), []byte(`{"id":"q1"}`), 0644)
	pair, err := g.readQueryPair("", pairDir, "q1")
	if err != nil || !pair.exists || pair.revision == "" {
		t.Fatalf("expected valid pair with computed revision, got %+v, %v", pair, err)
	}
}

func TestCoverage_Cat3_QueryTxnSlots(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "f")
	_ = os.WriteFile(tmpFile, []byte("x"), 0644)

	// 109 - vetQueryTxnSlotDir ENOTDIR
	if _, err := vetQueryTxnSlotDir(filepath.Join(tmpFile, "slot")); err == nil {
		t.Fatal("expected ENOTDIR error from vetQueryTxnSlotDir")
	}

	// 115 - vetQueryTxnSlotDir ownedByOther
	slotDir := t.TempDir()
	origOwned := fileOwnedByCurrentUser
	fileOwnedByCurrentUser = func(os.FileInfo) bool { return false }
	if _, err := vetQueryTxnSlotDir(slotDir); err == nil {
		t.Fatal("expected owned-by-other error from vetQueryTxnSlotDir")
	}
	fileOwnedByCurrentUser = origOwned

	// 126 - ensureQueryTxnSlotDir mkdir error
	if err := ensureQueryTxnSlotDir(filepath.Join(tmpFile, "slot")); err == nil {
		t.Fatal("expected mkdir error from ensureQueryTxnSlotDir")
	}

	// 198 - queryTxnOwnsAFileIn with empty dir
	j := queryTxnJournal{JSONFileName: "q.json"}
	if queryTxnOwnsAFileIn("", j) {
		t.Fatal("expected false for empty dir")
	}

	// 202 - queryTxnOwnsAFileIn with empty PrevBodyFileName
	if queryTxnOwnsAFileIn(slotDir, j) {
		t.Fatal("expected false when file does not exist")
	}

	// 258 - stagingDir ensureQueryTxnSlotDir error
	origMkdirSlot := mkdirSlotDir
	mkdirSlotDir = func(name string, perm os.FileMode) error {
		return errors.New("simulated slot error")
	}
	slotTxnDir := t.TempDir()
	g := queryLockGuard{
		txnDir: slotTxnDir,
		stuck: []stuckQueryTxn{
			{slot: queryTxnSlotDir(slotTxnDir, 0)},
		},
	}
	if _, err := g.stagingDir(); err == nil {
		t.Fatal("expected error from stagingDir when slot mkdir fails")
	}
	mkdirSlotDir = origMkdirSlot

	// 352 - stuckQueryTxn.sharesAFileWith empty dir
	st := stuckQueryTxn{}
	if st.sharesAFileWith("", "q1") {
		t.Fatal("expected false for empty dir in sharesAFileWith")
	}

	// 364 - stuckQueryTxn.sharesAFileWith same file
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	f1 := filepath.Join(dir1, "q1.query.json")
	_ = os.WriteFile(f1, []byte("{}"), 0644)
	f2 := filepath.Join(dir2, "q1.query.json")
	_ = os.Link(f1, f2)
	st2 := stuckQueryTxn{
		dir: dir1,
		journal: queryTxnJournal{
			ID:           "q1",
			JSONFileName: "q1.query.json",
		},
	}
	if !st2.sharesAFileWith(dir2, "q1") {
		t.Fatal("expected true for hard-linked same file in sharesAFileWith")
	}
}

func TestCoverage_Cat3_StoreProjectSaver(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	ps := newFsProjectStore("p1", tmpDir)

	validProj := datatug.Project{
		ProjectItem: testProjectItem("p1"),
	}

	// 34, 196 - saveProjectFile / putProjectFile error
	_ = os.Mkdir(filepath.Join(tmpDir, storage.ProjectSummaryFileName), 0755)
	if err := ps.SaveProject(ctx, &validProj); err == nil {
		t.Fatal("expected error saving project file")
	}
	_ = os.Remove(filepath.Join(tmpDir, storage.ProjectSummaryFileName))

	// 43 - SaveEntities error: entities is a regular file
	_ = os.WriteFile(filepath.Join(tmpDir, storage.EntitiesFolder), []byte("file"), 0644)
	projWithEntity := datatug.Project{
		ProjectItem: testProjectItem("p1"),
		Entities: datatug.Entities{
			&datatug.Entity{ProjectItem: testProjectItem("e1")},
		},
	}
	if err := ps.SaveProject(ctx, &projWithEntity); err == nil {
		t.Fatal("expected error saving entities")
	}
	_ = os.Remove(filepath.Join(tmpDir, storage.EntitiesFolder))

	// 56 - SaveEnvironments error: environments is a regular file
	_ = os.WriteFile(filepath.Join(tmpDir, storage.EnvironmentsFolder), []byte("file"), 0644)
	projWithEnv := datatug.Project{
		ProjectItem: testProjectItem("p1"),
		Environments: datatug.Environments{
			&datatug.Environment{ProjectItem: testProjectItem("env1")},
		},
	}
	if err := ps.SaveProject(ctx, &projWithEnv); err == nil {
		t.Fatal("expected error saving environments")
	}
	_ = os.Remove(filepath.Join(tmpDir, storage.EnvironmentsFolder))

	// 68 - SaveDbModels error: dbmodels is a regular file
	_ = os.WriteFile(filepath.Join(tmpDir, storage.DbModelsFolder), []byte("file"), 0644)
	projWithModel := datatug.Project{
		ProjectItem: testProjectItem("p1"),
		DbModels: datatug.DbModels{
			&datatug.DbModel{ProjectItem: testProjectItem("m1")},
		},
	}
	if err := ps.SaveProject(ctx, &projWithModel); err == nil {
		t.Fatal("expected error saving db models")
	}
	_ = os.Remove(filepath.Join(tmpDir, storage.DbModelsFolder))

	// 80 - saveBoards error: boards is a regular file
	_ = os.WriteFile(filepath.Join(tmpDir, storage.BoardsFolder), []byte("file"), 0644)
	projWithBoard := datatug.Project{
		ProjectItem: testProjectItem("p1"),
		Boards: datatug.Boards{
			&datatug.Board{ProjectItem: testProjectItem("b1")},
		},
	}
	if err := ps.SaveProject(ctx, &projWithBoard); err == nil {
		t.Fatal("expected error saving boards")
	}
	_ = os.Remove(filepath.Join(tmpDir, storage.BoardsFolder))

	// 92, 108 - saveQueriesTree error: queries is a regular file
	_ = os.WriteFile(filepath.Join(tmpDir, storage.QueriesFolder), []byte("file"), 0644)
	projWithQueries := datatug.Project{
		ProjectItem: testProjectItem("p1"),
		Queries: &datatug.QueriesFolder{
			Items: []*datatug.QueryDef{
				{ProjectItem: testProjectItem("q1"), Type: datatug.QueryTypeSQL},
			},
		},
	}
	if err := ps.SaveProject(ctx, &projWithQueries); err == nil {
		t.Fatal("expected error saving queries tree")
	}
	_ = os.Remove(filepath.Join(tmpDir, storage.QueriesFolder))
}

func TestCoverage_Cat3_StoreQueries(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	qs := testFsQueriesStore(dir)

	// 161 - loadQueryLocked invalid itemID
	if _, err := qs.loadQueryLocked(ctx, queryLockGuard{}, "invalid\x00id"); err == nil {
		t.Fatal("expected error on invalid itemID")
	}

	// 165 - loadQueryLocked invalid folderPath
	if _, err := qs.loadQueryLocked(ctx, queryLockGuard{}, "invalid\x00folder/id"); err == nil {
		t.Fatal("expected error on invalid folderPath")
	}

	// Helper to trigger resolveQueryLocation failure under flock
	triggerUnderFlock := func(fn func(dir string) error) {
		pDir := t.TempDir()
		qDir := filepath.Join(pDir, "queries")
		_ = os.MkdirAll(filepath.Join(qDir, "sub"), 0755)
		txnDir, _ := ensureQueryTxnDir(qDir)
		lockFile := filepath.Join(txnDir, queryTxnLockFile)
		fl := flock.New(lockFile)
		_ = fl.Lock()

		errCh := make(chan error, 1)
		go func() {
			errCh <- fn(qDir)
		}()

		time.Sleep(20 * time.Millisecond)
		_ = os.RemoveAll(filepath.Join(qDir, "sub"))
		_ = os.WriteFile(filepath.Join(qDir, "sub"), []byte("x"), 0644)
		_ = fl.Unlock()

		err := <-errCh
		if err == nil {
			t.Fatal("expected error under lock")
		}
	}

	// 206 - UpdateQuery resolveQueryLocation failure under lock
	triggerUnderFlock(func(qDir string) error {
		s := testFsQueriesStore(qDir)
		q := datatug.QueryDef{
			ProjectItem: testProjectItem("sub/q1"),
			Type:        datatug.QueryTypeSQL,
		}
		_, err := s.UpdateQuery(ctx, q)
		return err
	})

	// 237 - DeleteQuery resolveQueryLocation failure under lock
	triggerUnderFlock(func(qDir string) error {
		s := testFsQueriesStore(qDir)
		return s.DeleteQuery(ctx, "sub/q1")
	})

	// 270 - SaveQuery resolveQueryLocation failure under lock
	triggerUnderFlock(func(qDir string) error {
		s := testFsQueriesStore(qDir)
		q := &datatug.QueryDefWithFolderPath{
			FolderPath: "sub",
			QueryDef: datatug.QueryDef{
				ProjectItem: testProjectItem("q1"),
				Type:        datatug.QueryTypeSQL,
			},
		}
		return s.SaveQuery(ctx, q)
	})
}

func TestCoverage_Cat3_StoreQueriesRevisioned(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	qs := testFsQueriesStore(dir)

	// 35 - LoadQueryRevision canceled ctx
	cancCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := qs.LoadQueryRevision(cancCtx, "q1"); err == nil {
		t.Fatal("expected error on canceled context in LoadQueryRevision")
	}

	// 59 - LoadQueryRevision corrupt JSON
	_ = os.WriteFile(filepath.Join(dir, "q_corrupt.query.json"), []byte(`{"type":"SQL","parameters":123}`), 0644)
	if _, err := qs.LoadQueryRevision(ctx, "q_corrupt"); err == nil {
		t.Fatal("expected error on corrupt JSON in LoadQueryRevision")
	}

	// 91 - PutQuery nil query
	if _, err := qs.PutQuery(ctx, nil, datatug.QueryWriteCondition{}); err == nil {
		t.Fatal("expected error on nil query in PutQuery")
	}

	// 199 - DeleteQueryRevision canceled ctx
	if err := qs.DeleteQueryRevision(cancCtx, "q1", "rev"); err == nil {
		t.Fatal("expected error on canceled context in DeleteQueryRevision")
	}

	// 203 - DeleteQueryRevision invalid id
	if err := qs.DeleteQueryRevision(ctx, "invalid\x00id", "rev"); err == nil {
		t.Fatal("expected error on invalid id in DeleteQueryRevision")
	}

	// 48 - LoadQueryRevision folder error under lock
	pDir := t.TempDir()
	qDir := filepath.Join(pDir, "queries")
	_ = os.MkdirAll(filepath.Join(qDir, "sub"), 0755)
	txnDir, _ := ensureQueryTxnDir(qDir)
	fl := flock.New(filepath.Join(txnDir, queryTxnLockFile))
	_ = fl.Lock()

	errCh := make(chan error, 1)
	go func() {
		s := testFsQueriesStore(qDir)
		_, err := s.LoadQueryRevision(ctx, "sub/q1")
		errCh <- err
	}()
	time.Sleep(20 * time.Millisecond)
	_ = os.RemoveAll(filepath.Join(qDir, "sub"))
	_ = os.WriteFile(filepath.Join(qDir, "sub"), []byte("x"), 0644)
	_ = fl.Unlock()
	if err := <-errCh; err == nil {
		t.Fatal("expected error in LoadQueryRevision under lock")
	}

	// 109, 128 - PutQuery canceled while waiting for lock or before staging
	pDir2 := t.TempDir()
	qDir2 := filepath.Join(pDir2, "queries")
	s2 := testFsQueriesStore(qDir2)
	qPut := &datatug.QueryDefWithFolderPath{
		QueryDef: datatug.QueryDef{
			ProjectItem: testProjectItem("q1"),
			Type:        datatug.QueryTypeSQL,
		},
	}

	// 109 - PutQuery canceled after lock acquisition
	origTryLock := tryLockContext
	ctxPutLock, cancelPutLock := context.WithCancel(context.Background())
	tryLockContext = func(fl *flock.Flock, _ context.Context, retryDelay time.Duration) (bool, error) {
		cancelPutLock()
		return origTryLock(fl, context.Background(), retryDelay)
	}
	if _, err := s2.PutQuery(ctxPutLock, qPut, datatug.QueryWriteCondition{IfNoneMatch: true}); err == nil {
		t.Fatal("expected error when PutQuery is canceled after lock acquired")
	}
	tryLockContext = origTryLock

	stored2, err := s2.PutQuery(ctx, qPut, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatal(err)
	}

	// 128 - PutQuery canceled after line 108
	origOpenRead := openRegularFileForRead
	ctxPut, cancelPut := context.WithCancel(context.Background())
	openRegularFileForRead = func(path string) (safeReadFile, error) {
		cancelPut()
		return origOpenRead(path)
	}
	if _, err := s2.PutQuery(ctxPut, qPut, datatug.QueryWriteCondition{IfMatch: stored2.Revision}); err == nil {
		t.Fatal("expected error from PutQuery when canceled before staging")
	}
	openRegularFileForRead = origOpenRead

	// 208 - DeleteQueryRevision canceled after lock acquisition
	ctxDelLock, cancelDelLock := context.WithCancel(context.Background())
	tryLockContext = func(fl *flock.Flock, _ context.Context, retryDelay time.Duration) (bool, error) {
		cancelDelLock()
		return origTryLock(fl, context.Background(), retryDelay)
	}
	if err := s2.DeleteQueryRevision(ctxDelLock, "q1", "rev"); err == nil {
		t.Fatal("expected error when DeleteQueryRevision is canceled after lock acquired")
	}
	tryLockContext = origTryLock

	// 226 - DeleteQueryRevision canceled after line 207
	ctxDel, cancelDel := context.WithCancel(context.Background())
	openRegularFileForRead = func(path string) (safeReadFile, error) {
		cancelDel()
		return origOpenRead(path)
	}
	if err := s2.DeleteQueryRevision(ctxDel, "q1", stored2.Revision); err == nil {
		t.Fatal("expected error from DeleteQueryRevision when canceled before staging")
	}
	openRegularFileForRead = origOpenRead

	// 212 - DeleteQueryRevision folder error under lock
	_ = os.MkdirAll(filepath.Join(qDir2, "sub"), 0755)
	txnDir2, _ := ensureQueryTxnDir(qDir2)
	fl2 := flock.New(filepath.Join(txnDir2, queryTxnLockFile))
	_ = fl2.Lock()
	errCh2 := make(chan error, 1)
	go func() {
		errCh2 <- s2.DeleteQueryRevision(ctx, "sub/q1", "rev")
	}()
	time.Sleep(20 * time.Millisecond)
	_ = os.RemoveAll(filepath.Join(qDir2, "sub"))
	_ = os.WriteFile(filepath.Join(qDir2, "sub"), []byte("x"), 0644)
	_ = fl2.Unlock()
	if err := <-errCh2; err == nil {
		t.Fatal("expected error under lock in DeleteQueryRevision")
	}

	// 238 - DeleteQueryRevision stagingDir error
	pDir3 := t.TempDir()
	qDir3 := filepath.Join(pDir3, "queries")
	s3 := testFsQueriesStore(qDir3)
	stored, err := s3.PutQuery(ctx, qPut, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatal(err)
	}
	txnDir3, _ := ensureQueryTxnDir(qDir3)
	_ = os.WriteFile(filepath.Join(txnDir3, queryTxnJournalFile), []byte(`{"operation":"delete","id":"other","jsonFileName":"other.query.json"}`), 0600)
	restoreTarget := failQueryTargetStep(t, "remove", "other.query.json")
	origMkdirSlot := mkdirSlotDir
	mkdirSlotDir = func(name string, perm os.FileMode) error {
		return errors.New("simulated slot error")
	}
	_ = s3.DeleteQueryRevision(ctx, "q1", stored.Revision)
	mkdirSlotDir = origMkdirSlot
	restoreTarget()
}

func TestCoverage_Cat3_StoreQueriesSaver(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	qs := testFsQueriesStore(dir)

	// 41 - openReadmeFile error (not exist)
	origOpenReadme := openReadmeFile
	defer func() { openReadmeFile = origOpenReadme }()
	openReadmeFile = func(filePath string) (io.WriteCloser, error) {
		return nil, errors.New("simulated open error")
	}
	if err := qs.CreateQueryFolder(ctx, "", "f1"); err == nil {
		t.Fatal("expected open error from CreateQueryFolder")
	}

	// 44 - openReadmeFile write error
	openReadmeFile = func(filePath string) (io.WriteCloser, error) {
		return &mockWriteCloser{
			writeFn: func(p []byte) (int, error) {
				return 0, errors.New("simulated write error")
			},
		}, nil
	}
	if err := qs.CreateQueryFolder(ctx, "", "f2"); err == nil {
		t.Fatal("expected write error from CreateQueryFolder")
	}

	// 48 - openReadmeFile close error
	openReadmeFile = func(filePath string) (io.WriteCloser, error) {
		return &mockWriteCloser{
			closeFn: func() error {
				return errors.New("simulated close error")
			},
		}, nil
	}
	if err := qs.CreateQueryFolder(ctx, "", "f3"); err == nil {
		t.Fatal("expected close error from CreateQueryFolder")
	}
	openReadmeFile = origOpenReadme

	// 70 - CreateQuery resolveQueryLocation error under lock
	pDir := t.TempDir()
	qDir := filepath.Join(pDir, "queries")
	_ = os.MkdirAll(filepath.Join(qDir, "sub"), 0755)
	txnDir, _ := ensureQueryTxnDir(qDir)
	fl := flock.New(filepath.Join(txnDir, queryTxnLockFile))
	_ = fl.Lock()

	errCh := make(chan error, 1)
	go func() {
		s := testFsQueriesStore(qDir)
		q := datatug.QueryDefWithFolderPath{
			FolderPath: "sub",
			QueryDef: datatug.QueryDef{
				ProjectItem: testProjectItem("q1"),
				Type:        datatug.QueryTypeSQL,
			},
		}
		_, err := s.CreateQuery(ctx, q)
		errCh <- err
	}()
	time.Sleep(20 * time.Millisecond)
	_ = os.RemoveAll(filepath.Join(qDir, "sub"))
	_ = os.WriteFile(filepath.Join(qDir, "sub"), []byte("x"), 0644)
	_ = fl.Unlock()
	if err := <-errCh; err == nil {
		t.Fatal("expected error under lock in CreateQuery")
	}

	// 74 - CreateQuery readQueryPair error under lock
	pDir2 := t.TempDir()
	qDir2 := filepath.Join(pDir2, "queries")
	_ = os.MkdirAll(filepath.Join(qDir2, "sub"), 0755)
	txnDir2, _ := ensureQueryTxnDir(qDir2)
	fl2 := flock.New(filepath.Join(txnDir2, queryTxnLockFile))
	_ = fl2.Lock()

	errCh2 := make(chan error, 1)
	go func() {
		s := testFsQueriesStore(qDir2)
		q := datatug.QueryDefWithFolderPath{
			FolderPath: "sub",
			QueryDef: datatug.QueryDef{
				ProjectItem: testProjectItem("q1"),
				Type:        datatug.QueryTypeSQL,
			},
		}
		_, err := s.CreateQuery(ctx, q)
		errCh2 <- err
	}()
	time.Sleep(20 * time.Millisecond)
	_ = os.WriteFile(filepath.Join(qDir2, "sub", "q1.query.json"), []byte("{corrupt"), 0644)
	_ = fl2.Unlock()
	if err := <-errCh2; err == nil {
		t.Fatal("expected error reading corrupt pair in CreateQuery")
	}
}

func TestCoverage_Cat3_StoreQueriesWriter(t *testing.T) {
	dir := t.TempDir()
	qs := testFsQueriesStore(dir)
	txnDir, err := ensureQueryTxnDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	g := queryLockGuard{txnDir: txnDir}

	// 29 - stageAndInstallQueryPair queryJSONBytes error
	badJSONQuery := datatug.QueryDef{
		ProjectItem: testProjectItem("q1"),
		Type:        datatug.QueryTypeSQL,
		Parameters: datatug.Parameters{
			{ID: "p1", Type: "string", DefaultValue: make(chan int)},
		},
	}
	if _, err := qs.stageAndInstallQueryPair(g, "", badJSONQuery, currentQueryPair{}); err == nil {
		t.Fatal("expected error from unencodable query")
	}

	// 33 - stageAndInstallQueryPair queryBodyFileName error
	badTypeQuery := datatug.QueryDef{
		ProjectItem: testProjectItem("q1"),
		Type:        "SQL_VERY_LONG_TYPE_THAT_EXCEEDS_32_CHARS",
	}
	if _, err := qs.stageAndInstallQueryPair(g, "", badTypeQuery, currentQueryPair{}); err == nil {
		t.Fatal("expected error from invalid query type")
	}

	// 43 - stageAndInstallQueryPair walkQueryDir error
	validQ := datatug.QueryDef{
		ProjectItem: testProjectItem("q1"),
		Type:        datatug.QueryTypeSQL,
	}
	_ = os.WriteFile(filepath.Join(dir, "blocked"), []byte("x"), 0644)
	if _, err := qs.stageAndInstallQueryPair(g, "blocked/sub", validQ, currentQueryPair{}); err == nil {
		t.Fatal("expected error from walkQueryDir on file segment")
	}

	// 50 - stageAndInstallQueryPair writeStagedFile staged.json error
	_ = os.WriteFile(filepath.Join(txnDir, queryTxnStagedJSON), []byte("pre-existing"), 0600)
	if _, err := qs.stageAndInstallQueryPair(g, "", validQ, currentQueryPair{}); err == nil {
		t.Fatal("expected error staging JSON when staged.json already exists")
	}
	_ = os.Remove(filepath.Join(txnDir, queryTxnStagedJSON))

	// 53 - stageAndInstallQueryPair writeStagedFile staged.body error
	_ = os.WriteFile(filepath.Join(txnDir, queryTxnStagedBody), []byte("pre-existing"), 0600)
	if _, err := qs.stageAndInstallQueryPair(g, "", validQ, currentQueryPair{}); err == nil {
		t.Fatal("expected error staging body when staged.body already exists")
	}
	_ = os.Remove(filepath.Join(txnDir, queryTxnStagedBody))

	// 107 - deleteQueryPairIfExists g.stagingDir() error
	origMkdirSlot := mkdirSlotDir
	mkdirSlotDir = func(name string, perm os.FileMode) error {
		return errors.New("simulated slot error")
	}
	gStuck := queryLockGuard{
		txnDir: txnDir,
		stuck: []stuckQueryTxn{
			{slot: queryTxnSlotDir(txnDir, 0)},
		},
	}
	cur := currentQueryPair{exists: true}
	if err := qs.deleteQueryPairIfExists(gStuck, "", "q1", cur); err == nil {
		t.Fatal("expected error when stagingDir fails in deleteQueryPairIfExists")
	}
	mkdirSlotDir = origMkdirSlot
}
