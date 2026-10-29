package datatug

import (
	"strings"
	"testing"
)

func TestParseProjectConnectionsJSON(t *testing.T) {
	valid := `{"format":"datatug-demo-connections/v1","connections":[{"id":"music","dataset":"music","storage":"ovdb","readiness":"public-api","environments":["dev"],"credentials":{"token":"private"}}],"private":{"keep":true}}`
	parsed, err := ParseProjectConnectionsJSON([]byte(valid))
	if err != nil || len(parsed.Connections) != 1 || parsed.Connections[0].ID != "music" {
		t.Fatalf("valid metadata unavailable %+v %v", parsed, err)
	}
	for _, bad := range []string{
		strings.Repeat("x", (1<<20)+1),
		`{"format":"datatug-demo-connections/v1","format":"other"}`,
		strings.Replace(valid, `"id":"music"`, `"id":"music","id":"other"`, 1),
		strings.Replace(valid, `"id":"music"`, `"id":"../music"`, 1),
		`{"format":"future"}`, `null`, `not json`,
	} {
		if _, err := ParseProjectConnectionsJSON([]byte(bad)); err == nil {
			t.Fatalf("invalid declaration accepted %.100s", bad)
		}
	}
}

func TestPublicConnectionCatalogSharesExactByteBoundary(t *testing.T) {
	const prefix = `{"format":"datatug-demo-connections/v1","private":"`
	const suffix = `"}`
	body := []byte(prefix + strings.Repeat("a", MaxPublicConnectionCatalogBytes-len(prefix)-len(suffix)) + suffix)
	if len(body) != MaxPublicConnectionCatalogBytes {
		t.Fatal("fixture does not meet exact public limit")
	}
	if _, err := ParsePublicProjectConnectionsJSON(body); err != nil {
		t.Fatalf("exact public limit refused: %v", err)
	}
	body = append(body, ' ')
	if _, err := ParsePublicProjectConnectionsJSON(body); err == nil {
		t.Fatal("over public limit accepted")
	}
	if _, err := ParseProjectConnectionsJSON(body); err != nil {
		t.Fatalf("existing private provider contract changed: %v", err)
	}
}
