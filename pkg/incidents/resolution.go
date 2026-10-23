package incidents

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/investigation"
)

// CheckRunRef identifies a saved Check run, never a query execution or definition.
type CheckRunRef struct {
	StoreID      string `json:"storeId"`
	ProjectID    string `json:"projectId"`
	Environment  string `json:"environment"`
	CheckID      string `json:"checkId"`
	CheckVersion uint64 `json:"checkVersion"`
	RunID        string `json:"runId"`
}

func (r CheckRunRef) Validate() error {
	if !validSegment(r.StoreID) || !validSegment(r.ProjectID) || !validSegment(r.Environment) || !validSegment(r.CheckID) || !validSegment(r.RunID) || r.CheckVersion == 0 {
		return fmt.Errorf("check run requires canonical store, project, environment, check, positive version and run identities")
	}
	return nil
}

// ResolutionScope pins logical authority and applied input identity. Attempt and
// configuration revisions deliberately belong to observations in the proof:
// recovery changes them without changing the criterion.
type ResolutionScope struct {
	Project            ProjectRef                          `json:"project"`
	SourceID           string                              `json:"sourceID"`
	Generation         string                              `json:"generation"`
	AffectedInputs     map[string]investigation.TypedValue `json:"affectedInputs"`
	SessionRef         string                              `json:"sessionRef"`
	BagID              string                              `json:"bagID"`
	AppliedManifestRef string                              `json:"appliedManifestRef"`
}

func (s ResolutionScope) Validate() error {
	if err := s.Project.Validate(); err != nil {
		return err
	}
	if s.Project.Environment == "" {
		return fmt.Errorf("resolution scope requires environment")
	}
	for _, value := range []string{s.SourceID, s.Generation, s.SessionRef, s.BagID, s.AppliedManifestRef} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return fmt.Errorf("resolution scope identities are required and canonical")
		}
	}
	if len(s.AffectedInputs) == 0 {
		return fmt.Errorf("affectedInputs are required")
	}
	for name, value := range s.AffectedInputs {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name {
			return fmt.Errorf("affected input name is required and canonical")
		}
		if err := value.Validate(); err != nil {
			return fmt.Errorf("affected input %s: %w", name, err)
		}
	}
	return nil
}

// ResolutionCriterion is opt-in. Required criteria cannot be bypassed with a
// generic status mutation. Draft revisions may be completed, then are immutable.
type ResolutionCriterion struct {
	FinalizedAt  *time.Time      `json:"finalizedAt,omitempty"`
	Version      uint64          `json:"version"`
	Finalized    bool            `json:"finalized"`
	Enforcement  string          `json:"enforcement"`
	Scope        ResolutionScope `json:"scope"`
	CheckID      string          `json:"checkID,omitempty"`
	CheckVersion uint64          `json:"checkVersion,omitempty"`
	// DefinitionDigest pins CanonicalDetectiveDefinition, derived by the host from
	// the complete admitted executable snapshot, never from client assertions.
	DefinitionDigest string `json:"definitionDigest,omitempty"`
	MaxAgeSeconds    int64  `json:"maxAgeSeconds"`
}

func (c ResolutionCriterion) Validate() error {
	if c.Version == 0 || c.Enforcement != "required" {
		return fmt.Errorf("criterion requires positive version and required enforcement")
	}
	if err := c.Scope.Validate(); err != nil {
		return err
	}
	if c.MaxAgeSeconds < 1 || c.MaxAgeSeconds > 86400 {
		return fmt.Errorf("maxAgeSeconds must be from 1 to 86400")
	}
	if c.Finalized != (c.FinalizedAt != nil) {
		return fmt.Errorf("only finalized criteria require finalizedAt")
	}
	if c.FinalizedAt != nil {
		_, offset := c.FinalizedAt.Zone()
		if c.FinalizedAt.IsZero() || offset != 0 {
			return fmt.Errorf("finalizedAt must be UTC wall time")
		}
	}
	if c.Finalized && (!validSegment(c.CheckID) || c.CheckVersion == 0) {
		return fmt.Errorf("finalized criterion requires check identity and positive revision")
	}
	if c.CheckID != "" && !validSegment(c.CheckID) {
		return fmt.Errorf("criterion checkID must be canonical")
	}
	if (c.CheckID == "") != (c.CheckVersion == 0) {
		return fmt.Errorf("check identity and revision must be supplied together")
	}
	if err := validateDefinitionDigest(c.DefinitionDigest); err != nil {
		return err
	}
	if c.DefinitionDigest != "" && c.CheckID == "" {
		return fmt.Errorf("definition digest requires check identity and revision")
	}
	return nil
}

