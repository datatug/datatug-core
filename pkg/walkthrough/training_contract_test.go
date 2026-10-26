package walkthrough

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The doubles below prove wiring only. They are not project authority: the
// production Apps adapter reuses the owning project normalization.
type testAuthority struct {
	bind  func(raw string) (ProjectBinding, error)
	admit func(raw []byte) (ProjectIdentity, error)
	equal func(a, b ProjectIdentity) bool
}

func (a testAuthority) BindProject(raw string) (ProjectBinding, error) {
	if a.bind != nil {
		return a.bind(raw)
	}
	return ProjectBinding{}, errors.New("unexpected BindProject")
}

func (a testAuthority) AdmitProject(raw []byte) (ProjectIdentity, error) {
	if a.admit != nil {
		return a.admit(raw)
	}
	return ProjectIdentity{}, errors.New("unexpected AdmitProject")
}

func (a testAuthority) EqualProject(left, right ProjectIdentity) bool {
	if a.equal != nil {
		return a.equal(left, right)
	}
	return left.Value == right.Value
}

type testOwners struct {
	inventory    PackageInventory
	packageErr   error
	assetErr     error
	challengeErr error
	binding      ChallengeBinding
}

func (o testOwners) AdmitPackage(PackageReference) (PackageInventory, error) {
	return o.inventory, o.packageErr
}

func (o testOwners) AdmitAsset(AssetReference) error { return o.assetErr }

func (o testOwners) AdmitChallenge(string, Integer) (ChallengeBinding, error) {
	return o.binding, o.challengeErr
}

func identity(name string) ProjectIdentity { return ProjectIdentity{Value: name} }

func trainingFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "declarations-v2", name+".json"))
	require.NoError(t, err)
	return data
}

func admittedTraining(t *testing.T, name string) *TrainingDeclaration {
	t.Helper()
	result := ParseTrainingDeclaration("training-demo", trainingFixture(t, name))
	require.True(t, result.Accepted, "%+v", result.Issues)
	require.NotNil(t, result.Definition)
	return result.Definition
}

func trainingSource(declaration *TrainingDeclaration) SourceContext {
	return SourceContext{
		Project:           identity("declaring-project"),
		ContentPin:        ContentPin{Kind: PinGit, Commit: strings.Repeat("a", 40)},
		DeclarationSHA256: declaration.Revision,
	}
}

func matchingInventory(declaration *TrainingDeclaration) PackageInventory {
	return PackageInventory{Ref: declaration.TrainingPackage, Cases: []string{"case-1"}}
}

func TestAssetReferenceVerifyBytes(t *testing.T) {
	data := []byte(`{"ok":true}`)
	asset := AssetReference{Path: "training/package.json", SHA256: sha256Hex(data), Bytes: Integer("11")}
	require.NoError(t, asset.VerifyBytes(data))

	wrongLength := asset
	wrongLength.Bytes = Integer("12")
	require.Equal(t, CodeByteCountMismatch, ErrorCode(wrongLength.VerifyBytes(data)))

	wrongDigest := asset
	wrongDigest.SHA256 = strings.Repeat("0", 64)
	require.Equal(t, CodeDigestMismatch, ErrorCode(wrongDigest.VerifyBytes(data)))

	pkg := PackageReference{ID: "training-demo", Revision: Integer("1"), Path: asset.Path, SHA256: asset.SHA256, Bytes: asset.Bytes}
	require.NoError(t, pkg.VerifyBytes(data))
	require.Equal(t, asset, pkg.Asset())
	tampered := pkg
	tampered.Bytes = Integer("1")
	require.Equal(t, CodeByteCountMismatch, ErrorCode(tampered.VerifyBytes(data)))
}

func TestReferenceEquality(t *testing.T) {
	asset := AssetReference{Path: "a", SHA256: strings.Repeat("a", 64), Bytes: Integer("1")}
	require.True(t, asset.Equal(asset))
	require.False(t, asset.Equal(AssetReference{Path: "b", SHA256: asset.SHA256, Bytes: asset.Bytes}))
	require.False(t, asset.Equal(AssetReference{Path: "a", SHA256: strings.Repeat("b", 64), Bytes: asset.Bytes}))
	require.False(t, asset.Equal(AssetReference{Path: "a", SHA256: asset.SHA256, Bytes: Integer("2")}))

	pkg := PackageReference{ID: "p", Revision: Integer("1"), Path: "a", SHA256: asset.SHA256, Bytes: Integer("1")}
	require.True(t, pkg.Equal(pkg))
	require.False(t, pkg.Equal(PackageReference{ID: "q", Revision: Integer("1"), Path: "a", SHA256: asset.SHA256, Bytes: Integer("1")}))
	require.False(t, pkg.Equal(PackageReference{ID: "p", Revision: Integer("2"), Path: "a", SHA256: asset.SHA256, Bytes: Integer("1")}))
	require.False(t, pkg.Equal(PackageReference{ID: "p", Revision: Integer("1"), Path: "b", SHA256: asset.SHA256, Bytes: Integer("1")}))
}

