package api

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/stretchr/testify/assert"
)

func TestGetServerDatabasesRequest_Validate(t *testing.T) {
	var request = dto.GetServerDatabasesRequest{}
	assert.NotNil(t, request.Validate())
}
