package incidents

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIncidentRefRoundTrip(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-2841"}
	require.NoError(t, ref.Validate())
	require.Equal(t, "ops/INC-2841", ref.String())

	parsed, err := ParseIncidentRef(ref.String())
	require.NoError(t, err)
	require.Equal(t, ref, parsed)

	data, err := json.Marshal(ref)
	require.NoError(t, err)
	require.JSONEq(t, `{"storeId":"ops","incidentId":"INC-2841"}`, string(data))
}

func TestIncidentRefRejectsAmbiguousParts(t *testing.T) {
	for _, value := range []string{"", "ops", "/INC-1", "ops/", "ops/INC-1/extra"} {
		t.Run(value, func(t *testing.T) {
			_, err := ParseIncidentRef(value)
			require.Error(t, err)
		})
	}

	require.Error(t, (IncidentRef{StoreID: "bad/store", IncidentID: "INC-1"}).Validate())
	require.Error(t, (IncidentRef{StoreID: "ops", IncidentID: "bad/id"}).Validate())
	require.Error(t, (IncidentRef{StoreID: " ops", IncidentID: "INC-1"}).Validate())
	require.Error(t, (IncidentRef{StoreID: "ops", IncidentID: "INC-1 "}).Validate())
}
