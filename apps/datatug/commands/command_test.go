package commands

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCommandIsNotNil(t *testing.T) {
	cmd := DatatugCommand()
	assert.NotNil(t, cmd)
}
