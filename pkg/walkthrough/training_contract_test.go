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
	require.True(t, validAssetPath("training/2024:01/package.json"))
	require.False(t, validAssetPath("mailto:package.json"))
	require.False(t, validAssetPath("c:/training/package.json"))
	require.True(t, validAssetPath("1abc:def/package.json"))
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
	digestEntry.ContentPin = ContentPin{Kind: PinDigest, SHA256: entry.DeclarationSHA256}
	digestBinding := testAuthority{bind: func(string) (ProjectBinding, error) {
		binding := publicBinding()
		binding.Commit = ""
		binding.DigestAddressed = true
		return binding, nil
	}}
	require.NoError(t, AdmitStoryChapter(digestEntry, "retries", declared, digestBinding).Err())

	mismatchedDigest := entry
	mismatchedDigest.ContentPin = ContentPin{Kind: PinDigest, SHA256: strings.Repeat("d", 64)}
	mismatchedAdmission := AdmitStoryChapter(mismatchedDigest, "retries", declared, digestBinding)
	require.Nil(t, mismatchedAdmission.Route)
	require.Equal(t, CodePinMismatch, ErrorCode(mismatchedAdmission.Err()))

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

// snapshotDeclarationJSON exercises every nested shape the admitted bundle must
// snapshot: chapters, former ids, steps, modes, named values, captured/literal
// scalars, target and condition mappings and a finish target.
const snapshotDeclarationJSON = `{
  "schema": "datatug-walkthrough/2",
  "id": "training-demo",
  "version": 1,
  "title": "Walkthrough training-demo",
  "trainingPackage": {
    "id": "training-demo",
    "revision": 1,
    "path": "training/package.json",
    "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "bytes": 100
  },
  "narrative": {
    "path": "walkthroughs/training-demo/narrative.json",
    "sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "bytes": 879
  },
  "chapters": [
    {
      "id": "find",
      "formerIds": ["old-find"],
      "title": "Chapter find",
      "why": "Why find",
      "steps": [
        {
          "id": "pick",
          "title": "Step pick",
          "why": "Why pick",
          "modes": ["guided", "explore"],
          "target": {
            "kind": "table-record",
            "source": "crm",
            "table": "customers",
            "key": {"alpha": "x", "beta": "y"}
          },
          "done": {"kind": "record-selected", "field": "customer_id", "equals": 7},
          "capture": "row"
        },
        {
          "id": "look",
          "title": "Step look",
          "why": "Why look",
          "target": {"kind": "incident", "incident": {"captured": "row", "default": null}},
          "done": {"kind": "query-run", "minRows": 2}
        }
      ]
    }
  ],
  "finish": {
    "target": {"kind": "query", "queryId": "summary", "parameters": {"scope": "all"}}
  }
}`

func snapshotDeclaration(t *testing.T) *TrainingDeclaration {
	t.Helper()
	result := ParseTrainingDeclaration("training-demo", []byte(snapshotDeclarationJSON))
	require.True(t, result.Accepted, "%+v", result.Issues)
	require.NotNil(t, result.Definition)
	return result.Definition
}

func digestTrainingSource(declaration *TrainingDeclaration) SourceContext {
	return SourceContext{
		Project:           identity("declaring-project"),
		ContentPin:        ContentPin{Kind: PinDigest, SHA256: declaration.Revision},
		DeclarationSHA256: declaration.Revision,
	}
}

