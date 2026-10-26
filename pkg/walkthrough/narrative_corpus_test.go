package walkthrough

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

type narrativeCorpus struct {
	CorpusVersion string            `json:"corpusVersion"`
	Vectors       []narrativeVector `json:"vectors"`
}

type narrativeVector struct {
	ID          string        `json:"id"`
	Description string        `json:"description"`
	Operation   string        `json:"operation"`
	InputFile   string        `json:"inputFile"`
	Want        narrativeWant `json:"want"`
}

type narrativeWant struct {
	Accepted   bool           `json:"accepted"`
	Digest     string         `json:"digest"`
	Definition map[string]any `json:"definition"`
	Selection  map[string]any `json:"selection"`
	Admission  map[string]any `json:"admission"`
	Issues     []corpusIssue  `json:"issues"`
}

type narrativeAdmissionCase struct {
	DeclarationFile string                `json:"declarationFile"`
	SidecarFile     string                `json:"sidecarFile"`
	CatalogueFiles  []string              `json:"catalogueFiles"`
	Request         *string               `json:"request"`
	Owner           narrativeOwnerFixture `json:"owner"`
}

type narrativeOwnerFixture struct {
	Assets              []narrativeAssetFixture    `json:"assets"`
	Cases               []string                   `json:"cases"`
	Incidents           []string                   `json:"incidents"`
	Checks              []string                   `json:"checks"`
	Queries             []string                   `json:"queries"`
	Proposals           []string                   `json:"proposals"`
	PresentationTargets []string                   `json:"presentationTargets"`
	CheckBindings       []narrativeCheckFixture    `json:"checkBindings"`
	ProposalBindings    []narrativeProposalFixture `json:"proposalBindings"`
	TargetBindings      []narrativeTargetFixture   `json:"targetBindings"`
	MutateOnSecondRead  bool                       `json:"mutateOnSecondRead"`
	WrongPackageBinding bool                       `json:"wrongPackageBinding"`
}

