package main

import (
	"github.com/datatug/datatug/apps/datatugcli/commands"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPackage(t *testing.T) {
	assert.NotNil(t, commands.GetParser())
}
