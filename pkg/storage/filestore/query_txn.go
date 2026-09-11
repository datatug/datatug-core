// Package filestore's query pair transaction protocol (this file) persists
// a query's JSON metadata and its body sidecar as one recoverable logical
// transaction, under the query store lock (query_lock.go):
//
//  1. Stage. Both new files are written under the reserved ".dt-query-txn"
//     directory as "staged.json" and "staged.body", each created
//     exclusively and fsynced; then the directory is fsynced.
//  2. Commit. The journal (queryTxnJournal) is written to "journal.tmp",
//     fsynced, renamed to "journal.json", and the directory is fsynced.
//     That rename is the commit point: "journal.json" either does not
//     exist or is complete, so no crash can leave a partly written journal
//     under that name. If the journal cannot be written, the attempt
//     removes its own staged files before returning the error.
//  3. Install. completeQueryTransaction verifies both staged files against
//     the journal's recorded hashes, then ensureInstalled renames each into
//     place - the body, then the JSON metadata, each followed by a
//     directory fsync. A delete removes the pair instead.
//  4. Clean up. Whatever staged file is left, then the journal, are
//     removed, and the directory is fsynced.
//
// Every withQueryLock call recovers before doing anything else
// (completeQueryTransaction). With a "journal.json" present, recovery runs
// steps 3 and 4 again, idempotently - the same code a normal write runs.
// With none, nothing was committed: recovery removes any "journal.tmp" and
// staged files as uncommitted leftovers, after checking that each is the
// user's own regular file (checkTxnArtifact; anything else fails closed
// and nothing is removed). A crash during steps 1-2 therefore never blocks
// a later read or write.
//
// Deviation from the plan: the plan's Approach text calls for the
// transaction to "back up the old pair" before installing the new one.
// This implementation has no backup file. It relies instead on three
// properties that together give the guarantee the backup step was meant to
// provide - a DataTug reader never observes a torn pair, and an
// interruption never loses committed data:
//
//  1. Nothing at the final location changes before the commit point, so an
//     interruption before it leaves the previous complete revision
//     untouched: there is nothing to back up yet.
//  2. After the commit point the transaction is completed forward, never
//     rolled back, so the old pair a backup would protect is never needed.
//     What an interruption can leave is the new pair's install half done,
//     which a backup of the old pair would not help with.
//  3. Recovery finishes a half-done install from the staged files and the
//     journal's hashes alone. A staged file leaves the transaction
//     directory in exactly two ways while a journal exists: ensureInstalled
//     renames it into place (that is its install), or step 4 removes it
//     after both installs are done. (The leftover sweep above runs only
//     when no journal exists.) So while a journal exists, each staged file
//     is either still present - and installed only if it hashes to the
//     recorded content - or already installed, in which case the final
//     file must hash to that content. If a final file was edited outside
//     DataTug between an interrupted install and recovery, it matches
//     neither: recovery then fails closed with an error naming the file,
//     rather than guessing, and the user restores the file or removes the
//     journal. Nothing here depends on a backup surviving the same crash.
//
// A physical backup file would be a weaker guarantee: it would need its own
// fsync to be trustworthy after a crash, and restoring it correctly would
// still depend on knowing whether the install it protects against
// completed - the question the journal's hashes already answer.
//
// Directory fsync is best effort (fsyncDirBestEffort) because it is not
// available on every platform, notably Windows. There, a power loss (not a
// process crash) just after a rename can in principle lose that rename;
// hash verification still refuses to install anything but the recorded
// content. See query_lock.go's withQueryLock and the recovery, crash and
// kill-loop tests (query_txn_recovery_test.go, query_txn_crash_test.go,
// query_txn_killloop_unix_test.go) for the crash-phase analysis this is
// verified against.
//
// Operational notes:
//
//   - Lock fairness. The query store lock is an advisory file lock that a
//     waiter retries every queryLockRetryDelay (20ms); it is not a queue.
//     Under sustained contention one process can re-acquire it again and
//     again while another keeps waiting, bounded only by the waiter's
//     context deadline. Correctness does not depend on fairness: writes are
//     still serialized, and IfMatch is still checked under the lock.
//   - Other local users. The transaction directory is created 0700 and must
//     be owned by the user running DataTug (vetExistingQueryTxnDir). Once
//     one local user has written to a project's queries, other local users
//     cannot read or write them through DataTug; they get an "owned by
//     another user" error. Each account needs its own checkout. A project
//     nobody has written to yet stays readable without write access
//     (withQueryReadLock).
//   - Stricter names for legacy writes. SaveQuery, CreateQuery,
//     UpdateQuery, DeleteQuery, CreateQueryFolder and project saves now
//     validate IDs and folder names exactly like the revisioned API
//     (validateQuerySegmentReason). An ID or folder that contains a
//     Windows-illegal character such as ":" or "?" or a control or
//     bidirectional-text character, starts with ".", ends with "." or a
//     space, or names a Windows device is now refused, where it used to be
//     written as given. Reading such a legacy record still works, because
//     LoadQuery and LoadQueries apply only the containment rules
//     (validateQueryReadSegmentReason).
//   - Concurrent tampering. Containment is proven against what is on disk
//     when each step runs: no symlinked segment, regular target files, and
//     names derived from the ID. Locations are resolved again after the
//     lock is acquired. That does not defend against another process with
//     write access to the project tree renaming a directory between those
//     checks and the rename that installs a file. Such a process could
//     modify the project's files directly anyway; the threat this store
//     closes is untrusted content that arrives in a clone or an archive.
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

	queryTxnJournalFile = "journal.json"
	// queryTxnJournalTmpFile is where writeJournal writes the journal
	// before renaming it to queryTxnJournalFile - the commit point.
	queryTxnJournalTmpFile = "journal.tmp"
	queryTxnStagedJSON     = "staged.json"
	queryTxnStagedBody     = "staged.body"
	queryTxnMaxFilePerm    = 0o600
	queryTxnFilePermMode   = 0o600
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
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
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

