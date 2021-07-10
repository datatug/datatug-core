package storage

import (
	"context"
	"fmt"
)

// Current holds currently active storage interface
//
// TODO: to be replaced with `func NewDatatugStore(id string) Store`
var Current Store

var stores map[string]Store

// NewDatatugStore creates new instance of Store for a specific storage
var NewDatatugStore = func(id string) (Store, error) {
	panic("var 'NewDatatugStore' is not initialized")
}

const storeContextKey = "datatug_store"

func ContextWithDatatugStore(ctx context.Context, store Store) context.Context {
	return context.WithValue(ctx, storeContextKey, store)
}

func StoreFromContext(ctx context.Context) (Store, error) {
	var store = ctx.Value(storeContextKey)
	if store == nil {
		return nil, fmt.Errorf("context of type %T have no `storage.Store` value", ctx)
	}
	return store.(Store), nil
}

func GetStore(ctx context.Context, id string) (Store, error) {
	if store, ok := stores[id]; ok && store != nil {
		return store, nil
	}
	store, err := StoreFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("no store configured for id=%v: %w", id, err)
	}
	if store == nil {
		return nil, fmt.Errorf("no store configured for id=" + id)
	}
	return store, nil
}
