package api

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetServerDatabasesRequest_Validate(t *testing.T) {
	var request = GetServerDatabasesRequest{}
	assert.NotNil(t, request.Validate())
}
