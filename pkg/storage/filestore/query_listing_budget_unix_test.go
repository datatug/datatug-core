//go:build unix

package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Review SF2: a listing checks each file's size against its budget before
// opening the file. A body over the remaining budget is refused even when
// it cannot be opened at all (mode 000): the refusal is the budget's, not
// the open's "permission denied".
func TestLoadQueries_ChecksTheBudgetBeforeOpeningAFile(t *testing.T) {
	skipIfRoot(t)
	ps, queriesDir := newBudgetTestProject(t, 1000, map[string]string{"a": strings.Repeat("x", 2000)})
	body := filepath.Join(queriesDir, "a.query.dtql")
	if err := os.Chmod(body, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(body, 0o600) })
	_, err := ps.LoadQueries(context.Background(), "")
	requireListingTooLarge(t, "LoadQueries", err)
	if strings.Contains(err.Error(), "permission denied") {
		t.Errorf("expected the size check to refuse the file before opening it, got: %v", err)
	}
}
