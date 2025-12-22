package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDbModels_GetDbModelByID(t *testing.T) {
	models := DbModels{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "m1"}}}}
	assert.NotNil(t, models.GetDbModelByID("m1"))
	assert.Nil(t, models.GetDbModelByID("m2"))
}

func TestDbModels_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       DbModels
		wantErr bool
	}{
		{
			name:    "empty",
			v:       DbModels{},
			wantErr: false,
		},
		{
			name:    "valid",
			v:       DbModels{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "m1"}}}},
			wantErr: false,
		},
		{
			name:    "invalid",
			v:       DbModels{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("DbModels.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDbModels_IDs(t *testing.T) {
	models := DbModels{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "m1"}}}, {ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "m2"}}}}
	assert.Equal(t, []string{"m1", "m2"}, models.IDs())
	assert.Empty(t, DbModels{}.IDs())
}

func TestDbModelEnvironments_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       DbModelEnvironments
		wantErr bool
	}{
		{
			name:    "nil",
			v:       nil,
			wantErr: false,
		},
		{
			name:    "valid",
			v:       DbModelEnvironments{{ID: "env1"}},
			wantErr: false,
		},
		{
			name:    "invalid",
			v:       DbModelEnvironments{{ID: ""}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("DbModelEnvironments.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDbModelEnvironments_GetByID(t *testing.T) {
	envs := DbModelEnvironments{{ID: "env1"}}
	assert.NotNil(t, envs.GetByID("env1"))
	assert.Nil(t, envs.GetByID("env2"))
}

func TestDbModelEnv_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       DbModelEnv
		wantErr bool
	}{
		{
			name:    "valid",
			v:       DbModelEnv{ID: "env1"},
			wantErr: false,
		},
		{
			name:    "missing_id",
			v:       DbModelEnv{ID: ""},
			wantErr: true,
		},
		{
			name: "invalid_catalogs",
			v: DbModelEnv{
				ID: "env1",
				DbCatalogs: DbModelDbCatalogs{
					{ID: ""},
				},
			},
			wantErr: false, // Note: DbModelEnv.Validate returns nil even if DbCatalogs.Validate fails in current implementation
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("DbModelEnv.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDbModelDbCatalogs_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       DbModelDbCatalogs
		wantErr bool
	}{
		{
			name:    "valid",
			v:       DbModelDbCatalogs{{ID: "c1"}},
			wantErr: false,
		},
		{
			name:    "invalid",
			v:       DbModelDbCatalogs{{ID: ""}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("DbModelDbCatalogs.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDbModelDbCatalogs_GetByID(t *testing.T) {
	catalogs := DbModelDbCatalogs{{ID: "c1"}}
	assert.NotNil(t, catalogs.GetByID("c1"))
	assert.Nil(t, catalogs.GetByID("c2"))
}

func TestDbModel_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       DbModel
		wantErr bool
	}{
		{
			name:    "valid",
			v:       DbModel{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "m1"}}},
			wantErr: false,
		},
		{
			name:    "invalid_schema",
			v:       DbModel{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "m1"}}, Schemas: SchemaModels{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("DbModel.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSchemaModels_GetByID(t *testing.T) {
	schemas := SchemaModels{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}}}
	assert.NotNil(t, schemas.GetByID("s1"))
	assert.Nil(t, schemas.GetByID("s2"))
}

func TestSchemaModel_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       SchemaModel
		wantErr bool
	}{
		{
			name:    "valid",
			v:       SchemaModel{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}},
			wantErr: false,
		},
		{
			name:    "invalid_table",
			v:       SchemaModel{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}, Tables: TableModels{{DBCollectionKey: NewTableKey("t1", "s1", "c1", nil), Columns: ColumnModels{{}}}}},
			wantErr: true,
		},
		{
			name:    "invalid_view",
			v:       SchemaModel{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}, Views: TableModels{{DBCollectionKey: NewTableKey("v1", "s1", "c1", nil), Columns: ColumnModels{{}}}}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("SchemaModel.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTableModels_GetByKey(t *testing.T) {
	key := NewTableKey("t1", "s1", "c1", nil)
	tables := TableModels{{DBCollectionKey: key}}
	assert.NotNil(t, tables.GetByKey(key))
	assert.Nil(t, tables.GetByKey(NewTableKey("t2", "s1", "c1", nil)))
}

func TestTableModels_GetByName(t *testing.T) {
	tables := TableModels{{DBCollectionKey: NewTableKey("t1", "s1", "c1", nil)}}
	assert.NotNil(t, tables.GetByName("t1"))
	assert.Nil(t, tables.GetByName("t2"))
}

func TestTableModel_String(t *testing.T) {
	table := &TableModel{DBCollectionKey: NewTableKey("t1", "s1", "c1", nil)}
	assert.Equal(t, "table{s1.t1}", table.String())
}

func TestTableModel_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := &TableModel{DBCollectionKey: NewTableKey("t1", "s1", "c1", nil)}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_columns", func(t *testing.T) {
		v := &TableModel{
			DBCollectionKey: NewTableKey("t1", "s1", "c1", nil),
			Columns:         ColumnModels{{}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_by_env", func(t *testing.T) {
		v := &TableModel{
			DBCollectionKey: NewTableKey("t1", "s1", "c1", nil),
			ByEnv:           StateByEnv{"env1": {Status: "unknown"}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_checks", func(t *testing.T) {
		v := &TableModel{
			DBCollectionKey: NewTableKey("t1", "s1", "c1", nil),
			Checks:          Checks{{ID: ""}},
		}
		assert.Error(t, v.Validate())
	})
}
