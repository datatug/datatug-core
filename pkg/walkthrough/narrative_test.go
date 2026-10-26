package walkthrough

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test doubles: typed inventories only, never production authority.
// ---------------------------------------------------------------------------

type fakeAuthority struct{}

func (fakeAuthority) BindProject(raw string) (ProjectBinding, error) {
	return ProjectBinding{Identity: ProjectIdentity{Value: raw}}, nil
}

func (fakeAuthority) AdmitProject(raw []byte) (ProjectIdentity, error) {
	return ProjectIdentity{Value: string(raw)}, nil
}

func (fakeAuthority) EqualProject(left, right ProjectIdentity) bool {
	return left.Value == right.Value
}

type fakeOwner struct {
	ref                       PackageReference
	packageAssets             []AssetReference
	packageCases              []string
	admittedAssets            map[string]bool
	challenge                 ChallengeBinding
	narrative                 NarrativeInventory
	mutateOnSecondRead        bool
	inventoryReads            int
	admitPackageErr           bool
	packageRefOverride        *PackageReference
	admitInventoryErr         bool
	admitInventoryErrOnSecond bool
	admitAssetErr             bool
	checks                    map[string]CheckBinding
	proposals                 map[string]ProposalBinding
	targets                   map[string]PresentationTargetBinding

	mutateReturnedInventory bool
}

func (o *fakeOwner) AdmitPackage(ref PackageReference) (PackageInventory, error) {
	if o.admitPackageErr || !ref.Equal(o.ref) {
		return PackageInventory{}, fmt.Errorf("package not admitted")
	}
	refValue := o.ref
	if o.packageRefOverride != nil {
		refValue = *o.packageRefOverride
	}
	return PackageInventory{
		Ref:    refValue,
		Assets: append([]AssetReference{}, o.packageAssets...),
		Cases:  append([]string{}, o.packageCases...),
	}, nil
}

func (o *fakeOwner) AdmitAsset(ref AssetReference) error {
	if o.admitAssetErr {
		return fmt.Errorf("asset not admitted")
	}
	if o.admittedAssets[assetReferenceKey(ref)] {
		return nil
	}
	return fmt.Errorf("asset not admitted")
}

func (o *fakeOwner) AdmitChallenge(challengeID string, revision Integer) (ChallengeBinding, error) {
	if o.challenge.ChallengeID == challengeID && o.challenge.Revision == revision {
		return o.challenge, nil
	}
	return ChallengeBinding{}, fmt.Errorf("challenge not admitted")
}

func (o *fakeOwner) AdmitNarrativeInventory(ref PackageReference) (NarrativeInventory, error) {
	if o.admitInventoryErr || !ref.Equal(o.ref) {
		return NarrativeInventory{}, fmt.Errorf("package not admitted")
	}
	o.inventoryReads++
	if o.admitInventoryErrOnSecond && o.inventoryReads > 1 {
		return NarrativeInventory{}, fmt.Errorf("inventory changed")
	}
	inventory := snapshotNarrativeInventory(o.narrative)
	if o.mutateOnSecondRead && o.inventoryReads > 1 {
		inventory.Cases = append(inventory.Cases, "late-case")
	}
	if o.mutateReturnedInventory {
		// A caller that mutates the object it just returned must never reach
		// the admitted snapshot.
		inventory.Cases = append(inventory.Cases, "mutated-by-owner")
	}
	return inventory, nil
}

func (o *fakeOwner) AdmitCheck(ref PackageReference, checkID string) (CheckBinding, error) {
	if !ref.Equal(o.ref) {
		return CheckBinding{}, fmt.Errorf("package not admitted")
	}
	binding, ok := o.checks[checkID]
	if !ok {
		return CheckBinding{}, fmt.Errorf("check %s not admitted", checkID)
	}
	return snapshotCheckBinding(binding), nil
}

func (o *fakeOwner) AdmitProposal(ref PackageReference, proposalID string) (ProposalBinding, error) {
	if !ref.Equal(o.ref) {
		return ProposalBinding{}, fmt.Errorf("package not admitted")
	}
	binding, ok := o.proposals[proposalID]
	if !ok {
		return ProposalBinding{}, fmt.Errorf("proposal %s not admitted", proposalID)
	}
	return snapshotProposalBinding(binding), nil
}

