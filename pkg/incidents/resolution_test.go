package incidents

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/stretchr/testify/require"
)

func resolutionFixture() (ResolutionCriterion, ResolutionProof, time.Time) {
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	finalized := at.Add(-time.Minute)
	c := ResolutionCriterion{Version: 2, Finalized: true, FinalizedAt: &finalized, Enforcement: "required", Scope: ResolutionScope{Project: ProjectRef{StoreID: "firestore", ProjectID: "training", Environment: "demo"}, SourceID: "shipping", Generation: "g1", AffectedInputs: map[string]investigation.TypedValue{"order": investigation.NewIntegerValue("42")}, SessionRef: "session-1", BagID: "bag-1", AppliedManifestRef: "manifest-1"}, CheckID: "assignment", CheckVersion: 3, MaxAgeSeconds: 300}
	p := ResolutionProof{CriterionVersion: c.Version, Scope: cloneResolutionScope(c.Scope), Run: CheckRunRef{StoreID: "firestore", ProjectID: "training", Environment: "demo", CheckID: c.CheckID, CheckVersion: 3, RunID: "run-green"}, EvaluatedAt: at.Add(-time.Second), Outcome: "pass", ObservationRefs: []ArtifactRef{{Kind: RefEvent, ID: "observed-current"}}, RecoveryRefs: []ArtifactRef{{Kind: RefEvent, ID: "replayed"}}}
	return c, p, at
}
func TestResolutionProofScopeRevisionFreshness(t *testing.T) {
	c, p, at := resolutionFixture()
	require.NoError(t, ValidateResolutionProof(c, p, at))
	cases := map[string]func(*ResolutionProof){
		"foreign project":  func(p *ResolutionProof) { p.Scope.Project.ProjectID = "original" },
		"generation":       func(p *ResolutionProof) { p.Scope.Generation = "g2" },
		"inputs":           func(p *ResolutionProof) { p.Scope.AffectedInputs["order"] = investigation.NewIntegerValue("43") },
		"input type":       func(p *ResolutionProof) { p.Scope.AffectedInputs["order"] = investigation.NewStringValue("42") },
		"manifest":         func(p *ResolutionProof) { p.Scope.AppliedManifestRef = "other" },
		"criterion":        func(p *ResolutionProof) { p.CriterionVersion++ },
		"check revision":   func(p *ResolutionProof) { p.Run.CheckVersion++ },
		"check identity":   func(p *ResolutionProof) { p.Run.CheckID = "other" },
		"run environment":  func(p *ResolutionProof) { p.Run.Environment = "prod" },
		"fail":             func(p *ResolutionProof) { p.Outcome = "fail" },
		"unknown":          func(p *ResolutionProof) { p.Outcome = "unknown" },
		"future":           func(p *ResolutionProof) { p.EvaluatedAt = at.Add(time.Second) },
		"stale":            func(p *ResolutionProof) { p.EvaluatedAt = at.Add(-10 * time.Minute) },
		"before finalized": func(p *ResolutionProof) { p.EvaluatedAt = at.Add(-2 * time.Minute) },
		"no observation":   func(p *ResolutionProof) { p.ObservationRefs = nil },
		"no recovery":      func(p *ResolutionProof) { p.RecoveryRefs = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			_, proof, _ := resolutionFixture()
			mutate(&proof)
			require.Error(t, ValidateResolutionProof(c, proof, at))
		})
	}
	c.Finalized = false
	c.FinalizedAt = nil
	require.Error(t, ValidateResolutionProof(c, p, at))
}
func TestRequiredCriterionFoldAndAtomicResolve(t *testing.T) {
	c, proof, at := resolutionFixture()
	ref := IncidentRef{StoreID: "ops", IncidentID: "incident-1"}
	draft := c
	draft.Version = 1
	draft.Finalized = false
	draft.FinalizedAt = nil
	draft.CheckID = ""
	draft.CheckVersion = 0
	events := []Event{createdEvent(t, ref, at.Add(-2*time.Minute)), eventWithPayload(t, ref, 2, at.Add(-90*time.Second), EventIncidentCriterionSet, draft), eventWithPayload(t, ref, 3, *c.FinalizedAt, EventIncidentCriterionSet, c)}
	for _, status := range []Status{StatusResolved, StatusWatching, StatusClosed} {
		t.Run(string(status), func(t *testing.T) {
			_, err := Fold(append(append([]Event(nil), events...), eventWithPayload(t, ref, 4, at, EventIncidentStatus, StatusPayload{Status: status})), nil)
			require.ErrorContains(t, err, "required resolution criterion")
		})
	}
	changed := c
	changed.Version++
	changed.FinalizedAt = &at
	_, err := Fold(append(append([]Event(nil), events...), eventWithPayload(t, ref, 4, at, EventIncidentCriterionSet, changed)), nil)
	require.ErrorContains(t, err, "immutable")
	resolved := eventWithPayload(t, ref, 4, at, EventIncidentResolved, ResolvedPayload{HumanNote: "Accepted in training; original unchanged", Proof: proof})
	resolved.Refs = append([]ArtifactRef{{Kind: RefCheckRun, CheckRun: &proof.Run}}, proof.ObservationRefs...)
	resolved.Refs = append(resolved.Refs, proof.RecoveryRefs...)
	projection, err := Fold(append(events, resolved), nil)
	require.NoError(t, err)
	require.Equal(t, StatusResolved, projection.Status)
	require.Equal(t, []string{"Accepted in training; original unchanged"}, projection.Notes)
	require.NotNil(t, projection.Resolution)
	view := ApplyIncidentView(projection, ViewPolicy{})
	require.NotNil(t, view.ResolutionCriterion)
	require.NotNil(t, view.Resolution)
	view.Resolution.Proof.Scope.AffectedInputs["order"] = investigation.NewIntegerValue("99")
	require.Equal(t, "42", projection.Resolution.Proof.Scope.AffectedInputs["order"].Str)
	hidden := ApplyIncidentView(projection, ViewPolicy{WithheldEvents: map[string]bool{resolved.ID: true, events[2].ID: true}})
	require.Nil(t, hidden.Resolution)
	require.Nil(t, hidden.ResolutionCriterion)
	require.Empty(t, hidden.Notes)
	resolved.Refs = nil
	require.Error(t, resolved.Validate())
	// Legacy ordinary incidents retain manual status semantics.
	_, err = Fold([]Event{createdEvent(t, ref, at.Add(-time.Minute)), eventWithPayload(t, ref, 2, at, EventIncidentStatus, StatusPayload{Status: StatusResolved})}, nil)
	require.NoError(t, err)
}
func TestCheckRunReferenceWire(t *testing.T) {
	_, p, _ := resolutionFixture()
	ref := ArtifactRef{Kind: RefCheckRun, CheckRun: &p.Run}
	data, err := json.Marshal(ref)
	require.NoError(t, err)
	var restored ArtifactRef
	require.NoError(t, json.Unmarshal(data, &restored))
	require.NoError(t, restored.Validate())
	require.Equal(t, ref, restored)
	restored.Execution = &ExecutionRef{StoreID: "a", ProjectID: "b", ExecutionID: "c"}
	require.Error(t, restored.Validate())
}
