package walkthrough

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTargetFixture(t *testing.T) *narrativeFixture {
	t.Helper()
	events := defaultNarrativeEvents + `,
{"id":"check-event","chapterId":"retries","type":"check.run","payload":{"checkId":"owner-check","referenceOutcome":"passed","referenceAssisted":false}},
{"id":"recommend-event","chapterId":"retries","type":"postmortem.recommend","payload":{"proposalId":"patch"}},
{"id":"focus-event","chapterId":"retries","type":"camera.focus","payload":{"targetId":"case-panel"}}`
	eventIDs := `["await-result","explain","check-event","recommend-event","focus-event"]`
	f := newNarrativeFixtureCatalogues(t, []catalogueFixture{{
		locale: "en",
		path:   "walkthroughs/training-demo/messages.en.json",
		data:   catalogueBytesFixture(""),
	}}, events, eventIDs, defaultNarrativeSegments)
	binding := f.owner.narrative.Binding
	f.owner.narrative.Checks = []string{"owner-check"}
	f.owner.narrative.Queries = []string{"queries/retries"}
	f.owner.narrative.Proposals = []string{"patch"}
	f.owner.narrative.PresentationTargets = []string{"case-panel"}
	f.owner.checks = map[string]CheckBinding{
		"owner-check": {
			Binding:           binding,
			CheckID:           "owner-check",
			DefinitionAssetID: "owner-check-definition",
		},
	}
	f.owner.proposals = map[string]ProposalBinding{
		"patch": {
			Binding:      binding,
			ProposalID:   "patch",
			Kind:         "query",
			DefinitionID: "queries/retries",
			EvidenceIDs:  []string{"evidence"},
			Rationale:    "Try the idempotency key.",
		},
	}
	f.owner.targets = map[string]PresentationTargetBinding{
		"case-panel": {
			Binding:  binding,
			TargetID: "case-panel",
			Kind:     "case",
			RefID:    "case-one",
			Closure:  &PresentationClosure{Kind: "case", ID: "case-one"},
		},
	}
	return f
}

func cloneFakeOwner(source *fakeOwner) *fakeOwner {
	clone := *source
	clone.packageAssets = append([]AssetReference{}, source.packageAssets...)
	clone.packageCases = append([]string{}, source.packageCases...)
	clone.admittedAssets = make(map[string]bool, len(source.admittedAssets))
	for key, value := range source.admittedAssets {
		clone.admittedAssets[key] = value
	}
	clone.narrative = snapshotNarrativeInventory(source.narrative)
	clone.checks = make(map[string]CheckBinding, len(source.checks))
	for key, value := range source.checks {
		clone.checks[key] = snapshotCheckBinding(value)
	}
	clone.proposals = make(map[string]ProposalBinding, len(source.proposals))
	for key, value := range source.proposals {
		clone.proposals[key] = snapshotProposalBinding(value)
	}
	clone.targets = make(map[string]PresentationTargetBinding, len(source.targets))
	for key, value := range source.targets {
		clone.targets[key] = snapshotPresentationTargetBinding(value)
	}
	return &clone
}

// rebuildFixture re-pins a declaration and bundle to mutated sidecar bytes.
// Any textual sidecar change alters the narrative byte pin, so a test that
// changes meaning must rebuild rather than reusing the old declaration.
func rebuildFixture(t *testing.T, f *narrativeFixture, sidecar []byte) *narrativeFixture {
	t.Helper()
	declaration := declarationBytesFixture(sidecar)
	parsed := ParseTrainingDeclaration("training-demo", declaration)
	require.True(t, parsed.Accepted, "%v", parsed.Issues)
	source := SourceContext{
		Project:           f.source.Project,
		ContentPin:        f.source.ContentPin,
		DeclarationSHA256: sha256Hex(declaration),
	}
	owner := cloneFakeOwner(f.owner)
	binding := OwnerBindingKey{
		Project:           source.Project,
		ContentPin:        source.ContentPin,
		DeclarationSHA256: source.DeclarationSHA256,
		Package:           f.packageRef,
	}
	owner.narrative.Binding = binding
	for key, value := range owner.checks {
		value.Binding = binding
		owner.checks[key] = value
	}
	for key, value := range owner.proposals {
		value.Binding = binding
		owner.proposals[key] = value
	}
	for key, value := range owner.targets {
		value.Binding = binding
		owner.targets[key] = value
	}
	narrativeRef := AssetReference{
		Path:   "walkthroughs/training-demo/narrative.json",
		SHA256: sha256Hex(sidecar),
		Bytes:  Integer(fmt.Sprint(len(sidecar))),
	}
	owner.admittedAssets[assetReferenceKey(narrativeRef)] = true
	resolution := ResolveTraining(parsed.Definition, source, f.authority, owner)
	require.True(t, resolution.Admitted, "%v", resolution.Issues)
	rebuilt := &narrativeFixture{
		declarationBytes: declaration,
		declaration:      parsed.Definition,
		sidecarBytes:     sidecar,
		catalogueBytes:   f.catalogueBytes,
		packageRef:       f.packageRef,
		owner:            owner,
		authority:        f.authority,
		bundle:           resolution.Bundle,
		source:           source,
	}
	return rebuilt
}

