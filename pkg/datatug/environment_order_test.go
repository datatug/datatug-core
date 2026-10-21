package datatug

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestEnvironmentOrderScalarAndRepair(t *testing.T) {
	ids := []string{"PROD", "😀", "𐀀", "\ue000", "Ω", "é", "UAT", "QA", "dev", "Dev", "LOCAL"}
	want := []string{"LOCAL", "Dev", "dev", "QA", "UAT", "é", "Ω", "\ue000", "𐀀", "😀", "PROD"}
	got, repair, err := ResolveEnvironmentOrder(ids, nil)
	if err != nil || repair || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q repair %v err %v", got, repair, err)
	}
	got, repair, err = ResolveEnvironmentOrder(ids, []string{"dev", "dev", "gone", "PROD"})
	if err != nil || !repair || got[0] != "dev" || got[1] != "PROD" || len(got) != len(ids) {
		t.Fatalf("repair %q %v %v", got, repair, err)
	}
	if ValidateEnvironmentPermutation(ids, []string{"dev"}) == nil {
		t.Fatal("partial accepted")
	}
	if _, _, err = ResolveEnvironmentOrder([]string{string([]byte{255})}, nil); err == nil {
		t.Fatal("invalid UTF8 accepted")
	}
}

func TestEnvironmentOrderPortableGoldens(t *testing.T) {
	body, e := os.ReadFile("testdata/environment-order.json")
	if e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Name        string
		IDs         []string
		SavedOrder  []string
		Order       []string
		NeedsRepair bool
	}
	if e = json.Unmarshal(body, &cases); e != nil {
		t.Fatal(e)
	}
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got, repair, e := ResolveEnvironmentOrder(c.IDs, c.SavedOrder)
			if e != nil || repair != c.NeedsRepair || !reflect.DeepEqual(got, c.Order) {
				t.Fatalf("got %q repair %v error %v want %q repair %v", got, repair, e, c.Order, c.NeedsRepair)
			}
		})
	}
}
