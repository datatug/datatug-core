package filestore

import (
	"errors"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Transaction slots: a write that cannot complete never wedges the store
// (review SF-A).
//
// A committed transaction whose install cannot be completed yet - because
// of something the checks before the commit cannot see (see
// checkQueryTargetReplaceable), or a change made since - stays committed
// until the entry it names is fixed. It keeps the slot it was committed
// in, and later transactions stage in the next free one. Slot 0 is the
// transaction directory itself (".dt-query-txn"), where every transaction
// stages while none is stuck; slot n (1 <= n < queryTxnSlotCount) is its
// subdirectory "slot-<n>", created 0700 only while every lower slot is
// held. Recovery (recoverQueryTransactions) completes every slot it can.
// Each one it cannot becomes a stuckQueryTxn, and every operation then
// refuses the query it names - its reads, its writes and the listing of
// its folder - with the transaction's fix-forward error, while every other
// query is read and written as usual. A stuck transaction is never served,
// rolled back or moved: the first access after the entry is fixed
// completes it forward.
//
// The named query is matched conservatively, so no spelling can reach its
// files: its folder by file identity or by queryPathKey, its ID by
// queryNameKey (case-insensitive, and blind to how non-ASCII characters
// are composed) or by sharing a file with it. Over-matching only refuses a
// few more IDs while a write is stuck.

// queryTxnSlotCount bounds how many committed transactions can wait for
// their entries to be fixed while the store keeps writing. With every slot
// held, writes are refused - with every stuck transaction's error - until
// one entry is fixed; reads of the other queries still work.
const queryTxnSlotCount = 8

// queryTxnSlotPrefix names the extra slots "slot-1" ... "slot-7".
const queryTxnSlotPrefix = "slot-"

// queryTxnSlotDir returns slot n of txnDir; slot 0 is txnDir itself.
func queryTxnSlotDir(txnDir string, n int) string {
	if n == 0 {
		return txnDir
	}
	return path.Join(txnDir, queryTxnSlotPrefix+strconv.Itoa(n))
}

// vetQueryTxnSlotDir checks an extra slot the way the transaction directory
// is checked: an ordinary directory, never a symlink, owned by the
// effective user and, where POSIX permission bits are meaningful, 0700 or
// tighter. It returns exists=false with a nil error when nothing is there.
func vetQueryTxnSlotDir(slot string) (exists bool, err error) {
	info, err := os.Lstat(slot)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
		return true, fmt.Errorf("query transaction slot %s is not a directory; refusing to use it", slot)
	case !fileOwnedByCurrentUser(info):
		return true, fmt.Errorf("query transaction slot %s is owned by another user; refusing to use it", slot)
	case runtime.GOOS != "windows" && info.Mode().Perm()&^0o700 != 0:
		return true, fmt.Errorf("query transaction slot %s has overly broad permissions %v; refusing to use it", slot, info.Mode().Perm())
	}
	return true, nil
}

// ensureQueryTxnSlotDir creates extra slot slot at 0700 unless it exists,
// then vets it.
func ensureQueryTxnSlotDir(slot string) error {
	if err := os.Mkdir(slot, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("failed to create query transaction slot %s: %w", slot, err)
	}
	_, err := vetQueryTxnSlotDir(slot)
	return err
}

// stuckQueryTxn is a committed transaction recovery could not complete.
type stuckQueryTxn struct {
	slot    string
	journal queryTxnJournal
	err     *queryTxnIncompleteError
	// dir and dirInfo are the query's folder, when it still resolves.
	dir     string
	dirInfo os.FileInfo
}

func newStuckQueryTxn(queriesRoot, slot string, incomplete *queryTxnIncompleteError) stuckQueryTxn {
	st := stuckQueryTxn{slot: slot, journal: incomplete.journal, err: incomplete}
	if dir, err := walkQueryDir(queriesRoot, st.journal.FolderPath, st.journal.ID, false); err == nil {
		if info, err := os.Lstat(dir); err == nil && info.IsDir() {
			st.dir, st.dirInfo = dir, info
		}
	}
	return st
}

// recoverQueryTransactions runs recovery (completeQueryTransaction) in slot
// 0 and in every extra slot that exists, and returns the committed
// transactions it could not complete. An extra slot left empty is removed.
// Anything recovery cannot attribute to a query - an untrustworthy journal
// or leftover, a tampered slot - is still an error for every call: the
// store fails closed rather than guess which query it concerns.
func recoverQueryTransactions(queriesRoot, txnDir string) ([]stuckQueryTxn, error) {
	var stuck []stuckQueryTxn
	for n := 0; n < queryTxnSlotCount; n++ {
		slot := queryTxnSlotDir(txnDir, n)
		if n > 0 {
			exists, err := vetQueryTxnSlotDir(slot)
			if err != nil {
				return nil, err
			}
			if !exists {
				continue
			}
		}
		err := completeQueryTransaction(queriesRoot, slot)
		var incomplete *queryTxnIncompleteError
		switch {
		case err == nil:
			if n > 0 {
				_ = os.Remove(slot) // fails harmlessly if anything is left in it
			}
		case errors.As(err, &incomplete):
			stuck = append(stuck, newStuckQueryTxn(queriesRoot, slot, incomplete))
		default:
			return nil, err
		}
	}
	return stuck, nil
}

