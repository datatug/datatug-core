package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvDb_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := EnvDb{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "db1"}},
			Server:      ServerRef{Driver: "mysql", Host: "localhost"},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := EnvDb{Server: ServerRef{Driver: "mysql", Host: "localhost"}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_server", func(t *testing.T) {
		v := EnvDb{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "db1"}},
		}
		assert.Error(t, v.Validate())
	})
}
