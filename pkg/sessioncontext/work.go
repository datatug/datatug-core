package sessioncontext

import "context"

// WorkBinding is trusted server configuration, never a transport payload. The
// caller must validate reciprocal WorkItem/session identity and current project,
// item and source permissions in the same transaction used by the repository.
// IndexOwner is the immutable creator/index partition, not the acting user.
type WorkBinding struct {
	WorkItemID, SessionID, IndexOwner, DatabaseID, ProjectID string
}

// NewWorkService reuses context commands under one explicit Work association.
// A caller-owned BufferedRepository allows authorization, Work activity and
// context writes to commit together. Authorize must fail closed on revocation.
func NewWorkService(repo Repository, binding WorkBinding, authorize Authorizer) (*Service, error) {
	if !safeID.MatchString(binding.WorkItemID) || !safeID.MatchString(binding.SessionID) || binding.IndexOwner == "" || !safeID.MatchString(binding.DatabaseID) || binding.ProjectID == "" {
		return nil, ErrInvalid
	}
	s, err := NewService(repo, authorize)
	if err != nil {
		return nil, err
	}
	s.work = &binding
	return s, nil
}
func (s *Service) ownsSession(p Principal, v Session) bool {
	if s.work == nil {
		return v.WorkItemID == "" && v.Owner == p.Actor
	}
	b := s.work
	return v.WorkItemID == b.WorkItemID && v.ID == b.SessionID && v.Owner == b.IndexOwner && v.DatabaseID == b.DatabaseID && v.ProjectID == b.ProjectID
}
func (s *Service) indexOwner(p Principal) string {
	if s.work != nil {
		return s.work.IndexOwner
	}
	return p.Actor
}
func (s *Service) workItemID() string {
	if s.work != nil {
		return s.work.WorkItemID
	}
	return ""
}
func workAction(action string) bool {
	switch action {
	case "read", "createSession", "createBag", "renameBag", "renameSession", "putEntry", "applyContext", "clearBag", "readOperation", "listEntryRevisions", "readEntryRevision", "copyContext":
		return true
	default:
		return false
	}
}

// Work sessions intentionally have no native execution/recovery owner. An
// execution must use an explicitly copied actor-owned private session instead.
func (s *Service) authorizeCopy(ctx context.Context, p Principal, id string, scope *Scope) error {
	return s.allow(ctx, p, "copyContext", id, scope)
}
