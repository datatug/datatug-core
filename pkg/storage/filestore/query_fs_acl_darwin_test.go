//go:build darwin

package filestore

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review SF-A, the part no check before the commit can see: an ACL that
// denies deleting a target passes access(2) and the file flags, and only
// the install's rename finds it. On the first target nothing has changed,
// so the writer rolls back; on the metadata the body is already installed,
// so only that query is refused until the ACL is removed.

func addDenyDeleteACL(t *testing.T, p string) (remove func()) {
	t.Helper()
	if out, err := exec.Command("chmod", "+a", "everyone deny delete", p).CombinedOutput(); err != nil {
		t.Skipf("cannot add an ACL here: %v: %s", err, out)
	}
	remove = func() { _ = exec.Command("chmod", "-N", p).Run() }
	t.Cleanup(remove)
	return remove
}

func TestACLDenyDeleteOnTheBody_RollsTheWriteBack(t *testing.T) {
	ps, queriesDir, rev := seedQ(t)
	addDenyDeleteACL(t, filepath.Join(queriesDir, "q.query.dtql"))
	requireRefusedWrite(t, putQ("t1", "T1", datatug.QueryTypeDTQL)(ps, rev), "nothing was changed")
	if got, err := ps.LoadQueryRevision(context.Background(), "q"); err != nil || got.Revision != rev || got.Query.Text != "T0" {
		t.Errorf("expected q unchanged, got %+v, %v", got, err)
	}
	assertStoreUsableAfterRefusal(t, ps, queriesDir)
}

func TestACLDenyDeleteOnTheMetadata_RefusesOnlyThatQuery(t *testing.T) {
	ctx := context.Background()
	ps, queriesDir, rev := seedQ(t)
	remove := addDenyDeleteACL(t, filepath.Join(queriesDir, "q.query.json"))
	requireStuck(t, "PutQuery(q)", putQ("t1", "T1", datatug.QueryTypeDTQL)(ps, rev))
	if q, err := ps.LoadQuery(ctx, "other"); err != nil || q.Text != "OTHER" {
		t.Errorf("LoadQuery(other): %v", err)
	}
	third := dtqlQuery("third", "", "THIRD")
	if err := ps.SaveQuery(ctx, &third); err != nil {
		t.Errorf("SaveQuery(third): %v", err)
	}
	_, err := ps.LoadQueryRevision(ctx, "q")
	requireStuck(t, "LoadQueryRevision(q)", err)
	remove()
	requireQ(t, ps, "t1", "T1", datatug.QueryTypeDTQL)
}
