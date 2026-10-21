package jsonstrict

import "testing"

func TestEnvironmentJSONUnicode(t *testing.T) {
	for _, s := range []string{`{"name":"\ud800"}`, `{"name":"\udc00"}`, `{"name":"\ud800x"}`, string([]byte{'"', 255, '"'})} {
		if ValidateUnicode([]byte(s)) == nil {
			t.Fatalf("accepted %q", s)
		}
	}
	for _, s := range []string{`{"name":"\ud83d\ude00"}`, `{"name":"𐀀"}`, `{"name":"\\ud800"}`} {
		if e := ValidateUnicode([]byte(s)); e != nil {
			t.Fatalf("refused %q: %v", s, e)
		}
	}
}

func TestUnicodeIncompleteEscapes(t *testing.T) {
	for _, body := range []string{`"\`, `"\u12`, `"\uXX00"`, `"\ud800\uXXXX"`, `"\ud800\u0041"`} {
		if err := ValidateUnicode([]byte(body)); err == nil {
			t.Fatalf("malformed Unicode accepted %q", body)
		}
	}
}
