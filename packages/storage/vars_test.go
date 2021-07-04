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
	store := NoOpStore{}
	ctx := ContextWithDatatugStore(context.Background(), store)
	assert.NotNil(t, ctx)
	assert.Equal(t, store, storeFromContext(ctx))
}
