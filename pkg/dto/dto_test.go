package dto

import (
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestCreateFolder_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := CreateFolder{ProjectRef: ProjectRef{StoreID: "s1", ProjectID: "p1"}, Name: "n1"}
		assert.Nil(t, v.Validate())
	})
	t.Run("missing_project", func(t *testing.T) {
		v := CreateFolder{Name: "n1"}
		assert.NotNil(t, v.Validate())
	})
	t.Run("missing_name", func(t *testing.T) {
		v := CreateFolder{ProjectRef: ProjectRef{StoreID: "s1", ProjectID: "p1"}}
		assert.NotNil(t, v.Validate())
	})
}

func TestGetServerDatabasesRequest_Validate(t *testing.T) {
	t.Run("valid_with_env", func(t *testing.T) {
		v := GetServerDatabasesRequest{Project: "p1", Environment: "e1"}
		assert.Nil(t, v.Validate())
	})
	t.Run("valid_with_host", func(t *testing.T) {
		v := GetServerDatabasesRequest{Project: "p1"}
		v.Host = "h1"
		assert.Nil(t, v.Validate())
	})
	t.Run("missing_project", func(t *testing.T) {
		v := GetServerDatabasesRequest{Environment: "e1"}
		assert.NotNil(t, v.Validate())
	})
	t.Run("missing_env_and_host", func(t *testing.T) {
		v := GetServerDatabasesRequest{Project: "p1"}
		assert.NotNil(t, v.Validate())
	})
	t.Run("invalid_credentials", func(t *testing.T) {
		v := GetServerDatabasesRequest{
			Project:     "p1",
			Environment: "e1",
			Credentials: &datatug.Credentials{Username: "error"},
		}
		assert.NotNil(t, v.Validate())
	})
	t.Run("with_credentials", func(t *testing.T) {
		v := GetServerDatabasesRequest{
			Project:     "p1",
			Environment: "e1",
			Credentials: &datatug.Credentials{Username: "u1"},
		}
		assert.Nil(t, v.Validate())
	})
}

func TestCreateProjectRequest_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := CreateProjectRequest{StoreID: "s1", ID: "p1", Title: "t1"}
		assert.Nil(t, v.Validate())
	})
	t.Run("missing_store", func(t *testing.T) {
		v := CreateProjectRequest{ID: "p1", Title: "t1"}
		assert.NotNil(t, v.Validate())
	})
	t.Run("missing_id", func(t *testing.T) {
		v := CreateProjectRequest{StoreID: "s1", Title: "t1"}
		assert.NotNil(t, v.Validate())
	})
	t.Run("missing_title", func(t *testing.T) {
		v := CreateProjectRequest{StoreID: "s1", ID: "p1"}
		assert.NotNil(t, v.Validate())
	})
	t.Run("invalid_id", func(t *testing.T) {
		v := CreateProjectRequest{StoreID: "s1", ID: "Not Valid", Title: "t1"}
		assert.NotNil(t, v.Validate())
	})
}

func TestValidateProjectID(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		for _, id := range []string{"p", "1", "p1", "demo-project-1", "a_b", "a-b_c9", strings.Repeat("a", maxProjectIDLength)} {
			assert.NoError(t, validateProjectID(id), "id %q must be valid", id)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		for _, id := range []string{
			"",         // empty
			"P",        // upper case
			"p roject", // space
			"a/b",      // path separator
			`a\b`,      // windows path separator
			"..",       // parent directory
			".",        // current directory
			"a.b",      // dot
			"-p",       // leading dash
			"p-",       // trailing dash
			"_p",       // leading underscore
			"p_",       // trailing underscore
			"-",        // a dash on its own
			"тест",     // non-ASCII
			"a\tb",     // control character
			strings.Repeat("a", maxProjectIDLength+1), // too long
		} {
			assert.Error(t, validateProjectID(id), "id %q must be rejected", id)
		}
	})
}
