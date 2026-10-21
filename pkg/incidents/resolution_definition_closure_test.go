package incidents_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/incidents"
	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/datatug/datatug-core/pkg/sessioncontext"
	"github.com/stretchr/testify/require"
)

func definitionResolutionFixture() (incidents.ResolutionCriterion, incidents.ResolutionProof, time.Time) {
	at := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	finalized := at.Add(-time.Minute)
	scope := incidents.ResolutionScope{Project: incidents.ProjectRef{StoreID: "firestore", ProjectID: "training", Environment: "demo"}, SourceID: "shipping", Generation: "g1", AffectedInputs: map[string]investigation.TypedValue{"order": investigation.NewIntegerValue("11045")}, SessionRef: "session-1", BagID: "bag-1", AppliedManifestRef: "manifest-1"}
	c := incidents.ResolutionCriterion{Version: 1, Finalized: true, FinalizedAt: &finalized, Enforcement: "required", Scope: scope, CheckID: "assignment", CheckVersion: 3, MaxAgeSeconds: 300}
	p := incidents.ResolutionProof{CriterionVersion: 1, Scope: scope, Run: incidents.CheckRunRef{StoreID: "firestore", ProjectID: "training", Environment: "demo", CheckID: "assignment", CheckVersion: 3, RunID: "green"}, EvaluatedAt: at, Outcome: "pass", ObservationRefs: []incidents.ArtifactRef{{Kind: incidents.RefEvent, ID: "observed"}}, RecoveryRefs: []incidents.ArtifactRef{{Kind: incidents.RefEvent, ID: "replay"}}}
	return c, p, at
}

func resolutionDefinition() sessioncontext.DetectiveCheckDefinitionPin {
	return sessioncontext.DetectiveCheckDefinitionPin{
		SchemaVersion:       1,
		Check:               detective.Check{ID: "assignment", Title: "Assignment", Version: 3, Steps: []detective.Step{{ID: "child", Kind: "check", CheckID: "child"}}},
		Dependencies:        []detective.Check{{ID: "child", Title: "Child", Version: 1, Steps: []detective.Step{{ID: "query", Kind: "query", QueryID: "shipping/assignment", Validators: []detective.Validator{{ID: "published", ValidatorID: "accepted"}}}}}},
		PublishedValidators: []detective.PublishedValidator{{ID: "accepted", Title: "Accepted", Version: 1, Validator: detective.Validator{ID: "accepted", Kind: "non-null", Scope: "row", Severity: "error", Field: "assignment"}}},
		QueryValidators:     []detective.QueryValidators{{QueryID: "shipping/assignment", Version: 1, Validators: []detective.Validator{{ID: "local", Kind: "non-null", Scope: "row", Severity: "error", Field: "assignment"}}}},
		Queries:             []sessioncontext.DetectiveAcceptedQueryPin{{QueryID: "shipping/assignment", Revision: "q1", DefinitionJSON: json.RawMessage(`{"id":"assignment","folderPath":"shipping","title":"Assignment","type":"SQL","text":"select assignment from assignments"}`)}},
	}
}

func TestResolutionPinsActualCanonicalExecutableClosure(t *testing.T) {
	digest := func(d sessioncontext.DetectiveCheckDefinitionPin) string {
		t.Helper()
		_, hash, err := sessioncontext.CanonicalDetectiveDefinition(context.Background(), d)
		require.NoError(t, err)
		return hash
	}
	c, p, at := definitionResolutionFixture()
	c.DefinitionDigest = digest(resolutionDefinition())
	p.DefinitionDigest = c.DefinitionDigest
	require.NoError(t, incidents.ValidateResolutionProof(c, p, at))
	for name, change := range map[string]func(*sessioncontext.DetectiveCheckDefinitionPin){
		"query body": func(d *sessioncontext.DetectiveCheckDefinitionPin) {
			d.Queries[0].DefinitionJSON = json.RawMessage(strings.ReplaceAll(string(d.Queries[0].DefinitionJSON), "select assignment", "select distinct assignment"))
		},
		"query revision":   func(d *sessioncontext.DetectiveCheckDefinitionPin) { d.Queries[0].Revision = "q2" },
		"child definition": func(d *sessioncontext.DetectiveCheckDefinitionPin) { d.Dependencies[0].Title = "Changed child" },
		"published validator": func(d *sessioncontext.DetectiveCheckDefinitionPin) {
			d.PublishedValidators[0].Validator.Field = "other"
		},
		"query validator": func(d *sessioncontext.DetectiveCheckDefinitionPin) {
			d.QueryValidators[0].Validators[0].Field = "other"
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := resolutionDefinition()
			change(&d)
			require.Equal(t, c.CheckVersion, uint64(d.Check.Version))
			changed := p
			changed.DefinitionDigest = digest(d)
			require.NotEqual(t, c.DefinitionDigest, changed.DefinitionDigest)
			require.ErrorContains(t, incidents.ValidateResolutionProof(c, changed, at), "digest mismatch")
		})
	}
	// Recovery changes run/observation identities and time, not the admitted definition.
	p.Run.RunID = "replay-green"
	p.ObservationRefs = []incidents.ArtifactRef{{Kind: incidents.RefEvent, ID: "configuration-18-attempt-2"}}
	p.EvaluatedAt = at
	require.Equal(t, c.DefinitionDigest, digest(resolutionDefinition()))
	require.NoError(t, incidents.ValidateResolutionProof(c, p, at))
}
