package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
)

type connectionDirectoryFault struct {
	connectionDirectoryReader
	readErr, closeErr error
	afterRead         func()
}

func (f connectionDirectoryFault) ReadDir(n int) ([]os.DirEntry, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	out, err := f.connectionDirectoryReader.ReadDir(n)
	if f.afterRead != nil {
		f.afterRead()
	}
	return out, err
}
func (f connectionDirectoryFault) Close() error {
	err := f.connectionDirectoryReader.Close()
	if f.closeErr != nil {
		return f.closeErr
	}
	return err
}

func TestPublicConnectionReadCorruptionAndNamespaceCapacity(t *testing.T) {
	for _, mode := range []string{"missing project", "project duplicate", "project syntax", "env null", "env duplicate", "env syntax", "env identity", "env invalid", "env disagreement", "duplicate catalogs", "catalog count", "entry count", "entry overflow", "invalid filename", "total bytes"} {
		t.Run(mode, func(t *testing.T) {
			root, _ := publicConnectionFixture(t)
			project := filepath.Join(root, "datatug-project.json")
			env := filepath.Join(root, "environments/dev/dev.env.json")
			dir := filepath.Join(root, "connections")
			put := func(p, body string) {
				t.Helper()
				if err := os.WriteFile(p, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			catalog := `{"format":"datatug-demo-connections/v1","bigQueryPlans":[{"id":"plan","readiness":"setup-required"}]}`
			switch mode {
			case "missing project":
				_ = os.Remove(project)
			case "project duplicate":
				put(project, `{"databases":[],"databases":[]}`)
			case "project syntax":
				put(project, `{"databases":"invalid"}`)
			case "env null":
				put(env, `null`)
			case "env duplicate":
				put(env, `{"id":"dev","id":"dev"}`)
			case "env syntax":
				put(env, `{"editionConnections":"wrong"}`)
			case "env identity":
				put(env, `{"id":"prod"}`)
			case "env invalid":
				put(env, `{"id":"dev","editionConnections":["bad/id"]}`)
			case "env disagreement":
				put(filepath.Join(root, "environments/dev/environment-summary.json"), `{"id":"dev","title":"Different"}`)
			default:
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "duplicate catalogs":
					put(filepath.Join(dir, "a.json"), catalog)
					put(filepath.Join(dir, "b.json"), catalog)
				case "catalog count":
					for i := 0; i < maxConnectionCatalogs; i++ {
						put(filepath.Join(dir, fmt.Sprintf("c%d.json", i)), `{"format":"datatug-demo-connections/v1"}`)
					}
				case "entry count", "entry overflow":
					n := 1024
					if mode == "entry overflow" {
						n++
					}
					for i := 0; i < n; i++ {
						put(filepath.Join(dir, fmt.Sprintf("note%d.txt", i)), "")
					}
				case "invalid filename":
					put(filepath.Join(dir, "bad name.json"), catalog)
				case "total bytes":
					for i := 0; i < 20; i++ {
						put(filepath.Join(dir, fmt.Sprintf("c%d.json", i)), `{"format":"datatug-demo-connections/v1","opaque":"`+strings.Repeat("x", 220000)+`"}`)
					}
				}
			}
			if out, err := (&EnvironmentFiles{Root: root}).PublicOVDBConnectionState(context.Background(), "dev", "music"); err == nil || out.Revision != "" {
				t.Fatalf("unknown %s advertised usable revision: %+v %v", mode, out, err)
			}
		})
	}
	root, _ := publicConnectionFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&EnvironmentFiles{Root: root}).PublicOVDBConnectionState(ctx, "dev", "music"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestPublicConnectionDirectoryIOAndConcurrentDisappearanceFailClosed(t *testing.T) {
	originalOpen, originalStat := openConnectionDirectory, dirLstat
	t.Cleanup(func() { openConnectionDirectory = originalOpen; dirLstat = originalStat })
	failure := errors.New("directory IO")
	for _, mode := range []string{"open", "read", "close", "stat", "disappear", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			openConnectionDirectory = originalOpen
			dirLstat = originalStat
			root, _ := publicConnectionFixture(t)
			dir := filepath.Join(root, "connections")
			_ = os.MkdirAll(dir, 0700)
			name := filepath.Join(dir, "catalog.json")
			_ = os.WriteFile(name, []byte(`{"format":"datatug-demo-connections/v1"}`), 0600)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "stat" {
				dirLstat = func(p string) (os.FileInfo, error) {
					if p == dir {
						return nil, failure
					}
					return originalStat(p)
				}
			} else {
				openConnectionDirectory = func(p string) (connectionDirectoryReader, error) {
					if mode == "open" {
						return nil, failure
					}
					f, err := originalOpen(p)
					if err != nil {
						return nil, err
					}
					w := connectionDirectoryFault{connectionDirectoryReader: f}
					switch mode {
					case "read":
						w.readErr = failure
					case "close":
						w.closeErr = failure
					case "disappear":
						w.afterRead = func() { _ = os.Remove(name) }
					case "cancel":
						w.afterRead = cancel
					}
					return w, nil
				}
			}
			out, err := (&EnvironmentFiles{Root: root}).PublicOVDBConnectionState(ctx, "dev", "music")
			if err == nil || out.Revision != "" {
				t.Fatal("IO failure exposed capability", out, err)
			}
		})
	}
}

