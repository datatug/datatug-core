package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvDbServers_Validate(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var v EnvDbServers
		assert.NoError(t, v.Validate())
	})
	t.Run("valid", func(t *testing.T) {
		v := EnvDbServers{{ServerRef: ServerRef{Driver: "mysql", Host: "localhost"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := EnvDbServers{{}}
		assert.Error(t, v.Validate())
	})
}

func TestEnvDbServers_GetByServerRef(t *testing.T) {
	ref := ServerRef{Driver: "mysql", Host: "localhost", Port: 3306}
	s1 := &EnvDbServer{ServerRef: ref}
	v := EnvDbServers{s1}
	assert.Equal(t, s1, v.GetByServerRef(ref))
	assert.Nil(t, v.GetByServerRef(ServerRef{Driver: "mysql", Host: "other"}))
}

func TestEnvDbServer_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := EnvDbServer{ServerRef: ServerRef{Driver: "mysql", Host: "localhost"}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_ref", func(t *testing.T) {
		v := EnvDbServer{}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_catalog", func(t *testing.T) {
		v := EnvDbServer{
			ServerRef: ServerRef{Driver: "mysql", Host: "localhost"},
			Catalogs:  []string{""},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("duplicate_catalog", func(t *testing.T) {
		v := EnvDbServer{
			ServerRef: ServerRef{Driver: "mysql", Host: "localhost"},
			Catalogs:  []string{"c1", "c1"},
		}
		assert.Error(t, v.Validate())
	})
}

func TestEnvDbServer_IDOfAServerWithoutAHost(t *testing.T) {
	server := EnvDbServer{ServerRef: ServerRef{Driver: "mysql"}}
	assert.Equal(t, "mysql", server.GetID())
	assert.Equal(t, "db:3306", (&EnvDbServer{ServerRef: ServerRef{Driver: "mysql", Host: "db", Port: 3306}}).GetID())

	var parsed EnvDbServer
	parsed.SetID("oracle")
	assert.Equal(t, ServerRef{Driver: "oracle"}, parsed.ServerRef)
	parsed.SetID("db:1521")
	assert.Equal(t, "db", parsed.Host)
	assert.Equal(t, 1521, parsed.Port)

	duplicates := EnvDbServers{{ServerRef: ServerRef{Driver: "mysql"}}, {ServerRef: ServerRef{Driver: "oracle"}}, {ServerRef: ServerRef{Driver: "mysql"}}}
	err := duplicates.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate server ID at indexes 0 & 2: mysql")
	assert.NoError(t, duplicates[:2].Validate())
}
