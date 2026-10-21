package dto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/datatug/datatug-core/pkg/datatug"
	"sort"
)

// EnvironmentStateDigest binds canonical safe membership, raw saved order and resolved order.
func EnvironmentStateDigest(state EnvironmentState) (string, error) {
	environments := append([]EnvironmentBrief{}, state.Environments...)
	var compareErr error
	sort.Slice(environments, func(i, j int) bool {
		d, e := datatug.CompareEnvironmentIDs(environments[i].ID, environments[j].ID)
		if e != nil {
			compareErr = e
		}
		return d < 0
	})
	if compareErr != nil {
		return "", compareErr
	}
	body, e := json.Marshal([]any{environments, state.SavedOrder, state.Order})
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
