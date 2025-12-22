package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironmentFile_Validate(t *testing.T) {
	assert.NoError(t, EnvironmentFile{ID: "f1"}.Validate())
	assert.Error(t, EnvironmentFile{}.Validate())
}

func TestStateByEnv_Validate(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var v StateByEnv
		assert.NoError(t, v.Validate())
	})
	t.Run("valid", func(t *testing.T) {
		v := StateByEnv{"env1": {Status: "exists"}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := StateByEnv{"env1": {Status: "unknown"}}
		assert.Error(t, v.Validate())
	})
}

func TestEnvState_Validate(t *testing.T) {
	t.Run("valid_exists", func(t *testing.T) {
		assert.NoError(t, EnvState{Status: "exists"}.Validate())
	})
	t.Run("valid_missing", func(t *testing.T) {
		assert.NoError(t, EnvState{Status: "missing"}.Validate())
	})
	t.Run("missing_status", func(t *testing.T) {
		assert.Error(t, EnvState{}.Validate())
	})
	t.Run("unknown_status", func(t *testing.T) {
		assert.Error(t, EnvState{Status: "unknown"}.Validate())
	})
}
