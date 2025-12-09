package endpoints

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSupportedOrigin(t *testing.T) {
	assert.True(t, IsSupportedOrigin("http://localhost:8100"))
	assert.True(t, IsSupportedOrigin("https://datatug.app"))
	assert.True(t, IsSupportedOrigin("https://test.datatug.app"))
	assert.False(t, IsSupportedOrigin("https://www.example.com"))
	assert.False(t, IsSupportedOrigin("http://www.example.com"))
}
