package store

// Current holds currently active store interface
//
// TODO: to be replaced with `func NewDatatugStore(id string) Interface`
var Current Interface

// NewDatatugStore creates new instance of Interface for a specific store
var NewDatatugStore = func(id string) (Interface, error) {
	panic("var 'NewDatatugStore' is not initialized")
}
