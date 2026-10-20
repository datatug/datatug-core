package incidents

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFolderQualifiedQueryReference(t *testing.T) {
	ref := ArtifactRef{Kind: RefQuery, Artifact: &ProjectArtifactRef{StoreID: "firestore", ProjectID: "training", Environment: "demo", Folder: "detective/shipping", ID: "receipt"}}
	require.NoError(t, ref.Validate())
	data, err := json.Marshal(ref)
	require.NoError(t, err)
	var decoded ArtifactRef
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, ref, decoded)
	for _, folder := range []string{"/a", "a/", "a//b", ".", "..", "a/../b", `a\b`, "a/%2e%2e/b", "a%2fb", "a%5Cb"} {
		t.Run(folder, func(t *testing.T) { copy := *ref.Artifact; copy.Folder = folder; require.Error(t, copy.Validate()) })
	}
	ref.Artifact.Folder = ""
	require.NoError(t, ref.Validate())
}