func admittedMinimal(t *testing.T, f *narrativeFixture) *AdmittedNarrative {
	t.Helper()
	result := AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.True(t, result.Admitted, "%v", result.Issues)
	require.NotNil(t, result.Narrative)
	return result.Narrative
}

func TestAdmitNarrativeMinimal(t *testing.T) {
	f := newNarrativeFixture(t)
	narrative := admittedMinimal(t, f)
	require.Equal(t, f.packageRef, narrative.Package)
	require.Equal(t, "en", narrative.Locale.SelectedLocale)
	require.Equal(t, "exact", narrative.Locale.Matched)
	expectedText := Integer(fmt.Sprint(len(f.sidecarBytes) + len(f.catalogueBytes) + 30))
	require.Equal(t, expectedText, narrative.TextBytes)
	require.Len(t, narrative.Events, 2)
	require.NotNil(t, narrative.Events[1].Message)
	require.Equal(t, "Repeated requests need an idempotency key.", narrative.Events[1].Message.Text)
	require.False(t, narrative.Events[1].Message.FromFallback)
	require.Equal(t, []NarrativeCatalogues{{Locale: "en", Values: map[string]string{
		"retry.explanation": "Repeated requests need an idempotency key.",
	}}}, narrative.Catalogues)

	projection := narrative.Projection()
	require.Equal(t, "training-demo", projection["scenarioId"])
	require.Equal(t, "1", projection["revision"])
	require.Equal(t, expectedText.String(), projection["textBytes"])
	require.Equal(t, []string{"en"}, projection["catalogueLocales"])
	require.Equal(t, []any{map[string]any{
		"eventId":      "explain",
		"key":          "retry.explanation",
		"text":         "Repeated requests need an idempotency key.",
		"locale":       "en",
		"fromFallback": false,
	}}, projection["messages"])
}

func TestAdmitNarrativeCheckProposalAndTargetClosure(t *testing.T) {
	f := newTargetFixture(t)
	narrative := admittedMinimal(t, f)
	require.Len(t, narrative.Events, 5)
	require.NotNil(t, narrative.Events[2].Check)
	require.Equal(t, "owner-check-definition", narrative.Events[2].Check.DefinitionAssetID)
	require.NotNil(t, narrative.Events[3].Proposal)
	require.Equal(t, []string{"evidence"}, narrative.Events[3].Proposal.EvidenceIDs)
	require.NotNil(t, narrative.Events[4].PresentationTarget)
	require.Equal(t, "case", narrative.Events[4].PresentationTarget.Kind)

	projection := narrative.Projection()
	require.Equal(t, []any{map[string]any{
		"checkId":           "owner-check",
		"definitionAssetId": "owner-check-definition",
	}}, projection["checks"])
	require.Equal(t, []any{map[string]any{
		"proposalId":   "patch",
		"kind":         "query",
		"definitionId": "queries/retries",
		"evidenceIds":  []string{"evidence"},
		"rationale":    "Try the idempotency key.",
	}}, projection["proposals"])
	require.Equal(t, []any{map[string]any{
		"targetId": "case-panel",
		"kind":     "case",
		"refId":    "case-one",
	}}, projection["presentationTargets"])
}

