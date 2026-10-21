package datatug

import (
	"encoding/json"
	"testing"
)

func TestProjectFileRetainsOpaqueMetadataAndEmptyOrder(t *testing.T) {
	var p ProjectFile
	if e := json.Unmarshal([]byte(`{"id":"p","environmentOrder":[],"future":{"large":9007199254740993}}`), &p); e != nil {
		t.Fatal(e)
	}
	if p.EnvironmentOrder == nil {
		t.Fatal("empty order became absent")
	}
	b, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	if e = json.Unmarshal(b, &fields); e != nil {
		t.Fatal(e)
	}
	if string(fields["environmentOrder"]) != "[]" {
		t.Fatal("empty order omitted")
	}
	if string(fields["future"]) != `{"large":9007199254740993}` {
		t.Fatalf("opaque field %s", fields["future"])
	}
}