func TestContentPinValidityAndEquality(t *testing.T) {
	git := ContentPin{Kind: PinGit, Commit: strings.Repeat("a", 40)}
	digest := ContentPin{Kind: PinDigest, SHA256: strings.Repeat("b", 64)}
	require.True(t, git.Valid())
	require.True(t, digest.Valid())
	require.False(t, ContentPin{Kind: PinGit, Commit: "main"}.Valid())
	require.False(t, ContentPin{Kind: PinGit, Commit: strings.Repeat("a", 40), SHA256: digest.SHA256}.Valid())
	require.False(t, ContentPin{Kind: PinDigest, SHA256: strings.ToUpper(digest.SHA256)}.Valid())
	require.False(t, ContentPin{Kind: PinDigest, Commit: git.Commit, SHA256: digest.SHA256}.Valid())
	require.False(t, ContentPin{Kind: "future"}.Valid())

	require.True(t, git.Equal(git))
	require.False(t, git.Equal(digest))
	require.False(t, git.Equal(ContentPin{Kind: PinGit, Commit: strings.Repeat("c", 40)}))
	require.True(t, digest.Equal(digest))
	require.False(t, digest.Equal(ContentPin{Kind: PinDigest, SHA256: strings.Repeat("c", 64)}))
	require.False(t, ContentPin{Kind: "future"}.Equal(ContentPin{Kind: "future"}))
}

func TestTrainingParserDefaultsAndVariants(t *testing.T) {
	declaration := admittedTraining(t, "v2-minimal")
	require.Equal(t, SchemaV2, declaration.Schema)
	require.Equal(t, "training-demo", declaration.TrainingPackage.ID)
	require.Equal(t, "walkthroughs/training-demo/narrative.json", declaration.Narrative.Path)
	require.Len(t, declaration.Chapters, 1)

	challenge := declaration.Chapters[0].Steps[0]
	require.Equal(t, "challenge", challenge.Kind)
	require.Empty(t, challenge.SegmentID)
	require.NotNil(t, challenge.Target.Challenge)
	require.Equal(t, "retry-query", challenge.Target.Challenge.ChallengeID)
	require.Equal(t, Integer("1"), challenge.Target.Challenge.Revision)
	require.NotNil(t, challenge.Done.ChallengePassed)
	require.Equal(t, []CheckpointMapping{{ID: "result", Completion: CompletionWholeChallenge}},
		challenge.Done.ChallengePassed.Checkpoints)

	dialogue := declaration.Chapters[0].Steps[1]
	require.Equal(t, "scripted-dialogue", dialogue.Kind)
	require.Equal(t, "discussion", dialogue.SegmentID)
	require.NotNil(t, dialogue.Done.DialogueFinished)
	require.Equal(t, "solve", declaration.Chapters[0].EntryStepID)

	caseDeclaration := admittedTraining(t, "v2-standard-training-case")
	standard := caseDeclaration.Chapters[0].Steps[0]
	require.Equal(t, "standard", standard.Kind)
	require.NotNil(t, standard.Target.TrainingCase)
	require.Equal(t, "case-1", standard.Target.TrainingCase.ManifestCaseID)
	require.True(t, standard.Target.TrainingCase.TrainingPackage.Equal(caseDeclaration.TrainingPackage))
}

func TestTrainingIdentityAndProjection(t *testing.T) {
	declaration := admittedTraining(t, "v2-large-integers")
	walkthrough := declaration.WalkthroughIdentity()
	require.Equal(t, "training-demo", walkthrough.ID)
	require.Equal(t, declaration.Revision, walkthrough.Revision)
	require.Equal(t, []string{"retries"}, walkthrough.Chapters)

	projected := declaration.Projection()
	require.Equal(t, "9007199254740992", projected["version"])
	narrative := projected["narrative"].(map[string]any)
	require.Equal(t, "1000000000000000000000000000000", narrative["bytes"])
	packageRef := projected["trainingPackage"].(map[string]any)
	require.Equal(t, "18446744073709551615", packageRef["revision"])

	registry := StoryRegistryEntry{ContentPin: ContentPin{Kind: "future"}}
	require.Equal(t, map[string]any{"kind": "future"}, registry.Projection()["contentPin"])
	draft := EnvelopeDraft{ContentPin: ContentPin{Kind: "future"}}
	require.Equal(t, false, draft.Projection()["projectPresent"])
	require.Equal(t, map[string]any{"kind": "future"}, draft.Projection()["contentPin"])
}

