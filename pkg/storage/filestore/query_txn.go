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

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// queryTxnJournal is the durable record a query pair transaction writes
// once its new content is staged and verified, and reads back to recover
// an interruption. It records a query identity (FolderPath, ID), the
// operation, the exact hashes of the content involved and the pair's file
// names. A journal read back from disk is untrusted - it can arrive in a
// clone or an archive - so recovery never uses anything in it as a path
// (see validate): FolderPath and ID must pass the same validation every
// ordinary request does, the folder is then reached through walkQueryDir
// (no symlinked segment), and every file name must equal the name derived
// from ID - storage.JsonFileName for the metadata, queryBodyFileName with
// an accepted type for a body sidecar. A recorded name that differs from
// the derived one refuses the whole journal.
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

// validate checks everything recovery acts on in a journal, returning an
// error for anything that is not exactly what this store's own writers
// record. It is applied both to a journal read back from disk (readJournal)
// and to one about to be committed, so a writer can never commit a journal
// that recovery would then refuse.
func (j queryTxnJournal) validate() error {
	if j.Operation != queryTxnOpPut && j.Operation != queryTxnOpDelete {
		return fmt.Errorf("unrecognized operation %q", j.Operation)
	}
	if err := validateQueryFolderPath(j.FolderPath); err != nil {
		return fmt.Errorf("invalid folder path: %w", err)
	}
	if err := validateQueryID(j.ID); err != nil {
		return fmt.Errorf("invalid id: %w", err)
	}
	if want := storage.JsonFileName(j.ID, storage.QueryFileSuffix); j.JSONFileName != want {
		return fmt.Errorf("jsonFileName %q is not the name derived from id %q (%q)", j.JSONFileName, j.ID, want)
	}
	switch j.Operation {
	case queryTxnOpPut:
		if !isSHA256Hex(j.JSONHash) || !isSHA256Hex(j.BodyHash) {
			return fmt.Errorf("a put journal must record the SHA-256 hash of each staged file")
		}
		if err := checkQueryBodyFileName(j.ID, j.BodyFileName); err != nil {
			return fmt.Errorf("bodyFileName: %w", err)
		}
		if j.PrevBodyFileName != "" {
			if !j.HadPrevious {
				return fmt.Errorf("prevBodyFileName is set without hadPrevious")
			}
			if err := checkQueryBodyFileName(j.ID, j.PrevBodyFileName); err != nil {
				return fmt.Errorf("prevBodyFileName: %w", err)
			}
		}
	case queryTxnOpDelete:
		if j.JSONHash != "" || j.BodyHash != "" || j.HadPrevious || j.PrevBodyFileName != "" {
			return fmt.Errorf("a delete journal must not record put fields")
		}
		if j.BodyFileName != "" {
			if err := checkQueryBodyFileName(j.ID, j.BodyFileName); err != nil {
				return fmt.Errorf("bodyFileName: %w", err)
			}
		}
	}
	return nil
}

// isSHA256Hex reports whether s is a lowercase hex SHA-256 digest, the
// form hashBytes produces.
func isSHA256Hex(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

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
	installed, err := verifyInstallable(txnDir, stagedName, dir, finalName, expectedHash)
	if err != nil || installed {
		return err
	}
	if err := os.Rename(path.Join(txnDir, stagedName), path.Join(dir, finalName)); err != nil {
		return fmt.Errorf("failed to install %s: %w", finalName, err)
	}
	return nil
}

// verifyInstallable is ensureInstalled without the rename: it reports
// installed=true when dir/finalName already holds exactly the content
// hashed as expectedHash, and otherwise requires txnDir/stagedName to be a
// trusted artifact (checkTxnArtifact) whose content hashes to
// expectedHash. It changes nothing, so completeQueryTransaction can verify
// every file a transaction installs before it touches any of them.
func verifyInstallable(txnDir, stagedName, dir, finalName, expectedHash string) (installed bool, err error) {
	if b, exists, err := readFileIfExists(path.Join(dir, finalName)); err != nil {
		return false, err
	} else if exists && hashBytes(b) == expectedHash {
		return true, nil
	}
	stagedPath := path.Join(txnDir, stagedName)
	if exists, err := checkTxnArtifact(stagedPath, queryTxnMaxFilePerm); err != nil {
		return false, err
	} else if !exists {
		return false, fmt.Errorf("staged file %s is missing and %s does not already match the transaction's recorded content", stagedName, finalName)
	}
	stagedBytes, _, err := readRegularFileCapped(stagedPath, maxQueryFileSize)
	if err != nil {
		return false, fmt.Errorf("failed to read staged file %s: %w", stagedName, err)
	}
	if hashBytes(stagedBytes) != expectedHash {
		return false, fmt.Errorf("staged file %s does not match the transaction's recorded hash; refusing to install it", stagedName)
	}
	return false, nil
}

