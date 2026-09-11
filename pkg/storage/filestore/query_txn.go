// Package filestore's query pair transaction protocol (this file) persists
// a query's JSON metadata and its body sidecar as one recoverable logical
// transaction: stage both new files under the reserved ".dt-query-txn"
// directory with exclusive creation, durably record a journal describing
// the target state (queryTxnJournal), then install (or remove, for a
// delete) each file idempotently via ensureInstalled, which verifies
// staged/already-installed content against the journal's recorded hash
// before trusting it. A fresh withQueryLock call always recovers any
// journal left by an earlier interrupted attempt before doing anything
// else, replaying exactly the same idempotent install path recovery and a
// normal write share.
//
// Deviation from the plan: the plan's Approach text calls for the
// transaction to "back up the old pair" before installing the new one.
// This implementation has no backup-file step anywhere. It relies instead
// on three properties that together give the same guarantee the plan's
// backup step was meant to provide - "never observe a torn pair, never
// lose data to an interruption" - without ever needing a copy of the
// previous content on disk:
//
//  1. The new content is durably staged (writeStagedFile fsyncs each
//     staged file) and journaled (writeJournal fsyncs the journal and its
//     directory) before anything at the final location is touched, so an
//     interruption before the journal exists leaves the previous complete
//     revision completely untouched - there is nothing to back up yet
//     because nothing has changed yet.
//  2. Once the journal exists, the transaction is committed forward, never
//     rolled back (see completeQueryTransaction's doc comment) - so the
//     "old pair" a backup would protect is never something this protocol
//     needs to restore. What could still be lost is the *new* pair's
//     install being interrupted partway, which a backup of the *old* pair
//     would not have helped with anyway.
//  3. ensureInstalled makes that partial-install case safe without a
//     backup: recovery re-derives the exact same staged content (still
//     present under txnDir - staged files are only ever removed by
//     cleanupTxnArtifacts, the transaction's very last step) and its
//     recorded hash from the journal, and only ever installs content that
//     hashes to exactly what was staged. A crash between installing the
//     body and the JSON (or during either rename) leaves the directory in
//     a state recovery always finishes identically, deterministically,
//     to the journal's target state - old or new, never mixed, and never
//     dependent on a backup file having survived the same crash.
//
// A physical backup file would in fact be a strictly weaker guarantee than
// this: it would need its own fsync to be trustworthy after a crash, and
// restoring it correctly still depends on knowing whether the install it
// is protecting against completed - exactly the question the journal's
// hash already answers without one. See query_lock.go's withQueryLock and
// the recovery tests (query_txn_recovery_test.go) for the crash-phase
// analysis this reasoning is verified against.
package filestore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// queryTxnJournal is the durable record a query pair transaction writes
// once its new content is staged and verified, and reads back to recover
// an interruption. It records only a validated query identity, the
// operation and the exact hashes of the content involved - never a raw
// file path from anywhere other than re-deriving it from FolderPath/ID
// through the same validation every ordinary request goes through
// (validateQueryFolderPath/validateQueryID), so a malformed or tampered
// journal can never address a path outside the validated query location.
//
// Its presence is the transaction's commit point: once written and
// durable, completeQueryTransaction always finishes by installing
// (Operation "put") or removing (Operation "delete") the recorded files -
// it never rolls back to the previous content. Before the journal exists,
// nothing outside the private staging area has been touched, so an
// aborted request before that point leaves the previous complete revision
// exactly as it was.
type queryTxnJournal struct {
	FolderPath string `json:"folderPath"`
	ID         string `json:"id"`
	Operation  string `json:"operation"` // "put" | "delete"

	// JSONFileName/BodyFileName name the files this transaction's target
	// state has: for "put", the new pair to install (staged under
	// "staged.json"/"staged.body"); for "delete", the pair to remove.
	JSONFileName string `json:"jsonFileName"`
	JSONHash     string `json:"jsonHash,omitempty"`
	BodyFileName string `json:"bodyFileName,omitempty"`
	BodyHash     string `json:"bodyHash,omitempty"`

	// HadPrevious/Prev* record what "put" is replacing, so a type change
	// (a different body file name) can remove the stale sidecar as part of
	// the same transaction.
	HadPrevious      bool   `json:"hadPrevious,omitempty"`
	PrevBodyFileName string `json:"prevBodyFileName,omitempty"`
}

const (
	queryTxnOpPut    = "put"
	queryTxnOpDelete = "delete"

	queryTxnJournalFile  = "journal.json"
	queryTxnStagedJSON   = "staged.json"
	queryTxnStagedBody   = "staged.body"
	queryTxnMaxFilePerm  = 0o600
	queryTxnFilePermMode = 0o600
)

