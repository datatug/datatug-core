package walkthrough

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func allEventSidecar(t *testing.T) []byte {
	t.Helper()
	events := `{"id":"await-result","chapterId":"retries","type":"challenge.await_validation","payload":{"stepId":"solve","checkpointId":"result"}},
{"id":"prompt","chapterId":"retries","type":"challenge.prompt","payload":{"stepId":"solve"}},
{"id":"case-create","chapterId":"retries","type":"detective.create","payload":{"manifestCaseId":"case-one"}},
{"id":"case-open","chapterId":"retries","type":"detective.open","payload":{"manifestCaseId":"case-one"}},
{"id":"reveal","chapterId":"retries","type":"evidence.reveal","payload":{"assetId":"photo"}},
{"id":"check","chapterId":"retries","type":"check.run","payload":{"checkId":"owner-check","referenceOutcome":"failed","referenceAssisted":true}},
{"id":"recommend","chapterId":"retries","type":"postmortem.recommend","payload":{"proposalId":"patch"}},
{"id":"focus","chapterId":"retries","type":"camera.focus","payload":{"targetId":"case-panel"}},
{"id":"chat","chapterId":"retries","type":"chat.message","payload":{"actorId":"guide","messageKey":"retry.explanation","attachmentIds":["photo"]},"presentation":{"typingSpeed":42,"pauseAfterMs":5}},
{"id":"branch","chapterId":"retries","type":"branch","payload":{"stepId":"solve","checkpointId":"result","outcome":"timed-out","assisted":true,"targetEventId":"checkpoint"}},
{"id":"checkpoint","chapterId":"retries","type":"checkpoint","payload":{"stateKey":"resume"}}`
	eventIDs := `["await-result","prompt","case-create","case-open","reveal","check","recommend","focus","chat","branch","checkpoint"]`
	segments := `{"id":"discussion","chapterId":"retries","stepId":"discussion","eventIds":` + eventIDs + `,"prerequisiteContextEventIds":[]}`
	return sidecarBytesWith(t, []catalogueFixture{{
		locale: "en",
		path:   "walkthroughs/training-demo/messages.en.json",
		data:   catalogueBytesFixture(""),
	}}, events, eventIDs, segments)
}

func TestNarrativeParserCoversEveryEventType(t *testing.T) {
	sidecar := allEventSidecar(t)
	result := ParseNarrativeSidecar(sidecar)
	require.True(t, result.Accepted, "%v", result.Issues)
	require.Len(t, result.Sidecar.Events, 11)
	projection := result.Sidecar.Projection()
	events, ok := projection["events"].([]any)
	require.True(t, ok)
	require.Len(t, events, 11)
	require.Equal(t, map[string]any{
		"typingSpeed":     42,
		"pauseAfterMs":    5,
		"delayBeforeMs":   0,
		"typingIndicator": false,
		"transition":      "none",
		"cameraTargetId":  "",
	}, events[8].(map[string]any)["presentation"])

	// Every payload projection is populated.
	for _, index := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10} {
		event := events[index].(map[string]any)
		require.NotNil(t, event["payload"], index)
	}
	require.Equal(t, "timed-out", events[9].(map[string]any)["payload"].(map[string]any)["outcome"])
	require.Equal(t, "resume", events[10].(map[string]any)["payload"].(map[string]any)["stateKey"])
}

func TestNarrativeParserCoversShapeBranches(t *testing.T) {
	valid := string(allEventSidecar(t))
	cases := []struct {
		name string
		data string
		code Code
		path string
	}{
		{"missing type", strings.Replace(valid, `"type":"challenge.prompt",`, ``, 1), CodeMissingField, "/events/1/type"},
		{"type not string", strings.Replace(valid, `"type":"challenge.prompt"`, `"type":1`, 1), CodeInvalidType, "/events/1/type"},
		{"missing payload", strings.Replace(valid, `,"payload":{"stepId":"solve"}`, ``, 1), CodeMissingField, "/events/1/payload"},
		{"payload not object", strings.Replace(valid, `"payload":{"stepId":"solve"}`, `"payload":[]`, 1), CodeInvalidType, "/events/1/payload"},
		{"presentation not object", strings.Replace(valid, `"presentation":{"typingSpeed":42,"pauseAfterMs":5}`, `"presentation":[]`, 1), CodeInvalidType, "/events/8/presentation"},
		{"typing speed wrong type", strings.Replace(valid, `"typingSpeed":42`, `"typingSpeed":true`, 1), CodeInvalidType, "/events/8/presentation/typingSpeed"},
		{"typing indicator wrong type", strings.Replace(valid, `"pauseAfterMs":5`, `"pauseAfterMs":5,"typingIndicator":1`, 1), CodeInvalidType, "/events/8/presentation/typingIndicator"},
		{"transition wrong type", strings.Replace(valid, `"pauseAfterMs":5`, `"pauseAfterMs":5,"transition":1`, 1), CodeInvalidType, "/events/8/presentation/transition"},
		{"delay wrong type", strings.Replace(valid, `"pauseAfterMs":5`, `"pauseAfterMs":5,"delayBeforeMs":true`, 1), CodeInvalidType, "/events/8/presentation/delayBeforeMs"},
		{"delay not canonical", strings.Replace(valid, `"pauseAfterMs":5`, `"pauseAfterMs":5,"delayBeforeMs":1.0`, 1), CodeInvalidValue, "/events/8/presentation/delayBeforeMs"},
		{"catalogue ref not object", strings.Replace(valid, `"messageCatalogues": [{`, `"messageCatalogues": [1,{`, 1), CodeInvalidType, "/messageCatalogues/0"},
		{"segment not object", strings.Replace(valid, `"segments": [{`, `"segments": [1,{`, 1), CodeInvalidType, "/segments/0"},
		{"event not object", strings.Replace(valid, `"events": [{`, `"events": [1,{`, 1), CodeInvalidType, "/events/0"},
		{"actor not object", strings.Replace(valid, `"actors": [{"id":"guide","name":"Guide"}]`, `"actors": [1]`, 1), CodeInvalidType, "/actors/0"},
		{"asset not object", strings.Replace(valid, `"assets": [{"id":"photo"`, `"assets": [1,{"id":"photo"`, 1), CodeInvalidType, "/assets/0"},
		{"bad message key", strings.Replace(valid, `"messageKey":"retry.explanation"`, `"messageKey":"Bad Key"`, 1), CodeInvalidValue, "/events/8/payload/messageKey"},
		{"empty check id", strings.Replace(valid, `"checkId":"owner-check"`, `"checkId":""`, 1), CodeInvalidValue, "/events/5/payload/checkId"},
		{"missing branch target", strings.Replace(valid, `"targetEventId":"checkpoint"`, `"targetEventId":"ghost"`, 1), CodeInvalidReference, "/events/9/payload/targetEventId"},
		{"unknown branch field", strings.Replace(valid, `"targetEventId":"checkpoint"`, `"targetEventId":"checkpoint","x":1`, 1), CodeUnknownField, "/events/9/payload/x"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			result := ParseNarrativeSidecar([]byte(test.data))
			require.False(t, result.Accepted)
			require.NotEmpty(t, result.Issues)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
			require.Equal(t, test.path, result.Issues[0].Path, "%v", result.Issues)
		})
	}
}

