package filestore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/strongo/validation"
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
//
// It also refuses a body over maxQueryFileSize here, before the lock, so
// PutQuery rejects it without creating anything; stageAndInstallQueryPair
// enforces the same cap on the encoded pair for every writer.
func validateQueryForWrite(query datatug.QueryDef) error {
	if len(query.Text) > maxQueryFileSize {
		return validation.NewErrBadRecordFieldValue("text", fmt.Sprintf("is %d bytes, over the %d-byte limit for a query body", len(query.Text), maxQueryFileSize))
	}
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

// maxQueryTypeFileExtensionLength bounds a query type used as a body
// sidecar file extension.
const maxQueryTypeFileExtensionLength = 32

// queryTypeFileExtension returns the file extension a query of queryType
// stores its body sidecar under - the lowercased type - or an error when
// the type cannot safely name a file. A query's type is read back from its
// own JSON metadata, which is project content (a clone, an archive, a hand
// edit) and therefore untrusted: joined into a path unchecked, a type such
// as "/../../x" would address a file outside the project, for a read and -
// through a delete or a type-changing write - for a removal. An accepted
// type is 1-32 ASCII letters, digits, "_" or "-", which can never add a
// separator, a dot, a control character or a Windows-illegal character to
// the derived name. Every type QueryDef.Validate accepts qualifies, and so
// do legacy types it no longer accepts (the demo projects' "JSON"), so
// existing records stay readable.
func queryTypeFileExtension(queryType datatug.QueryType) (string, error) {
	t := string(queryType)
	if t == "" || len(t) > maxQueryTypeFileExtensionLength {
		return "", fmt.Errorf("query type %q cannot name a body sidecar file: it must be 1-%d characters", t, maxQueryTypeFileExtensionLength)
	}
	for i := 0; i < len(t); i++ {
		if !isQueryTypeByte(t[i]) {
			return "", fmt.Errorf("query type %q cannot name a body sidecar file: only ASCII letters, digits, '_' and '-' are allowed", t)
		}
	}
	return strings.ToLower(t), nil
}

// isQueryTypeByte reports whether c may appear in a query type used as a
// file extension: an ASCII letter, digit, '_' or '-'.
func isQueryTypeByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

// queryBodyFileName returns the body sidecar file name a query of this
// type/id persists, following "<id>.query.<lowercase type>" (see
// pkg/datatug/doc.go's "Query text sidecar file naming" convention). It
// fails for a type queryTypeFileExtension refuses. id must already be a
// validated query ID.
func queryBodyFileName(id string, queryType datatug.QueryType) (string, error) {
	ext, err := queryTypeFileExtension(queryType)
	if err != nil {
		return "", err
	}
	return id + "." + storage.QueryFileSuffix + "." + ext, nil
}

// checkQueryBodyFileName verifies that name is exactly the body sidecar
// name queryBodyFileName derives for id and some accepted type. Recovery
// applies it to every body file name a journal records, so a journal can
// only ever name a body sidecar of its own validated query ID - never
// another file, and never a path.
func checkQueryBodyFileName(id, name string) error {
	ext, ok := strings.CutPrefix(name, id+"."+storage.QueryFileSuffix+".")
	if !ok {
		return fmt.Errorf("%q is not a body sidecar name of query %q", name, id)
	}
	derived, err := queryBodyFileName(id, datatug.QueryType(ext))
	if err != nil {
		return fmt.Errorf("%q is not a body sidecar name of query %q: %w", name, id, err)
	}
	if derived != name {
		return fmt.Errorf("%q is not a body sidecar name of query %q (the derived name is %q)", name, id, derived)
	}
	return nil
}
