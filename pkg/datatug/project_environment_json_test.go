package datatug

import (
	"encoding/json"
	"testing"
)

func TestProjectEnvironmentOrderUnicode(t *testing.T) {
	for _, body := range []string{`{"environmentOrder":["\ud800"]}`, `{"EnvironmentOrder":["\udc00"]}`, string(append([]byte(`{"environmentOrder":["`), append([]byte{255}, []byte(`"]}`)...)...)), `{"environmentOrder":["\ud800\u0041"]}`} {
		var file ProjectFile
		if err := json.Unmarshal([]byte(body), &file); err == nil {
			t.Fatalf("malformed order accepted %q", body)
		}
	}
	for _, body := range []string{`{"environmentOrder":["\ud83d\ude00","𐀀","�"]}`, `{"title":"\ud800","extension":"\udc00"}`, `null`} {
		var file ProjectFile
		if err := json.Unmarshal([]byte(body), &file); err != nil {
			t.Fatalf("compatible project rejected %s: %v", body, err)
		}
	}
	var file ProjectFile
	if err := file.UnmarshalJSON([]byte(`{`)); err == nil {
		t.Fatal("malformed JSON accepted")
	}
}
