package filestore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// validateQueryForWrite validates a query definition before it is staged
// for a revisioned write. It runs exactly the model's own Validate() - the
// same, and only, validation the legacy SaveQuery/CreateQuery/UpdateQuery
// paths apply - which already rejects a target password and an embedded
// user:pass@ URL (QueryDefTarget.Validate, pkg/datatug/query.go) while
// accepting a bare username, already confines Recordsets to schema
// definitions, never result rows (see TestQueryDef_ExcludesResultRows),
// and already restricts Type to its own recognized set (including
// "GraphQL" - see TestValidateQueryForWrite_AcceptsGraphQL/S6). It
// deliberately does not layer datatug.IsKnownQueryType on top: that gate
// disagreed with query.Validate() on both ends (rejecting "GraphQL",
// which Validate() accepts, while nominally allowing "StructuredSQL",
// which Validate() itself already rejects), so PutQuery must accept
// exactly what the legacy write path accepts for the additive,
// non-breaking claim to hold precisely.
func validateQueryForWrite(query datatug.QueryDef) error {
	return query.Validate()
}

// queryJSONBytes returns the exact bytes a query's "<id>.query.json"
// sidecar persists: query with Text always cleared, since the body lives
// only in its own sidecar file (see pkg/datatug/doc.go). It does not
// mutate the caller's query.
func queryJSONBytes(query datatug.QueryDef) ([]byte, error) {
	query.Text = ""
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "\t")
	if err := enc.Encode(query); err != nil {
		return nil, fmt.Errorf("failed to encode query metadata: %w", err)
	}
	return buf.Bytes(), nil
}

// queryBodyFileName returns the body sidecar file name a query of this
// type/id persists, following "<id>.query.<lowercase type>" (see
// pkg/datatug/doc.go's "Query text sidecar file naming" convention, which
// readQueryTextSidecar in store_queries.go already derives the same way).
func queryBodyFileName(id string, queryType datatug.QueryType) string {
	return fmt.Sprintf("%s.%s.%s", id, storage.QueryFileSuffix, strings.ToLower(string(queryType)))
}
