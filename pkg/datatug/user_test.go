package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidateStoreType(t *testing.T) {
	assert.True(t, IsValidateStoreType("firestore"))
	assert.True(t, IsValidateStoreType("github.com"))
	assert.True(t, IsValidateStoreType("agent"))
	assert.False(t, IsValidateStoreType("invalid"))
	assert.False(t, IsValidateStoreType(""))
}

func TestStoreBrief_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		s := StoreBrief{Type: "firestore"}
		assert.NoError(t, s.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		s := StoreBrief{}
		assert.Error(t, s.Validate())
	})
	t.Run("invalid_type", func(t *testing.T) {
		s := StoreBrief{Type: "invalid"}
		assert.Error(t, s.Validate())
	})
	t.Run("invalid_project", func(t *testing.T) {
		s := StoreBrief{
			Type: "firestore",
			Projects: map[string]ProjectBrief{
				"p1": {
					ProjectItem: ProjectItem{
						ProjItemBrief: ProjItemBrief{ID: ""},
					},
				},
			},
		}
		assert.Error(t, s.Validate())
	})
}

func TestUserDatatugInfo_Validate(t *testing.T) {
	t.Run("valid_empty", func(t *testing.T) {
		u := UserDatatugInfo{}
		assert.NoError(t, u.Validate())
	})
	t.Run("valid_with_stores", func(t *testing.T) {
		u := UserDatatugInfo{
			Stores: map[string]StoreBrief{
				"s1": {Type: "firestore"},
			},
		}
		assert.NoError(t, u.Validate())
	})
	t.Run("invalid_store", func(t *testing.T) {
		u := UserDatatugInfo{
			Stores: map[string]StoreBrief{
				"s1": {Type: "invalid"},
			},
		}
		assert.Error(t, u.Validate())
	})
}

func TestUser_Validate(t *testing.T) {
	t.Run("valid_nil_datatug", func(t *testing.T) {
		u := User{}
		assert.NoError(t, u.Validate())
	})
	t.Run("valid_datatug", func(t *testing.T) {
		u := User{Datatug: &UserDatatugInfo{}}
		assert.NoError(t, u.Validate())
	})
	t.Run("invalid_datatug", func(t *testing.T) {
		u := User{
			Datatug: &UserDatatugInfo{
				Stores: map[string]StoreBrief{
					"s1": {Type: "invalid"},
				},
			},
		}
		assert.Error(t, u.Validate())
	})
}