func TestAdmitNarrativeLocaleFallbackResolution(t *testing.T) {
	en := catalogueBytesFixture("")
	fr := []byte(`{"schemaVersion":1,"locale":"fr","messages":{"other.key":"Autre"}}`)
	f := newNarrativeFixtureCatalogues(t, []catalogueFixture{
		{locale: "en", path: "walkthroughs/training-demo/messages.en.json", data: en},
		{locale: "fr", path: "walkthroughs/training-demo/messages.fr.json", data: fr},
	}, defaultNarrativeEvents, defaultNarrativeEventIDs, defaultNarrativeSegments)
	request := "fr"
	input := NarrativeAdmissionInput{
		Sidecar: NarrativeAssetBytes{Reference: f.bundle.Narrative, Bytes: f.sidecarBytes},
		Catalogues: []NarrativeAssetBytes{
			{Reference: sidecarCatalogueReferenceAt(t, f.sidecarBytes, 0), Bytes: en},
			{Reference: sidecarCatalogueReferenceAt(t, f.sidecarBytes, 1), Bytes: fr},
		},
		Request: &request,
	}
	result := AdmitNarrative(f.bundle, f.authority, f.owner, input)
	require.True(t, result.Admitted, "%v", result.Issues)
	require.Equal(t, "fr", result.Narrative.Locale.SelectedLocale)
	require.Equal(t, "exact", result.Narrative.Locale.Matched)
	message := result.Narrative.Events[1].Message
	require.NotNil(t, message)
	require.Equal(t, "en", message.Locale)
	require.True(t, message.FromFallback)
	require.Equal(t, "Repeated requests need an idempotency key.", message.Text)

	// A missing fallback key refuses before any resolved projection.
	missing := []byte(`{"schemaVersion":1,"locale":"en","messages":{"other.key":"Other"}}`)
	missingSidecar := sidecarBytesWith(t, []catalogueFixture{{
		locale: "en", path: "walkthroughs/training-demo/messages.en.json", data: missing,
	}}, defaultNarrativeEvents, defaultNarrativeEventIDs, defaultNarrativeSegments)
	missingDeclaration := declarationBytesFixture(missingSidecar)
	parsed := ParseTrainingDeclaration("training-demo", missingDeclaration)
	require.True(t, parsed.Accepted, "%v", parsed.Issues)
	// Rebuild the fixture through the normal path to keep the declaration and
	// sidecar pins coherent.
	missingFixture := newNarrativeFixtureCatalogues(t, []catalogueFixture{{
		locale: "en", path: "walkthroughs/training-demo/messages.en.json", data: missing,
	}}, defaultNarrativeEvents, defaultNarrativeEventIDs, defaultNarrativeSegments)
	result = AdmitNarrative(missingFixture.bundle, missingFixture.authority, missingFixture.owner,
		admissionInputFor(t, missingFixture, missingFixture.sidecarBytes, missing))
	require.False(t, result.Admitted)
	require.Nil(t, result.Narrative)
	require.Equal(t, CodeInvalidReference, result.Issues[0].Code)
	require.Equal(t, "/events/1/payload/messageKey", result.Issues[0].Path)
	_ = parsed
}

func sidecarCatalogueReferenceAt(t *testing.T, sidecarBytes []byte, index int) AssetReference {
	t.Helper()
	parsed := ParseNarrativeSidecar(sidecarBytes)
	require.True(t, parsed.Accepted, "%v", parsed.Issues)
	require.Greater(t, len(parsed.Sidecar.MessageCatalogues), index)
	return parsed.Sidecar.MessageCatalogues[index].Asset()
}

func TestAdmitNarrativeRefusesMissingContext(t *testing.T) {
	f := newNarrativeFixture(t)
	input := admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes)
	cases := []struct {
		name      string
		bundle    *AdmittedTraining
		authority ProjectAuthority
		owners    NarrativeOwnerContext
	}{
		{"missing bundle", nil, f.authority, f.owner},
		{"missing authority", f.bundle, nil, f.owner},
		{"missing owners", f.bundle, f.authority, nil},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := AdmitNarrative(test.bundle, test.authority, test.owners, input)
			require.False(t, result.Admitted)
			require.Nil(t, result.Narrative)
			require.Equal(t, CodeMissingContext, result.Issues[0].Code)
		})
	}
}

