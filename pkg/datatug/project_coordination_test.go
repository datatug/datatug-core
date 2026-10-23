package datatug

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWatchCoordinationBoundaries(t *testing.T) {
	require.NoError(t, (*WatchCoordination)(nil).Validate())
	for _, endpoint := range []string{"https://coord.example.com", "https://coord.example.com/base", "http://localhost:8988", "http://127.0.0.1:8988", "http://[::1]:8988"} {
		require.NoError(t, (&WatchCoordination{Endpoint: endpoint, Namespace: "northwind"}).Validate())
	}
	for _, endpoint := range []string{"", "/local", "ftp://coord.example.com", "http://coord.example.com", "https://user:secret@coord.example.com", "https://coord.example.com?token=secret", "https://coord.example.com?", "https://coord.example.com#secret", " https://coord.example.com"} {
		require.Error(t, (&WatchCoordination{Endpoint: endpoint, Namespace: "northwind"}).Validate(), endpoint)
	}
	require.Error(t, (&WatchCoordination{Endpoint: "https://coord.example.com", Namespace: "../private"}).Validate())
	for _, body := range []string{`null`, `{`, `{"endpoint":"https://coord.example.com","namespace":"p","token":"secret"}`, `{"endpoint":"https://coord.example.com","namespace":"p"} {}`} {
		var descriptor WatchCoordination
		require.Error(t, descriptor.UnmarshalJSON([]byte(body)))
	}
	var file ProjectFile
	require.NoError(t, json.Unmarshal([]byte(`{"coordination":{"endpoint":"https://coord.example.com","namespace":"northwind"}}`), &file))
	require.Equal(t, "northwind", file.Coordination.Namespace)
	encoded, err := json.Marshal(file)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"coordination"`)
	var oldClient ProjectFile
	require.NoError(t, oldClient.MergePreservedMetadata(file))
	require.Equal(t, file.Coordination, oldClient.Coordination)
	require.Error(t, (&ProjectFile{}).MergePreservedMetadata(ProjectFile{Coordination: &WatchCoordination{Endpoint: "http://remote", Namespace: "p"}}))
	require.Error(t, (&ProjectFile{Coordination: &WatchCoordination{Endpoint: "bad", Namespace: "p"}}).MergePreservedMetadata(ProjectFile{}))
}
