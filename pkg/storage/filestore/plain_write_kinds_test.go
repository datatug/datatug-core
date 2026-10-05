package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// writeKind is one kind of file the file store writes: write saves one item
// of that kind into the project folder root, file is the slash-separated path
// inside the project of the file it writes, and remove (when the store has a
// delete for the kind) deletes that item again.
//
// leavesAlone is set for the one write that is create-only by design (the
// README of a new query folder): over an entry that is already there it does
// nothing and returns no error, instead of refusing; it still must not write
// through a link. extraFolders names folders besides the ones above file
// that the write also goes through.
type writeKind struct {
	name         string
	file         string
	write        func(root string) error
	remove       func(root string) error
	leavesAlone  bool
	extraFolders []string
	// removeFile is the file remove deletes when it is not file;
	// removesTree is set when remove deletes the folder holding file, which
	// unlinks a link inside it rather than refusing it.
	removeFile  string
	removesTree bool
}

func plainWriteProject() *datatug.Project {
	p := new(datatug.Project)
	p.ID = "p1"
	p.Title = "Project 1"
	p.Access = "public"
	p.Created = &datatug.ProjectCreated{At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	return p
}

func plainWriteItem[T interface{ SetID(string) }](v T, id string) T {
	v.SetID(id)
	return v
}

func plainWriteEnvironment() *datatug.Environment {
	env := plainWriteItem(new(datatug.Environment), "local")
	env.Title = "Local"
	return env
}

func plainWriteCatalog() *datatug.DbCatalog {
	cat := plainWriteItem(new(datatug.DbCatalog), "cat1")
	cat.Driver = "sqlite3"
	cat.Path = "test.db"
	return cat
}

func plainWriteDbModel() *datatug.DbModel {
	m := plainWriteItem(new(datatug.DbModel), "m1")
	m.Title = "Model 1"
	return m
}

func plainWriteBoard() *datatug.Board {
	b := plainWriteItem(new(datatug.Board), "b1")
	b.Title = "Board 1"
	return b
}

func plainWriteEntity() *datatug.Entity {
	e := plainWriteItem(new(datatug.Entity), "e1")
	e.Title = "Entity 1"
	return e
}

func plainWriteFolder() *datatug.Folder {
	return plainWriteItem(new(datatug.Folder), "f1")
}

func plainWriteDriver() *datatug.ProjDbDriver {
	d := plainWriteItem(new(datatug.ProjDbDriver), "sqlite3")
	d.Title = "SQLite"
	return d
}

func plainWriteDbServer() *datatug.ProjDbServer {
	s := &datatug.ProjDbServer{Server: datatug.ServerRef{Driver: "mysql", Host: "srv1"}}
	s.ID = s.Server.GetID()
	s.Title = "MySQL Server"
	return s
}

func plainWriteRecordset() *datatug.RecordsetDefinition {
	r := plainWriteItem(new(datatug.RecordsetDefinition), "rs1")
	r.Title = "Recordset 1"
	r.Type = "recordset"
	return r
}

// writeKinds lists every kind of file the file store writes into a project
// folder: the project file and its README, an environment, an environment's
// DB catalog and DB server, a DB model, a board, an entity, a folder, a
// recordset definition, a query (with its body), a DB driver, a DB server of
// a driver and a DB catalog of a server, a README of a DB server, and a file
// the project creator writes through the storage.
func writeKinds() []writeKind {
	ctx := context.Background()
	return []writeKind{
		{name: "project file", file: "datatug-project.json", write: func(root string) error {
			return newFsProjectStore("p1", root).SaveProject(ctx, plainWriteProject())
		}},
		{name: "project README", file: "README.md", write: func(root string) error {
			return newFsProjectStore("p1", root).SaveProject(ctx, plainWriteProject())
		}},
		{name: "environment", file: "environments/local/local.env.json",
			write: func(root string) error {
				return newFsEnvironmentsStore(root).SaveEnvironment(ctx, plainWriteEnvironment())
			},
			remove:      func(root string) error { return newFsEnvironmentsStore(root).DeleteEnvironment(ctx, "local") },
			removesTree: true},
		{name: "environment catalog", file: "environments/local/catalogs/cat1/cat1.db.json",
			write: func(root string) error {
				return newFsEnvCatalogsStore(root).SaveEnvDbCatalog(ctx, "local", "srv", "", plainWriteCatalog())
			},
			remove: func(root string) error {
				return newFsEnvCatalogsStore(root).DeleteEnvDbCatalog(ctx, "local", "srv", "cat1")
			}},
		{name: "environment server", file: "environments/local/srv1:5432.server.json",
			write: func(root string) error {
				return newFsEnvDbServersStore(root).SaveEnvDbServer(ctx, "local",
					&datatug.EnvDbServer{ServerRef: datatug.ServerRef{Driver: "postgres", Host: "srv1", Port: 5432}})
			},
			remove: func(root string) error {
				return newFsEnvDbServersStore(root).DeleteEnvDbServer(ctx, "local", "srv1:5432")
			}},
		{name: "model", file: "dbmodels/m1/m1.dbmodel.json",
			write:  func(root string) error { return newFsDbModelsStore(root).SaveDbModel(ctx, plainWriteDbModel()) },
			remove: func(root string) error { return newFsDbModelsStore(root).DeleteDbModel(ctx, "m1") }},
		{name: "board", file: "boards/b1/board.json",
			write:  func(root string) error { return newFsBoardsStore(root).SaveBoard(ctx, plainWriteBoard()) },
			remove: func(root string) error { return newFsBoardsStore(root).DeleteBoard(ctx, "b1") }},
		{name: "entity", file: "entities/e1/e1.entity.json",
			write:  func(root string) error { return newFsEntitiesStore(root).SaveEntity(ctx, plainWriteEntity()) },
			remove: func(root string) error { return newFsEntitiesStore(root).DeleteEntity(ctx, "e1") }},
		{name: "folder", file: "folders/f1/.datatug-folder.json",
			write:  func(root string) error { return newFsFoldersStore(root).SaveFolder(ctx, "", plainWriteFolder()) },
			remove: nil},
		{name: "recordset definition", file: "recordsets/rs1.recordset.json",
			write: func(root string) error {
				s := newFsRecordsetDefinitionsStore(root)
				return s.saveProjectItem(ctx, s.dirPath, plainWriteRecordset())
			},
			remove: func(root string) error {
				s := newFsRecordsetDefinitionsStore(root)
				return s.deleteProjectItem(ctx, s.dirPath, "rs1")
			}},
		{name: "query", file: "queries/q1.query.json", extraFolders: []string{"queries/.dt-query-txn"},
			write: func(root string) error {
				_, err := newFsQueriesStore(root).CreateQuery(ctx, dtqlQuery("q1", "", "from:\n  name: Invoice\n"))
				return err
			},
			remove: func(root string) error { return newFsQueriesStore(root).DeleteQuery(ctx, "q1") }},
		{name: "query in a folder", file: "queries/sub/q1.query.json",
			write: func(root string) error {
				_, err := newFsQueriesStore(root).CreateQuery(ctx, dtqlQuery("q1", "sub", "from:\n  name: Invoice\n"))
				return err
			}},
		{name: "query body", file: "queries/sub/q1.query.dtql",
			write: func(root string) error {
				_, err := newFsQueriesStore(root).CreateQuery(ctx, dtqlQuery("q1", "sub", "from:\n  name: Invoice\n"))
				return err
			}},
		{name: "query folder README", file: "queries/sub/README.md", leavesAlone: true,
			write: func(root string) error { return newFsQueriesStore(root).CreateQueryFolder(ctx, "", "sub") }},
		{name: "driver", file: "dbs/sqlite3/driver.json", removeFile: "dbs/sqlite3.json",
			write: func(root string) error {
				return newFsProjDbDriversStore(root).SaveProjDbDriver(ctx, plainWriteDriver())
			},
			remove: func(root string) error { return newFsProjDbDriversStore(root).DeleteProjDbDriver(ctx, "sqlite3") }},
		{name: "driver server", file: "dbs/mysql/mysql:srv1.dbserver.json",
			write: func(root string) error {
				return newFsProjDbDriversStore(root).DbServersStore("mysql").SaveProjDbServer(ctx, plainWriteDbServer())
			},
			remove: func(root string) error {
				return newFsProjDbDriversStore(root).DbServersStore("mysql").DeleteProjDbServer(ctx, plainWriteDbServer().ID)
			}},
		{name: "server catalog", file: "dbs/mysql/servers/dbs/cat1.json",
			write: func(root string) error {
				store := newFsProjDbDriversStore(root).DbServersStore("mysql")
				return store.CatalogsStore(datatug.ServerRef{Driver: "mysql", Host: "srv1"}).SaveDbCatalog(ctx, plainWriteCatalog())
			},
			remove: func(root string) error {
				store := newFsProjDbDriversStore(root).DbServersStore("mysql")
				return store.CatalogsStore(datatug.ServerRef{Driver: "mysql", Host: "srv1"}).DeleteDbCatalog(ctx, "cat1")
			}},
		{name: "server README", file: "dbs/mysql/README.md",
			write: func(root string) error {
				return saveReadme(root, filepath.Join(root, "dbs", "mysql"), func(w io.Writer) error {
					_, err := io.WriteString(w, "# DB server\n")
					return err
				})
			}},
		{name: "storage file", file: "docs/deep/file.txt",
			write: func(root string) error {
				return NewStorage(root).WriteFile(ctx, "docs/deep/file.txt", strings.NewReader("content"))
			}},
	}
}

// treeHash is a content hash of everything under dir: each folder's and
// file's slash-separated path, its kind (a link counts as a link, never
// followed) and a file's bytes or a link's presence. Two equal hashes mean
// the same tree.
func treeHash(t *testing.T, dir string) string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			entries = append(entries, "l "+rel)
		case d.IsDir():
			entries = append(entries, "d "+rel)
		default:
			b, readErr := os.ReadFile(p)
			if readErr != nil {
				return readErr
			}
			sum := sha256.Sum256(b)
			entries = append(entries, "f "+rel+" "+hex.EncodeToString(sum[:]))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}
