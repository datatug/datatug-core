package comparator

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCompareDatabases(t *testing.T) {
	_, err := CompareDatabases(DatabasesToCompare{})
	assert.Nil(t, err)
}
