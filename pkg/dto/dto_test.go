package dto

import (
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
	"testing"
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
		v := CreateProjectRequest{StoreID: "s1", Title: "t1"}
		assert.Nil(t, v.Validate())
	})
	t.Run("missing_store", func(t *testing.T) {
		v := CreateProjectRequest{Title: "t1"}
		assert.NotNil(t, v.Validate())
	})
	t.Run("missing_title", func(t *testing.T) {
		v := CreateProjectRequest{StoreID: "s1"}
		assert.NotNil(t, v.Validate())
	})
}