// stagingDir returns the slot the next transaction stages in: the first
// one no stuck transaction holds, created on demand. It also requires that
// slot to accept changes before anything is staged, so a refusal never
// leaves staged files it cannot remove.
func (g queryLockGuard) stagingDir() (string, error) {
	for n := 0; n < queryTxnSlotCount; n++ {
		slot := queryTxnSlotDir(g.txnDir, n)
		if g.holdsStuck(slot) {
			continue
		}
		if n > 0 {
			if err := ensureQueryTxnSlotDir(slot); err != nil {
				return "", err
			}
		}
		if err := checkQueryDirAcceptsChanges(slot); err != nil {
			return "", err
		}
		return slot, nil
	}
	errs := make([]error, 0, len(g.stuck))
	for _, st := range g.stuck {
		errs = append(errs, st.err)
	}
	return "", fmt.Errorf("all %d query transaction slots hold a write that cannot be completed yet; fix the entries these errors name first: %w",
		queryTxnSlotCount, errors.Join(errs...))
}

func (g queryLockGuard) holdsStuck(slot string) bool {
	for _, st := range g.stuck {
		if st.slot == slot {
			return true
		}
	}
	return false
}

// readQueryPair is readQueryPairAt for an operation under the lock: it
// first refuses a query a stuck transaction names (refuseStuckQuery), so
// no operation ever reads a half-installed pair.
func (g queryLockGuard) readQueryPair(folderPath, dir, id string) (currentQueryPair, error) {
	if err := g.refuseStuckQuery(folderPath, dir, id); err != nil {
		return currentQueryPair{}, err
	}
	return readQueryPairAt(folderPath, dir, id)
}

// refuseStuckQuery refuses (folderPath, id) in dir when it may address the
// query a stuck transaction names.
func (g queryLockGuard) refuseStuckQuery(folderPath, dir, id string) error {
	for _, st := range g.stuck {
		if st.covers(folderPath, dir, id) {
			return fmt.Errorf("query %q cannot be read or written until the interrupted write of query %q completes: %w",
				path.Join(folderPath, id), path.Join(st.journal.FolderPath, st.journal.ID), st.err)
		}
	}
	return nil
}

// refuseStuckFolder refuses listing folderPath (dir) when it holds a query
// a stuck transaction names: a listing never omits a query or serves one
// half installed.
func (g queryLockGuard) refuseStuckFolder(folderPath, dir string) error {
	for _, st := range g.stuck {
		if st.coversFolder(folderPath, dir) {
			return fmt.Errorf("the queries in folder %q cannot be listed until the interrupted write of query %q completes: %w",
				folderPath, path.Join(st.journal.FolderPath, st.journal.ID), st.err)
		}
	}
	return nil
}

// coversFolder reports whether folderPath (dir) may be the stuck query's
// folder: the same directory, or a spelling with the same queryPathKey.
func (st stuckQueryTxn) coversFolder(folderPath, dir string) bool {
	if st.dirInfo != nil && dir != "" {
		if info, err := os.Lstat(dir); err == nil && os.SameFile(info, st.dirInfo) {
			return true
		}
	}
	return queryPathKey(folderPath) == queryPathKey(st.journal.FolderPath)
}

// covers reports whether (folderPath, id) in dir may address the stuck
// query: in its folder, with the same queryNameKey or sharing a file.
func (st stuckQueryTxn) covers(folderPath, dir, id string) bool {
	if !st.coversFolder(folderPath, dir) {
		return false
	}
	return queryNameKey(id) == queryNameKey(st.journal.ID) || st.sharesAFileWith(dir, id)
}

// sharesAFileWith reports whether a file of id's pair in dir is one of the
// stuck query's files - another spelling of the same name on this file
// system that queryNameKey did not catch.
func (st stuckQueryTxn) sharesAFileWith(dir, id string) bool {
	if st.dir == "" || dir == "" {
		return false
	}
	for _, name := range [...]string{st.journal.JSONFileName, st.journal.BodyFileName, st.journal.PrevBodyFileName} {
		if name == "" {
			continue
		}
		stuckInfo, err := os.Lstat(path.Join(st.dir, name))
		if err != nil {
			continue
		}
		alias := id + strings.TrimPrefix(name, st.journal.ID)
		if info, err := os.Lstat(path.Join(dir, alias)); err == nil && os.SameFile(info, stuckInfo) {
			return true
		}
	}
	return false
}

// queryNameKey is a deliberately coarse key for a query ID or folder
// segment: two names a case- or normalization-insensitive file system may
// treat as one always get the same key. ASCII letters are lower-cased (and
// a character whose case fold is an ASCII letter, such as the Kelvin sign,
// becomes that letter), and every run of other characters - together with
// an ASCII letter a combining mark follows, as in a decomposed "é" -
// becomes a single "*". So "Q" and "q", or a composed and a decomposed
// "café", share a key; so, over-matching, do "café" and "cafè".
func queryNameKey(name string) string {
	runes := []rune(name)
	var b strings.Builder
	pending := false // a run of other characters not yet written as "*"
	for i, r := range runes {
		marked := i+1 < len(runes) && unicode.Is(unicode.Mn, runes[i+1])
		if a, ok := asciiFoldOf(r); ok && !marked {
			if pending {
				b.WriteByte('*')
				pending = false
			}
			b.WriteRune(a)
			continue
		}
		pending = true
	}
	if pending {
		b.WriteByte('*')
	}
	return b.String()
}

// queryPathKey is queryNameKey applied to each segment of a folder path.
func queryPathKey(folderPath string) string {
	if folderPath == "" {
		return ""
	}
	segments := strings.Split(folderPath, "/")
	for i, segment := range segments {
		segments[i] = queryNameKey(segment)
	}
	return strings.Join(segments, "/")
}

// asciiFoldOf returns the lower-case ASCII character r is or case-folds
// to, if any.
func asciiFoldOf(r rune) (rune, bool) {
	if r < utf8.RuneSelf {
		return unicode.ToLower(r), true
	}
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < utf8.RuneSelf {
			return unicode.ToLower(f), true
		}
	}
	return 0, false
}