func (o *fakeOwner) AdmitPresentationTarget(ref PackageReference, targetID string) (PresentationTargetBinding, error) {
	if !ref.Equal(o.ref) {
		return PresentationTargetBinding{}, fmt.Errorf("package not admitted")
	}
	binding, ok := o.targets[targetID]
	if !ok {
		return PresentationTargetBinding{}, fmt.Errorf("presentation target %s not admitted", targetID)
	}
	return snapshotPresentationTargetBinding(binding), nil
}

// ---------------------------------------------------------------------------
// Fixture construction.
// ---------------------------------------------------------------------------

const fixtureCommit = "0123456789abcdef0123456789abcdef01234567"

type narrativeFixture struct {
	declarationBytes []byte
	declaration      *TrainingDeclaration
	sidecarBytes     []byte
	catalogueBytes   []byte
	packageRef       PackageReference
	owner            *fakeOwner
	authority        fakeAuthority
	bundle           *AdmittedTraining
	source           SourceContext
}

func packageReferenceFixture() PackageReference {
	return PackageReference{
		ID:       "training-demo",
		Revision: "1",
		Path:     "training/package.json",
		SHA256:   strings.Repeat("a", 64),
		Bytes:    "100",
	}
}

func catalogueBytesFixture(messages string) []byte {
	if messages == "" {
		messages = `"retry.explanation":"Repeated requests need an idempotency key."`
	}
	return []byte(`{"schemaVersion":1,"locale":"en","messages":{` + messages + `}}`)
}

const defaultNarrativeEvents = `{"id":"await-result","chapterId":"retries","type":"challenge.await_validation","payload":{"stepId":"solve","checkpointId":"result"}},
{"id":"explain","chapterId":"retries","type":"chat.message","payload":{"actorId":"guide","messageKey":"retry.explanation","attachmentIds":["photo"]},"presentation":{"delayBeforeMs":10,"pauseAfterMs":20,"typingSpeed":"fast","typingIndicator":true,"transition":"fade"}}`

const defaultNarrativeEventIDs = `["await-result","explain"]`

const defaultNarrativeSegments = `{"id":"discussion","chapterId":"retries","stepId":"discussion","eventIds":["await-result","explain"],"prerequisiteContextEventIds":[]}`

type catalogueFixture struct {
	locale string
	path   string
	data   []byte
}

func sidecarBytesWith(
	t *testing.T,
	catalogues []catalogueFixture,
	events string,
	eventIDs string,
	segments string,
) []byte {
	t.Helper()
	references := make([]string, len(catalogues))
	for index, catalogue := range catalogues {
		references[index] = fmt.Sprintf(
			`{"locale":%q,"path":%q,"sha256":%q,"bytes":%d}`,
			catalogue.locale, catalogue.path, sha256Hex(catalogue.data), len(catalogue.data))
	}
	return []byte(fmt.Sprintf(`{
  "schemaVersion": 1,
  "scenarioId": "training-demo",
  "revision": 1,
  "locale": "en",
  "fallbackLocale": "en",
  "messageCatalogues": [%s],
  "chapterRefs": ["retries"],
  "actors": [{"id":"guide","name":"Guide"}],
  "assets": [{"id":"photo","mediaType":"image/png","path":"training/photo.png","sha256":"%s","bytes":5}],
  "segments": [%s],
  "events": [%s]
}`, strings.Join(references, ","), strings.Repeat("b", 64), segments, events))
}

const defaultDeclarationSteps = `{"id":"solve","kind":"challenge","title":"Inspect requests","why":"Validate the retry query.","target":{"kind":"challenge","challengeId":"retry-query","revision":1},"done":{"kind":"challenge-passed","checkpoints":[{"id":"result","completion":"whole-challenge"}]}},
      {"id":"discussion","kind":"scripted-dialogue","segmentId":"discussion","title":"Discuss","why":"Explain the result.","modes":["guided"],"target":{"kind":"none"},"done":{"kind":"dialogue-finished"}}`