func TestValidAssetPath(t *testing.T) {
	require.True(t, validAssetPath("training/package.json"))
	require.True(t, validAssetPath("walkthroughs/demo/narrative.json"))
	require.False(t, validAssetPath(""))
	require.False(t, validAssetPath("/training/package.json"))
	require.False(t, validAssetPath("training//package.json"))
	require.False(t, validAssetPath("training/./package.json"))
	require.False(t, validAssetPath("training/../package.json"))
	require.False(t, validAssetPath("training\\package.json"))
	require.False(t, validAssetPath("https://example.com/package.json"))
	require.False(t, validAssetPath("training/package.json?v=1"))
	require.False(t, validAssetPath("training/package.json#fragment"))
	require.False(t, validAssetPath("training/\x01package.json"))
	require.False(t, validAssetPath("training/\x7fpackage.json"))
	require.False(t, validAssetPath(" training/package.json"))
	require.False(t, validAssetPath("training /package.json"))
}

func TestHexHelpers(t *testing.T) {
	require.True(t, isLowerHex(strings.Repeat("a1", 32), 64))
	require.False(t, isLowerHex(strings.Repeat("A1", 32), 64))
	require.False(t, isLowerHex("abc", 64))
	require.False(t, isLowerHex("g"+strings.Repeat("a", 63), 64))
	require.True(t, isHex(strings.Repeat("A1", 32), 64))
	require.False(t, isHex("g"+strings.Repeat("a", 63), 64))
	require.False(t, isHex(strings.Repeat("a", 63), 64))
}

func TestSchemaMajorClassification(t *testing.T) {
	major, ok := schemaMajor("datatug-walkthrough/10")
	require.True(t, ok)
	require.Equal(t, "10", major)
	_, ok = schemaMajor("not-a-schema")
	require.False(t, ok)
	require.True(t, majorGreater("10", "2"))
	require.True(t, majorGreater("3", "2"))
	require.False(t, majorGreater("1", "2"))
	require.False(t, majorGreater("0", "2"))
}

func TestResolveTrainingRefusals(t *testing.T) {
	declaration := admittedTraining(t, "v2-minimal")
	source := trainingSource(declaration)
	authority := testAuthority{}
	owners := testOwners{inventory: matchingInventory(declaration)}

	require.Equal(t, CodeMissingContext, ErrorCode(ResolveTraining(nil, source, authority, owners).Err()))
	require.Equal(t, CodeMissingContext, ErrorCode(ResolveTraining(declaration, source, nil, owners).Err()))
	require.Equal(t, CodeMissingContext, ErrorCode(ResolveTraining(declaration, source, authority, nil).Err()))

	noProject := source
	noProject.Project = ProjectIdentity{}
	require.Equal(t, CodeMissingContext, ErrorCode(ResolveTraining(declaration, noProject, authority, owners).Err()))

	noPin := source
	noPin.ContentPin = ContentPin{Kind: PinGit, Commit: "main"}
	require.Equal(t, CodePinMismatch, ErrorCode(ResolveTraining(declaration, noPin, authority, owners).Err()))

	shortDigest := source
	shortDigest.DeclarationSHA256 = "abc"
	require.Equal(t, CodeDigestMismatch, ErrorCode(ResolveTraining(declaration, shortDigest, authority, owners).Err()))

	wrongDigest := source
	wrongDigest.DeclarationSHA256 = strings.Repeat("0", 64)
	require.Equal(t, CodeDigestMismatch, ErrorCode(ResolveTraining(declaration, wrongDigest, authority, owners).Err()))

	packageRefused := testOwners{packageErr: errors.New("package unavailable")}
	require.Equal(t, CodeNotAdmitted, ErrorCode(ResolveTraining(declaration, source, authority, packageRefused).Err()))

	mismatchedInventory := testOwners{inventory: PackageInventory{Ref: PackageReference{ID: "other"}}}
	require.Equal(t, CodeMismatch, ErrorCode(ResolveTraining(declaration, source, authority, mismatchedInventory).Err()))

	assetRefused := testOwners{inventory: matchingInventory(declaration), assetErr: errors.New("asset unavailable")}
	require.Equal(t, CodeNotAdmitted, ErrorCode(ResolveTraining(declaration, source, authority, assetRefused).Err()))

	challengeRefused := testOwners{inventory: matchingInventory(declaration), challengeErr: errors.New("manifest unavailable")}
	resolution := ResolveTraining(declaration, source, authority, challengeRefused)
	require.Equal(t, CodeNotAdmitted, ErrorCode(resolution.Err()))
	require.Equal(t, "/chapters/0/steps/0/target/challengeId", resolution.Issues[0].Path)

	wrongChallenge := testOwners{
		inventory: matchingInventory(declaration),
		binding:   ChallengeBinding{ChallengeID: "other", Revision: Integer("1")},
	}
	resolution = ResolveTraining(declaration, source, authority, wrongChallenge)
	require.Equal(t, CodeMismatch, ErrorCode(resolution.Err()))
	require.Equal(t, "/chapters/0/steps/0/target/challengeId", resolution.Issues[0].Path)
}

