package storage

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurrentIsNil(t *testing.T) {
	assert.Nil(t, Current)
}

func TestContextWithDatatugStore(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		store := NewNoOpStore()
		ctx := ContextWithDatatugStore(context.Background(), store)
		assert.NotNil(t, ctx)
		storeFromContext, err := StoreFromContext(ctx)
		assert.Nil(t, err)
		assert.Equal(t, store, storeFromContext)
	})
	t.Run("panic_on_nil", func(t *testing.T) {
		assert.Panics(t, func() {
			ContextWithDatatugStore(context.Background(), nil)
		})
	})
}

func TestStoreFromContext_Error(t *testing.T) {
	_, err := StoreFromContext(context.Background())
	assert.Error(t, err)
}

func TestGetStore(t *testing.T) {
	mockStore := NewNoOpStore()

	t.Run("from_stores_map", func(t *testing.T) {
		stores = map[string]Store{"s1": mockStore}
		defer func() { stores = nil }()

		store, err := GetStore(context.Background(), "s1")
		assert.NoError(t, err)
		assert.Equal(t, mockStore, store)
	})

	t.Run("from_context", func(t *testing.T) {
		stores = nil
		ctx := ContextWithDatatugStore(context.Background(), mockStore)

		store, err := GetStore(ctx, "s2")
		assert.NoError(t, err)
		assert.Equal(t, mockStore, store)
	})

	t.Run("not_found", func(t *testing.T) {
		stores = nil
		_, err := GetStore(context.Background(), "s3")
		assert.Error(t, err)
	})
}

func TestGetProjectStore(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		// NoOpStore panics on GetProjectStore, but we want to see it called
		mockStore := NewNoOpStore()
		stores = map[string]Store{"s1": mockStore}
		defer func() { stores = nil }()

		assert.Panics(t, func() {
			_, _ = GetProjectStore(context.Background(), "s1", "p1")
		})
	})
	t.Run("error", func(t *testing.T) {
		stores = nil
		_, err := GetProjectStore(context.Background(), "s1", "p1")
		assert.Error(t, err)
	})
}

func TestNewDatatugStore(t *testing.T) {
	t.Run("panic_if_not_initialized", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = NewDatatugStore("test")
		})
	})

	t.Run("custom_implementation", func(t *testing.T) {
		orig := NewDatatugStore
		defer func() { NewDatatugStore = orig }()

		mockStore := NewNoOpStore()
		NewDatatugStore = func(id string) (Store, error) {
			if id == "mock" {
				return mockStore, nil
			}
			return nil, fmt.Errorf("not found")
		}

		store, err := NewDatatugStore("mock")
		assert.NoError(t, err)
		assert.Equal(t, mockStore, store)

		_, err = NewDatatugStore("unknown")
		assert.Error(t, err)
	})
}
