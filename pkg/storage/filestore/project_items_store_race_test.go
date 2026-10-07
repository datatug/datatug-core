package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Directory items are loaded by one goroutine per entry. A successful read
// must not race an unsuccessful read through the outer load's named error.
func TestLoadProjectItemsConcurrentMixedResult(t *testing.T) {
	const count = 32
	dir := t.TempDir()
	for i := range count {
		if err := os.Mkdir(filepath.Join(dir, fmt.Sprintf("item-%02d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	store := newDirProjectItemsStore[datatug.ProjDbDrivers, *datatug.ProjDbDriver, datatug.ProjDbDriver](dir, dir, "driver")
	store.itemFileSuffix = "driver"
	var arrived sync.WaitGroup
	arrived.Add(count)
	store.readItemJSON = func(filePath string, dst any) error {
		arrived.Done()
		arrived.Wait()
		if strings.Contains(filePath, "item-00") || strings.Contains(filePath, "item-01") {
			return errors.New("simulated malformed item")
		}
		return json.Unmarshal([]byte(`{}`), dst)
	}
	_, err := store.loadProjectItems(context.Background(), dir)
	if err == nil || !strings.Contains(err.Error(), "simulated malformed item") {
		t.Fatalf("mixed read error lost: %v", err)
	}
}
