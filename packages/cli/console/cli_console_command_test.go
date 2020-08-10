package console

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCommand_Execute(t *testing.T) {
	cmd := command{}
	assert.Nil(t, cmd.Execute(nil))
}
