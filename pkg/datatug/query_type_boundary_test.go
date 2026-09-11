package datatug

import (
	"reflect"
	"testing"
)

// TestQueryDef_ExcludesResultRows proves the type boundary the query
// storage plan requires: a QueryDef can carry Recordsets as schema
// definitions (RecordsetDefinition), but nothing reachable from its type
// graph can hold an actual query result (Recordset, whose Rows field is
// [][]interface{} of live data). This is a structural guard - it fails at
// test time, not just at review time, if a future field addition lets a
// persisted/captured query carry result rows.
func TestQueryDef_ExcludesResultRows(t *testing.T) {
	forbidden := reflect.TypeOf(Recordset{})
	if walkTypeContains(reflect.TypeOf(QueryDef{}), forbidden, map[reflect.Type]bool{}) {
		t.Fatalf("QueryDef's type graph must not reach %v (a query result); it must only reach RecordsetDefinition (schema)", forbidden)
	}
}

// TestQueryDefWithFolderPath_ExcludesResultRows covers the capture-facing
// wrapper type too, since that is what RevisionedQueriesStore.PutQuery
// actually accepts.
func TestQueryDefWithFolderPath_ExcludesResultRows(t *testing.T) {
	forbidden := reflect.TypeOf(Recordset{})
	if walkTypeContains(reflect.TypeOf(QueryDefWithFolderPath{}), forbidden, map[reflect.Type]bool{}) {
		t.Fatalf("QueryDefWithFolderPath's type graph must not reach %v (a query result)", forbidden)
	}
}

// walkTypeContains reports whether target is reachable from t by
// following pointers, slices, arrays, maps (via their element type) and
// struct fields (including embedded/unexported ones - reflection sees
// them regardless of visibility). visited guards against infinite
// recursion through a self-referential or mutually-recursive type.
func walkTypeContains(t, target reflect.Type, visited map[reflect.Type]bool) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return walkTypeContains(t.Elem(), target, visited)
	case reflect.Struct:
		if t == target {
			return true
		}
		if visited[t] {
			return false
		}
		visited[t] = true
		for i := 0; i < t.NumField(); i++ {
			if walkTypeContains(t.Field(i).Type, target, visited) {
				return true
			}
		}
		return false
	default:
		return t == target
	}
}
