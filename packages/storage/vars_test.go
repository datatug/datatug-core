package storage

import (
	"context"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCurrentIsNil(t *testing.T) {
	assert.Nil(t, Current)
}

func TestContextWithDatatugStore(t *testing.T) {
	store := NewNoOpStore()
	ctx := ContextWithDatatugStore(context.Background(), store)
	assert.NotNil(t, ctx)
	storeFromContext, err := StoreFromContext(ctx)
	assert.Nil(t, err)
	assert.Equal(t, store, storeFromContext)
}
