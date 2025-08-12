package gauth_test

import (
	"github.com/datatug/datatug/packages/auth/gauth"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceAccountValidate(t *testing.T) {
	cases := []struct {
		name    string
		acc     gauth.ServiceAccountDbo
		wantErr bool
	}{
		{"empty both", gauth.ServiceAccountDbo{}, true},
		{"empty name", gauth.ServiceAccountDbo{Name: "", Path: "/tmp/a.json"}, true},
		{"empty path", gauth.ServiceAccountDbo{Name: "acc", Path: ""}, true},
		{"ok", gauth.ServiceAccountDbo{Name: "acc ", Path: "/tmp/a.json "}, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if err := c.acc.Validate(); (err != nil) != c.wantErr {
				t.Fatalf("Validate() err=%v wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestFileStore_DefaultPath(t *testing.T) {
	p, err := gauth.DefaultFilepath()
	if err != nil {
		t.Fatalf("DefaultFilepath() err: %v", err)
	}
	if filepath.Base(p) != "service_accounts.json" {
		t.Fatalf("unexpected base: %s", filepath.Base(p))
	}
}

func TestFileStore_LoadSave(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sa.json")
	store := gauth.FileStore{Filepath: p}

	// Load empty (non-existent) -> empty list
	list, err := store.Load()
	if err != nil {
		t.Fatalf("Load() err: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}

	// Save one
	accs := []gauth.ServiceAccountDbo{{Name: "a", Path: "/x/y/z.json"}}
	if err := store.Save(accs); err != nil {
		t.Fatalf("Save() err: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	// Load back
	list, err = store.Load()
	if err != nil {
		t.Fatalf("Load() 2 err: %v", err)
	}
	if len(list) != 1 || list[0].Name != "a" || list[0].Path != "/x/y/z.json" {
		t.Fatalf("unexpected data: %#v", list)
	}
}