func TestNarrativeParserCoversPrerequisiteAndSegmentBranches(t *testing.T) {
	catalogue := catalogueBytesFixture("")
	base := sidecarBytesWith(t, []catalogueFixture{{
		locale: "en", path: "walkthroughs/training-demo/messages.en.json", data: catalogue,
	}}, defaultNarrativeEvents, defaultNarrativeEventIDs, defaultNarrativeSegments)
	valid := string(base)

	cases := []struct {
		name string
		data string
		code Code
		path string
	}{
		{
			"prerequisite later",
			strings.Replace(valid, `"prerequisiteContextEventIds":[]`, `"prerequisiteContextEventIds":["explain"]`, 1),
			CodeInvalidReference, "/segments/0/prerequisiteContextEventIds/0",
		},
		{
			"prerequisite repeated",
			func() string {
				withContext := strings.Replace(valid, `"events": [`, `"events": [{"id":"context","chapterId":"retries","type":"checkpoint","payload":{"stateKey":"context"}},`, 1)
				return strings.Replace(withContext, `"prerequisiteContextEventIds":[]`, `"prerequisiteContextEventIds":["context","context"]`, 1)
			}(),
			CodeDuplicateID, "/segments/0/prerequisiteContextEventIds/1",
		},
		{
			"prerequisite unknown",
			strings.Replace(valid, `"prerequisiteContextEventIds":[]`, `"prerequisiteContextEventIds":["ghost"]`, 1),
			CodeInvalidReference, "/segments/0/prerequisiteContextEventIds/0",
		},
		{
			"segment event unknown",
			strings.Replace(valid, `"eventIds":["await-result","explain"]`, `"eventIds":["await-result","ghost"]`, 1),
			CodeInvalidReference, "/segments/0/eventIds/1",
		},
		{
			"segment event repeated",
			strings.Replace(valid, `"eventIds":["await-result","explain"]`, `"eventIds":["await-result","await-result"]`, 1),
			CodeDuplicateID, "/segments/0/eventIds/1",
		},
		{
			"segment event chapter",
			strings.Replace(valid, `"chapterId":"retries","type":"challenge.await_validation"`, `"chapterId":"other","type":"challenge.await_validation"`, 1),
			CodeInvalidReference, "/events/0/chapterId",
		},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			result := ParseNarrativeSidecar([]byte(test.data))
			require.False(t, result.Accepted)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
			require.Equal(t, test.path, result.Issues[0].Path, "%v", result.Issues)
		})
	}
}

func TestNarrativeResultErrors(t *testing.T) {
	accepted := NarrativeResult{Accepted: true}
	require.NoError(t, accepted.Err())
	refused := NarrativeResult{Issues: []Issue{{Code: CodeInvalidValue}}}
	require.Error(t, refused.Err())
	catalogueAccepted := CatalogueResult{Accepted: true}
	require.NoError(t, catalogueAccepted.Err())
	catalogueRefused := CatalogueResult{Issues: []Issue{{Code: CodeInvalidValue}}}
	require.Error(t, catalogueRefused.Err())
	localeAccepted := NarrativeLocaleResult{Accepted: true}
	require.NoError(t, localeAccepted.Err())
	localeRefused := NarrativeLocaleResult{Issues: []Issue{{Code: CodeInvalidValue}}}
	require.Error(t, localeRefused.Err())
	resolutionAccepted := NarrativeResolution{Admitted: true}
	require.NoError(t, resolutionAccepted.Err())
	resolutionRefused := NarrativeResolution{Issues: []Issue{{Code: CodeInvalidValue}}}
	require.Error(t, resolutionRefused.Err())
}

func TestAccountNarrativeTextBranches(t *testing.T) {
	_, issues := accountNarrativeText([]narrativeTextItem{{
		path: "a", sha256: "a", pinnedBytes: "not-a-number", actualBytes: 1,
	}})
	require.Equal(t, CodeInvalidValue, issues[0].Code)

	_, issues = accountNarrativeText([]narrativeTextItem{{
		path: "a", sha256: "a", pinnedBytes: "2", actualBytes: 1,
	}})
	require.Equal(t, CodeByteCountMismatch, issues[0].Code)

	_, issues = accountNarrativeText([]narrativeTextItem{
		{path: "a", sha256: "a", pinnedBytes: "1", actualBytes: 1},
		{path: "a", sha256: "a", pinnedBytes: "1", actualBytes: 1},
	})
	require.Empty(t, issues)

	_, issues = accountNarrativeText([]narrativeTextItem{
		{path: "a", sha256: "a", pinnedBytes: "1", actualBytes: 1},
		{path: "a", sha256: "b", pinnedBytes: "1", actualBytes: 1},
	})
	require.Equal(t, CodeMismatch, issues[0].Code)
}

func TestIntegerAsIntBranches(t *testing.T) {
	_, ok := integerAsInt("12x", 100)
	require.False(t, ok)
	_, ok = integerAsInt("999", 100)
	require.False(t, ok)
	value, ok := integerAsInt("99", 100)
	require.True(t, ok)
	require.Equal(t, int64(99), value)
	require.Equal(t, "0", itoa(0))
	require.Equal(t, "42", itoa(42))
}

func TestNarrativeLexicalHelpers(t *testing.T) {
	require.True(t, isEvidenceKind("evidence-log"))
	require.False(t, isEvidenceKind("check-definition"))
	require.True(t, knownPackageKind("challenge-fixture"))
	require.False(t, knownPackageKind("script"))
	require.True(t, isPackageTextKind("evidence-text"))
	require.False(t, isPackageTextKind("challenge-fixture"))
	require.False(t, positiveIntegerLexeme("0"))
}

