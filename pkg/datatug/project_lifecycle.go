package datatug

import (
	"errors"
	"fmt"
	"strings"
)

// ProjectObjectRef is a project-scoped definition identity, never a filesystem path.
type ProjectObjectRef struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// ProjectObjectUsage describes complete outgoing references of a definition or
// retained execution record. Archived items and completed history remain in the
// graph and prevent deletion of their targets. Favorites also prevent Delete
// only: their private navigation references are always Historical. Historical is provider-derived:
// completed runs and immutable snapshots are inactive references, not deleted
// references. Running executions remain active references. Archived applies only
// to definitions; history activity is classified solely through Historical.
// A reference means retained content or use. Identity-only audit receipts remain
// provenance and replay metadata; their object IDs alone must not create edges.
type ProjectObjectUsage struct {
	Ref             ProjectObjectRef   `json:"ref"`
	Title           string             `json:"title"`
	Archived        bool               `json:"archived"`
	Historical      bool               `json:"historical,omitempty"`
	ContentRevision string             `json:"contentRevision"`
	References      []ProjectObjectRef `json:"references"`
}

// ProjectUsageSnapshot must come from the authoritative provider, within the same
// lock/transaction as a write. Unknown, unsupported or unreadable sources must
// leave Complete false. Revision binds definitions, references and lifecycle state.
// Completeness includes retained history even when its owner is not the current
// actor or its expiry has passed. Expiry alone does not remove retained references.
type ProjectUsageSnapshot struct {
	Revision string               `json:"revision"`
	Complete bool                 `json:"complete"`
	Objects  []ProjectObjectUsage `json:"objects"`
}

type ProjectLifecycleChange struct {
	Object ProjectObjectRef `json:"object"`
	Action string           `json:"action"` // archive, restore or delete
}

type ProjectLifecycleWrite struct {
	OperationID      string                 `json:"operationId"`
	ExpectedRevision string                 `json:"expectedRevision"`
	Change           ProjectLifecycleChange `json:"change"`
}

type ProjectLifecycleBlocker struct {
	Target     ProjectObjectRef `json:"target"`
	UsedBy     ProjectObjectRef `json:"usedBy"`
	Archived   bool             `json:"archived"`
	Historical bool             `json:"historical,omitempty"`
}

type ProjectLifecyclePlan struct {
	Change ProjectLifecycleChange `json:"change"`
	// Admission details can include sources the actor cannot read. Public
	// responses use ReviewProjectLifecycle's visibility-filtered items instead.
	Blockers []ProjectLifecycleBlocker `json:"-"`
}

var ErrProjectUsageIncomplete = errors.New("project references could not be completely verified")
var ErrProjectLifecycleConflict = errors.New("project definitions or references changed")
var ErrProjectObjectReferenced = errors.New("project object has blocking references")
var ErrProjectLifecycleInvalid = errors.New("invalid project lifecycle operation")

func validProjectObjectRef(ref ProjectObjectRef) bool {
	switch ref.Kind {
	case "query", "query-folder", "board", "card", "check", "validator", "entity", "meaning", "environment", "connection", "database", "server", "scenario", "case", "incident", "watch", "work", "session", "data-model", "execution", "run", "snapshot", "favorite":
	default:
		return false
	}
	if ref.ID == "" || len(ref.ID) > 512 || strings.TrimSpace(ref.ID) != ref.ID || strings.ContainsAny(ref.ID, "\\\x00\r\n") {
		return false
	}
	for _, segment := range strings.Split(ref.ID, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

// Retained execution records participate in reference checks but are not targets
// of definition lifecycle mutations. Retention/purge is a separate capability.
func isProjectHistoryKind(kind string) bool {
	return kind == "execution" || kind == "run" || kind == "snapshot" || kind == "favorite"
}

// PlanProjectLifecycle admits exactly one object transition against the current
// authoritative snapshot. No nested or related-object mutations are supported.
// This function neither authorizes nor persists.
func PlanProjectLifecycle(snapshot ProjectUsageSnapshot, write ProjectLifecycleWrite) (ProjectLifecyclePlan, error) {
	plan := ProjectLifecyclePlan{Change: write.Change, Blockers: []ProjectLifecycleBlocker{}}
	if !snapshot.Complete {
		return plan, ErrProjectUsageIncomplete
	}
	objects, err := validateProjectUsageSnapshot(snapshot)
	if err != nil {
		return plan, err
	}
	if write.ExpectedRevision != snapshot.Revision {
		return plan, ErrProjectLifecycleConflict
	}
	change := write.Change
	if write.OperationID == "" || len(write.OperationID) > 128 || !validProjectObjectRef(change.Object) ||
		(change.Action != "archive" && change.Action != "restore" && change.Action != "delete") || isProjectHistoryKind(change.Object.Kind) {
		return plan, ErrProjectLifecycleInvalid
	}
	target, found := objects[change.Object]
	if !found {
		return plan, fmt.Errorf("%w: object unavailable", ErrProjectLifecycleConflict)
	}
	for _, source := range snapshot.Objects {
		for _, reference := range source.References {
			if reference == change.Object && (change.Action == "delete" ||
				(change.Action == "archive" && !source.Archived && !source.Historical)) {
				plan.Blockers = append(plan.Blockers, ProjectLifecycleBlocker{Target: reference, UsedBy: source.Ref, Archived: source.Archived, Historical: source.Historical})
				break
			}
		}
	}
	if change.Action == "restore" {
		seen := map[ProjectObjectRef]bool{}
		for _, dependency := range target.References {
			if objects[dependency].Archived && !seen[dependency] {
				plan.Blockers = append(plan.Blockers, ProjectLifecycleBlocker{Target: dependency, UsedBy: target.Ref, Archived: target.Archived})
				seen[dependency] = true
			}
		}
	}
	if len(plan.Blockers) > 0 {
		return plan, ErrProjectObjectReferenced
	}
	return plan, nil
}

// Structural validation is shared by planning and presentation. The provider
// must additionally establish completeness with schema-aware source readers.
func validateProjectUsageSnapshot(snapshot ProjectUsageSnapshot) (map[ProjectObjectRef]ProjectObjectUsage, error) {
	if snapshot.Revision == "" || len(snapshot.Objects) > 10000 {
		return nil, ErrProjectUsageIncomplete
	}
	objects := make(map[ProjectObjectRef]ProjectObjectUsage, len(snapshot.Objects))
	edges := 0
	for _, object := range snapshot.Objects {
		edges += len(object.References)
		if !validProjectObjectRef(object.Ref) || object.ContentRevision == "" || len(object.References) > 10000 || edges > 100000 ||
			(object.Historical && !isProjectHistoryKind(object.Ref.Kind)) ||
			(object.Ref.Kind == "favorite" && !object.Historical) ||
			(object.Archived && isProjectHistoryKind(object.Ref.Kind)) {
			return nil, ErrProjectUsageIncomplete
		}
		if _, duplicate := objects[object.Ref]; duplicate {
			return nil, ErrProjectUsageIncomplete
		}
		objects[object.Ref] = object
	}
	for _, object := range snapshot.Objects {
		for _, ref := range object.References {
			if !validProjectObjectRef(ref) {
				return nil, ErrProjectUsageIncomplete
			}
			if _, exists := objects[ref]; !exists {
				return nil, ErrProjectUsageIncomplete
			}
		}
	}
	return objects, nil
}
