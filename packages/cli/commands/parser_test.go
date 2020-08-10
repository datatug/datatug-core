package commands

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParserIsNotNil(t *testing.T) {
	assert.NotNil(t, Parser)
}