func declarationBytesFixture(narrative []byte) []byte {
	return declarationBytesFixtureWithSteps(narrative, defaultDeclarationSteps)
}

func declarationBytesFixtureWithSteps(narrative []byte, steps string) []byte {
	packageRef := packageReferenceFixture()
	return []byte(fmt.Sprintf(`{
  "schema": "datatug-walkthrough/2",
  "id": "training-demo",
  "version": 1,
  "title": "Retry investigation",
  "trainingPackage": {"id":"%s","revision":%s,"path":"%s","sha256":"%s","bytes":%s},
  "narrative": {"path":"walkthroughs/training-demo/narrative.json","sha256":"%s","bytes":%d},
  "chapters": [{
    "id": "retries",
    "title": "Inspect retries",
    "why": "Find repeated requests.",
    "steps": [
      %s
    ]
  }]
}`, packageRef.ID, packageRef.Revision, packageRef.Path, packageRef.SHA256, packageRef.Bytes,
		sha256Hex(narrative), len(narrative), steps))
}

func newNarrativeFixture(t *testing.T) *narrativeFixture {
	t.Helper()
	return newNarrativeFixtureWith(t, catalogueBytesFixture(""), "", "")
}

func newNarrativeFixtureWith(t *testing.T, catalogue []byte, extraEvents, extraSegments string) *narrativeFixture {
	t.Helper()
	return newNarrativeFixtureCatalogues(t, []catalogueFixture{{
		locale: "en",
		path:   "walkthroughs/training-demo/messages.en.json",
		data:   catalogue,
	}}, defaultNarrativeEvents+func() string {
		if extraEvents == "" {
			return ""
		}
		return ",\n" + extraEvents
	}(), defaultNarrativeEventIDs, defaultNarrativeSegments+func() string {
		if extraSegments == "" {
			return ""
		}
		return ",\n" + extraSegments
	}())
}

func newNarrativeFixtureCatalogues(
	t *testing.T,
	catalogues []catalogueFixture,
	events string,
	eventIDs string,
	segments string,
) *narrativeFixture {
	t.Helper()
	sidecar := sidecarBytesWith(t, catalogues, events, eventIDs, segments)
	return newNarrativeFixtureFromBytes(t, declarationBytesFixture(sidecar), sidecar, catalogues)
}

