package comparator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareDatabases(t *testing.T) {
	_, err := CompareDatabases(DatabasesToCompare{})
	assert.Nil(t, err)
}