func TestResolveTrainingRequiresDigestPinToEqualDeclaration(t *testing.T) {
	declaration := admittedTraining(t, "v2-minimal")
	owners := testOwners{
		inventory: matchingInventory(declaration),
		binding:   ChallengeBinding{ChallengeID: "retry-query", Revision: Integer("1")},
	}
	matching := ResolveTraining(declaration, digestTrainingSource(declaration), testAuthority{}, owners)
	require.True(t, matching.Admitted, "%+v", matching.Issues)
	require.Equal(t, ContentPin{Kind: PinDigest, SHA256: declaration.Revision}, matching.Bundle.ContentPin)

	mismatched := digestTrainingSource(declaration)
	mismatched.ContentPin.SHA256 = strings.Repeat("d", 64)
	refused := ResolveTraining(declaration, mismatched, testAuthority{}, owners)
	require.False(t, refused.Admitted)
	require.Nil(t, refused.Bundle)
	require.Equal(t, CodePinMismatch, ErrorCode(refused.Err()))

	// Two consistent but wrong fields cannot be paired to satisfy resolution:
	// the digest pin must equal the exact parsed declaration digest, not the
	// caller-supplied declaration digest.
	consistent := digestTrainingSource(declaration)
	consistent.ContentPin.SHA256 = strings.Repeat("d", 64)
	consistent.DeclarationSHA256 = strings.Repeat("d", 64)
	require.False(t, ResolveTraining(declaration, consistent, testAuthority{}, owners).Admitted)

	git := trainingSource(declaration)
	require.True(t, ResolveTraining(declaration, git, testAuthority{}, owners).Admitted)
}

func TestAdmitStoryChapterRequiresDigestPinToEqualDeclaration(t *testing.T) {
	entry := registryEntry()
	entry.ContentPin = ContentPin{Kind: PinDigest, SHA256: entry.DeclarationSHA256}
	declared := declaredWalkthrough()
	binding := testAuthority{bind: func(string) (ProjectBinding, error) {
		value := publicBinding()
		value.Commit = ""
		value.DigestAddressed = true
		return value, nil
	}}
	require.NoError(t, AdmitStoryChapter(entry, "retries", declared, binding).Err())

	mismatched := entry
	mismatched.ContentPin.SHA256 = strings.Repeat("d", 64)
	refused := AdmitStoryChapter(mismatched, "retries", declared, binding)
	require.Nil(t, refused.Route)
	require.Equal(t, CodePinMismatch, ErrorCode(refused.Err()))
}

func TestShapeValidUnboundRegistryEntryDoesNotAdmit(t *testing.T) {
	// registry-digest-pin.json is a valid closed shape with a digest pin that
	// does not equal its declarationSha256. Shape parsing must not turn it into
	// an admitted alias.
	result := ParseStoryRegistryEntry(trainingFixture(t, "registry-digest-pin"))
	require.NoError(t, result.Err())
	require.Equal(t, strings.Repeat("e", 64), result.Entry.ContentPin.SHA256)
	require.Equal(t, strings.Repeat("c", 64), result.Entry.DeclarationSHA256)

	declared := declaredWalkthrough()
	declared.Revision = result.Entry.DeclarationSHA256
	binding := testAuthority{bind: func(string) (ProjectBinding, error) {
		value := publicBinding()
		value.Commit = ""
		value.DigestAddressed = true
		return value, nil
	}}
	admission := AdmitStoryChapter(*result.Entry, "retries", declared, binding)
	require.Nil(t, admission.Route)
	require.Equal(t, CodePinMismatch, ErrorCode(admission.Err()))
}

func TestAdmitCheckpointEnvelopeRequiresDigestPinToEqualDeclaration(t *testing.T) {
	draft := matchingEnvelope().EnvelopeDraft
	draft.ContentPin = ContentPin{Kind: PinDigest, SHA256: draft.DeclarationSHA256}
	authority := testAuthority{admit: func([]byte) (ProjectIdentity, error) {
		return identity("envelope-project"), nil
	}}
	admitted := AdmitCheckpointEnvelope(draft, authority)
	require.NoError(t, admitted.Err())

	mismatched := draft
	mismatched.ContentPin.SHA256 = strings.Repeat("d", 64)
	refused := AdmitCheckpointEnvelope(mismatched, authority)
	require.Nil(t, refused.Envelope)
	require.Equal(t, CodePinMismatch, ErrorCode(refused.Err()))
}

func TestValidateActiveWaitRequiresDigestPinToEqualDeclaration(t *testing.T) {
	wait := activeWait()
	wait.ContentPin = ContentPin{Kind: PinDigest, SHA256: wait.DeclarationSHA256}
	require.Empty(t, ValidateActiveWait(wait))

	mismatched := wait
	mismatched.ContentPin.SHA256 = strings.Repeat("d", 64)
	require.NotEmpty(t, ValidateActiveWait(mismatched))
	require.Equal(t, noMatch(ReasonWaitInvalid),
		MatchCheckpointEnvelope(mismatched, matchingEnvelope(), testAuthority{}))
}

