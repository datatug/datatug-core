package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironments_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := Environments{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := Environments{{}}
		assert.Error(t, v.Validate())
	})
}

func TestEnvironments_GetEnvByID(t *testing.T) {
	e1 := &Environment{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}
	v := Environments{e1}
	assert.Equal(t, e1, v.GetByID("e1"))
	assert.Nil(t, v.GetByID("e2"))
}

func TestEnvironment_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := Environment{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := Environment{}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_db_servers", func(t *testing.T) {
		v := Environment{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}},
			DbServers:   EnvDbServers{{}},
		}
		assert.Error(t, v.Validate())
	})
}
