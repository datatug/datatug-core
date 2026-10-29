package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
)

func publicConnectionFixture(t *testing.T) (string, string) {
	t.Helper()
	root := environmentTestDir(t)
	if err := os.WriteFile(filepath.Join(root, "datatug-project.json"), []byte(`{"id":"p","databases":[{"id":"music","connectionId":"music-ovdb","ovdb":{"baseUrl":"https://public.example/ovdb","databaseId":"music-db"}}],"credentials":{"password":"private-project"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "environments", "dev"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "environments", "dev", "dev.env.json"), []byte(`{"id":"dev","title":"Development","dbServers":[],"custom":{"token":"private-env","keep":17}}`), 0600); err != nil {
		t.Fatal(err)
	}
	return root, t.TempDir()
}

func TestPublicOVDBConnectionRoundTripDuplicateAndPreservation(t *testing.T) {
	ctx := context.Background()
	root, private := publicConnectionFixture(t)
	projectBefore, _ := os.ReadFile(filepath.Join(root, "datatug-project.json"))
	var result dto.EnvironmentOperationResult
	err := WithEnvironmentFiles(ctx, root, private, func(f *EnvironmentFiles) error {
		state, err := f.PublicOVDBConnectionState(ctx, "dev", "music")
		if err != nil {
			return err
		}
		if state.Exists || state.ConnectionID != "music-ovdb" || state.Revision == "" {
			t.Fatalf("bad state %+v", state)
		}
		result.OperationID = "op"
		result.Status = "committed"
		if err = f.CommitPublicOVDBConnection(ctx, state, "binding", &result); err != nil {
			return err
		}
		after, err := f.PublicOVDBConnectionState(ctx, "dev", "music")
		if err != nil {
			return err
		}
		if !after.Exists || after.Revision != result.AfterStateDigest {
			t.Fatalf("receipt does not match state %+v %+v", after, result)
		}
		if err = f.CommitPublicOVDBConnection(ctx, after, "binding", &result); !errors.Is(err, datatug.ErrAmbiguousConnection) {
			t.Fatalf("duplicate accepted: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	store := newFsProjectStore("p", root)
	catalog, err := store.LoadProjectConnections(ctx)
	if err != nil || len(catalog.Connections) != 1 || catalog.Connections[0].Storage != "ovdb" {
		t.Fatalf("catalog %+v %v", catalog, err)
	}
	connection, err := store.ResolveEnvironmentConnection(ctx, "dev", "music-ovdb")
	if err != nil || len(connection.Environments) != 1 || connection.Environments[0] != "dev" {
		t.Fatalf("connection %+v %v", connection, err)
	}
	summary, err := store.LoadEnvironmentSummary(ctx, "dev")
	if err != nil {
		t.Fatal(err)
	}
	summaryBody, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded datatug.EnvironmentSummary
	if err = json.Unmarshal(summaryBody, &reloaded); err != nil || len(reloaded.EditionConnections) != 1 || reloaded.EditionConnections[0] != "music-ovdb" {
		t.Fatalf("assignment lost on summary wire reload %s %v", summaryBody, err)
	}
	if strings.Contains(string(summaryBody), "private-env") || strings.Contains(string(summaryBody), "token") {
		t.Fatal("summary leaked environment credentials")
	}
	projectAfter, _ := os.ReadFile(filepath.Join(root, "datatug-project.json"))
	if string(projectBefore) != string(projectAfter) {
		t.Fatal("project bytes changed")
	}
	env, _ := os.ReadFile(filepath.Join(root, "environments", "dev", "dev.env.json"))
	if !strings.Contains(string(env), "private-env") || !strings.Contains(string(env), `"keep": 17`) {
		t.Fatal("opaque environment fields lost")
	}
	declared, _ := os.ReadFile(filepath.Join(root, "connections", "music-ovdb.json"))
	if !strings.Contains(string(declared), "https://public.example/ovdb/v1/databases/music-db") || strings.Contains(string(declared), "private-") {
		t.Fatal("invalid source descriptor")
	}
}

func TestPublicOVDBConnectionRejectsUnsafeUnknownAndChangedSources(t *testing.T) {
	for _, mode := range []string{"wrong environment", "wrong database", "bad environment", "bad database", "duplicate plan", "symlink namespace", "symlink environment", "stale source", "unsafe source", "existing file"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			root, private := publicConnectionFixture(t)
			err := WithEnvironmentFiles(ctx, root, private, func(f *EnvironmentFiles) error {
				state, err := f.PublicOVDBConnectionState(ctx, "dev", "music")
				if err != nil {
					return err
				}
				environment, database := "dev", "music"
				switch mode {
				case "wrong environment":
					environment = "prod"
				case "wrong database":
					database = "foreign"
				case "bad environment":
					environment = "../dev"
				case "bad database":
					database = "../music"
				case "duplicate plan":
					_ = os.Mkdir(filepath.Join(root, "connections"), 0700)
					_ = os.WriteFile(filepath.Join(root, "connections", "planned.json"), []byte(`{"format":"datatug-demo-connections/v1","bigQueryPlans":[{"id":"MUSIC-OVDB","readiness":"setup-required"}]}`), 0600)
				case "symlink namespace":
					_ = os.Symlink(t.TempDir(), filepath.Join(root, "connections"))
				case "symlink environment":
					path := filepath.Join(root, "environments", "dev", "dev.env.json")
					body, _ := os.ReadFile(path)
					outside := filepath.Join(t.TempDir(), "outside.json")
					_ = os.WriteFile(outside, body, 0600)
					_ = os.Remove(path)
					_ = os.Symlink(outside, path)
				case "stale source":
					_ = os.WriteFile(filepath.Join(root, "datatug-project.json"), []byte(`{"id":"p","databases":[{"id":"music","connectionId":"other","ovdb":{"baseUrl":"https://public.example","databaseId":"music-db"}}]}`), 0600)
				case "unsafe source":
					_ = os.WriteFile(filepath.Join(root, "datatug-project.json"), []byte(`{"id":"p","databases":[{"id":"music","connectionId":"music-ovdb","ovdb":{"baseUrl":"https://secret@public.example","databaseId":"music-db"}}]}`), 0600)
				case "existing file":
					_ = os.Mkdir(filepath.Join(root, "connections"), 0700)
					_ = os.WriteFile(filepath.Join(root, "connections", "music-ovdb.json"), []byte(`{"format":"datatug-demo-connections/v1"}`), 0600)
				}
				after, err := f.PublicOVDBConnectionState(ctx, environment, database)
				if mode == "duplicate plan" || mode == "existing file" {
					if err != nil || !after.Exists {
						t.Fatalf("existing namespace unrecognized %+v %v", after, err)
					}
					return nil
				}
				if mode == "stale source" {
					err = f.CommitPublicOVDBConnection(ctx, state, "binding", &dto.EnvironmentOperationResult{})
					if !errors.Is(err, datatug.ErrProjectLifecycleConflict) {
						t.Fatalf("stale source accepted %v", err)
					}
					return nil
				}
				if err == nil {
					t.Fatalf("unsafe source accepted %+v", after)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPublicOVDBConnectionJournalRecoveryNeverExposesHalfAssociation(t *testing.T) {
	for _, step := range []string{"intent", "decision", "rename-0"} {
		t.Run(step, func(t *testing.T) {
			root, private := publicConnectionFixture(t)
			ctx := context.Background()
			func() {
				defer func() {
					environmentTransactionFault = nil
					if recover() == nil {
						t.Fatal("fault did not interrupt transaction")
					}
				}()
				_ = WithEnvironmentFiles(ctx, root, private, func(f *EnvironmentFiles) error {
					state, err := f.PublicOVDBConnectionState(ctx, "dev", "music")
					if err != nil {
						return err
					}
					environmentTransactionFault = func(actual string) {
						if actual == step {
							panic("stop")
						}
					}
					return f.CommitPublicOVDBConnection(ctx, state, "binding", &dto.EnvironmentOperationResult{OperationID: "op", Status: "committed"})
				})
			}()
			if err := ReadEnvironmentFiles(ctx, root, func(f *EnvironmentFiles) error {
				state, err := f.PublicOVDBConnectionState(ctx, "dev", "music")
				if err != nil {
					return err
				}
				if state.Exists != (step != "intent") {
					t.Fatalf("unexpected recovered state %+v", state)
				}
				body, _ := os.ReadFile(filepath.Join(root, "environments", "dev", "dev.env.json"))
				var env datatug.Environment
				_ = json.Unmarshal(body, &env)
				_, fileErr := os.Stat(filepath.Join(root, "connections", "music-ovdb.json"))
				present := fileErr == nil
				if present != (len(env.EditionConnections) == 1) {
					t.Fatal("partial association survived recovery")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPublicOVDBConnectionReservesOutputCapacityBeforeOfferingWrite(t *testing.T) {
	root, private := publicConnectionFixture(t)
	body := `{"id":"dev","dbServers":[],"opaque":[` + strings.Repeat(`0,`, 230000) + `0]}`
	if len(body) >= 1<<20 {
		t.Fatal("fixture must fit input budget")
	}
	if err := os.WriteFile(filepath.Join(root, "environments", "dev", "dev.env.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	err := WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error {
		_, err := f.PublicOVDBConnectionState(context.Background(), "dev", "music")
		return err
	})
	if !errors.Is(err, datatug.ErrProjectUsageIncomplete) {
		t.Fatalf("expanded output capacity advertised: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "connections", "music-ovdb.json")); !os.IsNotExist(err) {
		t.Fatal("capacity refusal wrote descriptor")
	}
}

func TestPublicOVDBConnectionAcceptsActualMixedDemoDirectoryWithoutChangingLogs(t *testing.T) {
	root, private := publicConnectionFixture(t)
	if err := os.Mkdir(filepath.Join(root, "connections"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"demo-db.json", "shipping.json", "log-sources.json"} {
		body, err := os.ReadFile(filepath.Join("testdata", "connection-directory", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(root, "connections", name), body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	before, _ := os.ReadFile(filepath.Join(root, "connections", "log-sources.json"))
	ctx := context.Background()
	if err := WithEnvironmentFiles(ctx, root, private, func(f *EnvironmentFiles) error {
		state, err := f.PublicOVDBConnectionState(ctx, "dev", "music")
		if err != nil {
			return err
		}
		if state.Exists {
			t.Fatal("log sources occupied unrelated connection identity")
		}
		return f.CommitPublicOVDBConnection(ctx, state, "binding", &dto.EnvironmentOperationResult{OperationID: "mixed-demo"})
	}); err != nil {
		t.Fatal(err)
	}
	catalog, err := newFsProjectStore("p", root).LoadProjectConnections(ctx)
	if err != nil || len(catalog.Connections) != 21 {
		t.Fatalf("actual mixed directory did not roundtrip: count=%d %v", len(catalog.Connections), err)
	}
	after, _ := os.ReadFile(filepath.Join(root, "connections", "log-sources.json"))
	if string(before) != string(after) {
		t.Fatal("log contract was modified")
	}
	if err := os.WriteFile(filepath.Join(root, "connections", "log-sources.json"), []byte(`{"format":"datatug-log-sources/v1","sources":[],"futureConnection":"hidden"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := newFsProjectStore("p", root).LoadProjectConnections(ctx); err == nil {
		t.Fatal("unknown log extension was silently skipped")
	}
}

func TestPublicOVDBConnectionCannotClaimReservedLogSourcesFilename(t *testing.T) {
	root, private := publicConnectionFixture(t)
	path := filepath.Join(root, "datatug-project.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.Replace(string(body), `"connectionId":"music-ovdb"`, `"connectionId":"log-sources"`, 1))
	if err = os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	err = WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error {
		state, err := f.PublicOVDBConnectionState(context.Background(), "dev", "music")
		if err != nil {
			return err
		}
		if !state.Exists {
			t.Fatal("reserved log namespace advertised for creation")
		}
		err = f.CommitPublicOVDBConnection(context.Background(), state, "binding", &dto.EnvironmentOperationResult{})
		if !errors.Is(err, datatug.ErrAmbiguousConnection) {
			t.Fatalf("reserved namespace was writable: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "connections", "log-sources.json")); !os.IsNotExist(err) {
		t.Fatal("log namespace was replaced by connection catalog")
	}
}
