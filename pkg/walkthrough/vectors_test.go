package walkthrough

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

// corpusFile mirrors the checked-in shared corpora. The expected definitions
// are authored from the owner tables by testdata/generate_corpus.py, never
// produced by this implementation. `operation` selects the explicit entry
// point: the original vectors stay on ordinary /1 admission, and only the
// training-declaration vectors use the /2 parser.
type corpusFile struct {
	CorpusVersion string         `json:"corpusVersion"`
	Vectors       []corpusVector `json:"vectors"`
}

type corpusVector struct {
	ID          string     `json:"id"`
	Description string     `json:"description"`
	Operation   string     `json:"operation"`
	InputFile   string     `json:"inputFile"`
	DirectoryID string     `json:"directoryId"`
	Want        corpusWant `json:"want"`
}

type corpusWant struct {
	Accepted    bool           `json:"accepted"`
	NewerClient bool           `json:"newerClient"`
	Digest      string         `json:"digest"`
	Definition  map[string]any `json:"definition"`
	Issues      []corpusIssue  `json:"issues"`
}

type corpusIssue struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

type corpusProvenance struct {
	CorpusVersion     string            `json:"corpusVersion"`
	BaseCorpusVersion string            `json:"baseCorpusVersion"`
	BaseCorpusSHA256  string            `json:"baseCorpusSha256"`
	CorpusSHA256      string            `json:"corpusSha256"`
	Files             map[string]string `json:"files"`
}

func loadCorpus(t *testing.T, name string) corpusFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var corpus corpusFile
	require.NoError(t, apicontract.DecodeStrict(data, &corpus))
	return corpus
}

func loadProvenance(t *testing.T, name string) corpusProvenance {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	var provenance corpusProvenance
	require.NoError(t, apicontract.DecodeStrict(raw, &provenance))
	return provenance
}

func TestSharedWalkthroughVectorInventory(t *testing.T) {
	corpus := loadCorpus(t, "corpus.json")
	require.Equal(t, "walkthrough-contracts-v1", corpus.CorpusVersion)
	require.Len(t, corpus.Vectors, 119)
	seen := make(map[string]bool, len(corpus.Vectors))
	for _, vector := range corpus.Vectors {
		require.NotEmpty(t, vector.ID)
		require.False(t, seen[vector.ID], "duplicate vector id %q", vector.ID)
		seen[vector.ID] = true
		require.NotEmpty(t, vector.Description)
		require.Equal(t, "declaration", vector.Operation)
		require.NotEmpty(t, vector.InputFile)
		require.NotEmpty(t, vector.DirectoryID)
	}
}

func TestTrainingWalkthroughVectorInventory(t *testing.T) {
	corpus := loadCorpus(t, "corpus-v2.json")
	require.Equal(t, "walkthrough-contracts-v2", corpus.CorpusVersion)
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
		case "training-declaration":
			require.NotEmpty(t, vector.DirectoryID)
		case "checkpoint-envelope", "story-registry-entry":
			require.Empty(t, vector.DirectoryID)
		default:
			require.Failf(t, "unknown operation", "%s", vector.Operation)
		}
		operations[vector.Operation]++
	}
	require.NotZero(t, operations["training-declaration"])
	require.NotZero(t, operations["checkpoint-envelope"])
	require.NotZero(t, operations["story-registry-entry"])
}

func TestSharedWalkthroughVectors(t *testing.T) {
	corpus := loadCorpus(t, "corpus.json")
	for _, vector := range corpus.Vectors {
		vector := vector
		t.Run(vector.ID, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(vector.InputFile)))
			require.NoError(t, err)
			result := ParseDeclaration(vector.DirectoryID, data)
			require.Equal(t, vector.Want.Accepted, result.Accepted)
			require.Equal(t, vector.Want.NewerClient, result.NewerClient)
			require.Equal(t, vector.Want.Digest, result.Digest)

			if vector.Want.Accepted {
				require.NotNil(t, result.Definition)
				require.NoError(t, result.Err())
				require.Equal(t, vector.Want.Definition, jsonPlain(t, result.Definition.Projection()))
				return
			}

			require.Nil(t, result.Definition, "a refused declaration must not expose an executable definition")
			if vector.Want.NewerClient {
				require.Empty(t, result.Issues)
				require.ErrorIs(t, result.Err(), ErrNewerClient)
				return
			}
			require.Error(t, result.Err())
			require.Equal(t, vector.Want.Issues, corpusIssues(result.Issues))
		})
	}
}