// removeIfExists removes path, treating "already gone" as success.
func removeIfExists(filePath string) error {
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// writeStagedFile creates name under txnDir with the private staging
// permissions (0600), exclusively (O_EXCL - under the lock, recovery has
// already removed any uncommitted leftover by the time a transaction
// stages, so an existing file here means an invariant broke, and it is
// never clobbered), writes data and flushes it to durable storage.
func writeStagedFile(txnDir, name string, data []byte) error {
	return writeTxnFileExclusive(txnDir, name, data)
}

// writeTxnFileExclusive creates txnDir/name exclusively at 0600, writes
// data, fsyncs and closes it. If anything fails after the file was
// created, it removes that file - its own partial write - before returning
// the error; it never removes a file it did not create.
func writeTxnFileExclusive(txnDir, name string, data []byte) (err error) {
	filePath := path.Join(txnDir, name)
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, queryTxnFilePermMode)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", name, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close %s: %w", name, closeErr)
		}
		if err != nil {
			_ = os.Remove(filePath)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write %s: %w", name, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("failed to flush %s: %w", name, err)
	}
	return nil
}

// writeJournal durably and atomically records j as the transaction's commit
// point: it writes journal.tmp, fsyncs it, renames it to journal.json and
// fsyncs the directory. The rename is the commit point, so journal.json is
// never observed partly written. Once this returns successfully, the
// transaction MUST be completed forward by completeQueryTransaction
// (immediately, or by a later recovery); it can no longer be abandoned.
// When it returns an error, journal.json was not created (every failure
// happens before the rename), so the caller's staged files are its own
// uncommitted leftovers to remove. j must pass validate - the same check
// recovery applies - so a writer can never commit a journal recovery would
// refuse; an existing journal.json is never replaced.
func writeJournal(txnDir string, j queryTxnJournal) error {
	if err := j.validate(); err != nil {
		return fmt.Errorf("refusing to commit an invalid query transaction journal: %w", err)
	}
	b, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("failed to encode query transaction journal: %w", err)
	}
	if len(b) > maxQueryTxnJournalSize {
		return fmt.Errorf("query transaction journal would be %d bytes, over the %d-byte limit", len(b), maxQueryTxnJournalSize)
	}
	journalPath := path.Join(txnDir, queryTxnJournalFile)
	if _, err := os.Lstat(journalPath); err == nil {
		return fmt.Errorf("a committed query transaction journal already exists at %s; refusing to replace it", journalPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check for an existing query transaction journal: %w", err)
	}
	if err := writeTxnFileExclusive(txnDir, queryTxnJournalTmpFile, b); err != nil {
		return fmt.Errorf("failed to write query transaction journal: %w", err)
	}
	tmpPath := path.Join(txnDir, queryTxnJournalTmpFile)
	if err := os.Rename(tmpPath, journalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to commit query transaction journal: %w", err)
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

// cleanupTxnArtifacts removes every file a completed query transaction may
// have left under txnDir, then fsyncs the directory so the journal's
// removal is durable. Staged files are removed best-effort (their absence
// is never itself a problem); the journal's removal is the one that must
// succeed, since its presence is what says a transaction still needs
// completing.
func cleanupTxnArtifacts(txnDir string) error {
	_ = os.Remove(path.Join(txnDir, queryTxnStagedJSON))
	_ = os.Remove(path.Join(txnDir, queryTxnStagedBody))
	if err := os.Remove(path.Join(txnDir, queryTxnJournalFile)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove query transaction journal: %w", err)
	}
	fsyncDirBestEffort(txnDir)
	return nil
}

// discardStagedFiles removes this attempt's own staged files after a
// failure before the commit point. Best effort: anything it cannot remove
// is an uncommitted leftover the next recovery sweeps.
func discardStagedFiles(txnDir string, names ...string) {
	for _, name := range names {
		_ = os.Remove(path.Join(txnDir, name))
	}
}

// sweepUncommittedTxnArtifacts removes what an attempt interrupted before
// its commit point leaves behind - journal.tmp and staged files, with no
// journal.json - so a crash during staging or while the journal is being
// written never blocks later writes (staging creates its files
// exclusively). Every leftover is vetted first (checkTxnArtifact: the
// user's own regular file, 0600 or tighter); if any fails, it returns an
// error and removes nothing. It is called only when readJournal found no
// journal.json.
func sweepUncommittedTxnArtifacts(txnDir string) error {
	var present []string
	for _, name := range [...]string{queryTxnJournalTmpFile, queryTxnStagedJSON, queryTxnStagedBody} {
		exists, err := checkTxnArtifact(path.Join(txnDir, name), queryTxnMaxFilePerm)
		if err != nil {
			return fmt.Errorf("refusing to clean up an uncommitted query transaction: %w", err)
		}
		if exists {
			present = append(present, name)
		}
	}
	if len(present) == 0 {
		return nil
	}
	for _, name := range present {
		if err := removeIfExists(path.Join(txnDir, name)); err != nil {
			return fmt.Errorf("failed to remove uncommitted query transaction leftover %s: %w", name, err)
		}
	}
	fsyncDirBestEffort(txnDir)
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
	// The final file must itself be a regular file (readRegularFileCapped):
	// a symlink, FIFO or device planted at the target name refuses the
	// transaction instead of being read through or silently replaced.
	if b, exists, err := readRegularFileCapped(path.Join(dir, finalName), maxQueryFileSize); err != nil {
		return false, fmt.Errorf("refusing to install %s: %w", finalName, err)
	} else if exists && hashBytes(b) == expectedHash {
		return true, nil
	}
	stagedPath := path.Join(txnDir, stagedName)
	if exists, err := checkTxnArtifact(stagedPath, queryTxnMaxFilePerm); err != nil {
		return false, err
	} else if !exists {
		return false, fmt.Errorf("%s does not hold the content the interrupted query transaction recorded, and its staged copy %s was already installed: "+
			"the file changed outside DataTug before recovery finished; restore it, or remove %s to abandon the transaction",
			path.Join(dir, finalName), stagedName, path.Join(txnDir, queryTxnJournalFile))
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
		return sweepUncommittedTxnArtifacts(txnDir)
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
		removeStale := j.HadPrevious && j.PrevBodyFileName != "" && j.PrevBodyFileName != j.BodyFileName
		if removeStale {
			if err := checkRemovableQueryFile(path.Join(dir, j.PrevBodyFileName)); err != nil {
				return err
			}
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
		// Check both targets before removing either, so a refusal leaves
		// the pair exactly as it was.
		targets := []string{path.Join(dir, j.JSONFileName)}
		if j.BodyFileName != "" {
			targets = append([]string{path.Join(dir, j.BodyFileName)}, targets...)
		}
		for _, target := range targets {
			if err := checkRemovableQueryFile(target); err != nil {
				return err
			}
		}
		for _, target := range targets {
			if err := removeIfExists(target); err != nil {
				return fmt.Errorf("failed to remove %s: %w", target, err)
			}
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
	jsonBytes, exists, err := readRegularFileCapped(jsonPath, maxQueryFileSize)
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
	bodyBytes, bodyExists, err := readRegularFileCapped(path.Join(dir, bodyFileName), maxQueryFileSize)
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