func TestAdmitNarrativeRefusesSidecarAndBindingMismatch(t *testing.T) {
	f := newNarrativeFixture(t)
	tampered := []byte(strings.Replace(string(f.sidecarBytes), "Guide", "Guida", 1))
	result := AdmitNarrative(f.bundle, f.authority, f.owner, NarrativeAdmissionInput{
		Sidecar: NarrativeAssetBytes{Reference: f.bundle.Narrative, Bytes: tampered},
		Catalogues: []NarrativeAssetBytes{{
			Reference: sidecarCatalogueReference(t, f.sidecarBytes),
			Bytes:     f.catalogueBytes,
		}},
	})
	require.False(t, result.Admitted)
	require.Equal(t, CodeDigestMismatch, result.Issues[0].Code)

	otherScenario := rebuildFixture(t, f,
		[]byte(strings.Replace(string(f.sidecarBytes), `"scenarioId": "training-demo"`, `"scenarioId": "other-package"`, 1)))
	result = AdmitNarrative(otherScenario.bundle, otherScenario.authority, otherScenario.owner,
		admissionInputFor(t, otherScenario, otherScenario.sidecarBytes, otherScenario.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
	require.Equal(t, "/scenarioId", result.Issues[0].Path)

	otherRevision := rebuildFixture(t, f,
		[]byte(strings.Replace(string(f.sidecarBytes), `"revision": 1,`, `"revision": 2,`, 1)))
	result = AdmitNarrative(otherRevision.bundle, otherRevision.authority, otherRevision.owner,
		admissionInputFor(t, otherRevision, otherRevision.sidecarBytes, otherRevision.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
	require.Equal(t, "/revision", result.Issues[0].Path)

	otherChapters := rebuildFixture(t, f,
		[]byte(strings.ReplaceAll(string(f.sidecarBytes), `"retries"`, `"other"`)))
	result = AdmitNarrative(otherChapters.bundle, otherChapters.authority, otherChapters.owner,
		admissionInputFor(t, otherChapters, otherChapters.sidecarBytes, otherChapters.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
	require.Equal(t, "/chapterRefs", result.Issues[0].Path)
}

func TestAdmitNarrativeRefusesSegmentAndEventClosure(t *testing.T) {
	f := newNarrativeFixture(t)
	missingSegment := rebuildFixture(t, f,
		[]byte(strings.Replace(string(f.sidecarBytes), `"id":"discussion","chapterId"`, `"id":"different","chapterId"`, 1)))
	result := AdmitNarrative(missingSegment.bundle, missingSegment.authority, missingSegment.owner,
		admissionInputFor(t, missingSegment, missingSegment.sidecarBytes, missingSegment.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeInvalidReference, result.Issues[0].Code)

	unknownActor := rebuildFixture(t, f,
		[]byte(strings.Replace(string(f.sidecarBytes), `"actorId":"guide"`, `"actorId":"ghost"`, 1)))
	result = AdmitNarrative(unknownActor.bundle, unknownActor.authority, unknownActor.owner,
		admissionInputFor(t, unknownActor, unknownActor.sidecarBytes, unknownActor.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeInvalidReference, result.Issues[0].Code)
	require.Equal(t, "/events/1/payload/actorId", result.Issues[0].Path)

	unknownAttachment := rebuildFixture(t, f,
		[]byte(strings.Replace(string(f.sidecarBytes), `"attachmentIds":["photo"]`, `"attachmentIds":["ghost"]`, 1)))
	result = AdmitNarrative(unknownAttachment.bundle, unknownAttachment.authority, unknownAttachment.owner,
		admissionInputFor(t, unknownAttachment, unknownAttachment.sidecarBytes, unknownAttachment.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeInvalidReference, result.Issues[0].Code)
	require.Equal(t, "/events/1/payload/attachmentIds/0", result.Issues[0].Path)

	missingStep := rebuildFixture(t, f,
		[]byte(strings.Replace(string(f.sidecarBytes), `"stepId":"solve","checkpointId":"result"`, `"stepId":"ghost","checkpointId":"result"`, 1)))
	result = AdmitNarrative(missingStep.bundle, missingStep.authority, missingStep.owner,
		admissionInputFor(t, missingStep, missingStep.sidecarBytes, missingStep.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeInvalidReference, result.Issues[0].Code)

	badCheckpoint := rebuildFixture(t, f,
		[]byte(strings.Replace(string(f.sidecarBytes), `"stepId":"solve","checkpointId":"result"`, `"stepId":"solve","checkpointId":"other"`, 1)))
	result = AdmitNarrative(badCheckpoint.bundle, badCheckpoint.authority, badCheckpoint.owner,
		admissionInputFor(t, badCheckpoint, badCheckpoint.sidecarBytes, badCheckpoint.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
}

func TestAdmitNarrativeRefusesCatalogueMismatch(t *testing.T) {
	f := newNarrativeFixture(t)
	// Missing bytes.
	result := AdmitNarrative(f.bundle, f.authority, f.owner, NarrativeAdmissionInput{
		Sidecar: NarrativeAssetBytes{Reference: f.bundle.Narrative, Bytes: f.sidecarBytes},
	})
	require.False(t, result.Admitted)
	require.Equal(t, CodeMissingContext, result.Issues[0].Code)

	// Extra bytes.
	input := admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes)
	input.Catalogues = append(input.Catalogues, input.Catalogues[0])
	result = AdmitNarrative(f.bundle, f.authority, f.owner, input)
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)

	// Tampered catalogue bytes.
	tampered := append([]byte{}, f.catalogueBytes...)
	tampered[len(tampered)-2] = 'x'
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, tampered))
	require.False(t, result.Admitted)
	require.Equal(t, CodeDigestMismatch, result.Issues[0].Code)

	// Match a supplied reference but present a document with another locale.
	mismatchedLocale := []byte(`{"schemaVersion":1,"locale":"fr","messages":{"retry.explanation":"Bonjour"}}`)
	mismatchedSidecar := sidecarBytesWith(t, []catalogueFixture{{
		locale: "en", path: "walkthroughs/training-demo/messages.en.json", data: mismatchedLocale,
	}}, defaultNarrativeEvents, defaultNarrativeEventIDs, defaultNarrativeSegments)
	mismatchedFixture := newNarrativeFixtureCatalogues(t, []catalogueFixture{{
		locale: "en", path: "walkthroughs/training-demo/messages.en.json", data: mismatchedLocale,
	}}, defaultNarrativeEvents, defaultNarrativeEventIDs, defaultNarrativeSegments)
	result = AdmitNarrative(mismatchedFixture.bundle, mismatchedFixture.authority, mismatchedFixture.owner,
		admissionInputFor(t, mismatchedFixture, mismatchedSidecar, mismatchedLocale))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
}

func TestAdmitNarrativeRefusesMissingOwnerBindings(t *testing.T) {
	f := newTargetFixture(t)
	f.owner.checks = map[string]CheckBinding{}
	result := AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeNotAdmitted, result.Issues[0].Code)
	require.Equal(t, "/events/2/payload/checkId", result.Issues[0].Path)

	f = newTargetFixture(t)
	f.owner.proposals = map[string]ProposalBinding{}
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeNotAdmitted, result.Issues[0].Code)

	f = newTargetFixture(t)
	delete(f.owner.targets, "case-panel")
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeNotAdmitted, result.Issues[0].Code)
}

func TestAdmitNarrativeRefusesBindingMismatch(t *testing.T) {
	f := newTargetFixture(t)
	otherBinding := f.owner.narrative.Binding
	otherBinding.Package = PackageReference{ID: "other", Revision: "1", Path: "training/package.json", SHA256: strings.Repeat("a", 64), Bytes: "100"}
	check := f.owner.checks["owner-check"]
	check.Binding = otherBinding
	f.owner.checks["owner-check"] = check
	result := AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)

	f = newTargetFixture(t)
	badDefinition := f.owner.checks["owner-check"]
	badDefinition.DefinitionAssetID = "evidence"
	f.owner.checks["owner-check"] = badDefinition
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeNotAdmitted, result.Issues[0].Code)

	f = newTargetFixture(t)
	badProposal := f.owner.proposals["patch"]
	badProposal.Kind = "incident"
	f.owner.proposals["patch"] = badProposal
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeInvalidValue, result.Issues[0].Code)

	f = newTargetFixture(t)
	badEvidence := f.owner.proposals["patch"]
	badEvidence.EvidenceIDs = []string{"owner-check-definition"}
	f.owner.proposals["patch"] = badEvidence
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeInvalidReference, result.Issues[0].Code)

	f = newTargetFixture(t)
	target := f.owner.targets["case-panel"]
	target.Closure = nil
	f.owner.targets["case-panel"] = target
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)

	f = newTargetFixture(t)
	target = f.owner.targets["case-panel"]
	target.Closure = &PresentationClosure{Kind: "evidence", ID: "case-one"}
	f.owner.targets["case-panel"] = target
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
}

