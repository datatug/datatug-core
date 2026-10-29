package datatug

// ProjectUsageItem is presentation metadata only. Providers must authorize its
// disclosure; callers with restricted source visibility receive an unnamed item.
type ProjectUsageItem struct {
	Ref        ProjectObjectRef `json:"ref"`
	Title      string           `json:"title,omitempty"`
	Archived   bool             `json:"archived"`
	Historical bool             `json:"historical,omitempty"`
	Restricted bool             `json:"restricted,omitempty"`
}

type ProjectLifecycleReview struct {
	Revision   string             `json:"revision"`
	Complete   bool               `json:"complete"`
	Target     ProjectUsageItem   `json:"target"`
	Incoming   []ProjectUsageItem `json:"incoming"`
	CanArchive bool               `json:"canArchive"`
	CanDelete  bool               `json:"canDelete"`
	CanRestore bool               `json:"canRestore"`
}

// ReviewProjectLifecycle does not authorize. A provider calls it only after
// target access passes, with a visibility decision for every referenced source.
// A nil visible callback redacts all related resources, never assumes access.
func ReviewProjectLifecycle(snapshot ProjectUsageSnapshot, target ProjectObjectRef, visible func(ProjectObjectRef) bool) (ProjectLifecycleReview, error) {
	review := ProjectLifecycleReview{Revision: snapshot.Revision, Complete: snapshot.Complete, Incoming: []ProjectUsageItem{}}
	objects, err := validateProjectUsageSnapshot(snapshot)
	if err != nil {
		review.Complete = false
		return review, err
	}
	var found *ProjectObjectUsage
	if object, ok := objects[target]; ok {
		found = &object
	}
	if found == nil {
		return review, ErrProjectLifecycleConflict
	}
	review.Target = ProjectUsageItem{Ref: found.Ref, Title: found.Title, Archived: found.Archived, Historical: found.Historical}
	item := func(object ProjectObjectUsage) ProjectUsageItem {
		if visible == nil || !visible(object.Ref) {
			// Activity classification is public admission metadata; identity and title remain hidden.
			return ProjectUsageItem{Restricted: true, Archived: object.Archived, Historical: object.Historical}
		}
		return ProjectUsageItem{Ref: object.Ref, Title: object.Title, Archived: object.Archived, Historical: object.Historical}
	}
	allowed := func(change ProjectLifecycleChange) bool {
		_, err := PlanProjectLifecycle(snapshot, ProjectLifecycleWrite{OperationID: "review", ExpectedRevision: snapshot.Revision, Change: change})
		return err == nil
	}
	review.CanArchive = !found.Archived && allowed(ProjectLifecycleChange{target, "archive"})
	review.CanDelete = allowed(ProjectLifecycleChange{target, "delete"})
	review.CanRestore = found.Archived && allowed(ProjectLifecycleChange{target, "restore"})
	for _, object := range snapshot.Objects {
		for _, ref := range object.References {
			if ref == target {
				review.Incoming = append(review.Incoming, item(object))
				break
			}
		}
	}
	return review, nil
}
