package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEntities_GetEntityByID(t *testing.T) {
	v := Entities{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}}
	assert.NotNil(t, v.GetByID("e1"))
	assert.Nil(t, v.GetByID("e2"))
}

func TestEntities_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := Entities{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := Entities{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}}
		assert.Error(t, v.Validate())
	})
}

func TestEntities_IDs(t *testing.T) {
	v := Entities{
		{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}},
		{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e2"}}},
	}
	assert.Equal(t, []string{"e1", "e2"}, v.IDs())
	assert.Empty(t, Entities{}.IDs())
}

func TestEntity_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := Entity{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_fields", func(t *testing.T) {
		v := Entity{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}},
			Fields:      EntityFields{{ID: ""}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_tables", func(t *testing.T) {
		v := Entity{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}},
			Tables:      TableKeys{NewTableKey("t1", "s1", "c1", nil)},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_tags", func(t *testing.T) {
		v := Entity{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}},
			ListOfTags:  ListOfTags{Tags: []string{""}},
		}
		assert.Error(t, v.Validate())
	})
}

func TestStringPatterns_Validate(t *testing.T) {
	v := StringPatterns{{Type: "exact", Value: "v1"}}
	assert.NoError(t, v.Validate())
	v = StringPatterns{{Type: ""}}
	assert.Error(t, v.Validate())
}

func TestStringPattern_Validate(t *testing.T) {
	t.Run("valid_exact", func(t *testing.T) {
		v := StringPattern{Type: "exact", Value: "v1"}
		assert.NoError(t, v.Validate())
	})
	t.Run("valid_regexp", func(t *testing.T) {
		v := StringPattern{Type: "regexp", Value: "^v1$"}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		v := StringPattern{Value: "v1"}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_regexp", func(t *testing.T) {
		v := StringPattern{Type: "regexp", Value: "["}
		assert.Error(t, v.Validate())
	})
	t.Run("unknown_type", func(t *testing.T) {
		v := StringPattern{Type: "unknown", Value: "v1"}
		assert.Error(t, v.Validate())
	})
	t.Run("missing_value", func(t *testing.T) {
		v := StringPattern{Type: "exact", Value: ""}
		assert.Error(t, v.Validate())
	})
}

func TestEntityField_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := EntityField{ID: "f1", Type: "string"}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_id", func(t *testing.T) {
		v := EntityField{Type: "string"}
		assert.Error(t, v.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		v := EntityField{ID: "f1"}
		assert.Error(t, v.Validate())
	})
	t.Run("unknown_type", func(t *testing.T) {
		v := EntityField{ID: "f1", Type: "unknown"}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_patterns", func(t *testing.T) {
		v := EntityField{ID: "f1", Type: "string", NamePatterns: StringPatterns{{}}}
		assert.Error(t, v.Validate())
	})
}

func TestEntityFieldRef_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := EntityFieldRef{Entity: "e1", Field: "f1"}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_entity", func(t *testing.T) {
		v := EntityFieldRef{Field: "f1"}
		assert.Error(t, v.Validate())
	})
	t.Run("missing_field", func(t *testing.T) {
		v := EntityFieldRef{Entity: "e1"}
		assert.Error(t, v.Validate())
	})
}
