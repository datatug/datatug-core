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
	assert.Equal(t, e1, v.GetEnvByID("e1"))
	assert.Nil(t, v.GetEnvByID("e2"))
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

func TestEnvDbServers_Validate(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var v EnvDbServers
		assert.NoError(t, v.Validate())
	})
	t.Run("valid", func(t *testing.T) {
		v := EnvDbServers{{ServerReference: ServerReference{Driver: "mysql", Host: "localhost"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := EnvDbServers{{}}
		assert.Error(t, v.Validate())
	})
}

func TestEnvDbServers_GetByServerRef(t *testing.T) {
	ref := ServerReference{Driver: "mysql", Host: "localhost", Port: 3306}
	s1 := &EnvDbServer{ServerReference: ref}
	v := EnvDbServers{s1}
	assert.Equal(t, s1, v.GetByServerRef(ref))
	assert.Nil(t, v.GetByServerRef(ServerReference{Driver: "mysql", Host: "other"}))
}

func TestEnvDbServer_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := EnvDbServer{ServerReference: ServerReference{Driver: "mysql", Host: "localhost"}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_ref", func(t *testing.T) {
		v := EnvDbServer{}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_catalog", func(t *testing.T) {
		v := EnvDbServer{
			ServerReference: ServerReference{Driver: "mysql", Host: "localhost"},
			Catalogs:        []string{""},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("duplicate_catalog", func(t *testing.T) {
		v := EnvDbServer{
			ServerReference: ServerReference{Driver: "mysql", Host: "localhost"},
			Catalogs:        []string{"c1", "c1"},
		}
		assert.Error(t, v.Validate())
	})
}

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

func TestEnvDb_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := EnvDb{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "db1"}},
			Server:      ServerReference{Driver: "mysql", Host: "localhost"},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := EnvDb{Server: ServerReference{Driver: "mysql", Host: "localhost"}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_server", func(t *testing.T) {
		v := EnvDb{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "db1"}},
		}
		assert.Error(t, v.Validate())
	})
}
