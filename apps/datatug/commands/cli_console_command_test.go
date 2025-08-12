package commands

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCommand_Execute(t *testing.T) {
	cmd := consoleCommand{}
	assert.Nil(t, cmd.Execute(nil))
}