func newNarrativeFixtureFromBytes(
	t *testing.T,
	declaration []byte,
	sidecar []byte,
	catalogues []catalogueFixture,
) *narrativeFixture {
	t.Helper()
	parsed := ParseTrainingDeclaration("training-demo", declaration)
	require.True(t, parsed.Accepted, "%v", parsed.Issues)

	packageRef := packageReferenceFixture()
	source := SourceContext{
		Project:           ProjectIdentity{Value: "project-one"},
		ContentPin:        ContentPin{Kind: PinGit, Commit: fixtureCommit},
		DeclarationSHA256: sha256Hex(declaration),
	}
	admitted := map[string]bool{}
	catalogueRefs := make([]AssetReference, len(catalogues))
	for index, catalogue := range catalogues {
		ref := AssetReference{
			Path:   catalogue.path,
			SHA256: sha256Hex(catalogue.data),
			Bytes:  Integer(fmt.Sprint(len(catalogue.data))),
		}
		catalogueRefs[index] = ref
		admitted[assetReferenceKey(ref)] = true
	}
	narrativeRef := AssetReference{
		Path:   "walkthroughs/training-demo/narrative.json",
		SHA256: sha256Hex(sidecar),
		Bytes:  Integer(fmt.Sprint(len(sidecar))),
	}
	photoRef := AssetReference{Path: "training/photo.png", SHA256: strings.Repeat("b", 64), Bytes: "5"}
	evidenceRef := AssetReference{Path: "training/evidence.txt", SHA256: strings.Repeat("c", 64), Bytes: "10"}
	checkRef := AssetReference{Path: "training/check.json", SHA256: strings.Repeat("d", 64), Bytes: "20"}
	admitted[assetReferenceKey(narrativeRef)] = true
	admitted[assetReferenceKey(photoRef)] = true
	owner := &fakeOwner{
		ref:            packageRef,
		packageAssets:  []AssetReference{evidenceRef, checkRef},
		packageCases:   []string{"case-one"},
		admittedAssets: admitted,
		challenge:      ChallengeBinding{ChallengeID: "retry-query", Revision: "1"},
	}
	binding := OwnerBindingKey{
		Project:           source.Project,
		ContentPin:        source.ContentPin,
		DeclarationSHA256: source.DeclarationSHA256,
		Package:           packageRef,
	}
	owner.narrative = NarrativeInventory{
		Binding: binding,
		Assets: []PackageAssetBinding{
			{AssetID: "evidence", Kind: "evidence-text", Reference: evidenceRef, Bytes: evidenceRef.Bytes},
			{AssetID: "owner-check-definition", Kind: "check-definition", Reference: checkRef, Bytes: checkRef.Bytes},
		},
		Cases:     []string{"case-one"},
		Incidents: []string{"incident-one"},
	}
	resolution := ResolveTraining(parsed.Definition, source, fakeAuthority{}, owner)
	require.True(t, resolution.Admitted, "%v", resolution.Issues)
	fixture := &narrativeFixture{
		declarationBytes: declaration,
		declaration:      parsed.Definition,
		sidecarBytes:     sidecar,
		packageRef:       packageRef,
		owner:            owner,
		authority:        fakeAuthority{},
		bundle:           resolution.Bundle,
		source:           source,
	}
	if len(catalogues) > 0 {
		fixture.catalogueBytes = catalogues[0].data
	}
	return fixture
}

func admissionInputFor(t *testing.T, f *narrativeFixture, sidecar, catalogue []byte) NarrativeAdmissionInput {
	t.Helper()
	return NarrativeAdmissionInput{
		Sidecar: NarrativeAssetBytes{Reference: f.bundle.Narrative, Bytes: sidecar},
		Catalogues: []NarrativeAssetBytes{{
			Reference: sidecarCatalogueReference(t, sidecar),
			Bytes:     catalogue,
		}},
	}
}

func sidecarCatalogueReference(t *testing.T, sidecarBytes []byte) AssetReference {
	t.Helper()
	parsed := ParseNarrativeSidecar(sidecarBytes)
	require.True(t, parsed.Accepted, "%v", parsed.Issues)
	require.NotEmpty(t, parsed.Sidecar.MessageCatalogues)
	return parsed.Sidecar.MessageCatalogues[0].Asset()
}

// ---------------------------------------------------------------------------
// Sidecar parser tests.
// ---------------------------------------------------------------------------

func TestParseNarrativeSidecarAcceptsMinimal(t *testing.T) {
	f := newNarrativeFixture(t)
	result := ParseNarrativeSidecar(f.sidecarBytes)
	require.True(t, result.Accepted, "%v", result.Issues)
	require.Equal(t, sha256Hex(f.sidecarBytes), result.Digest)
	require.Empty(t, result.Issues)
	sidecar := result.Sidecar
	require.Equal(t, Integer("1"), sidecar.SchemaVersion)
	require.Equal(t, "training-demo", sidecar.ScenarioID)
	require.Equal(t, []string{"retries"}, sidecar.ChapterRefs)
	require.Len(t, sidecar.Events, 2)
	require.NotNil(t, sidecar.Events[1].ChatMessage)
	require.Equal(t, []string{"photo"}, sidecar.Events[1].ChatMessage.AttachmentIDs)
	require.NotNil(t, sidecar.Events[1].Presentation)
	require.Equal(t, 10, sidecar.Events[1].Presentation.DelayBeforeMs)
	require.Equal(t, "fast", sidecar.Events[1].Presentation.TypingSpeedPreset)
	require.Empty(t, sidecar.Events[1].Presentation.CameraTargetID)

	projected := sidecar.Projection()
	require.Equal(t, "1", projected["schemaVersion"])
	require.Equal(t, "training-demo", projected["scenarioId"])
}

