package filestore

import (
	"fmt"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// stageAndInstallQueryPair stages query's JSON+body pair and installs it as
// one recoverable transaction under the caller's already-held query store
// lock (g), replacing whatever pair (if any) current describes at
// folderPath. It
// is the single write primitive every query write funnels through: the
// revisioned PutQuery (after checking its own precondition) and the legacy
// SaveQuery/CreateQuery/UpdateQuery/saveQueriesTree adapters (which impose
// none, matching their long-documented unconditional upsert behavior)
// alike, so every query pair this store ever persists - old API or new -
// goes through the same recoverable pair transaction, never a direct
// write.
//
// It resolves the pair's folder itself, under the lock, with walkQueryDir -
// creating any missing segment without following a symlink - so the
// directory a pair is installed into is always one proven under the lock,
// never one a caller resolved before acquiring it (review N3).
func (s fsQueriesStore) stageAndInstallQueryPair(g queryLockGuard, folderPath string, query datatug.QueryDef, current currentQueryPair) (datatug.QueryRevision, error) {
	jsonBytes, err := queryJSONBytes(query)
	if err != nil {
		return "", err
	}
	bodyFileName, err := queryBodyFileName(query.ID, query.Type)
	if err != nil {
		return "", err
	}
	bodyBytes := []byte(query.Text)
	// A file over the read cap would commit a transaction that recovery and
	// every read then refuse, so it is refused here, before anything.
	if len(jsonBytes) > maxQueryFileSize || len(bodyBytes) > maxQueryFileSize {
		return "", fmt.Errorf("query %q is over the %d-byte limit for a query file; refusing to write it", query.ID, maxQueryFileSize)
	}

	if _, err := walkQueryDir(s.dirPath, folderPath, query.ID, true); err != nil {
		return "", err
	}
	txnDir, err := g.stagingDir()
	if err != nil {
		return "", err
	}
	if err := writeStagedFile(txnDir, queryTxnStagedJSON, jsonBytes); err != nil {
		return "", err
	}
	if err := writeStagedFile(txnDir, queryTxnStagedBody, bodyBytes); err != nil {
		discardStagedFiles(txnDir, queryTxnStagedJSON)
		return "", err
	}
	fsyncDirBestEffort(txnDir)

	j := queryTxnJournal{
		FolderPath:   folderPath,
		ID:           query.ID,
		Operation:    queryTxnOpPut,
		JSONFileName: storage.JsonFileName(query.ID, storage.QueryFileSuffix),
		JSONHash:     hashBytes(jsonBytes),
		BodyFileName: bodyFileName,
		BodyHash:     hashBytes(bodyBytes),
	}
	if current.exists {
		j.HadPrevious = true
		j.PrevBodyFileName = current.bodyFileName
	}
	// commitQueryTransaction first runs every check recovery would run on
	// this transaction - the target names included, which may hold a
	// symlink, directory, FIFO, over-cap or unreadable file this write
	// cannot replace - so a refusal is an ordinary error here, never a
	// committed journal that wedges every later call.
	if err := commitQueryTransaction(s.dirPath, txnDir, j); err != nil {
		// Not committed (it fails only before writeJournal's rename), so
		// the staged files are this attempt's own leftovers.
		discardStagedFiles(txnDir, queryTxnStagedJSON, queryTxnStagedBody)
		return "", err
	}
	if err := finishQueryTransaction(s.dirPath, txnDir, j); err != nil {
		return "", err
	}
	return computeQueryRevision(jsonBytes, queryBodyFileExt(query.Type), bodyBytes), nil
}

// deleteQueryPairIfExists removes id's pair (its JSON metadata, and its
// body sidecar if current found one) as one recoverable transaction under
// the caller's already-held query store lock. It is a no-op, not an error,
// when current says no record exists: legacy DeleteQuery's long-documented
// behavior. DeleteQueryRevision enforces its own revision precondition
// separately and does not use this.
func (s fsQueriesStore) deleteQueryPairIfExists(g queryLockGuard, folderPath, id string, current currentQueryPair) error {
	if !current.exists {
		return nil
	}
	j := queryTxnJournal{
		FolderPath:   folderPath,
		ID:           id,
		Operation:    queryTxnOpDelete,
		JSONFileName: storage.JsonFileName(id, storage.QueryFileSuffix),
		BodyFileName: current.bodyFileName,
	}
	txnDir, err := g.stagingDir()
	if err != nil {
		return err
	}
	if err := commitQueryTransaction(s.dirPath, txnDir, j); err != nil {
		return err
	}
	return finishQueryTransaction(s.dirPath, txnDir, j)
}
