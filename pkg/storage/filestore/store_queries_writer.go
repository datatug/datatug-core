package filestore

import (
	"fmt"
	"os"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// stageAndInstallQueryPair stages query's JSON+body pair and installs it as
// one recoverable transaction under the caller's already-held query store
// lock (g), replacing whatever pair (if any) current describes at dir. It
// is the single write primitive every query write funnels through: the
// revisioned PutQuery (after checking its own precondition) and the legacy
// SaveQuery/CreateQuery/UpdateQuery/saveQueriesTree adapters (which impose
// none, matching their long-documented unconditional upsert behavior)
// alike, so every query pair this store ever persists - old API or new -
// goes through the same recoverable pair transaction, never a direct
// write.
func (s fsQueriesStore) stageAndInstallQueryPair(g queryLockGuard, dir, folderPath string, query datatug.QueryDef, current currentQueryPair) (datatug.QueryRevision, error) {
	jsonBytes, err := queryJSONBytes(query)
	if err != nil {
		return "", err
	}
	bodyFileName := queryBodyFileName(query.ID, query.Type)
	bodyBytes := []byte(query.Text)

	if err := os.MkdirAll(dir, 0o777); err != nil {
		return "", fmt.Errorf("failed to create query folder: %w", err)
	}
	if err := writeStagedFile(g.txnDir, queryTxnStagedJSON, jsonBytes); err != nil {
		return "", err
	}
	if err := writeStagedFile(g.txnDir, queryTxnStagedBody, bodyBytes); err != nil {
		_ = os.Remove(path.Join(g.txnDir, queryTxnStagedJSON))
		return "", err
	}
	fsyncDirBestEffort(g.txnDir)

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
	if err := writeJournal(g.txnDir, j); err != nil {
		return "", err
	}
	if err := completeQueryTransaction(s.dirPath, g.txnDir); err != nil {
		return "", err
	}
	return computeQueryRevision(jsonBytes, bodyFileName, bodyBytes), nil
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
	if err := writeJournal(g.txnDir, j); err != nil {
		return err
	}
	return completeQueryTransaction(s.dirPath, g.txnDir)
}
