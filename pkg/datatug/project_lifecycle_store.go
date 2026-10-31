package datatug

import "context"

// ProjectLifecycleScope is created from the serving authority, never decoded
// from a client-supplied actor. All identities and branch selectors are bound to
// one provider scope; a receipt from another scope cannot authorize a mutation.
type ProjectLifecycleScope struct {
	ActorID string
	Storage string
	SpaceID string
	Project string
	Branch  string
	// Environment is permission context, not a partition of the reference graph.
	// For example, hosted cards have one project-wide identity even when access
	// is admitted through a particular environment. Inventory remains project-wide.
	Environment string
}

type ProjectLifecycleAck struct {
	OperationID string                 `json:"operationId"`
	Revision    string                 `json:"revision"`
	Change      ProjectLifecycleChange `json:"change"`
}

type ProjectLifecycleCatalog struct {
	Revision string `json:"revision"`
	Complete bool   `json:"complete"`
	// ArchiveSupported is authoritative visibility support. Complete describes
	// the reference graph required for Delete, not permission to hide an item.
	ArchiveSupported bool               `json:"archiveSupported,omitempty"`
	Items            []ProjectUsageItem `json:"items"`
}

// ProjectLifecycleStore is an optional authoritative provider capability. A
// plain CRUD ProjectStore MUST NOT be treated as this capability. Unsupported
// providers leave lifecycle actions unavailable; no legacy delete fallback.
//
// Archive and restore change active-list visibility only. They preserve the
// definition, its references, direct access and accepted executions. Providers
// may implement these through an authoritative visibility catalog even when a
// complete reference graph is unavailable; see PlanProjectVisibility.
//
// A destructive ChangeProjectLifecycle must authorize the current serving actor, inventory
// every retained reference (including history from other actors and expired but
// unpurged records), and run PlanProjectLifecycle on that fresh complete graph
// inside the SAME provider lock/transaction as metadata/definition persistence
// and its operation receipt. Every concurrent reference-bearing writer must
// share that admission boundary. A client review or a definitions-only scan is
// not sufficient. Operation receipts bind actor, full scope and request digest;
// duplicate operations return the original acknowledgement, never execute twice.
//
// ReviewProjectLifecycle must authorize the target and redact source identity
// separately from admission. ListProjectLifecycle includes only items the actor
// can read. It must retain archived objects in the reference graph even when an
// active-only catalog hides them. Definitions cannot purge execution history.
// Watch object IDs are qualified as namespace/environment/watch-ID; providers
// must validate all three against the serving project and current authority.
// A bare Watch ID is not sufficient to select a lifecycle target.
type ProjectLifecycleStore interface {
	ReviewProjectLifecycle(context.Context, ProjectLifecycleScope, ProjectObjectRef) (ProjectLifecycleReview, error)
	ChangeProjectLifecycle(context.Context, ProjectLifecycleScope, ProjectLifecycleWrite) (ProjectLifecycleAck, error)
	ListProjectLifecycle(context.Context, ProjectLifecycleScope, bool) (ProjectLifecycleCatalog, error)
}

// Validate rejects malformed identities before a provider sees them.
func (ref ProjectObjectRef) Validate() error {
	if !validProjectObjectRef(ref) {
		return ErrProjectLifecycleInvalid
	}
	return nil
}

func (write ProjectLifecycleWrite) Validate() error {
	if write.OperationID == "" || len(write.OperationID) > 128 ||
		write.ExpectedRevision == "" || len(write.ExpectedRevision) > 512 ||
		!validProjectObjectRef(write.Change.Object) || isProjectHistoryKind(write.Change.Object.Kind) ||
		(write.Change.Action != "archive" && write.Change.Action != "restore" && write.Change.Action != "delete") {
		return ErrProjectLifecycleInvalid
	}
	return nil
}
