package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/gofrs/flock"
)

func TestPublicConnectionProspectiveFailureAndPostProjectionCancellation(t *testing.T) {
	original := buildPublicConnectionChanges
	t.Cleanup(func() { buildPublicConnectionChanges = original })
	for _, mode := range []string{"builder failure", "cancel after build"} {
		t.Run(mode, func(t *testing.T) {
			root, private := publicConnectionFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			failure := errors.New("prospective state failed")
			err := WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error {
				state, err := f.PublicOVDBConnectionState(ctx, "dev", "music")
				if err != nil {
					return err
				}
				buildPublicConnectionChanges = func(read publicOVDBConnectionRead) (map[string][]byte, map[string]string, error) {
					calls++
					if calls == 2 {
						if mode == "builder failure" {
							return nil, nil, failure
						}
						changes, expected, err := original(read)
						cancel()
						return changes, expected, err
					}
					return original(read)
				}
				return f.CommitPublicOVDBConnection(ctx, state, "binding", &dto.EnvironmentOperationResult{OperationID: "op"})
			})
			buildPublicConnectionChanges = original
			if mode == "builder failure" && !errors.Is(err, failure) || mode == "cancel after build" && !errors.Is(err, context.Canceled) {
				t.Fatal("post-projection failure lost", mode, err)
			}
			if _, err := os.Stat(filepath.Join(root, "connections/music-ovdb.json")); !os.IsNotExist(err) {
				t.Fatal("projection failure published new connection")
			}
		})
	}
	if err := validatePublicConnectionChangeCapacity(nil, map[string][]byte{"new": make([]byte, (1<<20)+1)}); !errors.Is(err, datatug.ErrProjectUsageIncomplete) {
		t.Fatal("oversized fresh output admitted", err)
	}
	root, _ := publicConnectionFixture(t)
	read, err := (&EnvironmentFiles{Root: root}).readPublicOVDBConnection(context.Background(), "dev", "music")
	if err != nil {
		t.Fatal(err)
	}
	read.database.OVDB.BaseURL = "https://public.example/" + strings.Repeat("x", 150000)
	if changes, _, err := publicOVDBConnectionChanges(read); err == nil || changes != nil {
		t.Fatal("prospective output exceeded actual public reader limit")
	}
}

func TestMetadataReadersRefuseCorruptProjectAndEnvironmentSummary(t *testing.T) {
	root, _ := publicConnectionFixture(t)
	projectPath := filepath.Join(root, "datatug-project.json")
	originalProject, _ := os.ReadFile(projectPath)
	outside := filepath.Join(t.TempDir(), "project.json")
	_ = os.WriteFile(outside, originalProject, 0600)
	_ = os.Remove(projectPath)
	_ = os.Symlink(outside, projectPath)
	if _, err := (&EnvironmentFiles{Root: root}).PublicOVDBConnectionState(context.Background(), "dev", "music"); err == nil {
		t.Fatal("project symlink accepted")
	}
	_ = os.Remove(projectPath)
	if err := os.WriteFile(filepath.Join(root, "datatug-project.json"), []byte(`{`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&EnvironmentFiles{Root: root}).PublicOVDBConnectionState(context.Background(), "dev", "music"); err == nil {
		t.Fatal("malformed project passed duplicate-key parser")
	}
	root, _ = publicConnectionFixture(t)
	dir := filepath.Join(root, "connections")
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "oversize.json"), []byte(strings.Repeat("x", datatug.MaxPublicConnectionCatalogBytes+1)), 0600)
	if _, err := (&EnvironmentFiles{Root: root}).PublicOVDBConnectionState(context.Background(), "dev", "music"); err == nil {
		t.Fatal("oversized catalog was ignored")
	}
	_ = os.WriteFile(filepath.Join(dir, "oversize.json"), []byte(`{"format":"future"}`), 0600)
	if _, err := (&EnvironmentFiles{Root: root}).PublicOVDBConnectionState(context.Background(), "dev", "music"); err == nil {
		t.Fatal("unknown catalog namespace ignored")
	}
	envPath := filepath.Join(root, "environments/dev/dev.env.json")
	_ = os.WriteFile(envPath, []byte(`{"id":"dev","editionConnections":["bad/id"]}`), 0600)
	if out, err := newFsEnvironmentsStore(root).loadEnvironmentSummary(context.Background(), "dev"); out != nil || err == nil {
		t.Fatal("invalid environment IDs exposed summary")
	}
}

func TestQueryFolderReservesFullTreeBudgetAndHonorsCancelledCreate(t *testing.T) {
	store, root := newTestQueriesStore(t)
	_ = os.MkdirAll(root, 0700)
	previous := queryDeclarationListingBudget
	t.Cleanup(func() { queryDeclarationListingBudget = previous })
	calls := 0
	queryDeclarationListingBudget = func(s fsQueriesStore) *queryReadBudget {
		calls++
		if calls == 2 {
			return &queryReadBudget{remaining: 0}
		}
		return s.newListingBudget()
	}
	if _, err := store.admitQueryFolderDeclaration(context.Background(), queryLockGuard{}, "", "reports"); !errors.Is(err, errQueryListingTooLarge) {
		t.Fatal("full-tree budget ignored", err)
	}
	queryDeclarationListingBudget = previous
	// Cancellation after taking the write lock is checked before directory writes.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	oldLock := tryLockContext
	t.Cleanup(func() { tryLockContext = oldLock })
	tryLockContext = func(f *flock.Flock, c context.Context, d time.Duration) (bool, error) {
		ok, err := oldLock(f, c, d)
		cancel()
		return ok, err
	}
	if err := store.CreateQueryFolder(ctx, "", "cancelled"); err == nil {
		t.Fatal("cancelled create accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "cancelled")); !os.IsNotExist(err) {
		t.Fatal("cancelled create changed files")
	}
}
