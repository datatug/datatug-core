package api

import (
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetServerDatabasesRequest_Validate(t *testing.T) {
	var request = dto.GetServerDatabasesRequest{}
	assert.NotNil(t, request.Validate())
}