// hashBytes returns the hex SHA-256 digest of b, used to let recovery
// verify staged/installed content against what the journal recorded
// without needing a physical backup copy of the previous pair: the
// previous content's hash alone is enough to recognize "not yet
// installed" versus "already installed" at each step.
func hashBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// readFileIfExists reads path, returning (nil, false, nil) when it does
// not exist instead of an error.
func readFileIfExists(filePath string) (data []byte, exists bool, err error) {
	data, err = os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return data, true, nil
}

// removeIfExists removes path, treating "already gone" as success.
func removeIfExists(filePath string) error {
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// lstatTrustedArtifact checks a recovery artifact (the journal, a staged
// file) before it is trusted: it must not be a symlink, and - where the
// platform's permission bits are meaningful - must be no more permissive
// than maxPerm. Windows does not expose POSIX permission bits through
// os.FileMode (Go reports a synthetic value there), so that half of the
// check is skipped there. Returns exists=false, err=nil when the path
// simply does not exist.
func lstatTrustedArtifact(filePath string, maxPerm os.FileMode) (exists bool, err error) {
	info, err := os.Lstat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return true, fmt.Errorf("%s is a symlink; refusing to use it", filePath)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&^maxPerm != 0 {
		return true, fmt.Errorf("%s has overly broad permissions %v; refusing to use it", filePath, info.Mode().Perm())
	}
	return true, nil
}

// writeStagedFile creates name under txnDir with the private staging
// permissions (0600), exclusively (O_EXCL - recovery already guarantees
// no leftover staged file exists by the time a fresh transaction stages
// one), writes data and flushes it to durable storage before returning.
func writeStagedFile(txnDir, name string, data []byte) error {
	filePath := path.Join(txnDir, name)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, queryTxnFilePermMode)
	if err != nil {
		return fmt.Errorf("failed to create staged file %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write staged file %s: %w", name, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to flush staged file %s: %w", name, err)
	}
	return nil
}

// writeJournal durably records j as the transaction's commit point: once
// this returns successfully, the transaction MUST be completed forward by
// completeQueryTransaction (immediately, or by a later recovery) - it can
// no longer be abandoned.
func writeJournal(txnDir string, j queryTxnJournal) error {
	b, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to encode query transaction journal: %w", err)
	}
	filePath := path.Join(txnDir, queryTxnJournalFile)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, queryTxnFilePermMode)
	if err != nil {
		return fmt.Errorf("failed to create query transaction journal: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(b); err != nil {
		return fmt.Errorf("failed to write query transaction journal: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to flush query transaction journal: %w", err)
	}
	fsyncDirBestEffort(txnDir)
	return nil
}

// fsyncDirBestEffort flushes a directory's entries (so a rename/create
// inside it survives a crash immediately after) where the platform
// supports it. Directory fsync is not uniformly supported by Go's stdlib
// across every OS datatug-core targets (notably Windows), and the
// transaction protocol's real safety net is the journal-plus-hash
// verification recovery below, not perfect directory-entry durability -
// so a failure here is logged (via the standard log package, this
// package's existing convention - see utils.go), never fatal and never
// returned to the caller.
func fsyncDirBestEffort(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		log.Printf("failed to open directory %s to flush it (best-effort, non-fatal): %v", dir, err)
		return
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		log.Printf("failed to flush directory %s (best-effort, non-fatal): %v", dir, err)
	}
}

// cleanupTxnArtifacts removes every file a query transaction may have left
// under txnDir. Staged files are removed best-effort (their absence is
// never itself a problem); the journal's removal is the one that must
// succeed, since its presence is what says a transaction still needs
// completing.
func cleanupTxnArtifacts(txnDir string) error {
	_ = os.Remove(path.Join(txnDir, queryTxnStagedJSON))
	_ = os.Remove(path.Join(txnDir, queryTxnStagedBody))
	if err := os.Remove(path.Join(txnDir, queryTxnJournalFile)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove query transaction journal: %w", err)
	}
	return nil
}