func TestResolveTrainingAdmitsChallengeAndCase(t *testing.T) {
	declaration := admittedTraining(t, "v2-minimal")
	owners := testOwners{
		inventory: matchingInventory(declaration),
		binding:   ChallengeBinding{ChallengeID: "retry-query", Revision: Integer("1")},
	}
	resolution := ResolveTraining(declaration, trainingSource(declaration), testAuthority{}, owners)
	require.True(t, resolution.Admitted)
	require.NoError(t, resolution.Err())
	bundle := resolution.Bundle
	require.NotNil(t, bundle)
	require.Equal(t, "training-demo", bundle.WalkthroughID)
	require.Equal(t, []string{"retries"}, bundle.Chapters)
	require.Equal(t, declaration.Revision, bundle.DeclarationSHA256)
	require.Equal(t, declaration.Narrative.SHA256, bundle.NarrativeSHA256)
	require.Equal(t, declaration.TrainingPackage.SHA256, bundle.TrainingPackageSHA256)
	require.Equal(t, []AdmittedChallengeStep{{
		ChapterID:    "retries",
		StepID:       "solve",
		ChallengeID:  "retry-query",
		Revision:     Integer("1"),
		CheckpointID: "result",
	}}, bundle.ChallengeSteps)
	require.Equal(t, identity("declaring-project"), bundle.Project)

	caseDeclaration := admittedTraining(t, "v2-standard-training-case")
	caseResolution := ResolveTraining(
		caseDeclaration,
		trainingSource(caseDeclaration),
		testAuthority{},
		testOwners{inventory: matchingInventory(caseDeclaration)},
	)
	require.True(t, caseResolution.Admitted)
	require.Equal(t, []string{"case-1"}, caseResolution.Bundle.Cases)

	missingCase := ResolveTraining(
		caseDeclaration,
		trainingSource(caseDeclaration),
		testAuthority{},
		testOwners{inventory: PackageInventory{Ref: caseDeclaration.TrainingPackage}},
	)
	require.Equal(t, CodeNotAdmitted, ErrorCode(missingCase.Err()))
	require.Equal(t, "/chapters/0/steps/0/target/manifestCaseId", missingCase.Issues[0].Path)
}

func registryEntry() StoryRegistryEntry {
	return StoryRegistryEntry{
		StoryID:           "phantom-orders",
		Revision:          Integer("1"),
		ProjectURL:        "/project/github.com/datatug/datatug-demo-project/demo-project-1",
		WalkthroughID:     "training-demo",
		ContentPin:        ContentPin{Kind: PinGit, Commit: strings.Repeat("a", 40)},
		DeclarationSHA256: strings.Repeat("c", 64),
	}
}

func declaredWalkthrough() DeclaredWalkthrough {
	return DeclaredWalkthrough{
		ID:       "training-demo",
		Revision: strings.Repeat("C", 64),
		Chapters: []string{"retries", "discussion"},
	}
}

func publicBinding() ProjectBinding {
	return ProjectBinding{
		Identity:     identity("registered-project"),
		CanonicalURL: registryEntry().ProjectURL,
		Public:       true,
		Commit:       strings.Repeat("a", 40),
	}
}

