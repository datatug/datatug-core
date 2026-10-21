package dto

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEnvironmentUnicodeMalformedEscapes(t *testing.T) {
	for _, body := range []string{`"\`, `"\u12`, `"\uXX00"`, `"\ud800\uXXXX"`, `"\ud800\u0041"`} {
		if err := ValidateEnvironmentJSONUnicode([]byte(body)); err == nil {
			t.Fatalf("accepted malformed escape %q", body)
		}
	}
}
func TestEnvironmentDigestCanonicalMembership(t *testing.T) {
	state := EnvironmentState{Environments: []EnvironmentBrief{{ID: "prod"}, {ID: "dev"}}, SavedOrder: []string{"prod", "dev"}, Order: []string{"prod", "dev"}}
	original := append([]EnvironmentBrief(nil), state.Environments...)
	a, err := EnvironmentStateDigest(state)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original, state.Environments) {
		t.Fatal("digest mutated caller membership")
	}
	state.Environments[0], state.Environments[1] = state.Environments[1], state.Environments[0]
	b, err := EnvironmentStateDigest(state)
	if err != nil || a != b || len(a) != 64 {
		t.Fatalf("unstable digest %q %q: %v", a, b, err)
	}
	state.SavedOrder = []string{"dev", "prod"}
	c, err := EnvironmentStateDigest(state)
	if err != nil || c == a {
		t.Fatalf("saved order not bound: %v", err)
	}
	state.Environments[0].ID = string([]byte{255})
	if _, err = EnvironmentStateDigest(state); err == nil {
		t.Fatal("invalid Unicode accepted")
	}
}
func TestEnvironmentReceiptEmptyOrderEncoding(t *testing.T) {
	for _, order := range [][]string{nil, {}, {"dev"}} {
		body, err := json.Marshal(EnvironmentOperationResult{OperationID: "operation", EnvironmentOrder: order})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		value, ok := fields["environmentOrder"]
		if order == nil {
			if ok {
				t.Fatal("nil order must be absent")
			}
		} else if !ok {
			t.Fatal("accepted order missing")
		} else {
			var got []string
			if err = json.Unmarshal(value, &got); err != nil || !reflect.DeepEqual(got, order) {
				t.Fatalf("order lost: %s", body)
			}
		}
	}
}

func TestEnvironmentReceiptWireParity(t *testing.T) {
	for _, order := range [][]string{nil, {}, {"prod", "dev"}} {
		receipt := EnvironmentOperationResult{OperationID: "op", Kind: "save-order", EnvironmentOrder: order, Status: "committed"}
		type plain EnvironmentOperationResult
		want, err := json.Marshal(plain(receipt))
		if err != nil {
			t.Fatal(err)
		}
		if order != nil && len(order) == 0 {
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(want, &fields); err != nil {
				t.Fatal(err)
			}
			fields["environmentOrder"] = json.RawMessage("[]")
			want, err = json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
		}
		got, err := json.Marshal(receipt)
		if err != nil || string(got) != string(want) {
			t.Fatalf("wire bytes changed: got %s want %s: %v", got, want, err)
		}
	}
}