func TestParseNarrativeSidecarRefusals(t *testing.T) {
	base := newNarrativeFixture(t)
	valid := string(base.sidecarBytes)
	cases := []struct {
		name string
		data string
		code Code
		path string
	}{
		{"unknown top field", strings.Replace(valid, `"schemaVersion": 1,`, `"schemaVersion": 1, "extra": 1,`, 1), CodeUnknownField, "/extra"},
		{"schemaVersion string", strings.Replace(valid, `"schemaVersion": 1`, `"schemaVersion": "1"`, 1), CodeInvalidType, "/schemaVersion"},
		{"schemaVersion two", strings.Replace(valid, `"schemaVersion": 1`, `"schemaVersion": 2`, 1), CodeInvalidValue, "/schemaVersion"},
		{"scenario id bad", strings.Replace(valid, `"scenarioId": "training-demo"`, `"scenarioId": "Training Demo"`, 1), CodeInvalidValue, "/scenarioId"},
		{"revision zero", strings.Replace(valid, `"revision": 1`, `"revision": 0`, 1), CodeInvalidValue, "/revision"},
		{"revision fraction", strings.Replace(valid, `"revision": 1`, `"revision": 1.5`, 1), CodeInvalidValue, "/revision"},
		{"locale malformed", strings.Replace(valid, `"locale": "en"`, `"locale": "en_US"`, 1), Code(LocaleCodeMalformed), "/locale"},
		{"duplicate actor id", strings.Replace(valid, `{"id":"guide","name":"Guide"}`, `{"id":"guide","name":"Guide"},{"id":"guide","name":"Other"}`, 1), CodeDuplicateID, "/actors/1/id"},
		{"actor name long", strings.Replace(valid, `"name":"Guide"`, `"name":"`+strings.Repeat("g", 121)+`"`, 1), CodeTooLong, "/actors/0/name"},
		{"asset media type long", strings.Replace(valid, `"mediaType":"image/png"`, `"mediaType":"`+strings.Repeat("m", 121)+`"`, 1), CodeTooLong, "/assets/0/mediaType"},
		{"unknown event type", strings.Replace(valid, `"type":"challenge.await_validation"`, `"type":"chat.literal"`, 1), CodeInvalidValue, "/events/0/type"},
		{"literal chat message", strings.Replace(valid, `"messageKey":"retry.explanation"`, `"message":"literal"`, 1), CodeUnknownField, "/events/1/payload/message"},
		{"unknown payload field", strings.Replace(valid, `"messageKey":"retry.explanation"`, `"messageKey":"retry.explanation","extra":1`, 1), CodeUnknownField, "/events/1/payload/extra"},
		{"branch target earlier", strings.Replace(strings.Replace(valid, `"eventIds":["await-result","explain"]`, `"eventIds":["branch-back","explain"]`, 1),
			`{"id":"await-result","chapterId":"retries","type":"challenge.await_validation","payload":{"stepId":"solve","checkpointId":"result"}}`,
			`{"id":"branch-back","chapterId":"retries","type":"branch","payload":{"stepId":"solve","checkpointId":"result","outcome":"passed","assisted":false,"targetEventId":"await-result"}}`, 1), CodeInvalidReference, "/events/0/payload/targetEventId"},
		{"segment chapter mismatch", strings.Replace(valid, `"chapterId":"retries","stepId":"discussion"`, `"chapterId":"other","stepId":"discussion"`, 1), CodeInvalidReference, "/segments/0/chapterId"},
		{"presentation unknown", strings.Replace(valid, `"transition":"fade"`, `"transition":"fade","extra":1`, 1), CodeUnknownField, "/events/1/presentation/extra"},
		{"typing speed low", strings.Replace(valid, `"typingSpeed":"fast"`, `"typingSpeed":4`, 1), CodeInvalidValue, "/events/1/presentation/typingSpeed"},
		{"typing speed high", strings.Replace(valid, `"typingSpeed":"fast"`, `"typingSpeed":201`, 1), CodeInvalidValue, "/events/1/presentation/typingSpeed"},
		{"delay too high", strings.Replace(valid, `"delayBeforeMs":10`, `"delayBeforeMs":10001`, 1), CodeInvalidValue, "/events/1/presentation/delayBeforeMs"},
		{"null actor", strings.Replace(valid, `"actors": [{"id":"guide","name":"Guide"}]`, `"actors": null`, 1), CodeInvalidType, "/actors"},
		{"malformed json", strings.Replace(valid, `"revision": 1`, `"revision": 1,`, 1), CodeInvalidJSON, ""},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			result := ParseNarrativeSidecar([]byte(test.data))
			require.False(t, result.Accepted)
			require.Nil(t, result.Sidecar)
			require.NotEmpty(t, result.Issues)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
			require.Equal(t, test.path, result.Issues[0].Path, "%v", result.Issues)
		})
	}
}

