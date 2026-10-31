package datatug

// PlanProjectVisibility validates a non-destructive visibility transition.
// The provider must authorize the actor and enumerate readable current targets.
// No definition or retained execution is changed, so outgoing references and
// reference-writer fences are deliberately not prerequisites. Delete MUST use
// the reference-complete PlanProjectLifecycle path instead.
func PlanProjectVisibility(catalog ProjectLifecycleCatalog, write ProjectLifecycleWrite) (ProjectLifecyclePlan, error) {
	plan := ProjectLifecyclePlan{Change: write.Change}
	if err := write.Validate(); err != nil {
		return plan, err
	}
	if write.Change.Action != "archive" && write.Change.Action != "restore" {
		return plan, ErrProjectUsageIncomplete
	}
	if !catalog.ArchiveSupported || catalog.Revision == "" || len(catalog.Items) > 10000 {
		return plan, ErrProjectUsageIncomplete
	}
	if write.ExpectedRevision != catalog.Revision {
		return plan, ErrProjectLifecycleConflict
	}
	seen := make(map[ProjectObjectRef]bool, len(catalog.Items))
	found := false
	for _, item := range catalog.Items {
		if item.Restricted || item.Historical || item.Ref.Validate() != nil || isProjectHistoryKind(item.Ref.Kind) || seen[item.Ref] {
			return plan, ErrProjectUsageIncomplete
		}
		seen[item.Ref] = true
		if item.Ref == write.Change.Object {
			found = true
		}
	}
	if !found {
		return plan, ErrProjectLifecycleConflict
	}
	return plan, nil
}

// ReviewProjectVisibility advertises Archive/Restore separately from complete
// reference discovery. It never grants Delete and never claims the graph is
// complete. Providers still own target authorization and source redaction.
func ReviewProjectVisibility(catalog ProjectLifecycleCatalog, target ProjectObjectRef) (ProjectLifecycleReview, error) {
	review := ProjectLifecycleReview{Revision: catalog.Revision, ArchiveSupported: catalog.ArchiveSupported, Incoming: []ProjectUsageItem{}}
	_, err := PlanProjectVisibility(catalog, ProjectLifecycleWrite{OperationID: "review", ExpectedRevision: catalog.Revision, Change: ProjectLifecycleChange{Object: target, Action: "archive"}})
	if err != nil {
		return review, err
	}
	for _, item := range catalog.Items {
		if item.Ref == target {
			review.Target = item
			review.CanArchive = !item.Archived
			review.CanRestore = item.Archived
			break
		}
	}
	return review, nil
}