func TestAdmitStoryChapter(t *testing.T) {
	entry := registryEntry()
	declared := declaredWalkthrough()
	authority := testAuthority{bind: func(string) (ProjectBinding, error) { return publicBinding(), nil }}

	admission := AdmitStoryChapter(entry, "retries", declared, authority)
	require.NoError(t, admission.Err())
	require.Equal(t, &RouteDescriptor{
		StoryID:           entry.StoryID,
		Revision:          entry.Revision,
		ProjectURL:        publicBinding().CanonicalURL,
		WalkthroughID:     entry.WalkthroughID,
		ChapterID:         "retries",
		ContentPin:        entry.ContentPin,
		DeclarationSHA256: entry.DeclarationSHA256,
	}, admission.Route)

	require.Equal(t, CodeMissingContext, ErrorCode(AdmitStoryChapter(entry, "retries", declared, nil).Err()))
	require.Equal(t, CodeInvalidReference, ErrorCode(AdmitStoryChapter(entry, "unknown", declared, authority).Err()))

	wrongWalkthrough := declared
	wrongWalkthrough.ID = "other"
	require.Equal(t, CodeMismatch, ErrorCode(AdmitStoryChapter(entry, "retries", wrongWalkthrough, authority).Err()))

	wrongRevision := declared
	wrongRevision.Revision = strings.Repeat("0", 64)
	require.Equal(t, CodeDigestMismatch, ErrorCode(AdmitStoryChapter(entry, "retries", wrongRevision, authority).Err()))

	invalidPin := entry
	invalidPin.ContentPin = ContentPin{Kind: PinGit, Commit: "main"}
	require.Equal(t, CodePinMismatch, ErrorCode(AdmitStoryChapter(invalidPin, "retries", declared, authority).Err()))

	refusing := testAuthority{bind: func(string) (ProjectBinding, error) { return ProjectBinding{}, errors.New("not a project") }}
	require.Equal(t, CodeNotAdmitted, ErrorCode(AdmitStoryChapter(entry, "retries", declared, refusing).Err()))

	privateBinding := testAuthority{bind: func(string) (ProjectBinding, error) {
		binding := publicBinding()
		binding.Public = false
		return binding, nil
	}}
	require.Equal(t, CodeIneligible, ErrorCode(AdmitStoryChapter(entry, "retries", declared, privateBinding).Err()))

	otherURL := testAuthority{bind: func(string) (ProjectBinding, error) {
		binding := publicBinding()
		binding.CanonicalURL = "/project/github.com/other/repo"
		return binding, nil
	}}
	require.Equal(t, CodeMismatch, ErrorCode(AdmitStoryChapter(entry, "retries", declared, otherURL).Err()))

	noCommit := testAuthority{bind: func(string) (ProjectBinding, error) {
		binding := publicBinding()
		binding.Commit = ""
		return binding, nil
	}}
	require.Equal(t, CodePinMismatch, ErrorCode(AdmitStoryChapter(entry, "retries", declared, noCommit).Err()))

	otherCommit := testAuthority{bind: func(string) (ProjectBinding, error) {
		binding := publicBinding()
		binding.Commit = strings.Repeat("b", 40)
		return binding, nil
	}}
	require.Equal(t, CodePinMismatch, ErrorCode(AdmitStoryChapter(entry, "retries", declared, otherCommit).Err()))

	digestEntry := entry
	digestEntry.ContentPin = ContentPin{Kind: PinDigest, SHA256: strings.Repeat("d", 64)}
	digestBinding := testAuthority{bind: func(string) (ProjectBinding, error) {
		binding := publicBinding()
		binding.Commit = ""
		binding.DigestAddressed = true
		return binding, nil
	}}
	require.NoError(t, AdmitStoryChapter(digestEntry, "retries", declared, digestBinding).Err())

	notDigestAddressed := testAuthority{bind: func(string) (ProjectBinding, error) {
		binding := publicBinding()
		binding.Commit = ""
		return binding, nil
	}}
	require.Equal(t, CodePinMismatch, ErrorCode(AdmitStoryChapter(digestEntry, "retries", declared, notDigestAddressed).Err()))

	digestWithCommit := testAuthority{bind: func(string) (ProjectBinding, error) {
		return publicBinding(), nil
	}}
	require.Equal(t, CodePinMismatch, ErrorCode(AdmitStoryChapter(digestEntry, "retries", declared, digestWithCommit).Err()))
}

func TestStoryRegistryShape(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "declarations-v2", "registry-valid.json"))
	require.NoError(t, err)
	result := ParseStoryRegistryEntry(data)
	require.NoError(t, result.Err())
	require.Equal(t, "phantom-orders", result.Entry.StoryID)

	require.Equal(t, CodeInvalidJSON, ErrorCode(ParseStoryRegistryEntry([]byte("{")).Err()))
	require.Equal(t, CodeNotObject, ErrorCode(ParseStoryRegistryEntry([]byte("[]")).Err()))
	require.Equal(t, CodeTooLarge, ErrorCode(ParseStoryRegistryEntry(make([]byte, MaxDeclarationBytes+1)).Err()))
}

