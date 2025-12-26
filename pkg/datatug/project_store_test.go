package datatug

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDeep(t *testing.T) {
	opt := Deep()
	var opts StoreOptions
	opt(&opts)
	assert.True(t, opts.deep)
}

func TestStoreOptions_Deep(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		opts := StoreOptions{deep: true}
		assert.True(t, opts.Deep())
	})
	t.Run("false", func(t *testing.T) {
		opts := StoreOptions{deep: false}
		assert.False(t, opts.Deep())
	})
}

func TestGetStoreOptions(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		opts := GetStoreOptions()
		assert.False(t, opts.deep)
	})

	t.Run("with_deep", func(t *testing.T) {
		opts := GetStoreOptions(Deep())
		assert.True(t, opts.deep)
	})

	t.Run("multiple_deep", func(t *testing.T) {
		opts := GetStoreOptions(Deep(), Deep())
		assert.True(t, opts.deep)
	})
}
