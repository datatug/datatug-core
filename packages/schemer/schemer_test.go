package schemer

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNewInformationSchema(t *testing.T) {
	var server = models.ServerReference{Driver: "sql"}
	v := NewInformationSchema(server, nil)
	assert.EqualValues(t, server, v.server)
	assert.Nil(t, v.db) // TODO: this assert does not make sense, populate DB with some mock
}