func TestEnvelopeRawProjectAndAdmission(t *testing.T) {
	data := []byte(`{"project":"demo@datatug@x@HEAD","authorityGeneration":1,` +
		`"contentPin":{"kind":"git","commit":"0123456789abcdef0123456789abcdef01234567"},` +
		`"declarationSha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",` +
		`"narrativeSha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",` +
		`"trainingPackageSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",` +
		`"walkthroughId":"training-demo","chapterId":"retries","stepId":"solve",` +
		`"challengeId":"retry-query","challengeRevision":1,"checkpointId":"result",` +
		`"attemptId":"a","submissionGeneration":1,"outcome":"passed","complete":true,"assisted":false}`)
	parsed := ParseCheckpointEnvelope(data)
	require.NoError(t, parsed.Err())
	require.Equal(t, []byte(`"demo@datatug@x@HEAD"`), parsed.Draft.ProjectRaw)

	authority := testAuthority{admit: func(raw []byte) (ProjectIdentity, error) {
		require.Equal(t, `"demo@datatug@x@HEAD"`, string(raw))
		return identity("envelope-project"), nil
	}}
	admission := AdmitCheckpointEnvelope(*parsed.Draft, authority)
	require.NoError(t, admission.Err())
	require.Equal(t, identity("envelope-project"), admission.Envelope.Project)

	require.Equal(t, CodeMissingContext, ErrorCode(AdmitCheckpointEnvelope(*parsed.Draft, nil).Err()))
	refusing := testAuthority{admit: func([]byte) (ProjectIdentity, error) { return ProjectIdentity{}, errors.New("unknown project") }}
	require.Equal(t, CodeNotAdmitted, ErrorCode(AdmitCheckpointEnvelope(*parsed.Draft, refusing).Err()))
	empty := testAuthority{admit: func([]byte) (ProjectIdentity, error) { return ProjectIdentity{}, nil }}
	require.Equal(t, CodeNotAdmitted, ErrorCode(AdmitCheckpointEnvelope(*parsed.Draft, empty).Err()))
}

func activeWait() ActiveWait {
	return ActiveWait{
		Project:               identity("envelope-project"),
		AuthorityGeneration:   Integer("1"),
		ContentPin:            ContentPin{Kind: PinGit, Commit: strings.Repeat("a", 40)},
		DeclarationSHA256:     strings.Repeat("c", 64),
		NarrativeSHA256:       strings.Repeat("b", 64),
		TrainingPackageSHA256: strings.Repeat("a", 64),
		WalkthroughID:         "training-demo",
		ChapterID:             "retries",
		ChallengeStepID:       "solve",
		ChallengeID:           "retry-query",
		ChallengeRevision:     Integer("1"),
		CheckpointID:          "result",
		AttemptID:             "attempt-1",
		SubmissionGeneration:  Integer("1"),
		Dialogue: DialogueChallengeBinding{
			Validated:       true,
			SegmentID:       "discussion",
			ChapterID:       "retries",
			DialogueStepID:  "discussion",
			ChallengeStepID: "solve",
		},
	}
}

func matchingEnvelope() CheckpointEnvelope {
	return CheckpointEnvelope{
		Project: identity("envelope-project"),
		EnvelopeDraft: EnvelopeDraft{
			AuthorityGeneration:   Integer("1"),
			ContentPin:            ContentPin{Kind: PinGit, Commit: strings.Repeat("a", 40)},
			DeclarationSHA256:     strings.Repeat("c", 64),
			NarrativeSHA256:       strings.Repeat("b", 64),
			TrainingPackageSHA256: strings.Repeat("a", 64),
			WalkthroughID:         "training-demo",
			ChapterID:             "retries",
			StepID:                "solve",
			ChallengeID:           "retry-query",
			ChallengeRevision:     Integer("1"),
			CheckpointID:          "result",
			AttemptID:             "attempt-1",
			SubmissionGeneration:  Integer("1"),
			Outcome:               "passed",
			Complete:              true,
			Assisted:              true,
		},
	}
}

