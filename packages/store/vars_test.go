package store

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCurrentIsNil(t *testing.T) {
	assert.Nil(t, Current)
}
