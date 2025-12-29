package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjItemBrief_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ProjItemBrief{ID: "i1", Title: "T"}
		assert.NoError(t, v.Validate(true))
	})
	t.Run("missing_id", func(t *testing.T) {
		v := ProjItemBrief{Title: "T"}
		assert.Error(t, v.Validate(true))
	})
	t.Run("missing_title", func(t *testing.T) {
		v := ProjItemBrief{ID: "i1"}
		assert.Error(t, v.Validate(true))
	})
	t.Run("valid_folder", func(t *testing.T) {
		v := ProjItemBrief{ID: "i1", Folder: "~"}
		assert.NoError(t, v.Validate(false))
	})
	t.Run("invalid_folder", func(t *testing.T) {
		v := ProjItemBrief{ID: "i1", Folder: "invalid"}
		assert.Error(t, v.Validate(false))
	})
	t.Run("invalid_tags", func(t *testing.T) {
		v := ProjItemBrief{ID: "i1", ListOfTags: ListOfTags{Tags: []string{""}}}
		assert.Error(t, v.Validate(false))
	})
}

func TestValidateFolderPath(t *testing.T) {
	t.Run("valid_shared_root", func(t *testing.T) {
		assert.NoError(t, ValidateFolderPath("~"))
	})
	t.Run("valid_shared_subfolder", func(t *testing.T) {
		assert.NoError(t, ValidateFolderPath("~/f1"))
	})
	t.Run("valid_user_root", func(t *testing.T) {
		assert.NoError(t, ValidateFolderPath("user:u1"))
	})
	t.Run("valid_user_subfolder", func(t *testing.T) {
		assert.NoError(t, ValidateFolderPath("user:u1/f1"))
	})
	t.Run("empty", func(t *testing.T) {
		assert.Error(t, ValidateFolderPath(""))
	})
	t.Run("invalid_root", func(t *testing.T) {
		assert.Error(t, ValidateFolderPath("invalid"))
	})
	t.Run("invalid_user_id", func(t *testing.T) {
		assert.Error(t, ValidateFolderPath("user: "))
	})
	t.Run("empty_subfolder_name", func(t *testing.T) {
		assert.Error(t, ValidateFolderPath("~/ "))
	})
	t.Run("subfolder_name_with_spaces", func(t *testing.T) {
		assert.Error(t, ValidateFolderPath("~/ f1 "))
	})
	t.Run("subfolder_named_root", func(t *testing.T) {
		assert.Error(t, ValidateFolderPath("~/~"))
	})
}

func TestProjectItem_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ProjectItem{ProjItemBrief: ProjItemBrief{ID: "i1"}, Access: "public", UserIDs: []string{"u1"}}
		assert.NoError(t, v.Validate(false))
	})
	t.Run("invalid_access", func(t *testing.T) {
		v := ProjectItem{ProjItemBrief: ProjItemBrief{ID: "i1"}, Access: "unknown"}
		assert.Error(t, v.Validate(false))
	})
	t.Run("invalid_user_id", func(t *testing.T) {
		v := ProjectItem{ProjItemBrief: ProjItemBrief{ID: "i1"}, UserIDs: []string{""}}
		assert.Error(t, v.Validate(false))
	})
	t.Run("duplicate_user_id", func(t *testing.T) {
		v := ProjectItem{ProjItemBrief: ProjItemBrief{ID: "i1"}, UserIDs: []string{"u1", "u1"}}
		assert.Error(t, v.Validate(false))
	})
}

func TestValidateStringField(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, validateStringField("f1", "val", true, 10))
	})
	t.Run("missing_required", func(t *testing.T) {
		assert.Error(t, validateStringField("f1", "", true, 10))
	})
	t.Run("too_long", func(t *testing.T) {
		assert.Error(t, validateStringField("f1", "too long value", true, 5))
	})
	t.Run("not_required_empty", func(t *testing.T) {
		assert.NoError(t, validateStringField("f1", "", false, 10))
	})
}

func TestProjItemBrief_GetIDSetID(t *testing.T) {
	v := ProjItemBrief{}
	v.SetID("i1")
	assert.Equal(t, "i1", v.GetID())
}
