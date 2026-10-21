package datatug

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEnvironmentOrderRejectsMalformedMembership(t *testing.T) {
	bad := string([]byte{255})
	if _, err := CompareEnvironmentIDs(bad, "dev"); err == nil {
		t.Fatal("invalid Unicode compared")
	}
	if _, _, err := ResolveEnvironmentOrder([]string{"dev"}, []string{bad}); err == nil {
		t.Fatal("invalid saved ID accepted")
	}
	if err := ValidateEnvironmentPermutation([]string{"dev", "dev"}, []string{"dev"}); err == nil {
		t.Fatal("duplicate membership accepted")
	}
	if err := ValidateEnvironmentPermutation([]string{"dev", "prod"}, []string{"prod", "dev"}); err != nil {
		t.Fatal(err)
	}
}
func TestProjectFileMalformedJSONAndMetadata(t *testing.T) {
	var file ProjectFile
	if err := json.Unmarshal([]byte(`{"environmentOrder":1}`), &file); err == nil {
		t.Fatal("invalid typed order accepted")
	}
	if err := json.Unmarshal([]byte(`{"extension":{"retained":true}}`), &file); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["extension"]) != `{"retained":true}` {
		t.Fatalf("extension lost %s", body)
	}
	file.jsonMetadata["broken"] = json.RawMessage(`{`)
	if _, err = json.Marshal(file); err == nil {
		t.Fatal("invalid raw metadata accepted")
	}
}

func TestProjectFileMarshalRejectsInvalidCreatedTime(t *testing.T) {
	file := ProjectFile{Created: &ProjectCreated{At: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}}
	if _, err := json.Marshal(file); err == nil {
		t.Fatal("invalid created timestamp encoded")
	}
}