func TestAdmitNarrativeRefusesMalformedInventory(t *testing.T) {
	base := newNarrativeFixture(t)
	cases := []struct {
		name   string
		mutate func(*fakeOwner)
		code   Code
	}{
		{"bad asset id", func(owner *fakeOwner) { owner.narrative.Assets[0].AssetID = "Bad ID" }, CodeInvalidValue},
		{"duplicate asset id", func(owner *fakeOwner) {
			owner.packageAssets = append(owner.packageAssets, owner.narrative.Assets[0].Reference)
			owner.narrative.Assets = append(owner.narrative.Assets, owner.narrative.Assets[0])
		}, CodeDuplicateID},
		{"unknown kind", func(owner *fakeOwner) { owner.narrative.Assets[0].Kind = "script" }, CodeInvalidValue},
		{"bad path", func(owner *fakeOwner) { owner.narrative.Assets[0].Reference.Path = "/absolute" }, CodeInvalidValue},
		{"bad digest", func(owner *fakeOwner) { owner.narrative.Assets[0].Reference.SHA256 = "XYZ" }, CodeInvalidValue},
		{"bad bytes", func(owner *fakeOwner) { owner.narrative.Assets[0].Reference.Bytes = "0" }, CodeInvalidValue},
		{"bytes disagree", func(owner *fakeOwner) { owner.narrative.Assets[0].Bytes = "11" }, CodeByteCountMismatch},
		{"duplicate cases", func(owner *fakeOwner) {
			owner.narrative.Cases = []string{"case-one", "case-one"}
			owner.packageCases = owner.narrative.Cases
		}, CodeDuplicateID},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNarrativeFixture(t)
			test.mutate(fixture.owner)
			result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
				admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
			require.False(t, result.Admitted)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
		})
	}
	_ = base
}

func TestAdmitNarrativeRefusesInventoryReferenceMismatch(t *testing.T) {
	fixture := newNarrativeFixture(t)
	fixture.owner.narrative.Assets = append(fixture.owner.narrative.Assets, PackageAssetBinding{
		AssetID: "extra", Kind: "evidence-text",
		Reference: AssetReference{Path: "training/extra.txt", SHA256: strings.Repeat("e", 64), Bytes: "1"},
		Bytes:     "1",
	})
	result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
		admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
}

func TestAdmitNarrativeRefusesUnboundProjectAndPin(t *testing.T) {
	fixture := newNarrativeFixture(t)
	noProject := fixture.owner.narrative.Binding
	noProject.Project = ProjectIdentity{}
	fixture.owner.narrative.Binding = noProject
	result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
		admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)

	fixture = newNarrativeFixture(t)
	otherPin := fixture.owner.narrative.Binding
	otherPin.ContentPin = ContentPin{Kind: PinGit, Commit: strings.Repeat("f", 40)}
	fixture.owner.narrative.Binding = otherPin
	result = AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
		admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
}

func TestAdmitNarrativeRefusesCatalogueAdmissionErrors(t *testing.T) {
	fixture := newNarrativeFixture(t)
	// Unknown owner asset.
	input := admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes)
	delete(fixture.owner.admittedAssets, assetReferenceKey(input.Catalogues[0].Reference))
	result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner, input)
	require.False(t, result.Admitted)
	require.Equal(t, CodeNotAdmitted, result.Issues[0].Code)

	// Catalogue parser refusal reached through admission. Keep the byte count
	// equal to the pin so the digest check is the first refusal.
	refusedFixture := newNarrativeFixture(t)
	bad := append([]byte{}, refusedFixture.catalogueBytes...)
	bad[len(bad)-2] = 'x'
	result = AdmitNarrative(refusedFixture.bundle, refusedFixture.authority, refusedFixture.owner,
		NarrativeAdmissionInput{
			Sidecar: NarrativeAssetBytes{Reference: refusedFixture.bundle.Narrative, Bytes: refusedFixture.sidecarBytes},
			Catalogues: []NarrativeAssetBytes{{
				Reference: sidecarCatalogueReference(t, refusedFixture.sidecarBytes),
				Bytes:     bad,
			}},
		})
	require.False(t, result.Admitted)
	// The supplied bad bytes do not match the pinned catalogue digest first.
	require.Equal(t, CodeDigestMismatch, result.Issues[0].Code)
}