type narrativeAssetFixture struct {
	AssetID string `json:"assetId"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Bytes   string `json:"bytes"`
}

type narrativeCheckFixture struct {
	CheckID           string `json:"checkId"`
	DefinitionAssetID string `json:"definitionAssetId"`
}

type narrativeProposalFixture struct {
	ProposalID   string   `json:"proposalId"`
	Kind         string   `json:"kind"`
	DefinitionID string   `json:"definitionId"`
	EvidenceIDs  []string `json:"evidenceIds"`
	Rationale    string   `json:"rationale"`
}

type narrativeTargetFixture struct {
	TargetID    string `json:"targetId"`
	Kind        string `json:"kind"`
	RefID       string `json:"refId"`
	ClosureKind string `json:"closureKind"`
	ClosureID   string `json:"closureId"`
}

func loadNarrativeCorpus(t *testing.T) narrativeCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "corpus-v3.json"))
	require.NoError(t, err)
	var corpus narrativeCorpus
	require.NoError(t, apicontract.DecodeStrict(raw, &corpus))
	return corpus
}

func TestNarrativeCorpusProvenance(t *testing.T) {
	base, err := os.ReadFile(filepath.Join("testdata", "corpus-v2.json"))
	require.NoError(t, err)
	corpusBytes, err := os.ReadFile(filepath.Join("testdata", "corpus-v3.json"))
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join("testdata", "provenance-v3.json"))
	require.NoError(t, err)
	var provenance corpusProvenance
	require.NoError(t, apicontract.DecodeStrict(raw, &provenance))
	require.Equal(t, "walkthrough-contracts-v3", provenance.CorpusVersion)
	require.Equal(t, "walkthrough-contracts-v2", provenance.BaseCorpusVersion)
	require.Equal(t, sha256Hex(base), provenance.BaseCorpusSHA256)
	require.Equal(t, sha256Hex(corpusBytes), provenance.CorpusSHA256)
	require.NotEmpty(t, provenance.Files)
	for relative, expected := range provenance.Files {
		data, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(relative)))
		require.NoError(t, err, relative)
		require.Equal(t, expected, sha256Hex(data), relative)
	}
}

func TestNarrativeCorpusInventory(t *testing.T) {
	corpus := loadNarrativeCorpus(t)
	require.Equal(t, "walkthrough-contracts-v3", corpus.CorpusVersion)
	require.NotEmpty(t, corpus.Vectors)
	seen := make(map[string]bool, len(corpus.Vectors))
	operations := make(map[string]int)
	for _, vector := range corpus.Vectors {
		require.NotEmpty(t, vector.ID)
		require.False(t, seen[vector.ID], "duplicate vector id %q", vector.ID)
		seen[vector.ID] = true
		require.NotEmpty(t, vector.Description)
		require.NotEmpty(t, vector.InputFile)
		switch vector.Operation {
		case "narrative-sidecar", "message-catalogue", "locale-selection", "narrative-admission":
			operations[vector.Operation]++
		default:
			require.Failf(t, "unknown operation", "%s", vector.Operation)
		}
	}
	for _, operation := range []string{
		"narrative-sidecar", "message-catalogue", "locale-selection", "narrative-admission",
	} {
		require.NotZero(t, operations[operation], operation)
	}
}

func TestNarrativeCorpusVectors(t *testing.T) {
	corpus := loadNarrativeCorpus(t)
	for _, vector := range corpus.Vectors {
		vector := vector
		t.Run(vector.ID, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(vector.InputFile)))
			require.NoError(t, err)
			if vector.Want.Digest != "" {
				require.Equal(t, vector.Want.Digest, sha256Hex(data))
			}
			switch vector.Operation {
			case "narrative-sidecar":
				assertNarrativeSidecarVector(t, vector, data)
			case "message-catalogue":
				assertCatalogueVector(t, vector, data)
			case "locale-selection":
				assertLocaleSelectionVector(t, vector, data)
			case "narrative-admission":
				assertNarrativeAdmissionVector(t, vector, data)
			}
		})
	}
}

func assertNarrativeSidecarVector(t *testing.T, vector narrativeVector, data []byte) {
	t.Helper()
	result := ParseNarrativeSidecar(data)
	require.Equal(t, vector.Want.Accepted, result.Accepted)
	require.Equal(t, vector.Want.Digest, result.Digest)
	if vector.Want.Accepted {
		require.NoError(t, result.Err())
		require.NotNil(t, result.Sidecar)
		require.Equal(t, vector.Want.Definition, jsonPlain(t, result.Sidecar.Projection()))
		return
	}
	require.Nil(t, result.Sidecar)
	require.Equal(t, vector.Want.Issues, corpusIssues(result.Issues))
}

func assertCatalogueVector(t *testing.T, vector narrativeVector, data []byte) {
	t.Helper()
	result := ParseMessageCatalogue(data)
	require.Equal(t, vector.Want.Accepted, result.Accepted)
	require.Equal(t, vector.Want.Digest, result.Digest)
	if vector.Want.Accepted {
		require.NoError(t, result.Err())
		require.NotNil(t, result.Catalogue)
		require.Equal(t, vector.Want.Definition, jsonPlain(t, result.Catalogue.Projection()))
		return
	}
	require.Nil(t, result.Catalogue)
	require.Equal(t, vector.Want.Issues, corpusIssues(result.Issues))
}

func assertLocaleSelectionVector(t *testing.T, vector narrativeVector, data []byte) {
	t.Helper()
	var selection struct {
		Request        *string  `json:"request"`
		Declared       []string `json:"declared"`
		DefaultLocale  string   `json:"defaultLocale"`
		FallbackLocale string   `json:"fallbackLocale"`
	}
	require.NoError(t, apicontract.DecodeStrict(data, &selection))
	result := SelectNarrativeLocale(
		selection.Request, selection.Declared, selection.DefaultLocale, selection.FallbackLocale)
	require.Equal(t, vector.Want.Accepted, result.Accepted)
	if vector.Want.Accepted {
		require.NoError(t, result.Err())
		require.Equal(t, vector.Want.Selection, jsonPlain(t, result.Choice.Projection()))
		return
	}
	require.Equal(t, vector.Want.Issues, corpusIssues(result.Issues))
}

func assertNarrativeAdmissionVector(t *testing.T, vector narrativeVector, data []byte) {
	t.Helper()
	var admissionCase narrativeAdmissionCase
	require.NoError(t, apicontract.DecodeStrict(data, &admissionCase))
	declarationBytes := readNarrativeFixture(t, admissionCase.DeclarationFile)
	declaration := ParseTrainingDeclaration("training-demo", declarationBytes)
	require.True(t, declaration.Accepted, "%v", declaration.Issues)
	sidecarBytes := readNarrativeFixture(t, admissionCase.SidecarFile)
	sidecar := ParseNarrativeSidecar(sidecarBytes)
	require.True(t, sidecar.Accepted, "%v", sidecar.Issues)

	catalogues := make([]NarrativeAssetBytes, len(admissionCase.CatalogueFiles))
	catalogueData := make([][]byte, len(admissionCase.CatalogueFiles))
	for index, file := range admissionCase.CatalogueFiles {
		catalogueData[index] = readNarrativeFixture(t, "narrative/catalogues/"+file+".json")
		require.Less(t, index, len(sidecar.Sidecar.MessageCatalogues))
		catalogues[index] = NarrativeAssetBytes{
			Reference: sidecar.Sidecar.MessageCatalogues[index].Asset(),
			Bytes:     catalogueData[index],
		}
	}

	source := SourceContext{
		Project:           ProjectIdentity{Value: "project-one"},
		ContentPin:        ContentPin{Kind: PinGit, Commit: fixtureCommit},
		DeclarationSHA256: sha256Hex(declarationBytes),
	}
	packageRef := declaration.Definition.TrainingPackage
	binding := OwnerBindingKey{
		Project:           source.Project,
		ContentPin:        source.ContentPin,
		DeclarationSHA256: source.DeclarationSHA256,
		Package:           packageRef,
	}
	owner := &fakeOwner{
		ref:                packageRef,
		admittedAssets:     make(map[string]bool),
		checks:             make(map[string]CheckBinding),
		proposals:          make(map[string]ProposalBinding),
		targets:            make(map[string]PresentationTargetBinding),
		mutateOnSecondRead: admissionCase.Owner.MutateOnSecondRead,
	}
	owner.packageCases = append([]string{}, admissionCase.Owner.Cases...)
	owner.narrative = NarrativeInventory{
		Binding:             binding,
		Cases:               append([]string{}, admissionCase.Owner.Cases...),
		Incidents:           append([]string{}, admissionCase.Owner.Incidents...),
		Checks:              append([]string{}, admissionCase.Owner.Checks...),
		Queries:             append([]string{}, admissionCase.Owner.Queries...),
		Proposals:           append([]string{}, admissionCase.Owner.Proposals...),
		PresentationTargets: append([]string{}, admissionCase.Owner.PresentationTargets...),
	}
	for _, asset := range admissionCase.Owner.Assets {
		reference := AssetReference{Path: asset.Path, SHA256: asset.SHA256, Bytes: Integer(asset.Bytes)}
		owner.packageAssets = append(owner.packageAssets, reference)
		owner.narrative.Assets = append(owner.narrative.Assets, PackageAssetBinding{
			AssetID: asset.AssetID, Kind: asset.Kind, Reference: reference, Bytes: reference.Bytes,
		})
	}
	owner.admittedAssets[assetReferenceKey(declaration.Definition.Narrative)] = true
	for _, ref := range sidecar.Sidecar.MessageCatalogues {
		owner.admittedAssets[assetReferenceKey(ref.Asset())] = true
	}
	for _, asset := range sidecar.Sidecar.Assets {
		owner.admittedAssets[assetReferenceKey(asset.Asset())] = true
	}
	for _, step := range declaration.Definition.Chapters[0].Steps {
		if step.Target.Challenge != nil {
			owner.challenge = ChallengeBinding{
				ChallengeID: step.Target.Challenge.ChallengeID,
				Revision:    step.Target.Challenge.Revision,
			}
		}
	}
	for _, fixture := range admissionCase.Owner.CheckBindings {
		bindingCopy := binding
		if admissionCase.Owner.WrongPackageBinding {
			bindingCopy.Package = PackageReference{
				ID: "other-package", Revision: "1", Path: "training/package.json",
				SHA256: strings.Repeat("a", 64), Bytes: "100",
			}
		}
		owner.checks[fixture.CheckID] = CheckBinding{
			Binding: bindingCopy, CheckID: fixture.CheckID, DefinitionAssetID: fixture.DefinitionAssetID,
		}
	}
	for _, fixture := range admissionCase.Owner.ProposalBindings {
		owner.proposals[fixture.ProposalID] = ProposalBinding{
			Binding: binding, ProposalID: fixture.ProposalID, Kind: fixture.Kind,
			DefinitionID: fixture.DefinitionID, EvidenceIDs: append([]string{}, fixture.EvidenceIDs...),
			Rationale: fixture.Rationale,
		}
	}
	for _, fixture := range admissionCase.Owner.TargetBindings {
		owner.targets[fixture.TargetID] = PresentationTargetBinding{
			Binding: binding, TargetID: fixture.TargetID, Kind: fixture.Kind, RefID: fixture.RefID,
			Closure: &PresentationClosure{Kind: fixture.ClosureKind, ID: fixture.ClosureID},
		}
	}

	resolution := ResolveTraining(declaration.Definition, source, fakeAuthority{}, owner)
	require.True(t, resolution.Admitted, "%v", resolution.Issues)
	result := AdmitNarrative(resolution.Bundle, fakeAuthority{}, owner, NarrativeAdmissionInput{
		Sidecar:    NarrativeAssetBytes{Reference: resolution.Bundle.Narrative, Bytes: sidecarBytes},
		Catalogues: catalogues,
		Request:    admissionCase.Request,
	})
	require.Equal(t, vector.Want.Accepted, result.Admitted)
	if vector.Want.Accepted {
		require.NoError(t, result.Err())
		require.NotNil(t, result.Narrative)
		require.Equal(t, vector.Want.Admission, jsonPlain(t, result.Narrative.Projection()))
		return
	}
	require.Nil(t, result.Narrative)
	require.Equal(t, vector.Want.Issues, corpusIssues(result.Issues))
}

func readNarrativeFixture(t *testing.T, relative string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(relative)))
	require.NoError(t, err)
	return data
}