func TestTrainingWalkthroughVectors(t *testing.T) {
	corpus := loadCorpus(t, "corpus-v2.json")
	for _, vector := range corpus.Vectors {
		vector := vector
		t.Run(vector.ID, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(vector.InputFile)))
			require.NoError(t, err)
			require.Equal(t, vector.Want.Digest, sha256Hex(data))
			switch vector.Operation {
			case "training-declaration":
				assertTrainingDeclarationVector(t, vector, data)
			case "checkpoint-envelope":
				assertEnvelopeVector(t, vector, data)
			case "story-registry-entry":
				assertRegistryVector(t, vector, data)
			}
		})
	}
}

func assertTrainingDeclarationVector(t *testing.T, vector corpusVector, data []byte) {
	t.Helper()
	result := ParseTrainingDeclaration(vector.DirectoryID, data)
	require.Equal(t, vector.Want.Accepted, result.Accepted)
	require.Equal(t, vector.Want.NewerClient, result.NewerClient)
	require.Equal(t, vector.Want.Digest, result.Digest)
	if vector.Want.Accepted {
		require.NotNil(t, result.Definition)
		require.NoError(t, result.Err())
		require.Equal(t, vector.Want.Definition, jsonPlain(t, result.Definition.Projection()))
		return
	}
	require.Nil(t, result.Definition, "a refused declaration must not expose an executable definition")
	if vector.Want.NewerClient {
		require.Empty(t, result.Issues)
		require.ErrorIs(t, result.Err(), ErrNewerClient)
		return
	}
	require.Error(t, result.Err())
	require.Equal(t, vector.Want.Issues, corpusIssues(result.Issues))
}

func assertEnvelopeVector(t *testing.T, vector corpusVector, data []byte) {
	t.Helper()
	result := ParseCheckpointEnvelope(data)
	require.Equal(t, vector.Want.Accepted, result.Draft != nil)
	if vector.Want.Accepted {
		require.NoError(t, result.Err())
		require.Equal(t, vector.Want.Definition, jsonPlain(t, result.Draft.Projection()))
		return
	}
	require.Error(t, result.Err())
	require.Equal(t, vector.Want.Issues, corpusIssues(result.Issues))
}

func assertRegistryVector(t *testing.T, vector corpusVector, data []byte) {
	t.Helper()
	result := ParseStoryRegistryEntry(data)
	require.Equal(t, vector.Want.Accepted, result.Entry != nil)
	if vector.Want.Accepted {
		require.NoError(t, result.Err())
		require.Equal(t, vector.Want.Definition, jsonPlain(t, result.Entry.Projection()))
		return
	}
	require.Error(t, result.Err())
	require.Equal(t, vector.Want.Issues, corpusIssues(result.Issues))
}

func corpusIssues(issues []Issue) []corpusIssue {
	converted := make([]corpusIssue, len(issues))
	for index, issue := range issues {
		converted[index] = corpusIssue{Code: string(issue.Code), Path: issue.Path}
	}
	return converted
}

func TestCorpusProvenanceMatchesBytes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "corpus.json"))
	require.NoError(t, err)
	provenance := loadProvenance(t, "provenance.json")
	require.Equal(t, "walkthrough-contracts-v1", provenance.CorpusVersion)
	require.Equal(t, sha256Hex(raw), provenance.CorpusSHA256)
	require.Equal(t, sha256Hex(raw), sha256Hex(raw))
	require.NotEmpty(t, provenance.Files)
	for relative, expected := range provenance.Files {
		data, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(relative)))
		require.NoError(t, err, relative)
		require.Equal(t, expected, sha256Hex(data), relative)
	}
}