func TestAdmitNarrativeRefusesInventoryDrift(t *testing.T) {
	f := newNarrativeFixture(t)
	f.owner.mutateOnSecondRead = true
	result := AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Nil(t, result.Narrative)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
	require.Contains(t, result.Issues[0].Message, "changed during admission")
}

func TestAdmitNarrativeRefusesInventoryMismatch(t *testing.T) {
	f := newNarrativeFixture(t)
	f.owner.packageCases = []string{"other-case"}
	result := AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)

	f = newNarrativeFixture(t)
	f.owner.narrative.Assets = f.owner.narrative.Assets[:1]
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)

	f = newNarrativeFixture(t)
	f.owner.narrative.Binding.Project = ProjectIdentity{Value: "other-project"}
	result = AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
}

func TestAdmitNarrativeTextAccounting(t *testing.T) {
	f := newNarrativeFixture(t)
	narrative := admittedMinimal(t, f)
	total, err := strconv.Atoi(string(narrative.TextBytes))
	require.NoError(t, err)
	base := total - len(f.sidecarBytes) - len(f.catalogueBytes)
	require.Equal(t, 30, base)

	// Identical source+path aliases count once only.
	alias := newNarrativeFixture(t)
	aliasRef := alias.owner.narrative.Assets[0].Reference
	alias.owner.packageAssets = append(alias.owner.packageAssets, aliasRef)
	alias.owner.narrative.Assets = append(alias.owner.narrative.Assets,
		PackageAssetBinding{AssetID: "evidence-alias", Kind: "evidence-text", Reference: aliasRef, Bytes: aliasRef.Bytes})
	narrative = admittedMinimal(t, alias)
	require.Equal(t, Integer(fmt.Sprint(len(alias.sidecarBytes)+len(alias.catalogueBytes)+30)), narrative.TextBytes)

	// Different paths count separately even with equal bytes and digest.
	different := newNarrativeFixture(t)
	secondRef := different.owner.narrative.Assets[0].Reference
	secondRef.Path = "training/evidence-2.txt"
	different.owner.packageAssets = append(different.owner.packageAssets, secondRef)
	different.owner.narrative.Assets = append(different.owner.narrative.Assets,
		PackageAssetBinding{AssetID: "evidence-second", Kind: "evidence-text", Reference: secondRef, Bytes: secondRef.Bytes})
	narrative = admittedMinimal(t, different)
	require.Equal(t, Integer(fmt.Sprint(len(different.sidecarBytes)+len(different.catalogueBytes)+40)), narrative.TextBytes)

	// Conflicting pins at one canonical source+path refuse.
	conflict := newNarrativeFixture(t)
	conflictRef := conflict.owner.narrative.Assets[0].Reference
	conflictRef.SHA256 = strings.Repeat("e", 64)
	conflict.owner.packageAssets = append(conflict.owner.packageAssets, conflictRef)
	conflict.owner.narrative.Assets = append(conflict.owner.narrative.Assets,
		PackageAssetBinding{AssetID: "evidence-conflict", Kind: "evidence-text", Reference: conflictRef, Bytes: conflictRef.Bytes})
	result := AdmitNarrative(conflict.bundle, conflict.authority, conflict.owner,
		admissionInputFor(t, conflict, conflict.sidecarBytes, conflict.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)

	// A text asset over 64 KiB refuses.
	over := newNarrativeFixture(t)
	overRef := AssetReference{Path: "training/large.txt", SHA256: strings.Repeat("f", 64), Bytes: Integer(fmt.Sprint(MaxTextAssetBytes + 1))}
	over.owner.packageAssets = append(over.owner.packageAssets, overRef)
	over.owner.narrative.Assets = append(over.owner.narrative.Assets,
		PackageAssetBinding{AssetID: "large-text", Kind: "evidence-text", Reference: overRef, Bytes: overRef.Bytes})
	result = AdmitNarrative(over.bundle, over.authority, over.owner,
		admissionInputFor(t, over, over.sidecarBytes, over.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeTooLong, result.Issues[0].Code)
}

func TestAdmitNarrativeCombinedTextBudget(t *testing.T) {
	f := newNarrativeFixture(t)
	for index := 0; index < 100; index++ {
		ref := AssetReference{
			Path:   fmt.Sprintf("training/text-%03d.txt", index),
			SHA256: fmt.Sprintf("%064x", index+1),
			Bytes:  Integer(fmt.Sprint(MaxTextAssetBytes)),
		}
		f.owner.packageAssets = append(f.owner.packageAssets, ref)
		f.owner.narrative.Assets = append(f.owner.narrative.Assets,
			PackageAssetBinding{
				AssetID:   fmt.Sprintf("text-%03d", index),
				Kind:      "evidence-text",
				Reference: ref,
				Bytes:     ref.Bytes,
			})
	}
	result := AdmitNarrative(f.bundle, f.authority, f.owner,
		admissionInputFor(t, f, f.sidecarBytes, f.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeTooLarge, result.Issues[0].Code)
}

func TestAdmitNarrativeSnapshotDoesNotAliasOwner(t *testing.T) {
	f := newTargetFixture(t)
	narrative := admittedMinimal(t, f)
	before := narrative.Projection()

	// Mutate every nested owner structure after admission. The admitted
	// narrative must keep its own copies.
	f.owner.narrative.Cases[0] = "mutated-case"
	f.owner.narrative.Assets[0].Reference.Path = "training/mutated.txt"
	f.owner.narrative.Assets[0].Reference.SHA256 = strings.Repeat("0", 64)
	check := f.owner.checks["owner-check"]
	check.CheckID = "mutated-check"
	check.DefinitionAssetID = "mutated-definition"
	f.owner.checks["owner-check"] = check
	proposal := f.owner.proposals["patch"]
	proposal.EvidenceIDs[0] = "mutated-evidence"
	f.owner.proposals["patch"] = proposal
	target := f.owner.targets["case-panel"]
	target.Closure.Kind = "mutated"
	target.RefID = "mutated-target"
	f.owner.targets["case-panel"] = target

	after := narrative.Projection()
	require.Equal(t, before, after)
	require.Equal(t, "owner-check", narrative.Events[2].Check.CheckID)
	require.Equal(t, []string{"evidence"}, narrative.Events[3].Proposal.EvidenceIDs)
	require.Equal(t, "case", narrative.Events[4].PresentationTarget.Closure.Kind)
}

func TestAdmitNarrativeCheckpointAndBranchReferences(t *testing.T) {
	f := newNarrativeFixture(t)
	branchSidecar := []byte(strings.Replace(
		strings.Replace(string(f.sidecarBytes),
			`"eventIds":["await-result","explain"]`, `"eventIds":["branch-event","explain"]`, 1),
		`{"id":"await-result","chapterId":"retries","type":"challenge.await_validation","payload":{"stepId":"solve","checkpointId":"result"}}`,
		`{"id":"branch-event","chapterId":"retries","type":"branch","payload":{"stepId":"solve","checkpointId":"result","outcome":"passed","assisted":true,"targetEventId":"explain"}}`, 1))
	branchFixture := rebuildFixture(t, f, branchSidecar)
	result := AdmitNarrative(branchFixture.bundle, branchFixture.authority, branchFixture.owner,
		admissionInputFor(t, branchFixture, branchFixture.sidecarBytes, branchFixture.catalogueBytes))
	require.True(t, result.Admitted, "%v", result.Issues)

	// A branch outcome outside the closed registry is refused by the parser.
	badOutcome := rebuildFixture(t, f,
		[]byte(strings.Replace(string(branchSidecar), `"outcome":"passed"`, `"outcome":"maybe"`, 1)))
	result = AdmitNarrative(badOutcome.bundle, badOutcome.authority, badOutcome.owner,
		NarrativeAdmissionInput{
			Sidecar: NarrativeAssetBytes{Reference: badOutcome.bundle.Narrative, Bytes: badOutcome.sidecarBytes},
			Catalogues: []NarrativeAssetBytes{{
				Reference: sidecarCatalogueReference(t, f.sidecarBytes),
				Bytes:     f.catalogueBytes,
			}},
		})
	require.False(t, result.Admitted)
	require.Equal(t, CodeInvalidValue, result.Issues[0].Code)
}