func TestParseNarrativeSidecarBounds(t *testing.T) {
	base := newNarrativeFixture(t)
	valid := string(base.sidecarBytes)
	cases := []struct {
		name string
		data string
		code Code
	}{
		{"catalogues over", strings.Replace(valid, `"messageCatalogues": [{"locale":"en","path":"walkthroughs/training-demo/messages.en.json","sha256":"`+sha256Hex(base.catalogueBytes)+`","bytes":`+fmt.Sprint(len(base.catalogueBytes))+`}]`, `"messageCatalogues": [`+strings.Repeat(`{"locale":"en","path":"a/b.json","sha256":"`+strings.Repeat("a", 64)+`","bytes":1},`, 20)+`{"locale":"en","path":"a/b.json","sha256":"`+strings.Repeat("a", 64)+`","bytes":1}]`, 1), CodeTooMany},
		{"chapter refs empty", strings.Replace(valid, `"chapterRefs": ["retries"]`, `"chapterRefs": []`, 1), CodeTooMany},
		{"actors empty", strings.Replace(valid, `"actors": [{"id":"guide","name":"Guide"}]`, `"actors": []`, 1), CodeTooMany},
		{"segments empty", strings.Replace(valid, `"segments": [`, `"segments": [], "unused": [`, 1), CodeUnknownField},
		{"events empty", strings.Replace(valid, `"events": [`, `"events": [], "unused": [`, 1), CodeUnknownField},
		{"too large", strings.Repeat(" ", MaxNarrativeSidecarBytes+1), CodeTooLarge},
		{"invalid utf8", string([]byte{0xff, 0xfe}), CodeInvalidUTF8},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := ParseNarrativeSidecar([]byte(test.data))
			require.False(t, result.Accepted)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
		})
	}
}

func TestParseNarrativeSidecarDuplicateAndChapterRefs(t *testing.T) {
	base := newNarrativeFixture(t)
	valid := string(base.sidecarBytes)
	repeated := strings.Replace(valid, `"chapterRefs": ["retries"]`, `"chapterRefs": ["retries","retries"]`, 1)
	result := ParseNarrativeSidecar([]byte(repeated))
	require.False(t, result.Accepted)
	require.Equal(t, CodeDuplicateID, result.Issues[0].Code)
	require.Equal(t, "/chapterRefs/1", result.Issues[0].Path)

	duplicateKey := strings.Replace(valid, `"scenarioId": "training-demo",`, `"scenarioId": "training-demo", "scenarioId": "training-demo",`, 1)
	result = ParseNarrativeSidecar([]byte(duplicateKey))
	require.False(t, result.Accepted)
	require.Equal(t, CodeDuplicateKey, result.Issues[0].Code)

	nullField := strings.Replace(valid, `"locale": "en"`, `"locale": null`, 1)
	result = ParseNarrativeSidecar([]byte(nullField))
	require.False(t, result.Accepted)
	require.Equal(t, CodeInvalidType, result.Issues[0].Code)
}