func TestTrainingCorpusProvenanceMatchesBytes(t *testing.T) {
	base, err := os.ReadFile(filepath.Join("testdata", "corpus.json"))
	require.NoError(t, err)
	v2, err := os.ReadFile(filepath.Join("testdata", "corpus-v2.json"))
	require.NoError(t, err)
	provenance := loadProvenance(t, "provenance-v2.json")
	require.Equal(t, "walkthrough-contracts-v2", provenance.CorpusVersion)
	require.Equal(t, "walkthrough-contracts-v1", provenance.BaseCorpusVersion)
	require.Equal(t, sha256Hex(base), provenance.BaseCorpusSHA256)
	require.Equal(t, sha256Hex(v2), provenance.CorpusSHA256)
	require.NotEmpty(t, provenance.Files)
	for relative, expected := range provenance.Files {
		data, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(relative)))
		require.NoError(t, err, relative)
		require.Equal(t, expected, sha256Hex(data), relative)
	}
}

func TestExactByteRevision(t *testing.T) {
	pretty, err := os.ReadFile(filepath.Join("testdata", "declarations", "v1-minimal.json"))
	require.NoError(t, err)
	compact, err := os.ReadFile(filepath.Join("testdata", "declarations", "v1-minimal-compact.json"))
	require.NoError(t, err)
	require.NotEqual(t, string(pretty), string(compact))

	prettyResult := ParseDeclaration("tour", pretty)
	compactResult := ParseDeclaration("tour", compact)
	require.True(t, prettyResult.Accepted)
	require.True(t, compactResult.Accepted)
	require.NotEqual(t, prettyResult.Digest, compactResult.Digest)
	require.Equal(t, sha256Hex(pretty), prettyResult.Digest)
	require.Equal(t, sha256Hex(compact), compactResult.Digest)
	require.Equal(t, prettyResult.Digest, prettyResult.Definition.Revision)
	require.Equal(t, compactResult.Digest, compactResult.Definition.Revision)
}

func TestCallerMutationDoesNotChangeAdmittedSnapshot(t *testing.T) {
	original, err := os.ReadFile(filepath.Join("testdata", "declarations", "v1-minimal.json"))
	require.NoError(t, err)
	input := append([]byte{}, original...)
	admitted := ParseDeclaration("tour", input)
	require.True(t, admitted.Accepted)
	require.NotNil(t, admitted.Definition)
	title := admitted.Definition.Title
	digest := admitted.Digest

	// Mutating the caller's buffer must not alter the admitted snapshot or its
	// digest.
	for index := range input {
		input[index] = 'x'
	}
	require.Equal(t, title, admitted.Definition.Title)
	require.Equal(t, digest, admitted.Digest)
	require.Equal(t, digest, admitted.Definition.Revision)

	// Mutating one admitted definition must not leak into another parse.
	other := ParseDeclaration("tour", original)
	require.True(t, other.Accepted)
	admitted.Definition.Title = "changed"
	admitted.Definition.Chapters[0].Steps[0].Modes[0] = "changed"
	require.Equal(t, "Walkthrough tour", other.Definition.Title)
	require.Equal(t, "guided", other.Definition.Chapters[0].Steps[0].Modes[0])
}

func TestDeclarationPath(t *testing.T) {
	require.Equal(t, "walkthroughs/tour/walkthrough.json", DeclarationPath("tour"))
}

func TestEscapedSurrogateTreatment(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "declarations", "v1-escaped-surrogates.json"))
	require.NoError(t, err)
	result := ParseDeclaration("tour", data)
	require.True(t, result.Accepted)
	require.Equal(t, "Pair \U0001F600 lone \uFFFD literal \uFFFD end", result.Definition.Title)
	require.Equal(t, "Caf\uFFFD Orders", result.Definition.Chapters[0].Steps[0].Target.TableRecord.Table)
}

func jsonPlain(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	var plain any
	require.NoError(t, json.Unmarshal(raw, &plain))
	return plain
}

func sha256Hex(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