// ensureInstalled makes dir/finalName hold exactly the content hashed as
// expectedHash, idempotently: a no-op if it already does (the common case
// when this runs as part of the same call that just staged it, and the
// case recovery finds when an earlier attempt finished this step before
// being interrupted), otherwise a rename from txnDir/stagedName - which
// must itself match expectedHash, or the staged content is refused rather
// than trusted blindly.
func ensureInstalled(txnDir, stagedName, dir, finalName, expectedHash string) error {
	finalPath := path.Join(dir, finalName)
	if b, exists, err := readFileIfExists(finalPath); err != nil {
		return err
	} else if exists && hashBytes(b) == expectedHash {
		return nil
	}

	stagedPath := path.Join(txnDir, stagedName)
	if exists, err := lstatTrustedArtifact(stagedPath, queryTxnMaxFilePerm); err != nil {
		return err
	} else if !exists {
		return fmt.Errorf("staged file %s is missing and %s does not already match the transaction's recorded content", stagedName, finalName)
	}
	stagedBytes, err := os.ReadFile(stagedPath)
	if err != nil {
		return fmt.Errorf("failed to read staged file %s: %w", stagedName, err)
	}
	if hashBytes(stagedBytes) != expectedHash {
		return fmt.Errorf("staged file %s does not match the transaction's recorded hash; refusing to install it", stagedName)
	}
	if err := os.Rename(stagedPath, finalPath); err != nil {
		return fmt.Errorf("failed to install %s: %w", finalName, err)
	}
	return nil
}

// readJournal reads txnDir's journal, if any. A present-but-malformed
// journal (bad permissions, a symlink, corrupt JSON, an unrecognized
// operation, a location that no longer validates) is a hard error - fail
// closed rather than silently ignore or, worse, act on an untrusted
// journal.
func readJournal(txnDir string) (j queryTxnJournal, present bool, err error) {
	journalPath := path.Join(txnDir, queryTxnJournalFile)
	exists, err := lstatTrustedArtifact(journalPath, queryTxnMaxFilePerm)
	if err != nil {
		return queryTxnJournal{}, false, err
	}
	if !exists {
		return queryTxnJournal{}, false, nil
	}
	b, err := os.ReadFile(journalPath)
	if err != nil {
		return queryTxnJournal{}, false, fmt.Errorf("failed to read query transaction journal: %w", err)
	}
	if err := json.Unmarshal(b, &j); err != nil {
		return queryTxnJournal{}, false, fmt.Errorf("query transaction journal is corrupt: %w", err)
	}
	if j.Operation != queryTxnOpPut && j.Operation != queryTxnOpDelete {
		return queryTxnJournal{}, false, fmt.Errorf("query transaction journal has an unrecognized operation %q", j.Operation)
	}
	if err := validateQueryFolderPath(j.FolderPath); err != nil {
		return queryTxnJournal{}, false, fmt.Errorf("query transaction journal has an invalid folder path: %w", err)
	}
	if err := validateQueryID(j.ID); err != nil {
		return queryTxnJournal{}, false, fmt.Errorf("query transaction journal has an invalid id: %w", err)
	}
	return j, true, nil
}

// completeQueryTransaction finishes whatever transaction txnDir's journal
// currently describes, idempotently: it is safe to call on a journal that
// is brand new (nothing installed yet), one an earlier call already
// partially installed, or no journal at all (a no-op). It always finishes
// by leaving the query's directory holding exactly the journal's target
// state and removing every transaction artifact, never by rolling back -
// so calling it is how both "finish the write I just staged" and "recover
// whatever an earlier process left behind" are implemented, with one
// tested code path for both.
//
// It takes no context: once a journal is durable, the transaction it
// describes is committed and must run to completion regardless of
// cancellation - that guarantee holds structurally here, not by checking
// and ignoring ctx.Err().
//
// queriesRoot is the project's canonical "queries/" directory; the target
// directory is re-derived from it plus the journal's own
// (re-)validated FolderPath, never trusted as a path the journal supplies
// directly.
func completeQueryTransaction(queriesRoot, txnDir string) error {
	j, present, err := readJournal(txnDir)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}

	dir := queriesRoot
	if j.FolderPath != "" {
		for _, seg := range strings.Split(j.FolderPath, "/") {
			dir = path.Join(dir, seg)
		}
	}
	if err := os.MkdirAll(dir, 0o777); err != nil {
		return fmt.Errorf("failed to create query folder: %w", err)
	}

	switch j.Operation {
	case queryTxnOpPut:
		if j.HadPrevious && j.PrevBodyFileName != "" && j.PrevBodyFileName != j.BodyFileName {
			if err := removeIfExists(path.Join(dir, j.PrevBodyFileName)); err != nil {
				return fmt.Errorf("failed to remove stale body sidecar: %w", err)
			}
		}
		if j.BodyFileName != "" {
			if err := ensureInstalled(txnDir, queryTxnStagedBody, dir, j.BodyFileName, j.BodyHash); err != nil {
				return err
			}
			// Flush the body's rename before installing the JSON metadata:
			// each install is its own durability step, not only the pair as
			// a whole - see the plan's "flush the files, journal and
			// containing directories" applied at each step. Recovery's
			// idempotent hash-reverification (ensureInstalled) makes every
			// DataTug-reader-observable guarantee hold regardless of
			// whether this barrier runs, but it still narrows the window in
			// which a real power loss could otherwise leave the directory
			// entry for this rename non-durable while a later one already
			// is.
			fsyncDirBestEffort(dir)
		}
		if err := ensureInstalled(txnDir, queryTxnStagedJSON, dir, j.JSONFileName, j.JSONHash); err != nil {
			return err
		}
	case queryTxnOpDelete:
		if j.BodyFileName != "" {
			if err := removeIfExists(path.Join(dir, j.BodyFileName)); err != nil {
				return fmt.Errorf("failed to remove query body sidecar: %w", err)
			}
		}
		if err := removeIfExists(path.Join(dir, j.JSONFileName)); err != nil {
			return fmt.Errorf("failed to remove query metadata: %w", err)
		}
	}

	fsyncDirBestEffort(dir)
	return cleanupTxnArtifacts(txnDir)
}

