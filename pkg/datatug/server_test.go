package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServerReferences_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, ServerReferences{{Driver: "mysql", Host: "localhost"}}.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		assert.Error(t, ServerReferences{{}}.Validate())
	})
}

func TestServerReference_FileName(t *testing.T) {
	assert.Equal(t, "localhost", ServerRef{Host: "localhost"}.FileName())
	assert.Equal(t, "localhost@3306", ServerRef{Host: "localhost", Port: 3306}.FileName())
}

func TestServerReference_Address(t *testing.T) {
	assert.Equal(t, "localhost", ServerRef{Host: "localhost"}.Address())
	assert.Equal(t, "localhost:3306", ServerRef{Host: "localhost", Port: 3306}.Address())
}

func TestNewDbServer(t *testing.T) {
	t.Run("with_port", func(t *testing.T) {
		s, err := NewDbServer("mysql", "localhost:3306", ":")
		assert.NoError(t, err)
		assert.Equal(t, "localhost", s.Host)
		assert.Equal(t, 3306, s.Port)
	})
	t.Run("without_port", func(t *testing.T) {
		s, err := NewDbServer("mysql", "localhost", ":")
		assert.NoError(t, err)
		assert.Equal(t, "localhost", s.Host)
		assert.Equal(t, 0, s.Port)
	})
}

func TestServerReference_ID(t *testing.T) {
	assert.Equal(t, "mysql:localhost", ServerRef{Driver: "mysql", Host: "localhost"}.GetID())
	assert.Equal(t, "mysql:localhost:3306", ServerRef{Driver: "mysql", Host: "localhost", Port: 3306}.GetID())
}

func TestServerReference_Validate(t *testing.T) {
	t.Run("missing_driver", func(t *testing.T) {
		assert.Error(t, ServerRef{Host: "localhost"}.Validate())
	})
	t.Run("sqlite_with_host", func(t *testing.T) {
		assert.Error(t, ServerRef{Driver: "sqlite3", Host: "localhost"}.Validate())
	})
	t.Run("sqlite_with_port", func(t *testing.T) {
		assert.Error(t, ServerRef{Driver: "sqlite3", Port: 123}.Validate())
	})
	t.Run("unknown_driver", func(t *testing.T) {
		assert.Error(t, ServerRef{Driver: "unknown", Host: "localhost"}.Validate())
	})
	t.Run("missing_host", func(t *testing.T) {
		assert.Error(t, ServerRef{Driver: "mysql"}.Validate())
	})
	t.Run("negative_port", func(t *testing.T) {
		assert.Error(t, ServerRef{Driver: "mysql", Host: "localhost", Port: -1}.Validate())
	})
}

func TestProjDbServer_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ProjDbServer{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "mysql:localhost"}},
			Server:      ServerRef{Driver: "mysql", Host: "localhost"},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := ProjDbServer{Server: ServerRef{Driver: "mysql", Host: "localhost"}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_server", func(t *testing.T) {
		v := ProjDbServer{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_catalogs", func(t *testing.T) {
		v := ProjDbServer{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}},
			Server:      ServerRef{Driver: "mysql", Host: "localhost"},
			Catalogs:    DbCatalogs{{}},
		}
		assert.Error(t, v.Validate())
	})
}

func TestProjDbServers_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ProjDbServers{{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "mysql:localhost"}},
			Server:      ServerRef{Driver: "mysql", Host: "localhost"},
		}}
		assert.NoError(t, v.Validate())
	})
	t.Run("nil_item", func(t *testing.T) {
		v := ProjDbServers{nil}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_item", func(t *testing.T) {
		v := ProjDbServers{{}}
		assert.Error(t, v.Validate())
	})
}

func TestProjDbServers_GetProjDbServer(t *testing.T) {
	ref := ServerRef{Driver: "mysql", Host: "localhost", Port: 3306}
	s1 := &ProjDbServer{Server: ref}
	v := ProjDbServers{s1}
	assert.Equal(t, s1, v.GetProjDbServer(ref))
	assert.Nil(t, v.GetProjDbServer(ServerRef{Driver: "mysql", Host: "other"}))
}

func TestProjDbServerFile_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ProjDbServerFile{
			ServerRef: ServerRef{Driver: "mysql", Host: "localhost"},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_ref", func(t *testing.T) {
		v := ProjDbServerFile{}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_catalog", func(t *testing.T) {
		v := ProjDbServerFile{
			ServerRef: ServerRef{Driver: "mysql", Host: "localhost"},
			Catalogs:  []string{""},
		}
		assert.Error(t, v.Validate())
	})
}
