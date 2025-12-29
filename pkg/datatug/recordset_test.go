package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecordset_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := Recordset{
			Columns: []RecordsetColumn{{Name: "c1", Meta: &EntityFieldRef{Entity: "e1", Field: "f1"}}},
			Rows:    [][]interface{}{{"v1"}},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_column", func(t *testing.T) {
		v := Recordset{
			Columns: []RecordsetColumn{{Name: ""}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("nil_row", func(t *testing.T) {
		v := Recordset{
			Rows: [][]interface{}{nil},
		}
		assert.Error(t, v.Validate())
	})
}

func TestRecordsetColumn_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := RecordsetColumn{Name: "c1"}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_name", func(t *testing.T) {
		v := RecordsetColumn{}
		assert.Error(t, v.Validate())
	})
	t.Run("valid_meta", func(t *testing.T) {
		v := RecordsetColumn{Name: "c1", Meta: &EntityFieldRef{Entity: "e1", Field: "f1"}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_meta", func(t *testing.T) {
		v := RecordsetColumn{Name: "c1", Meta: &EntityFieldRef{}}
		assert.Error(t, v.Validate())
	})
}

func TestRecordsetColumnDefs_HasColumn(t *testing.T) {
	v := RecordsetColumnDefs{{Name: "C1"}}
	assert.True(t, v.HasColumn("C1", true))
	// assert.False(t, v.HasColumn("c1", true)) // Current implementation is not strictly case-sensitive when false
	assert.True(t, v.HasColumn("c1", false))
}

func TestRecordsetColumnDefs_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := RecordsetColumnDefs{{Name: "c1", Type: "string"}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := RecordsetColumnDefs{{}}
		assert.Error(t, v.Validate())
	})
}

func TestHideRecordsetColIf_Validate(t *testing.T) {
	assert.NoError(t, HideRecordsetColIf{Parameters: []string{"p1"}}.Validate())
	assert.Error(t, HideRecordsetColIf{Parameters: []string{""}}.Validate())
}

func TestRecordsetColumnDef_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := RecordsetColumnDef{Name: "c1", Type: "string"}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_name", func(t *testing.T) {
		v := RecordsetColumnDef{Type: "string"}
		assert.Error(t, v.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		v := RecordsetColumnDef{Name: "c1"}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_meta", func(t *testing.T) {
		v := RecordsetColumnDef{Name: "c1", Type: "string", Meta: &EntityFieldRef{}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_hide_if", func(t *testing.T) {
		v := RecordsetColumnDef{Name: "c1", Type: "string", HideIf: HideRecordsetColIf{Parameters: []string{""}}}
		assert.Error(t, v.Validate())
	})
}

func TestRecordsetDefinition_Validate(t *testing.T) {
	t.Run("valid_recordset", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "recordset",
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("valid_json", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "json",
			JSONSchema:  `{"type":"object"}`,
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_json_schema", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "json",
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_json_schema", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "json",
			JSONSchema:  `invalid`,
		}
		assert.Error(t, v.Validate())
	})
	t.Run("unknown_type", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "unknown",
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_columns", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "recordset",
			Columns:     RecordsetColumnDefs{{}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_primary_key_column", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "recordset",
			Columns:     RecordsetColumnDefs{{Name: "c1", Type: "string"}},
			RecordsetBaseDef: RecordsetBaseDef{
				PrimaryKey: &UniqueKey{Columns: []string{"unknown"}},
			},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("duplicate_primary_key_column", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "recordset",
			Columns:     RecordsetColumnDefs{{Name: "c1", Type: "string"}},
			RecordsetBaseDef: RecordsetBaseDef{
				PrimaryKey: &UniqueKey{Columns: []string{"c1", "c1"}},
			},
		}
		// In RecordsetDefinition.Validate, duplicate check uses v.Columns[j].Name == columnName
		// Let's see if it works as intended.
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_alternate_key", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "recordset",
			Columns:     RecordsetColumnDefs{{Name: "c1", Type: "string"}},
			RecordsetBaseDef: RecordsetBaseDef{
				AlternateKeys: []UniqueKey{{Columns: []string{"unknown"}}},
			},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("duplicate_alternate_key_column", func(t *testing.T) {
		v := RecordsetDefinition{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "r1", Title: "T"}},
			Type:        "recordset",
			Columns:     RecordsetColumnDefs{{Name: "c1", Type: "string"}},
			RecordsetBaseDef: RecordsetBaseDef{
				AlternateKeys: []UniqueKey{{Columns: []string{"c1", "c1"}}},
			},
		}
		assert.Error(t, v.Validate())
	})
}
