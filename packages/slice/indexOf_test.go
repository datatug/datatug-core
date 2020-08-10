package slice

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIndexOfInt(t *testing.T) {
	assert.EqualValues(t, -1, IndexOfInt([]int{}, 12))
	assert.EqualValues(t, 0, IndexOfInt([]int{12}, 12))
	assert.EqualValues(t, 0, IndexOfInt([]int{12, 13}, 12))
	assert.EqualValues(t, 1, IndexOfInt([]int{13, 12}, 12))
}

func TestIndexOfString(t *testing.T) {
	assert.EqualValues(t, -1, IndexOfString([]string{}, "12"))
	assert.EqualValues(t, 0, IndexOfString([]string{"12"}, "12"))
	assert.EqualValues(t, 0, IndexOfString([]string{"12", "13"}, "12"))
	assert.EqualValues(t, 1, IndexOfString([]string{"13", "12"}, "12"))
}