func TestAdmitNarrativeCoversEventTypes(t *testing.T) {
	catalogue := catalogueBytesFixture("")
	sidecar := allEventSidecar(t)
	declaration := declarationBytesFixture(sidecar)
	parsed := ParseTrainingDeclaration("training-demo", declaration)
	require.True(t, parsed.Accepted, "%v", parsed.Issues)
	packageRef := packageReferenceFixture()
	source := SourceContext{
		Project:           ProjectIdentity{Value: "project-one"},
		ContentPin:        ContentPin{Kind: PinGit, Commit: fixtureCommit},
		DeclarationSHA256: sha256Hex(declaration),
	}
	binding := OwnerBindingKey{
		Project: source.Project, ContentPin: source.ContentPin,
		DeclarationSHA256: source.DeclarationSHA256, Package: packageRef,
	}
	evidenceRef := AssetReference{Path: "training/evidence.txt", SHA256: strings.Repeat("c", 64), Bytes: "10"}
	checkRef := AssetReference{Path: "training/check.json", SHA256: strings.Repeat("d", 64), Bytes: "20"}
	photoRef := AssetReference{Path: "training/photo.png", SHA256: strings.Repeat("b", 64), Bytes: "5"}
	catalogueRef := sidecarCatalogueReference(t, sidecar)
	owner := &fakeOwner{
		ref:            packageRef,
		packageAssets:  []AssetReference{evidenceRef, checkRef},
		packageCases:   []string{"case-one"},
		admittedAssets: map[string]bool{assetReferenceKey(evidenceRef): true, assetReferenceKey(checkRef): true, assetReferenceKey(photoRef): true, assetReferenceKey(catalogueRef): true, assetReferenceKey(AssetReference{Path: "walkthroughs/training-demo/narrative.json", SHA256: sha256Hex(sidecar), Bytes: Integer(fmt.Sprint(len(sidecar)))}): true},
		challenge:      ChallengeBinding{ChallengeID: "retry-query", Revision: "1"},
		checks:         map[string]CheckBinding{"owner-check": {Binding: binding, CheckID: "owner-check", DefinitionAssetID: "owner-check-definition"}},
		proposals:      map[string]ProposalBinding{"patch": {Binding: binding, ProposalID: "patch", Kind: "watch", DefinitionID: "watch", EvidenceIDs: []string{"evidence"}, Rationale: "Try it."}},
		targets:        map[string]PresentationTargetBinding{"case-panel": {Binding: binding, TargetID: "case-panel", Kind: "incident", RefID: "incident-one", Closure: &PresentationClosure{Kind: "incident", ID: "incident-one"}}},
	}
	watchRef := AssetReference{Path: "training/watch.json", SHA256: strings.Repeat("e", 64), Bytes: "30"}
	owner.packageAssets = append(owner.packageAssets, watchRef)
	owner.narrative = NarrativeInventory{
		Binding: binding,
		Assets: []PackageAssetBinding{
			{AssetID: "evidence", Kind: "evidence-text", Reference: evidenceRef, Bytes: evidenceRef.Bytes},
			{AssetID: "owner-check-definition", Kind: "check-definition", Reference: checkRef, Bytes: checkRef.Bytes},
			{AssetID: "watch", Kind: "watch-recommendation", Reference: watchRef, Bytes: watchRef.Bytes},
		},
		Cases:               []string{"case-one"},
		Incidents:           []string{"incident-one"},
		Checks:              []string{"owner-check"},
		Proposals:           []string{"patch"},
		PresentationTargets: []string{"case-panel"},
	}
	resolution := ResolveTraining(parsed.Definition, source, fakeAuthority{}, owner)
	require.True(t, resolution.Admitted, "%v", resolution.Issues)
	result := AdmitNarrative(resolution.Bundle, fakeAuthority{}, owner, NarrativeAdmissionInput{
		Sidecar:    NarrativeAssetBytes{Reference: resolution.Bundle.Narrative, Bytes: sidecar},
		Catalogues: []NarrativeAssetBytes{{Reference: catalogueRef, Bytes: catalogue}},
	})
	require.True(t, result.Admitted, "%v", result.Issues)
	require.NotNil(t, result.Narrative.Events[7].PresentationTarget)
	_ = result
}

func TestAdmitNarrativeCoversProposalAndTargetKinds(t *testing.T) {
	for _, kind := range []string{"check", "watch", "other"} {
		t.Run("proposal-"+kind, func(t *testing.T) {
			fixture := newTargetFixture(t)
			binding := fixture.owner.narrative.Binding
			proposal := fixture.owner.proposals["patch"]
			proposal.Kind = kind
			if kind == "check" {
				proposal.DefinitionID = "owner-check"
				fixture.owner.narrative.Checks = []string{"owner-check"}
			}
			if kind == "watch" {
				proposal.DefinitionID = "watch"
				watchRef := AssetReference{Path: "training/watch.json", SHA256: strings.Repeat("e", 64), Bytes: "30"}
				fixture.owner.packageAssets = append(fixture.owner.packageAssets, watchRef)
				fixture.owner.narrative.Assets = append(fixture.owner.narrative.Assets, PackageAssetBinding{
					AssetID: "watch", Kind: "watch-recommendation", Reference: watchRef, Bytes: watchRef.Bytes,
				})
			}
			proposal.Binding = binding
			fixture.owner.proposals["patch"] = proposal
			result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
				admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
			if kind == "other" {
				require.False(t, result.Admitted)
				require.Equal(t, CodeInvalidValue, result.Issues[0].Code)
			} else {
				require.True(t, result.Admitted, "%v", result.Issues)
			}
		})
	}

	for _, kind := range []string{"incident", "evidence", "check", "proposal", "other"} {
		t.Run("target-"+kind, func(t *testing.T) {
			fixture := newTargetFixture(t)
			binding := fixture.owner.narrative.Binding
			target := fixture.owner.targets["case-panel"]
			target.Kind = kind
			target.Closure = &PresentationClosure{Kind: kind, ID: target.RefID}
			switch kind {
			case "incident":
				target.RefID = "incident-one"
				target.Closure.ID = "incident-one"
			case "evidence":
				target.RefID = "evidence"
				target.Closure.ID = "evidence"
			case "check":
				target.RefID = "owner-check"
				target.Closure.ID = "owner-check"
				fixture.owner.narrative.Checks = []string{"owner-check"}
				check := fixture.owner.checks["owner-check"]
				target.Closure.Check = &CheckBinding{
					Binding: check.Binding, CheckID: check.CheckID,
					DefinitionAssetID: check.DefinitionAssetID,
				}
			case "proposal":
				target.RefID = "patch"
				target.Closure.ID = "patch"
				proposal := fixture.owner.proposals["patch"]
				target.Closure.Proposal = &ProposalBinding{
					Binding: proposal.Binding, ProposalID: proposal.ProposalID,
					Kind: proposal.Kind, DefinitionID: proposal.DefinitionID,
					EvidenceIDs: append([]string{}, proposal.EvidenceIDs...),
					Rationale:   proposal.Rationale,
				}
			}
			target.Binding = binding
			fixture.owner.targets["case-panel"] = target
			result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
				admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
			if kind == "other" {
				require.False(t, result.Admitted)
				require.Equal(t, CodeInvalidValue, result.Issues[0].Code)
			} else {
				require.True(t, result.Admitted, "%v", result.Issues)
			}
		})
	}
}

func TestAdmitNarrativeOwnerPortFailures(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*fakeOwner)
		code   Code
		path   string
	}{
		{"package lookup error", func(owner *fakeOwner) { owner.admitPackageErr = true }, CodeNotAdmitted, "/trainingPackage"},
		{"package ref mismatch", func(owner *fakeOwner) {
			other := packageReferenceFixture()
			other.ID = "other"
			owner.packageRefOverride = &other
		}, CodeMismatch, "/trainingPackage"},
		{"inventory lookup error", func(owner *fakeOwner) { owner.admitInventoryErr = true }, CodeNotAdmitted, "/trainingPackage"},
		{"asset lookup error", func(owner *fakeOwner) { owner.admitAssetErr = true }, CodeNotAdmitted, "/assets/0"},
		{"inventory list changed", func(owner *fakeOwner) {
			owner.mutateOnSecondRead = true
			owner.narrative.Checks = []string{"late"}
			owner.packageCases = owner.narrative.Cases
		}, CodeMismatch, ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newNarrativeFixture(t)
			test.mutate(fixture.owner)
			result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
				admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
			require.False(t, result.Admitted)
			require.NotEmpty(t, result.Issues)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
			require.Equal(t, test.path, result.Issues[0].Path, "%v", result.Issues)
		})
	}
}

