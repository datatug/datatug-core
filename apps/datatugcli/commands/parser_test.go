package commands

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestParserIsNotNil(t *testing.T) {
	p := GetParser()
	assert.NotNil(t, p)
}
