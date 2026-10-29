package datatug

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestProjectLogSourceContractAndExactDirectoryNamespace(t *testing.T) {
	// Unmodified published demo package fixture; the owning Apps parser and
	// project-log-sources.md define this separate presentation-source contract.
	body, err := os.ReadFile("../storage/filestore/testdata/connection-directory/log-sources.json")
	if err != nil {
		t.Fatal(err)
	}
	file, err := ParseProjectLogSourcesJSON(body)
	if err != nil || len(file.Sources) != 1 || file.Sources[0].ID != "shipping-logs" {
		t.Fatalf("actual package rejected %+v %v", file, err)
	}
	if _, catalog, err := ParsePublicProjectConnectionDirectoryJSON(ProjectLogSourcesFile, body); err != nil || catalog {
		t.Fatalf("separate namespace not proved %v %v", catalog, err)
	}
	for _, name := range []string{"other.json", "LOG-SOURCES.json", "shipping.json"} {
		if _, _, err := ParsePublicProjectConnectionDirectoryJSON(name, body); err == nil {
			t.Fatalf("misplaced log contract accepted at %s", name)
		}
	}
	for _, url := range []string{"http://127.0.0.1:6832/v1/databases/x", "http://localhost/v1/databases/x", "http://[::1]:6832/v1/databases/x", "https://public.example/base/v1/databases/x"} {
		if !validLogDatabase(url) {
			t.Fatalf("owning URL contract rejected %s", url)
		}
	}
	for _, url := range []string{"http://remote.example/v1/databases/x", "https://user:secret@public.example/v1/databases/x", "https://public.example/v1/databases/x?token=secret", "https://public.example/v1/databases/x#fragment", "https://public.example/wrong/x", "https://public.example/v1/databases/"} {
		if validLogDatabase(url) {
			t.Fatalf("unsafe URL accepted %s", url)
		}
	}
	for _, mode := range []string{"format", "duplicate source", "empty collections", "collection id", "duplicate collection", "missing role", "unknown role", "bad field", "identifier type", "identifier param", "duplicate param", "record param", "record URL", "window", "reversed", "unknown member", "duplicate member", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			var next ProjectLogSources
			if err := json.Unmarshal(body, &next); err != nil {
				t.Fatal(err)
			}
			source := &next.Sources[0]
			switch mode {
			case "format":
				next.Format = "datatug-log-sources/v2"
			case "duplicate source":
				next.Sources = append(next.Sources, *source)
			case "empty collections":
				source.Collections = nil
			case "collection id":
				source.Collections[0].ID = "invalid/id"
			case "duplicate collection":
				source.Collections = append(source.Collections, source.Collections[0])
			case "missing role":
				delete(source.Fields, "message")
			case "unknown role":
				source.Fields["future"] = "field"
			case "bad field":
				source.Fields["message"] = "unsafe field"
			case "identifier type":
				source.Identifiers[0].Type = "number"
			case "identifier param":
				source.Identifiers[0].Param = "collection"
			case "duplicate param":
				source.Identifiers[1].Param = source.Identifiers[0].Param
			case "record param":
				source.Records[0].Param = "unknown"
			case "record URL":
				source.Records[0].Database = "http://remote.example/v1/databases/x"
			case "window":
				source.Window.From = "2018-04-01T00:00:00+00:00"
			case "reversed":
				source.Window.From = "2099-04-01T00:00:00Z"
			}
			candidate, _ := json.Marshal(next)
			if mode == "unknown member" {
				candidate = []byte(strings.Replace(string(candidate), `"format":`, `"connections":[{"id":"hidden"}],"format":`, 1))
			}
			if mode == "duplicate member" {
				candidate = []byte(strings.Replace(string(candidate), `"format":`, `"format":"datatug-log-sources/v1","format":`, 1))
			}
			if mode == "oversized" {
				candidate = []byte(strings.Repeat("x", (1<<20)+1))
			}
			if _, _, err := ParseProjectConnectionDirectoryJSON(ProjectLogSourcesFile, candidate); err == nil {
				t.Fatal("unknown or corrupt source document was skipped")
			}
		})
	}
}
