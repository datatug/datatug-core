package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestFsRecordsetDefinitionsStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_recordsets")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_project"
	projectPath := path.Join(tmpDir, projectID)
	err = os.MkdirAll(projectPath, 0777)
	assert.NoError(t, err)

	store := newFsRecordsetDefinitionsStore(projectPath)
	ctx := context.Background()

	t.Run("LoadRecordsetDefinitions", func(t *testing.T) {
		recordsetsDir := path.Join(projectPath, RecordsetsFolder)
		err = os.MkdirAll(recordsetsDir, 0777)
		assert.NoError(t, err)

		rsID := "rs1"
		rsData := datatug.RecordsetDefinition{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: rsID,
				},
			},
		}
		data, _ := json.Marshal(rsData)
		err = os.WriteFile(path.Join(recordsetsDir, rsID+"."+recordsetFileSuffix+".json"), data, 0666)
		assert.NoError(t, err)

		recordsets, err := store.LoadRecordsetDefinitions(ctx)
		assert.NoError(t, err)
		assert.Len(t, recordsets, 1)
		assert.Equal(t, rsID, recordsets[0].ID)
	})

	t.Run("LoadRecordsetDefinition", func(t *testing.T) {
		rsID := "rs1"
		rs, err := store.LoadRecordsetDefinition(ctx, rsID)
		assert.NoError(t, err)
		assert.NotNil(t, rs)
		assert.Equal(t, rsID, rs.ID)
	})

	t.Run("LoadRecordsetData_panic", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = store.LoadRecordsetData(ctx, "rs1")
		})
	})
}
