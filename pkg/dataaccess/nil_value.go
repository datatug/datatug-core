package dataaccess

import "reflect"

// isNilValue recognizes nil interface values and typed nil provider values
// before Core invokes an optional port or takes ownership of an opened handle.
func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	switch reflect.ValueOf(value).Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(value).IsNil()
	default:
		return false
	}
}
