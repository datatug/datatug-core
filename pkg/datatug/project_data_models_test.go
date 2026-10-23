package datatug

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

const referenceJSON = `{"version":1,"references":[{"id":"chinook-v1","address":"modelspec://github.com/Demo-DB/Chinook/chinook","commit":"3e7bb316d8a438eb7e5930067997a4e4543a275c"}],"catalogBindings":[{"environment":"local","catalog":"chinook-local","reference":"chinook-v1"}]}`

func TestProjectDataModelsJSONExactNamespace(t *testing.T) {
	var doc ProjectDataModels
	require.NoError(t, json.Unmarshal([]byte(referenceJSON), &doc))
	r, err := doc.ReferenceByID("chinook-v1")
	require.NoError(t, err)
	require.Equal(t, "modelspec://github.com/demo-db/chinook/chinook", r.NormalizedAddress())
	require.Contains(t, r.Address, "Demo-DB/Chinook")
	bound, err := doc.ReferenceForCatalog("local", "chinook-local")
	require.NoError(t, err)
	require.Equal(t, r, bound)
	_, err = doc.ReferenceForCatalog("local", "chinook-local.sqlite")
	require.Error(t, err)
	for _, body := range []string{"null", `{"Version":1,"references":[]}`, `{"version":1,"version":1,"references":[]}`, `{"version":1,"Version":1,"references":[]}`, `{"version":1,"references":null}`, `{"version":1,"references":[],"future":1}`, strings.Replace(referenceJSON, `"id":"chinook-v1"`, `"ID":"chinook-v1"`, 1), strings.Replace(referenceJSON, `"commit":`, `"Commit":`, 1), strings.Replace(referenceJSON, `"catalog":"chinook-local"`, `"catalog":"chinook-local","Catalog":"chinook-local"`, 1), strings.Replace(referenceJSON, `"reference":"chinook-v1"`, `"reference":"missing"`, 1), strings.Replace(referenceJSON, `"version":1`, `"version":2`, 1), strings.Replace(referenceJSON, `"commit":"3e7bb316d8a438eb7e5930067997a4e4543a275c"`, `"commit":"main"`, 1)} {
		t.Run(body[:min(35, len(body))], func(t *testing.T) { var v ProjectDataModels; require.Error(t, json.Unmarshal([]byte(body), &v)) })
	}
	for _, envelope := range []string{`{"dataModels":null}`, `{"DataModels":` + referenceJSON + `}`, `{"dataModels":` + referenceJSON + `,"dataModels":` + referenceJSON + `}`, `{"dataModels":` + referenceJSON + `,"datamodels":` + referenceJSON + `}`} {
		var p Project
		require.Error(t, json.Unmarshal([]byte(envelope), &p))
		var f ProjectFile
		require.Error(t, json.Unmarshal([]byte(envelope), &f))
	}
	var p Project
	require.NoError(t, json.Unmarshal([]byte(`{"dataModels":`+referenceJSON+`}`), &p))
	require.NotNil(t, p.DataModels)
	var f ProjectFile
	require.NoError(t, json.Unmarshal([]byte(`{"dataModels":`+referenceJSON+`,"future":{"big":900719925474099312345678901,"x":1,"x":2}}`), &f))
	body, err := json.Marshal(f)
	require.NoError(t, err)
	require.Contains(t, string(body), "900719925474099312345678901")
	require.Contains(t, string(body), `"x":1,"x":2`)
	clear := ProjectDataModels{Version: 1}
	body, err = json.Marshal(clear)
	require.NoError(t, err)
	require.JSONEq(t, `{"version":1,"references":[]}`, string(body))
}
func TestDataModelReferenceStrictAddressAndCommit(t *testing.T) {
	for _, address := range []string{"modelspec://github.com/o/r/m?ref=abc", "modelspec://github.com/o/r/m#x", "modelspec://github.com/o/../m", "modelspec://github.com/o/r/m%2f", "modelspec://user@github.com/o/r/m", "modelspec://example.com/o/r/m"} {
		require.Error(t, (DataModelReference{Address: address, Commit: strings.Repeat("a", 40)}).Validate())
	}
	var doc ProjectDataModels
	require.NoError(t, json.Unmarshal([]byte(referenceJSON), &doc))
	doc.References = append(doc.References, doc.References[0])
	require.Error(t, doc.Validate())
	doc.References = doc.References[:1]
	doc.CatalogBindings = append(doc.CatalogBindings, doc.CatalogBindings[0])
	require.Error(t, doc.Validate())
}