func TestMatchCheckpointEnvelope(t *testing.T) {
	authority := testAuthority{}
	decision := MatchCheckpointEnvelope(activeWait(), matchingEnvelope(), authority)
	require.Equal(t, MatchConsume, decision.Verdict)
	require.Equal(t, ReasonMatched, decision.Reason)

	require.Equal(t, noMatch(ReasonWaitInvalid), MatchCheckpointEnvelope(ActiveWait{}, matchingEnvelope(), authority))
	require.Equal(t, noMatch(ReasonMissingContext), MatchCheckpointEnvelope(activeWait(), matchingEnvelope(), nil))

	cases := []struct {
		reason string
		mutate func(*ActiveWait, *CheckpointEnvelope)
	}{
		{ReasonProjectMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.Project = identity("other") }},
		{ReasonAuthorityStale, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.AuthorityGeneration = Integer("2") }},
		{ReasonPinMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) {
			envelope.ContentPin = ContentPin{Kind: PinDigest, SHA256: strings.Repeat("d", 64)}
		}},
		{ReasonDeclarationMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) {
			envelope.DeclarationSHA256 = strings.Repeat("d", 64)
		}},
		{ReasonNarrativeMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) {
			envelope.NarrativeSHA256 = strings.Repeat("d", 64)
		}},
		{ReasonPackageMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) {
			envelope.TrainingPackageSHA256 = strings.Repeat("d", 64)
		}},
		{ReasonWalkthroughMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.WalkthroughID = "other" }},
		{ReasonChapterMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.ChapterID = "other" }},
		{ReasonStepMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.StepID = "other" }},
		{ReasonChallengeMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.ChallengeID = "other" }},
		{ReasonRevisionMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.ChallengeRevision = Integer("2") }},
		{ReasonCheckpointMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.CheckpointID = "other" }},
		{ReasonAttemptMismatch, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.AttemptID = "other" }},
		{ReasonSubmissionStale, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.SubmissionGeneration = Integer("2") }},
		{ReasonOutcomeNotPassed, func(wait *ActiveWait, envelope *CheckpointEnvelope) {
			envelope.Outcome = "failed"
			envelope.Complete = false
		}},
		{ReasonAttemptIncomplete, func(wait *ActiveWait, envelope *CheckpointEnvelope) { envelope.Complete = false }},
	}
	for _, testCase := range cases {
		wait := activeWait()
		envelope := matchingEnvelope()
		testCase.mutate(&wait, &envelope)
		require.Equal(t, noMatch(testCase.reason), MatchCheckpointEnvelope(wait, envelope, authority))
	}
}

func retainedCompletion() CompletionRecord {
	return CompletionRecord{
		Project:               identity("envelope-project"),
		AuthorityGeneration:   Integer("1"),
		ContentPin:            ContentPin{Kind: PinGit, Commit: strings.Repeat("a", 40)},
		DeclarationSHA256:     strings.Repeat("c", 64),
		NarrativeSHA256:       strings.Repeat("b", 64),
		TrainingPackageSHA256: strings.Repeat("a", 64),
		WalkthroughID:         "training-demo",
		ChapterID:             "retries",
		ChallengeStepID:       "solve",
		ChallengeID:           "retry-query",
		ChallengeRevision:     Integer("1"),
		CheckpointID:          "result",
		Outcome:               "passed",
		Complete:              true,
	}
}

func TestMatchRetainedCompletion(t *testing.T) {
	authority := testAuthority{}
	decision := MatchRetainedCompletion(retainedCompletion(), activeWait(), authority)
	require.Equal(t, MatchRevisit, decision.Verdict)
	require.Equal(t, ReasonMatched, decision.Reason)

	require.Equal(t, noMatch(ReasonWaitInvalid), MatchRetainedCompletion(retainedCompletion(), ActiveWait{}, authority))
	require.Equal(t, noMatch(ReasonMissingContext), MatchRetainedCompletion(retainedCompletion(), activeWait(), nil))

	cases := []struct {
		reason string
		mutate func(*CompletionRecord)
	}{
		{ReasonProjectMismatch, func(record *CompletionRecord) { record.Project = identity("other") }},
		{ReasonAuthorityStale, func(record *CompletionRecord) { record.AuthorityGeneration = Integer("2") }},
		{ReasonPinMismatch, func(record *CompletionRecord) {
			record.ContentPin = ContentPin{Kind: PinDigest, SHA256: strings.Repeat("d", 64)}
		}},
		{ReasonDeclarationMismatch, func(record *CompletionRecord) { record.DeclarationSHA256 = strings.Repeat("d", 64) }},
		{ReasonNarrativeMismatch, func(record *CompletionRecord) { record.NarrativeSHA256 = strings.Repeat("d", 64) }},
		{ReasonPackageMismatch, func(record *CompletionRecord) { record.TrainingPackageSHA256 = strings.Repeat("d", 64) }},
		{ReasonWalkthroughMismatch, func(record *CompletionRecord) { record.WalkthroughID = "other" }},
		{ReasonChapterMismatch, func(record *CompletionRecord) { record.ChapterID = "other" }},
		{ReasonStepMismatch, func(record *CompletionRecord) { record.ChallengeStepID = "other" }},
		{ReasonChallengeMismatch, func(record *CompletionRecord) { record.ChallengeID = "other" }},
		{ReasonRevisionMismatch, func(record *CompletionRecord) { record.ChallengeRevision = Integer("2") }},
		{ReasonCheckpointMismatch, func(record *CompletionRecord) { record.CheckpointID = "other" }},
		{ReasonOutcomeNotPassed, func(record *CompletionRecord) { record.Outcome = "failed"; record.Complete = false }},
		{ReasonAttemptIncomplete, func(record *CompletionRecord) { record.Complete = false }},
	}
	for _, testCase := range cases {
		record := retainedCompletion()
		testCase.mutate(&record)
		require.Equal(t, noMatch(testCase.reason), MatchRetainedCompletion(record, activeWait(), authority))
	}
}

