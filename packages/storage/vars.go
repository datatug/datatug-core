package storage

import "fmt"

// Current holds currently active storage interface
//
// TODO: to be replaced with `func NewDatatugStore(id string) Store`
var Current Store

// NewDatatugStore creates new instance of Store for a specific storage
var NewDatatugStore = func(id string) (Store, error) {
	panic("var 'NewDatatugStore' is not initialized")
}

var stores map[string]Store

func GetStore(id string) (Store, error) {
	store := stores[id]
	if store == nil {
		return nil, fmt.Errorf("no store configured for id=" + id)
	}
	return store, nil
}