func TestAdmitNarrativeSelectionRefusal(t *testing.T) {
	fixture := newNarrativeFixture(t)
	request := "en_US"
	result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner, NarrativeAdmissionInput{
		Sidecar: NarrativeAssetBytes{Reference: fixture.bundle.Narrative, Bytes: fixture.sidecarBytes},
		Catalogues: []NarrativeAssetBytes{{
			Reference: sidecarCatalogueReference(t, fixture.sidecarBytes), Bytes: fixture.catalogueBytes,
		}},
		Request: &request,
	})
	require.False(t, result.Admitted)
	require.Equal(t, Code(LocaleCodeMalformed), result.Issues[0].Code)
}

func TestAdmitNarrativeEventReferenceFailures(t *testing.T) {
	catalogue := catalogueBytesFixture("")
	sidecar := allEventSidecar(t)
	declaration := declarationBytesFixture(sidecar)
	parsed := ParseTrainingDeclaration("training-demo", declaration)
	require.True(t, parsed.Accepted, "%v", parsed.Issues)
	packageRef := packageReferenceFixture()
	source := SourceContext{
		Project: ProjectIdentity{Value: "project-one"}, ContentPin: ContentPin{Kind: PinGit, Commit: fixtureCommit},
		DeclarationSHA256: sha256Hex(declaration),
	}
	binding := OwnerBindingKey{Project: source.Project, ContentPin: source.ContentPin, DeclarationSHA256: source.DeclarationSHA256, Package: packageRef}
	evidenceRef := AssetReference{Path: "training/evidence.txt", SHA256: strings.Repeat("c", 64), Bytes: "10"}
	checkRef := AssetReference{Path: "training/check.json", SHA256: strings.Repeat("d", 64), Bytes: "20"}
	owner := &fakeOwner{
		ref: packageRef, packageAssets: []AssetReference{evidenceRef, checkRef}, packageCases: []string{"case-other"},
		admittedAssets: map[string]bool{assetReferenceKey(evidenceRef): true, assetReferenceKey(checkRef): true, assetReferenceKey(sidecarCatalogueReference(t, sidecar)): true, assetReferenceKey(AssetReference{Path: "walkthroughs/training-demo/narrative.json", SHA256: sha256Hex(sidecar), Bytes: Integer(fmt.Sprint(len(sidecar)))}): true},
		challenge:      ChallengeBinding{ChallengeID: "retry-query", Revision: "1"},
		checks:         map[string]CheckBinding{"owner-check": {Binding: binding, CheckID: "different", DefinitionAssetID: "owner-check-definition"}},
		proposals:      map[string]ProposalBinding{"patch": {Binding: binding, ProposalID: "patch", Kind: "query", DefinitionID: "queries/missing", EvidenceIDs: []string{"evidence"}, Rationale: strings.Repeat("x", 401)}},
		targets:        map[string]PresentationTargetBinding{"case-panel": {Binding: binding, TargetID: "case-panel", Kind: "case", RefID: "case-missing", Closure: &PresentationClosure{Kind: "case", ID: "case-missing"}}},
	}
	owner.narrative = NarrativeInventory{
		Binding: binding,
		Assets: []PackageAssetBinding{
			{AssetID: "evidence", Kind: "evidence-text", Reference: evidenceRef, Bytes: evidenceRef.Bytes},
			{AssetID: "owner-check-definition", Kind: "check-definition", Reference: checkRef, Bytes: checkRef.Bytes},
		},
		Cases: []string{"case-one"}, Incidents: []string{"incident-one"},
		Checks: []string{"owner-check"}, Proposals: []string{"patch"}, PresentationTargets: []string{"case-panel"},
	}
	resolution := ResolveTraining(parsed.Definition, source, fakeAuthority{}, owner)
	require.True(t, resolution.Admitted, "%v", resolution.Issues)
	input := NarrativeAdmissionInput{
		Sidecar:    NarrativeAssetBytes{Reference: resolution.Bundle.Narrative, Bytes: sidecar},
		Catalogues: []NarrativeAssetBytes{{Reference: sidecarCatalogueReference(t, sidecar), Bytes: catalogue}},
	}
	result := AdmitNarrative(resolution.Bundle, fakeAuthority{}, owner, input)
	require.False(t, result.Admitted)
	require.NotEmpty(t, result.Issues)
}

func TestNarrativeInventoryEqualityBranches(t *testing.T) {
	base := newNarrativeFixture(t).owner.narrative
	authority := fakeAuthority{}
	require.True(t, equalNarrativeInventory(base, base, authority))
	changed := snapshotNarrativeInventory(base)
	changed.Cases = []string{"other"}
	require.False(t, equalNarrativeInventory(base, changed, authority))
	changed = snapshotNarrativeInventory(base)
	changed.Assets = changed.Assets[:1]
	require.False(t, equalNarrativeInventory(base, changed, authority))
	changed = snapshotNarrativeInventory(base)
	changed.Assets[0].AssetID = "other"
	require.False(t, equalNarrativeInventory(base, changed, authority))
	changed = snapshotNarrativeInventory(base)
	changed.Binding.Project = ProjectIdentity{}
	require.False(t, equalNarrativeInventory(base, changed, authority))
}

func TestSnapshotPresentationTargetClosureCopies(t *testing.T) {
	target := PresentationTargetBinding{
		Binding:  OwnerBindingKey{Project: ProjectIdentity{Value: "project-one"}},
		TargetID: "target",
		Kind:     "check",
		RefID:    "check",
		Closure: &PresentationClosure{
			Kind:     "check",
			ID:       "check",
			Check:    &CheckBinding{CheckID: "check", Binding: OwnerBindingKey{Project: ProjectIdentity{Value: "project-one"}}},
			Proposal: &ProposalBinding{ProposalID: "proposal", EvidenceIDs: []string{"a"}},
		},
	}
	snapshot := snapshotPresentationTargetBinding(target)
	require.NotNil(t, snapshot.Closure)
	require.NotNil(t, snapshot.Closure.Check)
	require.NotNil(t, snapshot.Closure.Proposal)
	require.NotSame(t, target.Closure, snapshot.Closure)
}

