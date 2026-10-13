package filestore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func TestRevisionReaderRetainsLegacyQueryBodyAcrossTugQLTypeNames(t *testing.T) {
	for _, tc := range []struct {
		name, declared, sidecar string
	}{
		{"canonical metadata with legacy body", string(datatug.QueryTypeTugQL), "q.query.dtql"},
		{"legacy metadata with canonical body", string(datatug.QueryTypeDTQL), "q.query.tugql"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			metadata := []byte(`{"id":"q","title":"Existing","type":"` + tc.declared + `"}`)
			if err := os.WriteFile(filepath.Join(dir, "q.query.json"), metadata, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, tc.sidecar), []byte("from: Invoice\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			current, err := readCurrentQueryPair(dir, "q")
			if err != nil || !current.exists || !current.complete || current.bodyFileName != tc.sidecar || string(current.bodyBytes) != "from: Invoice\n" || current.revision == "" {
				t.Fatalf("legacy query revision lost its body: %+v %v", current, err)
			}
		})
	}
}
