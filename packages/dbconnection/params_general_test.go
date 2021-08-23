package dbconnection

import "testing"

func TestGeneralParams_Catalog(t *testing.T) {
	expected := "TestCatalog"
	v := GeneralParams{catalog: expected}
	if v.Catalog() != expected {
		t.Error("Unexpected catalog value")
	}
}
