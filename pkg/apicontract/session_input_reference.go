package apicontract

import (
	"encoding/hex"
	"sort"
	"strings"
)

// SessionInputEntryRef identifies the exact private bag revision consumed by an input.
type SessionInputEntryRef struct {
	BagID      string `json:"bagId"`
	BagVersion int64  `json:"bagVersion"`
	EntryID    string `json:"entryId"`
	RevisionID string `json:"revisionId"`
}

// SessionInputBindingRef records names and provenance only, never input values.
type SessionInputBindingRef struct {
	InputName   string                 `json:"inputName"`
	Origin      string                 `json:"origin"` // bag | case | manual
	Entries     []SessionInputEntryRef `json:"entries,omitempty"`
	CaseID      string                 `json:"caseId,omitempty"`
	CaseVersion int64                  `json:"caseVersion,omitempty"`
}

// SessionInputReference locates one private immutable input pin. The full private
// namespace is distinct from the native execution target. This reference grants
// no authority to read inputs; resolvers must authenticate and authorize it anew.
type SessionInputReference struct {
	SchemaVersion    int                      `json:"schemaVersion"`
	StorageID        string                   `json:"storageId"`
	SpaceID          string                   `json:"spaceId,omitempty"`
	DatabaseID       string                   `json:"databaseId"`
	ProjectID        string                   `json:"projectId"`
	NamespaceID      string                   `json:"namespaceId"`
	SessionID        string                   `json:"sessionId"`
	InputSnapshotID  string                   `json:"inputSnapshotId"`
	ContextSetID     string                   `json:"contextSetId"`
	EnvironmentID    string                   `json:"environmentId"`
	IdentityDomain   string                   `json:"identityDomain"`
	DefinitionDigest string                   `json:"definitionDigest"`
	Bindings         []SessionInputBindingRef `json:"bindings"`
}

const MaxSessionInputBindings = 100
const MaxSessionInputEntryRefs = 400

// Validate checks bounded metadata and consistent provenance. IDs follow the
// session contract's path-safe 1..128-byte form; hosted namespaces require SpaceID.
// It deliberately cannot validate resource ownership or source authorization.
func (r SessionInputReference) Validate() error {
	invalid := func(field string) error {
		return &ValidationError{Field: field, Message: "invalid session input reference"}
	}
	if r.SchemaVersion != 1 {
		return invalid("schemaVersion")
	}
	for field, value := range map[string]string{"storageId": r.StorageID, "databaseId": r.DatabaseID, "projectId": r.ProjectID, "namespaceId": r.NamespaceID, "sessionId": r.SessionID, "inputSnapshotId": r.InputSnapshotID, "contextSetId": r.ContextSetID, "environmentId": r.EnvironmentID, "identityDomain": r.IdentityDomain} {
		if !sessionReferenceID(value, false) {
			return invalid(field)
		}
	}
	if r.StorageID == "local" && r.SpaceID != "" || r.StorageID != "local" && !sessionReferenceID(r.SpaceID, false) {
		return invalid("spaceId")
	}
	decoded, err := hex.DecodeString(r.DefinitionDigest)
	if err != nil || len(decoded) != 32 || strings.ToLower(r.DefinitionDigest) != r.DefinitionDigest {
		return invalid("definitionDigest")
	}
	if len(r.Bindings) > MaxSessionInputBindings {
		return invalid("bindings")
	}
	names := map[string]bool{}
	entries := map[string]SessionInputEntryRef{}
	caseVersions := map[string]int64{}
	count := 0
	for _, binding := range r.Bindings {
		if !sessionReferenceID(binding.InputName, true) || names[binding.InputName] {
			return invalid("bindings.inputName")
		}
		names[binding.InputName] = true
		switch binding.Origin {
		case "bag":
			if len(binding.Entries) == 0 || binding.CaseID != "" || binding.CaseVersion != 0 {
				return invalid("bindings.entries")
			}
		case "case":
			if len(binding.Entries) != 0 || !sessionReferenceID(binding.CaseID, false) || binding.CaseVersion <= 0 {
				return invalid("bindings.caseId")
			}
			// BindCaseInputs consumes one authoritative version of each Case.
			if prior, ok := caseVersions[binding.CaseID]; ok && prior != binding.CaseVersion {
				return invalid("bindings.caseVersion")
			}
			caseVersions[binding.CaseID] = binding.CaseVersion
		case "manual":
			if len(binding.Entries) != 0 || binding.CaseID != "" || binding.CaseVersion != 0 {
				return invalid("bindings.origin")
			}
		default:
			return invalid("bindings.origin")
		}
		seen := map[string]bool{}
		for _, entry := range binding.Entries {
			count++
			if count > MaxSessionInputEntryRefs || !sessionReferenceID(entry.BagID, false) || !sessionReferenceID(entry.EntryID, false) || !sessionReferenceID(entry.RevisionID, false) || entry.BagVersion <= 0 || seen[entry.EntryID] {
				return invalid("bindings.entries")
			}
			seen[entry.EntryID] = true
			// Explicit historical attachments may consume different bag versions.
			// Only an entry's immutable bag/revision identity must agree across inputs.
			if prior, ok := entries[entry.EntryID]; ok && (prior.BagID != entry.BagID || prior.RevisionID != entry.RevisionID) {
				return invalid("bindings.entries")
			}
			entries[entry.EntryID] = entry
		}
	}
	return nil
}

func sessionReferenceID(value string, parameter bool) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for i, c := range []byte(value) {
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		if letter || digit && (!parameter || i > 0) || i > 0 && (c == '_' || c == '-' || c == '.' && !parameter) {
			continue
		}
		return false
	}
	return true
}

// Canonical returns validated, independently owned metadata in stable order.
// Repeated bindings/entries are rejected rather than merged by precedence.
func (r SessionInputReference) Canonical() (SessionInputReference, error) {
	if err := r.Validate(); err != nil {
		return SessionInputReference{}, err
	}
	out := r
	out.Bindings = make([]SessionInputBindingRef, len(r.Bindings))
	for i, binding := range r.Bindings {
		out.Bindings[i] = binding
		out.Bindings[i].Entries = append([]SessionInputEntryRef(nil), binding.Entries...)
		sort.Slice(out.Bindings[i].Entries, func(a, b int) bool {
			return out.Bindings[i].Entries[a].EntryID < out.Bindings[i].Entries[b].EntryID
		})
	}
	sort.Slice(out.Bindings, func(i, j int) bool { return out.Bindings[i].InputName < out.Bindings[j].InputName })
	return out, nil
}