func TestResolveTrainingSnapshotsDeclaration(t *testing.T) {
	declaration := snapshotDeclaration(t)
	owners := testOwners{inventory: matchingInventory(declaration)}
	resolution := ResolveTraining(declaration, trainingSource(declaration), testAuthority{}, owners)
	require.True(t, resolution.Admitted, "%+v", resolution.Issues)
	bundle := resolution.Bundle
	require.NotNil(t, bundle)
	require.NotSame(t, declaration, bundle.Declaration)

	// Mutate every caller-owned level after admission. None may reach the
	// admitted snapshot.
	declaration.ID = "changed"
	declaration.TrainingPackage.ID = "changed"
	declaration.TrainingPackage.SHA256 = strings.Repeat("0", 64)
	declaration.Narrative.Path = "changed"
	declaration.Chapters[0].ID = "changed"
	declaration.Chapters[0].FormerIDs[0] = "changed"
	declaration.Chapters[0].Steps[0].ID = "changed"
	declaration.Chapters[0].Steps[0].Modes[0] = "changed"
	declaration.Chapters[0].Steps[0].Target.TableRecord.Source = "changed"
	declaration.Chapters[0].Steps[0].Target.TableRecord.Key[0].Name = "changed"
	declaration.Chapters[0].Steps[0].Target.TableRecord.Key[0].Value.Literal.Text = "changed"
	declaration.Chapters[0].Steps[0].Done.RecordSelected.Equals.Number = "changed"
	*declaration.Chapters[0].Steps[1].Target.Incident.Incident = Value{Captured: "changed"}
	*declaration.Chapters[0].Steps[1].Done.QueryRun.MinRows = Integer("99")
	declaration.Finish.Target.Query.Parameters[0].Value.Literal.Text = "changed"

	snapshot := bundle.Declaration
	require.Equal(t, "training-demo", snapshot.ID)
	require.Equal(t, "training-demo", snapshot.TrainingPackage.ID)
	require.Equal(t, strings.Repeat("a", 64), snapshot.TrainingPackage.SHA256)
	require.Equal(t, "walkthroughs/training-demo/narrative.json", snapshot.Narrative.Path)
	require.Equal(t, "find", snapshot.Chapters[0].ID)
	require.Equal(t, []string{"old-find"}, snapshot.Chapters[0].FormerIDs)
	require.Equal(t, "pick", snapshot.Chapters[0].Steps[0].ID)
	require.Equal(t, "guided", snapshot.Chapters[0].Steps[0].Modes[0])
	require.Equal(t, "crm", snapshot.Chapters[0].Steps[0].Target.TableRecord.Source)
	require.Equal(t, "alpha", snapshot.Chapters[0].Steps[0].Target.TableRecord.Key[0].Name)
	require.Equal(t, "x", snapshot.Chapters[0].Steps[0].Target.TableRecord.Key[0].Value.Literal.Text)
	require.Equal(t, "7", snapshot.Chapters[0].Steps[0].Done.RecordSelected.Equals.Number)
	require.Equal(t, "row", snapshot.Chapters[0].Steps[1].Target.Incident.Incident.Captured)
	require.Equal(t, Integer("2"), *snapshot.Chapters[0].Steps[1].Done.QueryRun.MinRows)
	require.Equal(t, "all", snapshot.Finish.Target.Query.Parameters[0].Value.Literal.Text)

	// Sealed derived views are independent of the caller declaration too.
	require.Equal(t, []string{"find"}, bundle.Chapters)
	declaration.Chapters[0].ID = "later"
	require.Equal(t, []string{"find"}, bundle.Chapters)
}

