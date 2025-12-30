package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironmentSummary_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := EnvironmentSummary{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := EnvironmentSummary{}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_servers", func(t *testing.T) {
		v := EnvironmentSummary{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}},
			Servers:     EnvDbServers{{}},
		}
		assert.Error(t, v.Validate())
	})
}