func TestIssuesOfErrorFallback(t *testing.T) {
	issues := issuesOfError(fmt.Errorf("plain error"))
	require.Equal(t, CodeMismatch, issues[0].Code)
}

func TestAdmitNarrativeCatalogueLocaleBranches(t *testing.T) {
	fr := []byte(`{"schemaVersion":1,"locale":"fr","messages":{"other.key":"Autre"}}`)
	fixture := newNarrativeFixtureCatalogues(t, []catalogueFixture{{
		locale: "fr", path: "walkthroughs/training-demo/messages.fr.json", data: fr,
	}}, defaultNarrativeEvents, defaultNarrativeEventIDs, defaultNarrativeSegments)
	input := NarrativeAdmissionInput{
		Sidecar: NarrativeAssetBytes{Reference: fixture.bundle.Narrative, Bytes: fixture.sidecarBytes},
		Catalogues: []NarrativeAssetBytes{{
			Reference: sidecarCatalogueReferenceAt(t, fixture.sidecarBytes, 0), Bytes: fr,
		}},
	}
	result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner, input)
	require.False(t, result.Admitted)
	require.Equal(t, CodeInvalidReference, result.Issues[0].Code)
	require.Equal(t, "/locale", result.Issues[0].Path)

	// Now only the fallback catalogue is missing.
	fallbackOnly := rebuildFixture(t, fixture,
		[]byte(strings.Replace(string(fixture.sidecarBytes), `"locale": "en"`, `"locale": "fr"`, 1)))
	result = AdmitNarrative(fallbackOnly.bundle, fallbackOnly.authority, fallbackOnly.owner,
		NarrativeAdmissionInput{
			Sidecar: NarrativeAssetBytes{Reference: fallbackOnly.bundle.Narrative, Bytes: fallbackOnly.sidecarBytes},
			Catalogues: []NarrativeAssetBytes{{
				Reference: sidecarCatalogueReferenceAt(t, fallbackOnly.sidecarBytes, 0), Bytes: fr,
			}},
		})
	require.False(t, result.Admitted)
	for _, issue := range result.Issues {
		if issue.Code == CodeInvalidReference && issue.Path == "/fallbackLocale" {
			return
		}
	}
	require.Fail(t, "fallback catalogue refusal not reported: %v", result.Issues)
}

func TestAdmitNarrativeCatalogueParseRefusal(t *testing.T) {
	bad := []byte(`{"schemaVersion":1,"locale":"en","messages":{}}`)
	fixture := newNarrativeFixtureCatalogues(t, []catalogueFixture{{
		locale: "en", path: "walkthroughs/training-demo/messages.en.json", data: bad,
	}}, defaultNarrativeEvents, defaultNarrativeEventIDs, defaultNarrativeSegments)
	result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
		admissionInputFor(t, fixture, fixture.sidecarBytes, bad))
	require.False(t, result.Admitted)
	require.Equal(t, CodeTooMany, result.Issues[0].Code)
}

func TestAdmitNarrativeMissingEventReferences(t *testing.T) {
	sidecar := allEventSidecar(t)
	mutated := string(sidecar)
	mutated = strings.ReplaceAll(mutated, `"manifestCaseId":"case-one"`, `"manifestCaseId":"case-missing"`)
	mutated = strings.Replace(mutated, `"assetId":"photo"`, `"assetId":"ghost"`, 1)
	mutated = strings.Replace(mutated, `"pauseAfterMs":5`, `"pauseAfterMs":5,"cameraTargetId":"ghost"`, 1)
	fixture := newNarrativeFixtureCatalogues(t, []catalogueFixture{{
		locale: "en", path: "walkthroughs/training-demo/messages.en.json", data: catalogueBytesFixture(""),
	}}, defaultNarrativeEvents, defaultNarrativeEventIDs, defaultNarrativeSegments)
	rebuilt := rebuildFixture(t, fixture, []byte(mutated))
	result := AdmitNarrative(rebuilt.bundle, rebuilt.authority, rebuilt.owner,
		admissionInputFor(t, rebuilt, rebuilt.sidecarBytes, rebuilt.catalogueBytes))
	require.False(t, result.Admitted)
	codes := make(map[string]bool)
	for _, issue := range result.Issues {
		codes[issue.Path] = true
	}
	require.True(t, codes["/events/2/payload/manifestCaseId"] || codes["/events/3/payload/manifestCaseId"])
	require.True(t, codes["/events/4/payload/assetId"])
	require.True(t, codes["/events/8/presentation/cameraTargetId"])
}