func TestCheckpointEnvelopeOwnsProjectBytes(t *testing.T) {
	project := `{"storeId":"github.com","projectId":"demo"}`
	data := []byte(`{"project":` + project + `,"authorityGeneration":1,` +
		`"contentPin":{"kind":"git","commit":"0123456789abcdef0123456789abcdef01234567"},` +
		`"declarationSha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",` +
		`"narrativeSha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",` +
		`"trainingPackageSha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",` +
		`"walkthroughId":"training-demo","chapterId":"retries","stepId":"solve",` +
		`"challengeId":"retry-query","challengeRevision":1,"checkpointId":"result",` +
		`"attemptId":"a","submissionGeneration":1,"outcome":"passed","complete":true,"assisted":false}`)
	parsed := ParseCheckpointEnvelope(data)
	require.NoError(t, parsed.Err())
	require.Equal(t, project, string(parsed.Draft.ProjectRaw))

	// Mutating the caller's input buffer after parsing must not reach the draft.
	for index := range data {
		if data[index] == 'g' {
			data[index] = 'G'
		}
	}
	require.Equal(t, project, string(parsed.Draft.ProjectRaw))

	// The adapter receives an independent copy and cannot mutate the draft.
	authority := testAuthority{admit: func(raw []byte) (ProjectIdentity, error) {
		require.Equal(t, project, string(raw))
		for index := range raw {
			raw[index] = 'x'
		}
		return identity("envelope-project"), nil
	}}
	admission := AdmitCheckpointEnvelope(*parsed.Draft, authority)
	require.NoError(t, admission.Err())
	require.Equal(t, project, string(parsed.Draft.ProjectRaw))
	require.Equal(t, project, string(admission.Envelope.ProjectRaw))

	// The admitted envelope owns its own copy: neither mutating it nor the
	// caller's draft afterwards changes the other.
	for index := range admission.Envelope.ProjectRaw {
		admission.Envelope.ProjectRaw[index] = 'y'
	}
	require.Equal(t, project, string(parsed.Draft.ProjectRaw))
	parsed.Draft.ProjectRaw[0] = '{'
	require.NotEqual(t, string(parsed.Draft.ProjectRaw), string(admission.Envelope.ProjectRaw))
	require.Equal(t, byte('y'), admission.Envelope.ProjectRaw[0])
}

