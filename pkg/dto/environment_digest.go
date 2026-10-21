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
	// Every field in this payload is a string, slice, or pointer to those
	// values. Preserve the codec error in the API without an unreachable
	// alternate path around the digest calculation.
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), e
}