// currentQueryPair is the exact pair a revisioned operation reads before
// deciding anything: what readCurrentQueryPair found on disk for one
// query id in one folder, right under the query store lock.
type currentQueryPair struct {
	// exists is true once the JSON metadata file is present, regardless of
	// whether its body sidecar (if any is expected) is.
	exists bool
	// complete is true only when the pair is a strict, revisioned-store
	// complete record: JSON present, and a body sidecar present whenever
	// the JSON's own Type says one should exist. A query written by the
	// legacy SaveQuery/CreateQuery path with empty Text has no body
	// sidecar and so is never "complete" here - see LoadQueryRevision and
	// TestPutQuery_TreatsLegacyEmptyBodyRecordAsIncomplete for what that
	// means for revisioned reads/writes meeting legacy-written data.
	//
	// revision is set whenever exists is true, complete or not: an
	// incomplete pair still has a revision, computed the same framed way
	// but over an empty body extension/bytes (the "no body sidecar exists"
	// sentinel - see computeQueryRevision), so it can never collide with a
	// complete pair's revision even when that complete pair's own body
	// happens to be empty content (a real, present, empty-content sidecar
	// carries its non-empty type-derived extension in the hash; a missing
	// one never does).
	// This is what lets a caller that only ever saw the incomplete state
	// through LoadQueryRevision still supply a meaningful, verifiable
	// PutQuery IfMatch/DeleteQueryRevision expected revision for it - see
	// datatug.IncompleteQueryRecordError.Revision.
	complete     bool
	jsonBytes    []byte
	bodyFileName string
	bodyBytes    []byte
	revision     datatug.QueryRevision
}

// readCurrentQueryPair reads the exact current pair for id under dir. It
// determines the expected body sidecar name from the JSON metadata's own
// recorded Type (queryBodyFileName), not from any caller-supplied
// expectation, since recovery and revisioned reads must agree with
// whatever a previous writer - revisioned or legacy - actually persisted.
// The revision it computes depends only on the bytes it reads, never on
// how id is spelled: a file system that resolves differently-spelled ids to
// the same files yields one revision for them all (see
// computeQueryRevision).
func readCurrentQueryPair(dir, id string) (currentQueryPair, error) {
	jsonPath := path.Join(dir, storage.JsonFileName(id, storage.QueryFileSuffix))
	jsonBytes, exists, err := readFileIfExists(jsonPath)
	if err != nil {
		return currentQueryPair{}, fmt.Errorf("failed to read query metadata: %w", err)
	}
	if !exists {
		return currentQueryPair{}, nil
	}
	cur := currentQueryPair{exists: true, jsonBytes: jsonBytes}

	var meta struct {
		Type datatug.QueryType `json:"type"`
	}
	if err := json.Unmarshal(jsonBytes, &meta); err != nil {
		return cur, fmt.Errorf("failed to parse existing query metadata for %s: %w", id, err)
	}
	if meta.Type == "" {
		cur.revision = computeQueryRevision(jsonBytes, "", nil)
		return cur, nil
	}
	bodyFileName := queryBodyFileName(id, meta.Type)
	bodyBytes, bodyExists, err := readFileIfExists(path.Join(dir, bodyFileName))
	if err != nil {
		return cur, fmt.Errorf("failed to read query body: %w", err)
	}
	if !bodyExists {
		cur.revision = computeQueryRevision(jsonBytes, "", nil)
		return cur, nil
	}
	cur.complete = true
	cur.bodyFileName = bodyFileName
	cur.bodyBytes = bodyBytes
	cur.revision = computeQueryRevision(jsonBytes, queryBodyFileExt(meta.Type), bodyBytes)
	return cur, nil
}
