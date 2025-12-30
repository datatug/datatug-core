package filestore

import (
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestLoaderRecordsets(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_recordsets")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_project"
	projectPath := path.Join(tmpDir, projectID)
	recordsetsPath := path.Join(projectPath, DataFolder, RecordsetsFolder)
	err = os.MkdirAll(recordsetsPath, 0755)
	assert.NoError(t, err)

	loader := fileSystemLoader{
		pathByID: map[string]string{
			projectID: projectPath,
		},
	}

	t.Run("LoadRecordsetDefinitions_Empty", func(t *testing.T) {
		defs, err := loader.LoadRecordsetDefinitions(projectID)
		assert.NoError(t, err)
		assert.Empty(t, defs)
	})

	t.Run("LoadRecordsetDefinitions_WithData", func(t *testing.T) {
		rs1ID := "my_recordset1"
		rs1Dir := path.Join(recordsetsPath, rs1ID)
		err = os.MkdirAll(rs1Dir, 0755)
		assert.NoError(t, err)

		rsDef := datatug.RecordsetDefinition{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: rs1ID,
				},
			},
			Columns: datatug.RecordsetColumnDefs{
				{Name: "col1", Type: "string"},
			},
			Type: "recordset",
		}
		data, _ := json.Marshal(rsDef)
		// FIXED: Filename was incorrect in previous attempts
		err = os.WriteFile(path.Join(rs1Dir, jsonFileName(rs1ID, recordsetFileSuffix)), data, 0644)
		assert.NoError(t, err)

		// Nested recordset
		subFolder := "nested_folder"
		subRsID := "my_recordset2"
		subRsDir := path.Join(recordsetsPath, subFolder, subRsID)
		err = os.MkdirAll(subRsDir, 0755)
		assert.NoError(t, err)

		subRsDef := datatug.RecordsetDefinition{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: subRsID,
				},
			},
			Columns: datatug.RecordsetColumnDefs{
				{Name: "col2", Type: "int"},
			},
			Type: "recordset",
		}
		subData, _ := json.Marshal(subRsDef)
		err = os.WriteFile(path.Join(subRsDir, jsonFileName(subRsID, recordsetFileSuffix)), subData, 0644)
		assert.NoError(t, err)

		defs, err := loader.LoadRecordsetDefinitions(projectID)
		assert.NoError(t, err)
		assert.Len(t, defs, 2)

		// LoadRecordsetDefinition
		def, err := loader.LoadRecordsetDefinition(projectID, rs1ID)

		if assert.NoError(t, err) {
			assert.Equal(t, rs1ID, def.ID)
		}

		def, err = loader.LoadRecordsetDefinition(projectID, path.Join(subFolder, subRsID))

		if assert.NoError(t, err) {
			assert.Equal(t, path.Join(subFolder, subRsID), def.ID)
		}

		// LoadRecordsetData
		rsDataDir := path.Join(projectPath, DataFolder, rs1ID)
		err = os.MkdirAll(rsDataDir, 0755)
		assert.NoError(t, err)

		rows := []map[string]interface{}{
			{"col1": "val1"},
			{"col1": "val2"},
		}
		rowsData, _ := json.Marshal(rows)
		dataFilePath := path.Join(rsDataDir, "data.json")
		err = os.WriteFile(dataFilePath, rowsData, 0644)
		assert.NoError(t, err)

		rs, err := loader.LoadRecordsetData(projectID, rs1ID, "data.json")
		if assert.NoError(t, err) {
			assert.NotNil(t, rs)
			assert.Len(t, rs.Rows, 2)
			assert.Equal(t, "val1", rs.Rows[0][0])
			assert.Equal(t, "val2", rs.Rows[1][0])
		}

		// LoadRecordsetData - Missing project
		_, err = loader.LoadRecordsetData("missing", rs1ID, "data.json")
		assert.Error(t, err)

		// LoadRecordsetData - Missing file
		_, err = loader.LoadRecordsetData(projectID, rs1ID, "missing.json")
		assert.Error(t, err)
	})

	t.Run("LoadRecordsetDefinitions_MissingProjectID", func(t *testing.T) {
		_, err := loader.LoadRecordsetDefinitions("")
		assert.Error(t, err)
	})

	t.Run("LoadRecordsetData_InvalidRowType", func(t *testing.T) {
		rs1ID := "my_recordset1"
		rsDataDir := path.Join(projectPath, DataFolder)
		rowsData := []byte(`[1, 2, 3]`) // Not a slice of maps
		err = os.WriteFile(path.Join(rsDataDir, rs1ID, "invalid_data.json"), rowsData, 0644)
		assert.NoError(t, err)

		_, err := loader.LoadRecordsetData(projectID, rs1ID, "invalid_data.json")
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "unexpected row type")
		}
	})
}
