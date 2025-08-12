package commands

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCommandIsNotNil(t *testing.T) {
	cmd := GetCommand()
	assert.NotNil(t, cmd)
}
