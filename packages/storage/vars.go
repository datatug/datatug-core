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

func storeFromContext(ctx context.Context) Store {
	return ctx.Value(storeContextKey).(Store)
}

func GetStore(ctx context.Context, id string) (Store, error) {
	if store, ok := stores[id]; ok && store != nil {
		return store, nil
	}
	store := storeFromContext(ctx)
	if store == nil {
		return nil, fmt.Errorf("no store configured for id=" + id)
	}
	if storeID := store.ID(); storeID != id {
		return nil, fmt.Errorf("store.ID() != id: %v != %v", storeID, id)
	}
	return store, nil
}
