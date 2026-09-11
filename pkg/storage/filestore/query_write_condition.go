package filestore

import (
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
)

// validateQueryWriteCondition rejects a datatug.QueryWriteCondition that
// does not set exactly one of IfNoneMatch (create) or IfMatch (update).
// PutQuery calls this before any I/O so a malformed condition never reaches
// the file system.
func validateQueryWriteCondition(c datatug.QueryWriteCondition) error {
	hasIfMatch := c.IfMatch != ""
	switch {
	case c.IfNoneMatch && hasIfMatch:
		return validation.NewBadRequestError(validation.NewErrBadRequestFieldValue(
			"condition", "must not set both IfNoneMatch and IfMatch"))
	case !c.IfNoneMatch && !hasIfMatch:
		return validation.NewBadRequestError(validation.NewErrBadRequestFieldValue(
			"condition", "must set exactly one of IfNoneMatch or IfMatch"))
	default:
		return nil
	}
}
