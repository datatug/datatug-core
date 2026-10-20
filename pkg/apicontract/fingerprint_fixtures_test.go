package apicontract

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCanonicalRecordsetFingerprintFixtures(t *testing.T) {
	var fixture struct {
		Format string `json:"format"`
		Cases  []struct {
			Name      string    `json:"name"`
			Recordset Recordset `json:"recordset"`
			Canonical string    `json:"canonicalJSON"`
			SHA256    string    `json:"sha256"`
		} `json:"cases"`
	}
	data, err := os.ReadFile("testdata/recordset-fingerprints.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &fixture))
	require.Equal(t, "datatug-recordset-fingerprint/v1", fixture.Format)
	require.Len(t, fixture.Cases, 7)
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			sum, err := FingerprintRecordset(c.Recordset)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, sum)
			var canonical Recordset
			require.NoError(t, json.Unmarshal([]byte(c.Canonical), &canonical))
			b, err := json.Marshal(canonical)
			require.NoError(t, err)
			require.Equal(t, c.Canonical, string(b))
			sum, err = FingerprintRecordset(canonical)
			require.NoError(t, err)
			require.Equal(t, c.SHA256, sum)
		})
	}
}
