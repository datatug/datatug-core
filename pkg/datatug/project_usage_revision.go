package datatug

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

var marshalProjectUsageRevision = json.Marshal

// ProjectUsageRevision binds content revisions, explicit references and archive
// state in stable order. It excludes actor identity: access is rechecked by the
// provider at both review and commit, independently of this content hash.
func ProjectUsageRevision(objects []ProjectObjectUsage) (string, error) {
	if _, err := validateProjectUsageSnapshot(ProjectUsageSnapshot{Revision: "revision-input", Objects: objects}); err != nil {
		return "", err
	}
	ordered := append([]ProjectObjectUsage(nil), objects...)
	sort.Slice(ordered, func(i, j int) bool {
		a, b := ordered[i].Ref, ordered[j].Ref
		return a.Kind < b.Kind || (a.Kind == b.Kind && a.ID < b.ID)
	})
	for i := range ordered {
		ordered[i].References = append([]ProjectObjectRef(nil), ordered[i].References...)
		sort.Slice(ordered[i].References, func(a, b int) bool {
			x, y := ordered[i].References[a], ordered[i].References[b]
			return x.Kind < y.Kind || (x.Kind == y.Kind && x.ID < y.ID)
		})
	}
	body, err := marshalProjectUsageRevision(ordered)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}