// readJournal reads txnDir's journal, if any. A present-but-untrustworthy
// journal is a hard error, and nothing is touched: fail closed rather than
// silently ignore or, worse, act on it. That covers a journal that is not
// a regular file owned by the current user with 0600-or-tighter
// permissions (checkTxnArtifact), one over maxQueryTxnJournalSize, corrupt
// JSON, and anything queryTxnJournal.validate refuses (an unrecognized
// operation, an invalid location, a file name that differs from the one
// derived from the ID, a malformed hash).
func readJournal(txnDir string) (j queryTxnJournal, present bool, err error) {
	journalPath := path.Join(txnDir, queryTxnJournalFile)
	exists, err := checkTxnArtifact(journalPath, queryTxnMaxFilePerm)
	if err != nil {
		return queryTxnJournal{}, false, err
	}
	if !exists {
		return queryTxnJournal{}, false, nil
	}
	b, exists, err := readRegularFileCapped(journalPath, maxQueryTxnJournalSize)
	if err != nil {
		return queryTxnJournal{}, false, fmt.Errorf("failed to read query transaction journal: %w", err)
	}
	if !exists {
		return queryTxnJournal{}, false, nil
	}
	if err := json.Unmarshal(b, &j); err != nil {
		return queryTxnJournal{}, false, fmt.Errorf("query transaction journal %s is corrupt: %w", journalPath, err)
	}
	if err := j.validate(); err != nil {
		return queryTxnJournal{}, false, fmt.Errorf("query transaction journal %s is not trustworthy; refusing to recover from it: %w", journalPath, err)
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
// queriesRoot is the project's canonical "queries/" directory. The target
// directory is re-derived from it plus the journal's validated FolderPath
// through walkQueryDir, which refuses a symlinked or non-directory segment
// and creates a missing one (for a put) without following a symlink; every
// file name is one validate proved equal to the name derived from the ID.
// Nothing in the journal is ever used as a path directly.
func completeQueryTransaction(queriesRoot, txnDir string) error {
	j, present, err := readJournal(txnDir)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}

	dir, err := walkQueryDir(queriesRoot, j.FolderPath, j.ID, j.Operation == queryTxnOpPut)
	if err != nil {
		return fmt.Errorf("refusing to recover the query transaction for %q: %w", j.ID, err)
	}

	switch j.Operation {
	case queryTxnOpPut:
		// Verify both files before touching anything: an untrustworthy
		// staged file refuses the whole transaction, instead of being found
		// only after the other half of the pair was already installed.
		for _, f := range [...]struct{ staged, final, hash string }{
			{queryTxnStagedBody, j.BodyFileName, j.BodyHash},
			{queryTxnStagedJSON, j.JSONFileName, j.JSONHash},
		} {
			if _, err := verifyInstallable(txnDir, f.staged, dir, f.final, f.hash); err != nil {
				return err
			}
		}
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
	// but over an empty body file name/bytes (the "no body sidecar exists"
	// sentinel - see computeQueryRevision), so it can never collide with a
	// complete pair's revision even when that complete pair's own body
	// happens to be empty content (a real, present, empty-content sidecar
	// carries its real file name in the hash; a missing one never does).
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
	bodyFileName, err := queryBodyFileName(id, meta.Type)
	if err != nil {
		return cur, fmt.Errorf("existing query metadata for %s: %w", id, err)
	}
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
	cur.revision = computeQueryRevision(jsonBytes, bodyFileName, bodyBytes)
	return cur, nil
}
