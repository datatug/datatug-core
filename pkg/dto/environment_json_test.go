package dto

import "testing"

func TestEnvironmentJSONUnicode(t *testing.T) {
	for _, s := range []string{`{"name":"\ud800"}`, `{"name":"\udc00"}`, `{"name":"\ud800x"}`, string([]byte{'"', 255, '"'})} {
		if ValidateEnvironmentJSONUnicode([]byte(s)) == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	for _, s := range []string{`{"name":"\ud83d\ude00"}`, `{"name":"𐀀"}`, `{"name":"\\ud800"}`} {
		if e := ValidateEnvironmentJSONUnicode([]byte(s)); e != nil {
			t.Fatalf("refused %q: %v", s, e)
		}
	}
}