// ResolutionProof is a server-admitted receipt, not a client assertion of pass.
// Before emitting it the backend must resolve the saved run under current ACL,
// compare its real scope/revision/result and verify ObservationRefs identify the
// latest attempts/configuration for every affected input. Core validates the
// immutable contract and wall-clock freshness; it does not fetch source records.
type ResolutionProof struct {
	// ResolutionEpoch binds proof to the current reopening epoch (zero for legacy streams).
	ResolutionEpoch uint64 `json:"resolutionEpoch,omitempty"`
	// DefinitionDigest must come from the actual retained run definition, using
	// sessioncontext.CanonicalDetectiveDefinition, under current host admission.
	DefinitionDigest string          `json:"definitionDigest,omitempty"`
	CriterionVersion uint64          `json:"criterionVersion"`
	Scope            ResolutionScope `json:"scope"`
	Run              CheckRunRef     `json:"run"`
	EvaluatedAt      time.Time       `json:"evaluatedAt"`
	Outcome          string          `json:"outcome"`
	ObservationRefs  []ArtifactRef   `json:"observationRefs"`
	RecoveryRefs     []ArtifactRef   `json:"recoveryRefs"`
}

func (p ResolutionProof) Validate(at time.Time) error {
	if err := validateDefinitionDigest(p.DefinitionDigest); err != nil {
		return err
	}
	if p.CriterionVersion == 0 || p.Outcome != "pass" {
		return fmt.Errorf("resolution requires a positive criterion version and actual passing run")
	}
	if err := p.Scope.Validate(); err != nil {
		return err
	}
	if err := p.Run.Validate(); err != nil {
		return err
	}
	if p.EvaluatedAt.IsZero() || at.IsZero() || p.EvaluatedAt.After(at) {
		return fmt.Errorf("evaluation wall time is missing or in the future")
	}
	_, offset := p.EvaluatedAt.Zone()
	if offset != 0 {
		return fmt.Errorf("evaluation wall time must be UTC")
	}
	if len(p.ObservationRefs) == 0 || len(p.RecoveryRefs) == 0 {
		return fmt.Errorf("observed source and recovery references are required")
	}
	for _, refs := range [][]ArtifactRef{p.ObservationRefs, p.RecoveryRefs} {
		for _, ref := range refs {
			if err := ref.Validate(); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateResolutionProof checks an already resolved, trusted proof against its
// fixed criterion. Synthetic business clocks must never be supplied as at.
func ValidateResolutionProof(c ResolutionCriterion, p ResolutionProof, at time.Time) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if !c.Finalized {
		return fmt.Errorf("criterion is not finalized")
	}
	if err := p.Validate(at); err != nil {
		return err
	}
	if c.Version != p.CriterionVersion || !reflect.DeepEqual(c.Scope, p.Scope) {
		return fmt.Errorf("resolution criterion revision or applied scope mismatch")
	}
	if p.Run.StoreID != c.Scope.Project.StoreID || p.Run.ProjectID != c.Scope.Project.ProjectID || p.Run.Environment != c.Scope.Project.Environment || p.Run.CheckID != c.CheckID || p.Run.CheckVersion != c.CheckVersion {
		return fmt.Errorf("resolution check run scope or revision mismatch")
	}
	if c.DefinitionDigest != p.DefinitionDigest {
		return fmt.Errorf("resolution executable definition digest mismatch")
	}
	if p.EvaluatedAt.Before(*c.FinalizedAt) {
		return fmt.Errorf("run predates criterion finalization")
	}
	if at.Sub(p.EvaluatedAt) > time.Duration(c.MaxAgeSeconds)*time.Second {
		return fmt.Errorf("resolution check run is stale")
	}
	return nil
}

// Empty preserves legacy criteria. Supplied digests are canonical SHA-256 hex.
func validateDefinitionDigest(digest string) error {
	if digest == "" {
		return nil
	}
	if len(digest) != 64 {
		return fmt.Errorf("definition digest must be lowercase SHA-256 hex")
	}
	for _, c := range digest {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return fmt.Errorf("definition digest must be lowercase SHA-256 hex")
		}
	}
	return nil
}

// ResolvedPayload commits the human note and proof in the same event.
type ResolvedPayload struct {
	// LifecycleVersion 1 records the resolved outcome with verified evidence.
	// Version 0 retains historical streams with a separate outcome event.
	LifecycleVersion int             `json:"lifecycleVersion,omitempty"`
	HumanNote        string          `json:"humanNote"`
	Proof            ResolutionProof `json:"proof"`
}

func (p ResolvedPayload) Validate(at time.Time) error {
	if p.LifecycleVersion < 0 || p.LifecycleVersion > 1 {
		return fmt.Errorf("unsupported resolution lifecycle version")
	}
	if strings.TrimSpace(p.HumanNote) == "" {
		return fmt.Errorf("resolution humanNote is required")
	}
	return p.Proof.Validate(at)
}

func (p *Incident) applyResolutionEvent(event Event) error {
	if statusRank(p.Status) >= statusRank(StatusResolved) {
		return fmt.Errorf("cannot change criterion or resolve an already resolved incident")
	}
	switch event.Type {
	case EventIncidentCriterionSet:
		c := validatedPayload[ResolutionCriterion](event.Payload)
		if c.Finalized && !c.FinalizedAt.Equal(event.At) {
			return fmt.Errorf("criterion finalizedAt must match commit event wall time")
		}
		if p.ResolutionCriterion != nil {
			if p.ResolutionCriterion.Finalized {
				return fmt.Errorf("finalized criterion is immutable")
			}
			if c.Version != p.ResolutionCriterion.Version+1 {
				return fmt.Errorf("criterion revision must advance by one")
			}
		} else if c.Version != 1 {
			return fmt.Errorf("first criterion revision must be one")
		}
		p.ResolutionCriterion = &c
		p.CriterionEventID = event.ID
	case EventIncidentResolved:
		r := validatedPayload[ResolvedPayload](event.Payload)
		if p.ResolutionCriterion == nil {
			return fmt.Errorf("verified resolution requires a criterion")
		}
		if err := ValidateIncidentResolutionProof(*p, r.Proof, event.At); err != nil {
			return err
		}
		p.Resolution = &r
		p.ResolutionEventID = event.ID
		p.Status = StatusResolved
		if r.LifecycleVersion == 1 {
			p.Outcome = OutcomeResolved
		}
		p.NoteEntries = append(p.NoteEntries, Note{EventID: event.ID, Body: r.HumanNote})
		p.Notes = append(p.Notes, r.HumanNote)
	}
	return nil
}
func cloneResolutionScope(s ResolutionScope) ResolutionScope {
	s.AffectedInputs = cloneResolutionInputs(s.AffectedInputs)
	return s
}
func cloneResolutionInputs(inputs map[string]investigation.TypedValue) map[string]investigation.TypedValue {
	if inputs == nil {
		return nil
	}
	result := make(map[string]investigation.TypedValue, len(inputs))
	for key, value := range inputs {
		result[key] = value
	}
	return result
}
func visibleCriterion(p Incident, policy ViewPolicy) *ResolutionCriterion {
	if p.ResolutionCriterion == nil || policy.WithheldEvents[p.CriterionEventID] || (p.CriterionEventID == "" && len(policy.WithheldEvents) > 0) {
		return nil
	}
	c := *p.ResolutionCriterion
	c.Scope = cloneResolutionScope(c.Scope)
	if c.FinalizedAt != nil {
		at := *c.FinalizedAt
		c.FinalizedAt = &at
	}
	return &c
}
func visibleResolution(p Incident, policy ViewPolicy) *ResolvedPayload {
	if p.Resolution == nil || policy.WithheldEvents[p.ResolutionEventID] || (p.ResolutionEventID == "" && len(policy.WithheldEvents) > 0) {
		return nil
	}
	r := *p.Resolution
	r.Proof.Scope = cloneResolutionScope(r.Proof.Scope)
	r.Proof.ObservationRefs = cloneArtifactRefs(r.Proof.ObservationRefs)
	r.Proof.RecoveryRefs = cloneArtifactRefs(r.Proof.RecoveryRefs)
	return &r
}
