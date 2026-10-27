package datatug

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProjectDatabases(t *testing.T) {
	good := ProjectDatabases{{ID: "chinook-postgresql", ConnectionID: "chinook-postgresql", OVDB: PublicOVDBDatabase{BaseURL: "https://cloud.openvaultdb.com", DatabaseID: "chinook-postgresql"}}}
	require.NoError(t, good.Validate())
	original := ProjectFile{Databases: good}
	body, err := json.Marshal(original)
	require.NoError(t, err)
	var file ProjectFile
	require.NoError(t, json.Unmarshal(body, &file))
	require.Equal(t, good, file.Databases)
	var project Project
	require.NoError(t, json.Unmarshal(body, &project))
	require.Equal(t, good, project.Databases)
	replacement := ProjectFile{}
	require.NoError(t, replacement.MergePreservedMetadata(file))
	require.Equal(t, good, replacement.Databases)
	replacement.Databases = ProjectDatabases{}
	require.NoError(t, replacement.MergePreservedMetadata(file))
	body, err = json.Marshal(replacement)
	require.NoError(t, err)
	require.Contains(t, string(body), `"databases":[]`)
	for _, base := range []string{"http://public.example", "https://token@public.example", "https://public.example?token=x", "https://public.example#fragment"} {
		bad := append(ProjectDatabases{}, good...)
		bad[0].OVDB.BaseURL = base
		require.Error(t, bad.Validate())
	}
	require.Error(t, append(good, good...).Validate())
}
