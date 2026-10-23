package datatug

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProjectDataModelsLookupAndMergeFailureBoundaries(t *testing.T) {
	invalid := &ProjectDataModels{Version: 2}
	require.ErrorIs(t, (&Project{DataModels: invalid}).Validate(), ErrUnsupportedDataModels)
	require.ErrorIs(t, (ProjectFile{DataModels: invalid}).Validate(), ErrUnsupportedDataModels)
	_, err := invalid.ReferenceByID("missing")
	require.ErrorIs(t, err, ErrUnsupportedDataModels)
	_, err = invalid.ReferenceForCatalog("local", "chinook-local")
	require.ErrorIs(t, err, ErrUnsupportedDataModels)
	for _, d := range []*ProjectDataModels{nil, {Version: 1}} {
		_, err = d.ReferenceByID("missing")
		require.Error(t, err)
	}
	_, err = json.Marshal(invalid)
	require.ErrorIs(t, err, ErrUnsupportedDataModels)
	var existing ProjectFile
	require.NoError(t, json.Unmarshal([]byte(`{"dataModels":`+referenceJSON+`,"future":{"number":900719925474099312345678901},"shared":"old"}`), &existing))
	var incoming ProjectFile
	require.NoError(t, json.Unmarshal([]byte(`{"own":true,"shared":"new"}`), &incoming))
	require.NoError(t, incoming.MergePreservedMetadata(existing))
	require.Equal(t, existing.DataModels, incoming.DataModels)
	body, err := json.Marshal(incoming)
	require.NoError(t, err)
	require.Contains(t, string(body), "900719925474099312345678901")
	require.Contains(t, string(body), `"shared":"new"`)
	empty := ProjectFile{}
	require.NoError(t, empty.MergePreservedMetadata(existing))
	require.ErrorIs(t, empty.MergePreservedMetadata(ProjectFile{DataModels: invalid}), ErrUnsupportedDataModels)
	require.ErrorIs(t, (&ProjectFile{DataModels: invalid}).MergePreservedMetadata(ProjectFile{}), ErrUnsupportedDataModels)
}

func TestProjectDataModelsMalformedDocumentAndEnvelope(t *testing.T) {
	var unchanged Project
	require.NoError(t, unchanged.UnmarshalJSON([]byte(`null`)))
	for _, body := range []string{`{"references":[]}`, `{"version":1,"references":[],"catalogBindings":null}`, `{"version":1,"references":[],"catalogBindings":[{"unknown":true}]}`, `{"version":"one","references":[]}`, `{"version":1,"references":[],"catalogBindings":[null]}`, `{"version":1,"references":[null]}`, `{"version":1,"references":[],"catalogBindings":[{"environment":"\ud800"}]}`} {
		var d ProjectDataModels
		require.Error(t, d.UnmarshalJSON([]byte(body)), body)
	}
	for _, body := range []string{"", `[]`, `{"x":`, `{"x":1`, `{"x":1]`, `{"x":"value"`, `{"x":"value" `, `{"x":{} `, `{"x":{}`, `{} {}`, `{"dataModels":`, `{"x":1,`, `{"environmentOrder":1}`, `{"id":1}`} {
		var p Project
		require.Error(t, p.UnmarshalJSON([]byte(body)), body)
	}
	for _, body := range []string{"", `[]`, `{"x":`, `{"x":1`, `{"x":1]`, `{"x":"value"`, `{"x":"value" `, `{"x":{} `, `{"x":{}`, `{} {}`, `{"dataModels":`, `{"x":1,`} {
		require.Error(t, validateProjectDataModelsJSON([]byte(body)), body)
	}
	_, err := exactFields([]byte(`{"x":`), "x")
	require.Error(t, err)
}
