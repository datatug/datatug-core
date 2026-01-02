package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDepth(t *testing.T) {
	opt := Depth(1)
	var opts StoreOptions
	opt(&opts)
	assert.Equal(t, 1, opts.depth)
}

func TestStoreOptions_Depth(t *testing.T) {
	for i := 0; i <= 10; i++ {
		opts := StoreOptions{depth: i}
		assert.Equal(t, i, opts.Depth())
	}
}

func TestGetStoreOptions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		opts := GetStoreOptions()
		assert.Equal(t, 0, opts.depth)
	})

	t.Run("with_depth", func(t *testing.T) {
		opts := GetStoreOptions(Depth(1))
		assert.Equal(t, 1, opts.depth)
	})

	t.Run("multiple_deep", func(t *testing.T) {
		opts := GetStoreOptions(Depth(10), Depth(20))
		assert.Equal(t, 20, opts.depth)
	})
}