func TestParseNarrativeSidecarAliasesCollide(t *testing.T) {
	base := newNarrativeFixture(t)
	valid := string(base.sidecarBytes)
	// Two catalogue references whose spellings canonicalize to the same tag.
	aliased := strings.Replace(valid,
		`"messageCatalogues": [{"locale":"en","path":"walkthroughs/training-demo/messages.en.json","sha256":"`+sha256Hex(base.catalogueBytes)+`","bytes":`+fmt.Sprint(len(base.catalogueBytes))+`}]`,
		`"messageCatalogues": [{"locale":"en-US","path":"a/b.json","sha256":"`+strings.Repeat("a", 64)+`","bytes":1},{"locale":"EN-us","path":"a/c.json","sha256":"`+strings.Repeat("b", 64)+`","bytes":1}]`, 1)
	result := ParseNarrativeSidecar([]byte(aliased))
	require.False(t, result.Accepted)
	require.Equal(t, CodeDuplicateID, result.Issues[0].Code)
	require.Equal(t, "/messageCatalogues/1/locale", result.Issues[0].Path)
}

// ---------------------------------------------------------------------------
// Catalogue parser tests.
// ---------------------------------------------------------------------------

func TestParseMessageCatalogueAccepts(t *testing.T) {
	data := []byte(`{"schemaVersion":1,"locale":"en-US","messages":{"z.last":"Z","a.first":"A"}}`)
	result := ParseMessageCatalogue(data)
	require.True(t, result.Accepted, "%v", result.Issues)
	require.Equal(t, sha256Hex(data), result.Digest)
	require.Equal(t, "en-US", result.Catalogue.Locale)
	require.Equal(t, []string{"a.first", "z.last"}, result.Catalogue.Keys())
	require.Equal(t, map[string]any{
		"schemaVersion": "1",
		"locale":        "en-US",
		"messages":      map[string]string{"a.first": "A", "z.last": "Z"},
	}, result.Catalogue.Projection())
}

func TestParseMessageCatalogueRefusals(t *testing.T) {
	cases := []struct {
		name string
		data string
		code Code
		path string
	}{
		{"unknown field", `{"schemaVersion":1,"locale":"en","messages":{"a":"A"},"extra":1}`, CodeUnknownField, "/extra"},
		{"missing messages", `{"schemaVersion":1,"locale":"en"}`, CodeMissingField, "/messages"},
		{"empty messages", `{"schemaVersion":1,"locale":"en","messages":{}}`, CodeTooMany, "/messages"},
		{"bad key", `{"schemaVersion":1,"locale":"en","messages":{"A":"A"}}`, CodeInvalidValue, "/messages/A"},
		{"empty value", `{"schemaVersion":1,"locale":"en","messages":{"a":""}}`, CodeInvalidValue, "/messages/a"},
		{"value number", `{"schemaVersion":1,"locale":"en","messages":{"a":1}}`, CodeInvalidType, "/messages/a"},
		{"bad locale", `{"schemaVersion":1,"locale":"en_US","messages":{"a":"A"}}`, Code(LocaleCodeMalformed), "/locale"},
		{"schema string", `{"schemaVersion":"1","locale":"en","messages":{"a":"A"}}`, CodeInvalidType, "/schemaVersion"},
		{"schema two", `{"schemaVersion":2,"locale":"en","messages":{"a":"A"}}`, CodeInvalidValue, "/schemaVersion"},
		{"duplicate key", `{"schemaVersion":1,"locale":"en","messages":{"a":"A","a":"B"}}`, CodeDuplicateKey, "/messages/a"},
		{"non object", `[]`, CodeNotObject, ""},
		{"invalid utf8", string([]byte{0xff}), CodeInvalidUTF8, ""},
		{"too large", strings.Repeat(" ", MaxCatalogueBytes+1), CodeTooLarge, ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := ParseMessageCatalogue([]byte(test.data))
			require.False(t, result.Accepted)
			require.Nil(t, result.Catalogue)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
			require.Equal(t, test.path, result.Issues[0].Path, "%v", result.Issues)
		})
	}
}