func TestAdmitNarrativeSegmentMismatchBranch(t *testing.T) {
	fixture := newNarrativeFixture(t)
	segment := rebuildFixture(t, fixture,
		[]byte(strings.Replace(string(fixture.sidecarBytes), `"stepId":"discussion"`, `"stepId":"solve"`, 1)))
	result := AdmitNarrative(segment.bundle, segment.authority, segment.owner,
		admissionInputFor(t, segment, segment.sidecarBytes, segment.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
	require.Equal(t, "/segments/0", result.Issues[0].Path)
}

func TestAdmitNarrativeBindingFailureBranches(t *testing.T) {
	type mutation func(*narrativeFixture)
	cases := []struct {
		name   string
		mutate mutation
		code   Code
	}{
		{"check id mismatch", func(f *narrativeFixture) {
			binding := f.owner.checks["owner-check"]
			binding.CheckID = "different"
			f.owner.checks["owner-check"] = binding
		}, CodeMismatch},
		{"check definition absent", func(f *narrativeFixture) {
			binding := f.owner.checks["owner-check"]
			binding.DefinitionAssetID = "missing"
			f.owner.checks["owner-check"] = binding
		}, CodeNotAdmitted},
		{"proposal package mismatch", func(f *narrativeFixture) {
			binding := f.owner.proposals["patch"]
			binding.Binding.Package = PackageReference{ID: "other", Revision: "1", Path: "training/package.json", SHA256: strings.Repeat("a", 64), Bytes: "100"}
			f.owner.proposals["patch"] = binding
		}, CodeMismatch},
		{"proposal id mismatch", func(f *narrativeFixture) {
			binding := f.owner.proposals["patch"]
			binding.ProposalID = "other"
			f.owner.proposals["patch"] = binding
		}, CodeMismatch},
		{"proposal not inventory", func(f *narrativeFixture) {
			f.owner.narrative.Proposals = nil
		}, CodeNotAdmitted},
		{"proposal query missing", func(f *narrativeFixture) {
			binding := f.owner.proposals["patch"]
			binding.DefinitionID = "queries/missing"
			f.owner.proposals["patch"] = binding
		}, CodeInvalidReference},
		{"proposal check inventory missing", func(f *narrativeFixture) {
			binding := f.owner.proposals["patch"]
			binding.Kind = "check"
			binding.DefinitionID = "owner-check"
			f.owner.proposals["patch"] = binding
			f.owner.narrative.Checks = nil
		}, CodeInvalidReference},
		{"proposal check owner missing", func(f *narrativeFixture) {
			binding := f.owner.proposals["patch"]
			binding.Kind = "check"
			binding.DefinitionID = "owner-check"
			f.owner.proposals["patch"] = binding
			f.owner.checks = nil
		}, CodeNotAdmitted},
		{"proposal watch missing", func(f *narrativeFixture) {
			binding := f.owner.proposals["patch"]
			binding.Kind = "watch"
			binding.DefinitionID = "missing-watch"
			f.owner.proposals["patch"] = binding
		}, CodeInvalidReference},
		{"proposal duplicate evidence", func(f *narrativeFixture) {
			binding := f.owner.proposals["patch"]
			binding.EvidenceIDs = []string{"evidence", "evidence"}
			f.owner.proposals["patch"] = binding
		}, CodeDuplicateID},
		{"proposal bad evidence", func(f *narrativeFixture) {
			binding := f.owner.proposals["patch"]
			binding.EvidenceIDs = []string{"owner-check-definition"}
			f.owner.proposals["patch"] = binding
		}, CodeInvalidReference},
		{"proposal long rationale", func(f *narrativeFixture) {
			binding := f.owner.proposals["patch"]
			binding.Rationale = strings.Repeat("x", 401)
			f.owner.proposals["patch"] = binding
		}, CodeTooLong},
		{"target package mismatch", func(f *narrativeFixture) {
			binding := f.owner.targets["case-panel"]
			binding.Binding.Package = PackageReference{ID: "other", Revision: "1", Path: "training/package.json", SHA256: strings.Repeat("a", 64), Bytes: "100"}
			f.owner.targets["case-panel"] = binding
		}, CodeMismatch},
		{"target id mismatch", func(f *narrativeFixture) {
			binding := f.owner.targets["case-panel"]
			binding.TargetID = "other"
			f.owner.targets["case-panel"] = binding
		}, CodeMismatch},
		{"target not inventory", func(f *narrativeFixture) {
			f.owner.narrative.PresentationTargets = nil
		}, CodeNotAdmitted},
		{"target case missing", func(f *narrativeFixture) {
			binding := f.owner.targets["case-panel"]
			binding.RefID = "case-missing"
			binding.Closure.ID = "case-missing"
			f.owner.targets["case-panel"] = binding
		}, CodeInvalidReference},
		{"target incident missing", func(f *narrativeFixture) {
			binding := f.owner.targets["case-panel"]
			binding.Kind = "incident"
			binding.RefID = "incident-missing"
			binding.Closure = &PresentationClosure{Kind: "incident", ID: "incident-missing"}
			f.owner.targets["case-panel"] = binding
		}, CodeInvalidReference},
		{"target evidence missing", func(f *narrativeFixture) {
			binding := f.owner.targets["case-panel"]
			binding.Kind = "evidence"
			binding.RefID = "missing"
			binding.Closure = &PresentationClosure{Kind: "evidence", ID: "missing"}
			f.owner.targets["case-panel"] = binding
		}, CodeInvalidReference},
		{"target check missing", func(f *narrativeFixture) {
			binding := f.owner.targets["case-panel"]
			binding.Kind = "check"
			binding.RefID = "owner-check"
			binding.Closure = &PresentationClosure{Kind: "check", ID: "owner-check"}
			f.owner.narrative.Checks = nil
			f.owner.targets["case-panel"] = binding
		}, CodeInvalidReference},
		{"target proposal missing", func(f *narrativeFixture) {
			binding := f.owner.targets["case-panel"]
			binding.Kind = "proposal"
			binding.RefID = "patch"
			binding.Closure = &PresentationClosure{Kind: "proposal", ID: "patch"}
			f.owner.narrative.Proposals = nil
			f.owner.targets["case-panel"] = binding
		}, CodeInvalidReference},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			fixture := newTargetFixture(t)
			test.mutate(fixture)
			result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
				admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
			require.False(t, result.Admitted)
			codes := make(map[Code]bool)
			for _, issue := range result.Issues {
				codes[issue.Code] = true
			}
			require.True(t, codes[test.code], "wanted %s, got %v", test.code, result.Issues)
		})
	}
}

func TestCatalogueMessagesNotObject(t *testing.T) {
	result := ParseMessageCatalogue([]byte(`{"schemaVersion":1,"locale":"en","messages":[]}`))
	require.False(t, result.Accepted)
	require.Equal(t, CodeInvalidType, result.Issues[0].Code)
	require.Equal(t, "/messages", result.Issues[0].Path)
}

func TestNarrativeParserDuplicateAssetsAndSegments(t *testing.T) {
	valid := string(allEventSidecar(t))
	duplicateAsset := strings.Replace(valid, `"assets": [{"id":"photo","mediaType":"image/png","path":"training/photo.png","sha256":"`+strings.Repeat("b", 64)+`","bytes":5}]`, `"assets": [{"id":"photo","mediaType":"image/png","path":"training/photo.png","sha256":"`+strings.Repeat("b", 64)+`","bytes":5},{"id":"photo","mediaType":"image/png","path":"training/photo.png","sha256":"`+strings.Repeat("b", 64)+`","bytes":5}]`, 1)
	result := ParseNarrativeSidecar([]byte(duplicateAsset))
	require.False(t, result.Accepted)
	require.Equal(t, CodeDuplicateID, result.Issues[0].Code)

	duplicateSegment := strings.Replace(valid, `"segments": [{`, `"segments": [{"id":"discussion","chapterId":"retries","stepId":"discussion","eventIds":[],"prerequisiteContextEventIds":[]},{`, 1)
	result = ParseNarrativeSidecar([]byte(duplicateSegment))
	require.False(t, result.Accepted)
	require.Equal(t, CodeDuplicateID, result.Issues[0].Code)
}

