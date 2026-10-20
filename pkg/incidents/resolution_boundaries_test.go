package incidents

import (
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/stretchr/testify/require"
)

func TestResolutionScopeRejectsUnadmittedIdentityAndInputs(t *testing.T) {
	tests := map[string]struct {
		mutate func(*ResolutionScope)
		want   string
	}{
		"project":      {func(s *ResolutionScope) { s.Project.StoreID = "" }, "store"},
		"environment":  {func(s *ResolutionScope) { s.Project.Environment = "" }, "environment"},
		"source":       {func(s *ResolutionScope) { s.SourceID = " " }, "scope identities"},
		"generation":   {func(s *ResolutionScope) { s.Generation = " g1" }, "scope identities"},
		"session":      {func(s *ResolutionScope) { s.SessionRef = "" }, "scope identities"},
		"bag":          {func(s *ResolutionScope) { s.BagID = "" }, "scope identities"},
		"manifest":     {func(s *ResolutionScope) { s.AppliedManifestRef = "" }, "scope identities"},
		"empty inputs": {func(s *ResolutionScope) { s.AffectedInputs = nil }, "affectedInputs"},
		"blank name": {func(s *ResolutionScope) {
			s.AffectedInputs = map[string]investigation.TypedValue{" ": investigation.NewIntegerValue("42")}
		}, "input name"},
		"invalid type": {func(s *ResolutionScope) { s.AffectedInputs["order"] = investigation.TypedValue{Type: "unsupported"} }, "affected input order"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c, _, _ := resolutionFixture()
			tt.mutate(&c.Scope)
			require.ErrorContains(t, c.Scope.Validate(), tt.want)
		})
	}
}
func TestResolutionCriterionRejectsIncompleteAndInvalidFinalization(t *testing.T) {
	tests := map[string]struct {
		mutate func(*ResolutionCriterion)
		want   string
	}{
		"zero revision":                {func(c *ResolutionCriterion) { c.Version = 0 }, "positive version"},
		"unsupported enforcement":      {func(c *ResolutionCriterion) { c.Enforcement = "optional" }, "required enforcement"},
		"scope":                        {func(c *ResolutionCriterion) { c.Scope.AffectedInputs = nil }, "affectedInputs"},
		"no freshness":                 {func(c *ResolutionCriterion) { c.MaxAgeSeconds = 0 }, "maxAgeSeconds"},
		"unbounded freshness":          {func(c *ResolutionCriterion) { c.MaxAgeSeconds = 86401 }, "maxAgeSeconds"},
		"missing finalization time":    {func(c *ResolutionCriterion) { c.FinalizedAt = nil }, "finalizedAt"},
		"draft with finalization time": {func(c *ResolutionCriterion) { c.Finalized = false }, "finalizedAt"},
		"zero finalization time":       {func(c *ResolutionCriterion) { c.FinalizedAt = &time.Time{} }, "UTC wall time"},
		"nonUTC finalization": {func(c *ResolutionCriterion) {
			at := c.FinalizedAt.In(time.FixedZone("business", 3600))
			c.FinalizedAt = &at
		}, "UTC wall time"},
		"missing check":            {func(c *ResolutionCriterion) { c.CheckID = ""; c.CheckVersion = 0 }, "check identity"},
		"draft noncanonical check": {func(c *ResolutionCriterion) { c.Finalized = false; c.FinalizedAt = nil; c.CheckID = "bad/id" }, "canonical"},
		"draft missing revision":   {func(c *ResolutionCriterion) { c.Finalized = false; c.FinalizedAt = nil; c.CheckVersion = 0 }, "supplied together"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			c, _, _ := resolutionFixture()
			tt.mutate(&c)
			require.ErrorContains(t, c.Validate(), tt.want)
		})
	}
	c, p, at := resolutionFixture()
	c.Version = 0
	require.ErrorContains(t, ValidateResolutionProof(c, p, at), "positive version")
}
func TestResolutionProofRejectsInvalidRunsAndObservationReferences(t *testing.T) {
	tests := map[string]struct {
		mutate func(*ResolutionProof)
		want   string
	}{
		"invalid scope":         {func(p *ResolutionProof) { p.Scope.AffectedInputs = nil }, "affectedInputs"},
		"invalid run":           {func(p *ResolutionProof) { p.Run.RunID = "" }, "check run requires"},
		"nonUTC evaluated time": {func(p *ResolutionProof) { p.EvaluatedAt = p.EvaluatedAt.In(time.FixedZone("business", 3600)) }, "must be UTC"},
		"invalid observation":   {func(p *ResolutionProof) { p.ObservationRefs = []ArtifactRef{{Kind: RefEvent, ID: "bad/id"}} }, "canonical"},
		"invalid recovery":      {func(p *ResolutionProof) { p.RecoveryRefs = []ArtifactRef{{Kind: RefEvent, ID: ".."}} }, "canonical"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, p, at := resolutionFixture()
			tt.mutate(&p)
			require.ErrorContains(t, p.Validate(at), tt.want)
		})
	}
	c, p, at := resolutionFixture()
	finalized := at.Add(-time.Hour)
	c.FinalizedAt = &finalized
	p.EvaluatedAt = at.Add(-301 * time.Second)
	require.ErrorContains(t, ValidateResolutionProof(c, p, at), "stale")
	require.ErrorContains(t, (ResolvedPayload{HumanNote: " ", Proof: p}).Validate(at), "humanNote")
	// The Check-run identity stays distinct from an execution, even if its other identity is valid.
	require.ErrorContains(t, (ArtifactRef{Kind: RefCheckRun, Execution: &ExecutionRef{StoreID: "ops", ProjectID: "p", ExecutionID: "e"}}).Validate(), "requires CheckRunRef")
}
func TestResolutionEventRejectsMalformedPayloadsAndInvalidTransitions(t *testing.T) {
	c, p, at := resolutionFixture()
	ref := IncidentRef{StoreID: "ops", IncidentID: "incident-1"}
	for _, kind := range []EventType{EventIncidentCriterionSet, EventIncidentResolved} {
		t.Run(string(kind)+" malformed", func(t *testing.T) {
			e := eventWithPayload(t, ref, 2, at, kind, map[string]any{"unexpected": true})
			require.Error(t, e.Validate())
		})
	}
	e := eventWithPayload(t, ref, 2, at, EventIncidentResolved, ResolvedPayload{HumanNote: "", Proof: p})
	require.ErrorContains(t, e.Validate(), "humanNote")
	draft := c
	draft.Finalized = false
	draft.FinalizedAt = nil
	draft.Version = 1
	projection := Incident{Status: StatusResolved}
	require.ErrorContains(t, projection.applyResolutionEvent(eventWithPayload(t, ref, 2, at, EventIncidentCriterionSet, draft)), "already resolved")
	projection = Incident{Status: StatusOpen}
	require.ErrorContains(t, projection.applyResolutionEvent(eventWithPayload(t, ref, 2, at, EventIncidentCriterionSet, c)), "wall time")
	draft.Version = 2
	require.ErrorContains(t, projection.applyResolutionEvent(eventWithPayload(t, ref, 2, at, EventIncidentCriterionSet, draft)), "first criterion revision")
	draft.Version = 1
	projection.ResolutionCriterion = &draft
	require.ErrorContains(t, projection.applyResolutionEvent(eventWithPayload(t, ref, 2, at, EventIncidentCriterionSet, draft)), "advance by one")
	projection.ResolutionCriterion = nil
	resolved := eventWithPayload(t, ref, 2, at, EventIncidentResolved, ResolvedPayload{HumanNote: "Resolved", Proof: p})
	require.ErrorContains(t, projection.applyResolutionEvent(resolved), "requires a criterion")
	projection.ResolutionCriterion = &c
	p.Run.CheckVersion++
	resolved = eventWithPayload(t, ref, 2, at, EventIncidentResolved, ResolvedPayload{HumanNote: "Resolved", Proof: p})
	require.ErrorContains(t, projection.applyResolutionEvent(resolved), "revision mismatch")
}
func TestResolutionProjectionValidationAndDetachedCheckRunRef(t *testing.T) {
	c, p, _ := resolutionFixture()
	base := task3Projection("incident-1", "Investigation", StatusResolved)
	base.ResolutionCriterion = &c
	base.Resolution = &ResolvedPayload{HumanNote: "Resolved", Proof: p}
	require.NoError(t, base.Validate())
	base.Resolution.Proof.Run.CheckVersion++
	require.ErrorContains(t, base.Validate(), "revision mismatch")
	base.Resolution = nil
	base.ResolutionCriterion.Version = 0
	require.ErrorContains(t, base.Validate(), "positive version")
	base.ResolutionCriterion = nil
	base.Resolution = &ResolvedPayload{Proof: p}
	require.ErrorContains(t, base.Validate(), "humanNote")
	require.Nil(t, cloneResolutionInputs(nil))
	cloned := cloneArtifactRef(ArtifactRef{Kind: RefCheckRun, CheckRun: &p.Run})
	cloned.CheckRun.RunID = "other"
	require.Equal(t, "run-green", p.Run.RunID)
}