func TestParseMessageCatalogueBounds(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(`{"schemaVersion":1,"locale":"en","messages":{`)
	for index := 0; index < MaxCatalogueMessages+1; index++ {
		if index > 0 {
			builder.WriteString(",")
		}
		fmt.Fprintf(&builder, `"k%d":"v"`, index)
	}
	builder.WriteString(`}}`)
	result := ParseMessageCatalogue([]byte(builder.String()))
	require.False(t, result.Accepted)
	require.Equal(t, CodeTooMany, result.Issues[0].Code)

	// 64 KiB of multibyte text stays within the byte bound.
	message := strings.Repeat("\u00e9", MaxTextAssetBytes/2-100)
	data := []byte(`{"schemaVersion":1,"locale":"en","messages":{"a":"` + message + `"}}`)
	require.LessOrEqual(t, len(data), MaxCatalogueBytes)
	result = ParseMessageCatalogue(data)
	require.True(t, result.Accepted, "%v", result.Issues)
	require.Equal(t, message, result.Catalogue.Values["a"])
}

// ---------------------------------------------------------------------------
// Locale selection tests.
// ---------------------------------------------------------------------------

func TestSelectNarrativeLocale(t *testing.T) {
	request := func(value string) *string { return &value }
	cases := []struct {
		name      string
		request   *string
		declared  []string
		defaultL  string
		fallback  string
		selected  string
		matched   string
		requested string
	}{
		{"absent uses default", nil, []string{"en-US", "fr"}, "en-US", "en-US", "en-US", "exact", "en-US"},
		{"exact alias", request("en-us"), []string{"en-US", "fr"}, "en-US", "en-US", "en-US", "exact", "en-US"},
		{"primary", request("en-GB"), []string{"en", "fr"}, "en", "fr", "en", "primary", "en-GB"},
		{"fallback", request("de-DE"), []string{"en", "fr"}, "en", "fr", "fr", "fallback", "de-DE"},
		{"unknown well formed", request("zz-ZZ"), []string{"en", "fr"}, "en", "fr", "fr", "fallback", "zz-ZZ"},
		{"private only skips primary", request("x-private"), []string{"en", "x-private"}, "en", "en", "x-private", "exact", "x-private"},
		{"grandfathered alias", request("iw"), []string{"he", "en"}, "he", "en", "he", "exact", "he"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := SelectNarrativeLocale(test.request, test.declared, test.defaultL, test.fallback)
			require.True(t, result.Accepted, "%v", result.Issues)
			require.Equal(t, test.selected, result.Choice.SelectedLocale)
			require.Equal(t, test.matched, result.Choice.Matched)
			require.Equal(t, test.requested, result.Choice.RequestedLocale)
			require.Equal(t, test.fallback, result.Choice.FallbackLocale)
		})
	}
}

func TestSelectNarrativeLocaleRefusals(t *testing.T) {
	request := func(value string) *string { return &value }
	cases := []struct {
		name     string
		request  *string
		declared []string
		defaultL string
		fallback string
		code     Code
		path     string
	}{
		{"malformed request", request("en_US"), []string{"en"}, "en", "en", Code(LocaleCodeMalformed), "/request"},
		{"empty request", request(""), []string{"en"}, "en", "en", Code(LocaleCodeEmpty), "/request"},
		{"duplicate declared", nil, []string{"en-US", "en-us"}, "en-US", "en-US", CodeDuplicateID, "/declaredLocales/1"},
		{"default missing", nil, []string{"fr"}, "en", "fr", CodeInvalidReference, "/defaultLocale"},
		{"fallback missing", nil, []string{"en"}, "en", "fr", CodeInvalidReference, "/fallbackLocale"},
		{"unknown authored", nil, []string{"zz"}, "zz", "zz", Code(LocaleCodeUnsupportedRegistryVersion), "/declaredLocales/0"},
		{"too long", request(strings.Repeat("a", 65)), []string{"en"}, "en", "en", Code(LocaleCodeTooLong), "/request"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := SelectNarrativeLocale(test.request, test.declared, test.defaultL, test.fallback)
			require.False(t, result.Accepted)
			require.Equal(t, test.code, result.Issues[0].Code, "%v", result.Issues)
			require.Equal(t, test.path, result.Issues[0].Path, "%v", result.Issues)
		})
	}
}
