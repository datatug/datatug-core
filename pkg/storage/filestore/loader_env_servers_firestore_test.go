package filestore

import (
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// A firestore server has no host: the loader must not give it the file name
// as one, or the loaded server fails ServerRef.Validate.
func TestLoadEnvServersLeavesAFirestoreServerHostless(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(path.Join(dir, "prod.server.json"), []byte(`[{"driver":"firestore"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	env := &datatug.Environment{}
	if err := loadEnvServers(dir, env); err != nil {
		t.Fatal(err)
	}
	if len(env.DbServers) != 1 || env.DbServers[0].Host != "" {
		t.Fatalf("%+v", env.DbServers)
	}
	if err := env.DbServers[0].Validate(); err != nil {
		t.Fatal(err)
	}
}
