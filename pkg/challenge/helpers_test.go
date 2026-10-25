package challenge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	return data
}

func goldenManifestPath(challengeID string) string {
	return filepath.Join("testdata", "challenges", challengeID, "1.json")
}

func goldenFixturePath(challengeID, fixtureID string) string {
	return filepath.Join("testdata", "fixtures", challengeID, fixtureID+".db")
}

func goldenManifestData(t *testing.T, challengeID string) []byte {
	t.Helper()
	return readTestFile(t, goldenManifestPath(challengeID))
}

func goldenFixtures(t *testing.T, challengeID string) map[string][]byte {
	t.Helper()
	var probe struct {
		FixtureSuite FixtureSuite `json:"fixtureSuite"`
	}
	require.NoError(t, json.Unmarshal(goldenManifestData(t, challengeID), &probe))
	require.NotEmpty(t, probe.FixtureSuite.Fixtures)
	fixtures := make(map[string][]byte, len(probe.FixtureSuite.Fixtures))
	for _, fixture := range probe.FixtureSuite.Fixtures {
		fixtures[fixture.ID] = readTestFile(t, goldenFixturePath(challengeID, fixture.ID))
	}
	return fixtures
}

func openGolden(t *testing.T, challengeID string) *Opened {
	t.Helper()
	opened, err := Open(goldenManifestData(t, challengeID), goldenFixtures(t, challengeID))
	require.NoError(t, err)
	return opened
}

func mutateManifestJSON(t *testing.T, data []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var value map[string]any
	require.NoError(t, json.Unmarshal(data, &value))
	mutate(value)
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return encoded
}

func requireRefused(t *testing.T, err error, code Code) *Error {
	t.Helper()
	require.Error(t, err)
	var refusal *Error
	require.ErrorAs(t, err, &refusal)
	require.Equal(t, code, refusal.Code)
	require.True(t, refusal.Recoverable(), "definition refusals must be recoverable")
	return refusal
}

func findPart(t *testing.T, manifest Manifest, id string) Part {
	t.Helper()
	for _, part := range manifest.Parts {
		if part.ID == id {
			return part
		}
	}
	t.Fatalf("part %q not found", id)
	return Part{}
}

func findSQLCase(t *testing.T, part SQLPart, fixture string) SQLCase {
	t.Helper()
	for _, item := range part.Cases {
		if item.Fixture == fixture {
			return item
		}
	}
	t.Fatalf("case for fixture %q not found", fixture)
	return SQLCase{}
}

type goldenFile struct {
	GeneratedWith map[string]string `json:"generatedWith"`
	Challenge     string            `json:"challenge"`
	Revision      int               `json:"revision"`
	Answers       []goldenAnswer    `json:"answers"`
}

type goldenAnswer struct {
	ID       string              `json:"id"`
	Kind     string              `json:"kind"`
	SQL      string              `json:"sql,omitempty"`
	Fixtures []goldenObservation `json:"fixtures"`
}

type goldenObservation struct {
	Fixture       string                 `json:"fixture"`
	FixtureSHA256 string                 `json:"fixtureSha256"`
	Runs          int                    `json:"runs"`
	Tables        []TableState           `json:"tables,omitempty"`
	OnceTables    []TableState           `json:"onceTables,omitempty"`
	Rows          *apicontract.Recordset `json:"rows,omitempty"`
}