// TestSnapshotClonesEveryContractShape drives the defensive snapshot over every
// contract-owned shape, including each target/condition variant and the nil
// boundaries, and proves the clone is independent of its caller-owned inputs.
func TestSnapshotClonesEveryContractShape(t *testing.T) {
	number := &Scalar{Kind: ScalarNumber, Number: "7"}
	literal := Value{Literal: number}
	captured := Value{Captured: "row", Default: number}
	minRows := Integer("2")
	pkg := PackageReference{ID: "pkg", Revision: "1", Path: "a/b.json", SHA256: strings.Repeat("a", 64), Bytes: "1"}

	richTarget := Target{
		Kind: "table-record", Supported: true,
		TableRecord:        &TableRecordTarget{Source: "s", Table: "t", Key: []NamedValue{{Name: "k", Value: literal}}},
		Query:              &QueryTarget{QueryID: "q", Parameters: []NamedValue{{Name: "p", Value: captured}}},
		Board:              &BoardTarget{BoardID: "b", Parameters: []NamedValue{{Name: "bp", Value: literal}}},
		Card:               &CardTarget{CardID: "c", Parameters: []NamedValue{{Name: "cp", Value: literal}}},
		Related:            &RelatedTarget{Source: "s", Table: "t", Key: []NamedValue{{Name: "rk", Value: literal}}},
		LogSource:          &LogSourceTarget{Source: "s", Filter: []NamedValue{{Name: "lf", Value: literal}}},
		PublicDataProvider: &PublicDataProviderTarget{Provider: "p"},
		Incident:           &IncidentTarget{Incident: &captured},
		Check:              &CheckTarget{CheckID: "c"},
		None:               &struct{}{},
		Challenge:          &ChallengeTarget{ChallengeID: "retry-query", Revision: "1"},
		TrainingCase:       &TrainingCaseTarget{ManifestCaseID: "case-1", TrainingPackage: pkg},
	}
	richCondition := Condition{
		Kind: "parameter-value", Supported: true,
		ParameterValue:   &ParameterValueCondition{Name: "n", Equals: number},
		RecordSelected:   &RecordSelectedCondition{Field: "f", Equals: number},
		QueryRun:         &QueryRunCondition{MinRows: &minRows},
		PanelOpen:        &PanelOpenCondition{Panel: "p"},
		FilterApplied:    &FilterAppliedCondition{Field: "f", Equals: number},
		CheckRun:         &CheckRunCondition{Outcome: "pass"},
		AssetExists:      &AssetExistsCondition{Asset: "query", ID: "q"},
		BoardTutorial:    &BoardTutorialCondition{UntilCard: "c"},
		DialogueFinished: &DialogueFinishedCondition{},
		ChallengePassed: &ChallengePassedCondition{Checkpoints: []CheckpointMapping{
			{ID: "x", Completion: CompletionWholeChallenge},
		}},
	}
	emptyCondition := Condition{
		Kind: "query-run", Supported: true,
		QueryRun:        &QueryRunCondition{},
		ChallengePassed: &ChallengePassedCondition{},
	}

	declaration := &TrainingDeclaration{
		TrainingPackage: pkg,
		Narrative:       AssetReference{Path: "n", SHA256: strings.Repeat("b", 64), Bytes: "1"},
		Chapters: []Chapter{
			{
				ID:        "rich",
				FormerIDs: []string{"old"},
				Readiness: &Condition{Kind: "asset-exists", Supported: true,
					AssetExists: &AssetExistsCondition{Asset: "query", ID: "q"}},
				Steps: []Step{
					{ID: "rich", Modes: []string{"guided"}, Target: richTarget, Done: richCondition},
					{ID: "empty", Target: Target{Kind: "incident", Supported: true,
						Incident: &IncidentTarget{}}, Done: emptyCondition},
				},
			},
			{ID: "empty"},
		},
		Finish: Exit{Kind: "target", Target: &Target{
			Query: &QueryTarget{QueryID: "finish", Parameters: []NamedValue{{Name: "fp", Value: literal}}},
		}},
	}

	snapshot := cloneTrainingDeclaration(declaration)
	require.NotSame(t, declaration, snapshot)
	require.NotSame(t, declaration.Chapters[0].Readiness, snapshot.Chapters[0].Readiness)
	require.NotSame(t, declaration.Chapters[0].Steps[0].Target.TableRecord,
		snapshot.Chapters[0].Steps[0].Target.TableRecord)
	require.NotSame(t, declaration.Chapters[0].Steps[0].Done.ChallengePassed,
		snapshot.Chapters[0].Steps[0].Done.ChallengePassed)
	require.NotSame(t, declaration.Finish.Target, snapshot.Finish.Target)
	require.Nil(t, cloneTrainingDeclaration(nil))
	require.Nil(t, cloneNamedValues(nil))
	require.Nil(t, cloneCheckpoints(nil))

	// Mutate every caller-owned nested value after the snapshot.
	declaration.TrainingPackage.ID = "changed"
	declaration.Narrative.Path = "changed"
	declaration.Chapters[0].FormerIDs[0] = "changed"
	declaration.Chapters[0].Readiness.AssetExists.ID = "changed"
	declaration.Chapters[0].Steps[0].Modes[0] = "changed"
	rich := declaration.Chapters[0].Steps[0].Target
	rich.TableRecord.Source = "changed"
	rich.TableRecord.Key[0].Name = "changed"
	rich.Query.Parameters[0].Value.Captured = "changed"
	rich.Board.Parameters[0].Value.Literal.Text = "changed"
	rich.Card.Parameters[0].Name = "changed"
	rich.Related.Key[0].Name = "changed"
	rich.LogSource.Filter[0].Name = "changed"
	rich.PublicDataProvider.Provider = "changed"
	*rich.Incident.Incident = Value{Captured: "changed"}
	rich.Check.CheckID = "changed"
	rich.Challenge.ChallengeID = "changed"
	rich.TrainingCase.ManifestCaseID = "changed"
	rich.TrainingCase.TrainingPackage.ID = "changed"
	done := declaration.Chapters[0].Steps[0].Done
	done.ParameterValue.Equals.Number = "changed"
	done.RecordSelected.Field = "changed"
	*done.QueryRun.MinRows = Integer("9")
	done.PanelOpen.Panel = "changed"
	done.FilterApplied.Field = "changed"
	done.CheckRun.Outcome = "changed"
	done.AssetExists.ID = "changed"
	done.BoardTutorial.UntilCard = "changed"
	done.ChallengePassed.Checkpoints[0].ID = "changed"
	declaration.Chapters[0].Steps[1].Target.Incident.Incident = &Value{Captured: "changed"}
	declaration.Chapters[0].Steps[1].Done.QueryRun.MinRows = &minRows
	declaration.Finish.Target.Query.Parameters[0].Name = "changed"

	require.Equal(t, "pkg", snapshot.TrainingPackage.ID)
	require.Equal(t, "n", snapshot.Narrative.Path)
	require.Equal(t, []string{"old"}, snapshot.Chapters[0].FormerIDs)
	require.Equal(t, "q", snapshot.Chapters[0].Readiness.AssetExists.ID)
	require.Equal(t, "guided", snapshot.Chapters[0].Steps[0].Modes[0])
	require.Equal(t, "s", snapshot.Chapters[0].Steps[0].Target.TableRecord.Source)
	require.Equal(t, "k", snapshot.Chapters[0].Steps[0].Target.TableRecord.Key[0].Name)
	require.Equal(t, "p", snapshot.Chapters[0].Steps[0].Target.Query.Parameters[0].Name)
	require.Equal(t, "row", snapshot.Chapters[0].Steps[0].Target.Query.Parameters[0].Value.Captured)
	require.Equal(t, "bp", snapshot.Chapters[0].Steps[0].Target.Board.Parameters[0].Name)
	require.Equal(t, "cp", snapshot.Chapters[0].Steps[0].Target.Card.Parameters[0].Name)
	require.Equal(t, "rk", snapshot.Chapters[0].Steps[0].Target.Related.Key[0].Name)
	require.Equal(t, "lf", snapshot.Chapters[0].Steps[0].Target.LogSource.Filter[0].Name)
	require.Equal(t, "p", snapshot.Chapters[0].Steps[0].Target.PublicDataProvider.Provider)
	require.Equal(t, "row", snapshot.Chapters[0].Steps[0].Target.Incident.Incident.Captured)
	require.Equal(t, "c", snapshot.Chapters[0].Steps[0].Target.Check.CheckID)
	require.Equal(t, "retry-query", snapshot.Chapters[0].Steps[0].Target.Challenge.ChallengeID)
	require.Equal(t, "case-1", snapshot.Chapters[0].Steps[0].Target.TrainingCase.ManifestCaseID)
	require.Equal(t, "pkg", snapshot.Chapters[0].Steps[0].Target.TrainingCase.TrainingPackage.ID)
	require.Equal(t, "7", snapshot.Chapters[0].Steps[0].Done.ParameterValue.Equals.Number)
	require.Equal(t, "f", snapshot.Chapters[0].Steps[0].Done.RecordSelected.Field)
	require.Equal(t, Integer("2"), *snapshot.Chapters[0].Steps[0].Done.QueryRun.MinRows)
	require.Equal(t, "p", snapshot.Chapters[0].Steps[0].Done.PanelOpen.Panel)
	require.Equal(t, "f", snapshot.Chapters[0].Steps[0].Done.FilterApplied.Field)
	require.Equal(t, "pass", snapshot.Chapters[0].Steps[0].Done.CheckRun.Outcome)
	require.Equal(t, "q", snapshot.Chapters[0].Steps[0].Done.AssetExists.ID)
	require.Equal(t, "c", snapshot.Chapters[0].Steps[0].Done.BoardTutorial.UntilCard)
	require.Equal(t, "x", snapshot.Chapters[0].Steps[0].Done.ChallengePassed.Checkpoints[0].ID)
	require.Nil(t, snapshot.Chapters[0].Steps[1].Target.Incident.Incident)
	require.Nil(t, snapshot.Chapters[0].Steps[1].Done.QueryRun.MinRows)
	require.Equal(t, "fp", snapshot.Finish.Target.Query.Parameters[0].Name)
	require.Empty(t, snapshot.Chapters[1].Steps)
}