func TestValidateActiveWait(t *testing.T) {
	require.Empty(t, ValidateActiveWait(activeWait()))

	cases := []func(*ActiveWait){
		func(wait *ActiveWait) { wait.Project = ProjectIdentity{} },
		func(wait *ActiveWait) { wait.AuthorityGeneration = Integer("0") },
		func(wait *ActiveWait) { wait.ContentPin = ContentPin{Kind: PinGit, Commit: "main"} },
		func(wait *ActiveWait) { wait.DeclarationSHA256 = "abc" },
		func(wait *ActiveWait) { wait.NarrativeSHA256 = strings.Repeat("A", 64) },
		func(wait *ActiveWait) { wait.WalkthroughID = "Bad" },
		func(wait *ActiveWait) { wait.ChallengeID = "Bad" },
		func(wait *ActiveWait) { wait.ChallengeRevision = Integer("1000001") },
		func(wait *ActiveWait) { wait.AttemptID = "" },
		func(wait *ActiveWait) { wait.SubmissionGeneration = Integer("0") },
		func(wait *ActiveWait) { wait.Dialogue.Validated = false },
		func(wait *ActiveWait) { wait.Dialogue.ChapterID = "other" },
		func(wait *ActiveWait) { wait.Dialogue.ChallengeStepID = "other" },
		func(wait *ActiveWait) { wait.Dialogue.SegmentID = "Bad" },
	}
	for index, mutate := range cases {
		wait := activeWait()
		mutate(&wait)
		require.NotEmpty(t, ValidateActiveWait(wait), "case %d", index)
	}
}

func TestTrainingParserSchemaGate(t *testing.T) {
	require.Equal(t, CodeInvalidJSON, ErrorCode(ParseTrainingDeclaration("x", []byte("{")).Err()))
	require.Equal(t, CodeMissingField, ErrorCode(ParseTrainingDeclaration("x", []byte(`{}`)).Err()))
	require.Equal(t, CodeInvalidType, ErrorCode(ParseTrainingDeclaration("x", []byte(`{"schema":1}`)).Err()))
	require.Equal(t, CodeInvalidValue, ErrorCode(ParseTrainingDeclaration("x", []byte(`{"schema":"x"}`)).Err()))
	require.Equal(t, CodeTooLarge, ErrorCode(ParseTrainingDeclaration("x", make([]byte, MaxDeclarationBytes+1)).Err()))
	require.Equal(t, CodeInvalidJSON, ErrorCode(ParseCheckpointEnvelope([]byte("{")).Err()))
}

func TestTrainingResultAndAggregateErrors(t *testing.T) {
	accepted := ParseTrainingDeclaration("training-demo", trainingFixture(t, "v2-minimal"))
	require.NoError(t, accepted.Err())
	require.True(t, accepted.Accepted)

	newer := ParseTrainingDeclaration("training-demo", trainingFixture(t, "v2-schema-3-newer"))
	require.ErrorIs(t, newer.Err(), ErrNewerClient)

	refused := ParseTrainingDeclaration("training-demo", trainingFixture(t, "v2-missing-package"))
	require.False(t, refused.Accepted)
	require.Equal(t, CodeMissingField, ErrorCode(refused.Err()))

	require.Equal(t, Code(""), ErrorCode(TrainingResult{}.Err()))
	require.Equal(t, Code(""), ErrorCode(Result{}.Err()))
	require.Equal(t, Code(""), ErrorCode(Resolution{}.Err()))
	require.Equal(t, Code(""), ErrorCode(RegistryAdmission{}.Err()))
	require.Equal(t, Code(""), ErrorCode(EnvelopeResult{}.Err()))
	require.Equal(t, Code(""), ErrorCode(EnvelopeAdmission{}.Err()))

	require.Equal(t, "1", Integer("1").String())
	require.True(t, KnownTargetKind("query"))
	require.False(t, KnownTargetKind("challenge"))
	require.False(t, KnownConditionKind("challenge-passed"))
}