func TestNarrativeParserPresentationBranches(t *testing.T) {
	valid := string(allEventSidecar(t))
	cases := []struct {
		name string
		data string
		code Code
		path string
	}{
		{"bad typing preset", strings.Replace(valid, `"typingSpeed":42`, `"typingSpeed":"slow"`, 1), CodeInvalidValue, "/events/8/presentation/typingSpeed"},
		{"non canonical typing number", strings.Replace(valid, `"typingSpeed":42`, `"typingSpeed":4.5`, 1), CodeInvalidValue, "/events/8/presentation/typingSpeed"},
		{"bad transition value", strings.Replace(valid, `"pauseAfterMs":5`, `"pauseAfterMs":5,"transition":"slide"`, 1), CodeInvalidValue, "/events/8/presentation/transition"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := ParseNarrativeSidecar([]byte(test.data))
			require.False(t, result.Accepted)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
			require.Equal(t, test.path, result.Issues[0].Path, "%v", result.Issues)
		})
	}
}

func TestNarrativeParserMissingRequiredLeafFields(t *testing.T) {
	valid := string(allEventSidecar(t))
	cases := []struct {
		name string
		data string
		code Code
		path string
	}{
		{"missing check id", strings.Replace(valid, `"checkId":"owner-check",`, ``, 1), CodeMissingField, "/events/5/payload/checkId"},
		{"outcome not string", strings.Replace(valid, `"referenceOutcome":"failed"`, `"referenceOutcome":1`, 1), CodeInvalidType, "/events/5/payload/referenceOutcome"},
		{"missing schema version", strings.Replace(valid, `"schemaVersion": 1,`, ``, 1), CodeMissingField, "/schemaVersion"},
		{"schema version not number", strings.Replace(valid, `"schemaVersion": 1`, `"schemaVersion": "1"`, 1), CodeInvalidType, "/schemaVersion"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := ParseNarrativeSidecar([]byte(test.data))
			require.False(t, result.Accepted)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
			require.Equal(t, test.path, result.Issues[0].Path, "%v", result.Issues)
		})
	}
}

func TestAdmitNarrativeTextSkipAndIntegerFailure(t *testing.T) {
	fixture := newNarrativeFixture(t)
	binaryRef := AssetReference{Path: "training/fixture.sqlite", SHA256: strings.Repeat("9", 64), Bytes: "12"}
	fixture.owner.packageAssets = append(fixture.owner.packageAssets, binaryRef)
	fixture.owner.narrative.Assets = append(fixture.owner.narrative.Assets,
		PackageAssetBinding{AssetID: "fixture", Kind: "challenge-fixture", Reference: binaryRef, Bytes: binaryRef.Bytes})
	result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
		admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
	require.True(t, result.Admitted, "%v", result.Issues)

	large := newNarrativeFixture(t)
	huge := AssetReference{Path: "training/huge.txt", SHA256: strings.Repeat("8", 64), Bytes: "99999999999999999999"}
	large.owner.packageAssets = append(large.owner.packageAssets, huge)
	large.owner.narrative.Assets = append(large.owner.narrative.Assets,
		PackageAssetBinding{AssetID: "huge", Kind: "evidence-text", Reference: huge, Bytes: huge.Bytes})
	result = AdmitNarrative(large.bundle, large.authority, large.owner,
		admissionInputFor(t, large, large.sidecarBytes, large.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeInvalidValue, result.Issues[0].Code)
}

func TestAdmitNarrativeSecondInventoryReadFailure(t *testing.T) {
	fixture := newNarrativeFixture(t)
	fixture.owner.admitInventoryErrOnSecond = true
	result := AdmitNarrative(fixture.bundle, fixture.authority, fixture.owner,
		admissionInputFor(t, fixture, fixture.sidecarBytes, fixture.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeNotAdmitted, result.Issues[0].Code)
}

func TestAdmitNarrativeCachedAndFailedTargetLookups(t *testing.T) {
	fixture := newTargetFixture(t)
	base := string(fixture.sidecarBytes)
	withSecondFocus := strings.Replace(base,
		`{"id":"focus-event","chapterId":"retries","type":"camera.focus","payload":{"targetId":"case-panel"}}`,
		`{"id":"focus-event","chapterId":"retries","type":"camera.focus","payload":{"targetId":"case-panel"}},{"id":"focus-event-2","chapterId":"retries","type":"camera.focus","payload":{"targetId":"case-panel"}}`, 1)
	withSecondFocus = strings.Replace(withSecondFocus,
		`"eventIds":["await-result","explain","check-event","recommend-event","focus-event"]`,
		`"eventIds":["await-result","explain","check-event","recommend-event","focus-event","focus-event-2"]`, 1)
	rebuilt := rebuildFixture(t, fixture, []byte(withSecondFocus))
	result := AdmitNarrative(rebuilt.bundle, rebuilt.authority, rebuilt.owner,
		admissionInputFor(t, rebuilt, rebuilt.sidecarBytes, rebuilt.catalogueBytes))
	require.True(t, result.Admitted, "%v", result.Issues)

	failed := newTargetFixture(t)
	binding := failed.owner.targets["case-panel"]
	binding.Kind = "check"
	binding.RefID = "owner-check"
	binding.Closure = &PresentationClosure{Kind: "check", ID: "owner-check"}
	failed.owner.targets["case-panel"] = binding
	failed.owner.checks = nil
	result = AdmitNarrative(failed.bundle, failed.authority, failed.owner,
		admissionInputFor(t, failed, failed.sidecarBytes, failed.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeNotAdmitted, result.Issues[0].Code)

	failedProposal := newTargetFixture(t)
	binding = failedProposal.owner.targets["case-panel"]
	binding.Kind = "proposal"
	binding.RefID = "patch"
	binding.Closure = &PresentationClosure{Kind: "proposal", ID: "patch"}
	failedProposal.owner.targets["case-panel"] = binding
	failedProposal.owner.proposals = nil
	result = AdmitNarrative(failedProposal.bundle, failedProposal.authority, failedProposal.owner,
		admissionInputFor(t, failedProposal, failedProposal.sidecarBytes, failedProposal.catalogueBytes))
	require.False(t, result.Admitted)
	require.Equal(t, CodeNotAdmitted, result.Issues[0].Code)
}
