package dto

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCredentials_Validate(t *testing.T) {
	var credentials = Credentials{}
	assert.Nil(t, credentials.Validate())
}