func TestPublicConnectionCommitRechecksGuardAndRollsBackBeforeDecision(t *testing.T) {
	for _, mode := range []string{"nil result", "read failure", "cancel before", "cancel staged", "source removed staged", "changed staged", "authority"} {
		t.Run(mode, func(t *testing.T) {
			root, private := publicConnectionFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			originalFault := environmentTransactionFault
			t.Cleanup(func() { environmentTransactionFault = originalFault })
			err := WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error {
				state, err := f.PublicOVDBConnectionState(ctx, "dev", "music")
				if err != nil {
					return err
				}
				result := &dto.EnvironmentOperationResult{OperationID: "op", Status: "committed"}
				switch mode {
				case "nil result":
					result = nil
				case "read failure":
					_ = os.Remove(filepath.Join(root, "datatug-project.json"))
				case "cancel before":
					cancel()
				case "cancel staged", "source removed staged", "changed staged":
					environmentTransactionFault = func(stage string) {
						if stage != "intent" {
							return
						}
						switch mode {
						case "cancel staged":
							cancel()
						case "source removed staged":
							_ = os.Remove(filepath.Join(root, "datatug-project.json"))
						case "changed staged":
							p := filepath.Join(root, "datatug-project.json")
							body, _ := os.ReadFile(p)
							_ = os.WriteFile(p, append(body, ' '), 0600)
						}
					}
				}
				return f.CommitPublicOVDBConnectionGuarded(ctx, state, "binding", result, func() error {
					if mode == "authority" {
						return datatug.ErrProjectLifecycleConflict
					}
					return nil
				})
			})
			if err == nil {
				t.Fatal("failed authority/state committed")
			}
			if _, err := os.Stat(filepath.Join(root, "connections/music-ovdb.json")); !os.IsNotExist(err) {
				t.Fatal("undecided failure published connection", err)
			}
			if mode == "authority" {
				if err := WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error {
					state, err := f.PublicOVDBConnectionState(context.Background(), "dev", "music")
					if err != nil {
						return err
					}
					if state.Exists {
						t.Fatal("guard failure occupied namespace")
					}
					return nil
				}); err != nil {
					t.Fatal("rollback prevented retry", err)
				}
			}
		})
	}
}

func TestPublicConnectionProspectiveCapacityAndInvalidEnvironmentBytes(t *testing.T) {
	root, _ := publicConnectionFixture(t)
	read, err := (&EnvironmentFiles{Root: root}).readPublicOVDBConnection(context.Background(), "dev", "music")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"connection invalid", "environment malformed", "existing file too large", "aggregate too large"} {
		t.Run(mode, func(t *testing.T) {
			c := read
			c.files = map[string][]byte{}
			for p, b := range read.files {
				c.files[p] = b
			}
			switch mode {
			case "connection invalid":
				c.snapshot.ConnectionID = "bad/id"
			case "environment malformed":
				c.files[c.envFiles[0]] = []byte(`null`)
			case "existing file too large":
				c.files["opaque"] = make([]byte, (1<<20)+1)
			case "aggregate too large":
				for i := 0; i < 5; i++ {
					c.files[fmt.Sprint(i)] = make([]byte, 1<<20)
				}
			}
			if changes, expected, err := publicOVDBConnectionChanges(c); err == nil || changes != nil || expected != nil {
				t.Fatal("unreadable prospective output accepted", err)
			}
		})
	}
	// The durable journal itself must refuse replacing any existing connection.
	private := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "connections"), 0700)
	_ = os.WriteFile(filepath.Join(root, "connections/music-ovdb.json"), []byte("original"), 0600)
	err = WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error {
		return f.commitChecked(map[string][]byte{"connections/music-ovdb.json": []byte("replacement")}, nil, "binding", dto.EnvironmentOperationResult{OperationID: "op"}, nil)
	})
	if !errors.Is(err, datatug.ErrAmbiguousConnection) {
		t.Fatal("journal replaced occupied namespace", err)
	}
}
