package parallel

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRun(t *testing.T) {
	assert.Nil(t, Run())
}
