package filestore

import (
	"context"
	"encoding/json"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func dataModelsFixture() *datatug.ProjectDataModels {
	return &datatug.ProjectDataModels{Version: 1, References: []datatug.ProjectDataModelReference{{ID: "chinook-v1", DataModelReference: datatug.DataModelReference{Address: "modelspec://github.com/demo-db/chinook/chinook", Commit: strings.Repeat("a", 40)}}}, CatalogBindings: []datatug.DataModelCatalogBinding{{Environment: "local", Catalog: "chinook-local", Reference: "chinook-v1"}}}
}
func modelProject() *datatug.Project {
	return &datatug.Project{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "p", Title: "Pinned project"}, Access: "private"}, Created: &datatug.ProjectCreated{At: time.Now().UTC()}}
}
func TestProjectDataModelsStorePreservationClearAndOpaque(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := newFsProjectStore("p", dir)
	p := modelProject()
	require.NoError(t, store.SaveProject(ctx, p))
	p.DataModels = dataModelsFixture()
	require.NoError(t, store.SaveProject(ctx, p))
	for _, depth := range []datatug.StoreOption{datatug.Depth(1), datatug.Depth(0)} {
		loaded, err := store.LoadProject(ctx, depth)
		require.NoError(t, err)
		require.Equal(t, p.DataModels, loaded.DataModels)
	}
	file, err := store.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.Equal(t, p.DataModels, file.DataModels)
	summary := filepath.Join(dir, storage.ProjectSummaryFileName)
	body, err := os.ReadFile(summary)
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &raw))
	raw["future"] = json.RawMessage(`{"big":90071992547409931234567890123456789}`)
	body, err = json.Marshal(raw)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(summary, body, 0600))
	p.DataModels = nil
	p.Title = "Unrelated save"
	require.NoError(t, store.SaveProject(ctx, p))
	file, err = store.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.Equal(t, dataModelsFixture(), file.DataModels)
	body, err = os.ReadFile(summary)
	require.NoError(t, err)
	require.Contains(t, string(body), "90071992547409931234567890123456789")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- store.SaveProject(ctx, modelProject()) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	file, err = store.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.NotNil(t, file.DataModels)
	p.DataModels = &datatug.ProjectDataModels{Version: 1}
	require.NoError(t, store.SaveProject(ctx, p))
	file, err = store.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.NotNil(t, file.DataModels)
	require.Empty(t, file.DataModels.References)
	body, err = os.ReadFile(summary)
	require.NoError(t, err)
	require.Contains(t, string(body), "90071992547409931234567890123456789")
}
func TestProjectDataModelsPreflightBeforeAnyProjectPart(t *testing.T) {
	ctx := context.Background()
	for _, existing := range []string{`{"version":2,"references":[]}`, `null`, `{"version":1,"references":[],"unknown":true}`} {
		for _, incoming := range []*datatug.ProjectDataModels{nil, {Version: 1}} {
			t.Run(existing, func(t *testing.T) {
				dir := t.TempDir()
				s := newFsProjectStore("p", dir)
				p := modelProject()
				require.NoError(t, s.SaveProject(ctx, p))
				summary := filepath.Join(dir, storage.ProjectSummaryFileName)
				body, e := os.ReadFile(summary)
				require.NoError(t, e)
				var raw map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(body, &raw))
				raw["dataModels"] = json.RawMessage(existing)
				body, e = json.Marshal(raw)
				require.NoError(t, e)
				require.NoError(t, os.WriteFile(summary, body, 0600))
				before := snapshotProjectFiles(t, dir)
				p.DataModels = incoming
				p.Title = "must not persist"
				p.DbModels = datatug.DbModels{{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "new-model"}}}}
				require.Error(t, s.SaveProject(ctx, p))
				require.Equal(t, before, snapshotProjectFiles(t, dir))
				_, e = s.LoadProject(ctx, datatug.Depth(1))
				require.Error(t, e)
			})
		}
	}
	dir := filepath.Join(t.TempDir(), "not-created")
	s := newFsProjectStore("p", dir)
	p := modelProject()
	p.DataModels = dataModelsFixture()
	p.DataModels.References[0].Commit = "main"
	require.Error(t, s.SaveProject(ctx, p))
	_, err := os.Stat(dir)
	require.True(t, os.IsNotExist(err), "invalid submitted reference must not create even the project directory")
}
func snapshotProjectFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	require.NoError(t, filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(dir, path)
		if e != nil {
			return e
		}
		body, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		out[rel] = string(body)
		return nil
	}))
	return out
}
