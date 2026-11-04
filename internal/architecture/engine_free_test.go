// Package architecture holds tests that keep datatug-core's boundaries: see AGENTS.md,
// "Core is engine-free: database access goes through DALgo adapters".
package architecture

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// moduleRoot is the repository root, relative to this package.
const moduleRoot = "../.."

// driverModules are module path prefixes of database drivers and engine clients.
// They belong in dal-go/dalgo2<engine> adapters, never in datatug-core.
var driverModules = []string{
	"github.com/jackc/pgx",
	"github.com/lib/pq",
	"github.com/go-sql-driver/mysql",
	"github.com/mattn/go-sqlite3",
	"modernc.org/sqlite",
	"github.com/microsoft/go-mssqldb",
	"github.com/denisenkom/go-mssqldb",
	"go.mongodb.org/mongo-driver",
	"cloud.google.com/go/firestore",
	"cloud.google.com/go/bigquery",
	"cloud.google.com/go/spanner",
	"github.com/ClickHouse/clickhouse-go",
	"github.com/snowflakedb/gosnowflake",
	"github.com/sijms/go-ora",
	"github.com/godror/godror",
	"github.com/redis/go-redis",
	"github.com/elastic/go-elasticsearch",
	"github.com/duckdb/duckdb-go",
	"github.com/marcboeker/go-duckdb",
	"github.com/tursodatabase/libsql-client-go",
}

// engineCatalog matches engine-specific catalog names in Go source.
var engineCatalog = regexp.MustCompile(`(?i)\b(pg_catalog|pg_roles|pg_authid|pg_auth_members|pg_class|pg_namespace|information_schema|sqlite_master|sqlite_schema|sys\.objects|sys\.tables|sys\.database_principals)\b`)

func TestGoModRequiresNoDatabaseDriver(t *testing.T) {
	f, err := os.Open(filepath.Join(moduleRoot, "go.mod"))
	if err != nil {
		t.Fatalf("open go.mod: %v", err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		for _, module := range driverModules {
			if strings.HasPrefix(strings.TrimPrefix(text, "require "), module) {
				t.Errorf("go.mod:%d requires %s, a database driver: engine code belongs in a dal-go/dalgo2<engine> adapter (AGENTS.md)", line, module)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
}

func TestNonTestCodeHasNoEngineSQLOrDatabaseSQL(t *testing.T) {
	err := filepath.WalkDir(moduleRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(moduleRoot, path)
		if strings.Contains(string(content), `"database/sql"`) {
			t.Errorf("%s imports database/sql: database access goes through DALgo adapters (AGENTS.md)", rel)
		}
		if m := engineCatalog.FindString(string(content)); m != "" {
			t.Errorf("%s contains %q, an engine catalog name: engine-specific SQL belongs in a dal-go/dalgo2<engine> adapter (AGENTS.md)", rel, m)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk module: %v", err)
	}
}

func TestEngineCatalogPatternMatchesKnownNames(t *testing.T) {
	// A positive control, so a broken pattern cannot make the scan pass vacuously.
	for _, s := range []string{"SELECT * FROM pg_catalog.pg_roles", "from INFORMATION_SCHEMA.TABLES", "sqlite_master", "sys.objects"} {
		if !engineCatalog.MatchString(s) {
			t.Errorf("pattern misses %q", s)
		}
	}
	for _, s := range []string{"pg_roleset", "my_information_schemas", "system.objects"} {
		if engineCatalog.MatchString(s) {
			t.Errorf("pattern wrongly matches %q", s)
		}
	}
}
