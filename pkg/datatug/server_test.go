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
	// Regression test for #307: sqlite3 is file-based, so a ServerRef with an
	// empty Host and Port must validate - the "sqlite3" case used to fall
	// through into the generic "host is required" check below it instead of
	// returning, so no sqlite3 ServerRef could ever validate.
	t.Run("sqlite_file_based_with_empty_host_is_valid", func(t *testing.T) {
		assert.NoError(t, ServerRef{Driver: "sqlite3"}.Validate())
	})
	t.Run("sqlite_file_based_with_path_and_empty_host_is_valid", func(t *testing.T) {
		assert.NoError(t, ServerRef{Driver: "sqlite3", Path: "/var/data/chinook.sqlite"}.Validate())
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

func TestProjDbDriver_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ProjDbDriver{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "d1", Title: "Driver 1"}},
		}
		assert.NoError(t, v.Validate())
	})
	// Regression test: ProjDbDriver.Validate() used to call
	// v.ProjItemBrief.Validate(), which ProjItemBrief does not define itself
	// — Go resolved it to the promoted ListOfTags.Validate() (ProjItemBrief
	// embeds ListOfTags), silently skipping the id/title checks every other
	// project-item type enforces via ValidateWithOptions.
	t.Run("missing_id", func(t *testing.T) {
		v := ProjDbDriver{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{Title: "Driver 1"}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("missing_title", func(t *testing.T) {
		v := ProjDbDriver{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "d1"}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_servers", func(t *testing.T) {
		v := ProjDbDriver{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "d1", Title: "Driver 1"}},
			Servers:     ProjDbServers{nil},
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

func TestServerRef_Validate_Drivers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ref     ServerRef
		wantErr string // empty means valid
	}{
		{"postgres without host", ServerRef{Driver: "postgres"}, ""},
		{"postgres with host", ServerRef{Driver: "postgres", Host: "db.local"}, ""},
		{"postgres with host and port", ServerRef{Driver: "postgres", Host: "db.local", Port: 5432}, ""},
		{"postgres with port only", ServerRef{Driver: "postgres", Port: 5432}, ""},
		{"postgres negative port", ServerRef{Driver: "postgres", Port: -1}, "port"},
		{"ingitdb bare", ServerRef{Driver: "ingitdb"}, ""},
		{"ingitdb with path", ServerRef{Driver: "ingitdb", Path: "/x"}, ""},
		{"ingitdb with host", ServerRef{Driver: "ingitdb", Host: "h"}, "cannot be used with ingitdb"},
		{"ingitdb with port", ServerRef{Driver: "ingitdb", Port: 1}, "cannot be used with ingitdb"},
		{"openvaultdb bare", ServerRef{Driver: "openvaultdb"}, ""},
		{"openvaultdb with host", ServerRef{Driver: "openvaultdb", Host: "h"}, "cannot be used with openvaultdb"},
		{"openvaultdb with port", ServerRef{Driver: "openvaultdb", Port: 1}, "cannot be used with openvaultdb"},
		{"https-json bare", ServerRef{Driver: "https-json"}, ""},
		{"https-json with host", ServerRef{Driver: "https-json", Host: "h"}, "cannot be used with https-json"},
		{"https-json with port", ServerRef{Driver: "https-json", Port: 1}, "cannot be used with https-json"},
		{"sqlite3 with host", ServerRef{Driver: "sqlite3", Host: "h"}, "cannot be used with sqlite3"},
		{"sqlserver without host", ServerRef{Driver: "sqlserver"}, "host"},
		{"sqlserver with host", ServerRef{Driver: "sqlserver", Host: "h"}, ""},
		{"mysql without host", ServerRef{Driver: "mysql"}, "host"},
		{"oracle without host", ServerRef{Driver: "oracle"}, "host"},
		{"oracle with host", ServerRef{Driver: "oracle", Host: "h", Port: 1521}, ""},
		{"unknown", ServerRef{Driver: "mongo", Host: "h"}, "unexpected value: mongo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// every validator that embeds or wraps a ServerRef must agree
			validators := map[string]func() error{
				"ServerRef":        tc.ref.Validate,
				"ServerReferences": ServerReferences{tc.ref}.Validate,
				"EnvDbServer":      (&EnvDbServer{ServerRef: tc.ref}).Validate,
				"EnvDbServers":     EnvDbServers{{ServerRef: tc.ref}}.Validate,
				"ProjDbServerFile": ProjDbServerFile{ServerRef: tc.ref}.Validate,
			}
			for name, validate := range validators {
				err := validate()
				if tc.wantErr == "" {
					assert.NoError(t, err, name)
				} else if assert.Error(t, err, name) {
					assert.Contains(t, err.Error(), tc.wantErr, name)
				}
			}
		})
	}
}

func TestServerRef_Validate_UnknownDriverNamesAcceptedDrivers(t *testing.T) {
	err := ServerRef{Driver: "mongo", Host: "h"}.Validate()
	if assert.Error(t, err) {
		for _, d := range []string{"sqlite3", "sqlserver", "mysql", "oracle", "postgres", "ingitdb", "openvaultdb", "https-json"} {
			assert.Contains(t, err.Error(), d)
		}
	}
}

func TestProjDbServer_Validate_Postgres(t *testing.T) {
	ref := ServerRef{Driver: "postgres"}
	v := ProjDbServer{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ref.GetID()}}, Server: ref}
	assert.NoError(t, v.Validate())
}
