package incidents

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStoreLocationValidate(t *testing.T) {
	project := ProjectRef{StoreID: "ops", ProjectID: "billing"}
	for _, location := range []StoreLocation{
		{StoreID: "ops", Kind: StoreLocationProjectRepository, Project: &project},
		{StoreID: "ops", Kind: StoreLocationDedicatedRepository},
		{StoreID: "ops", Kind: StoreLocationApplicationRepository},
	} {
		require.NoError(t, location.Validate(), location.Kind)
	}

	require.Error(t, (StoreLocation{Kind: StoreLocationDedicatedRepository}).Validate())
	require.Error(t, (StoreLocation{StoreID: "ops", Kind: "database"}).Validate())
	require.Error(t, (StoreLocation{StoreID: "ops", Kind: StoreLocationProjectRepository}).Validate())
	require.Error(t, (StoreLocation{
		StoreID: "ops", Kind: StoreLocationProjectRepository,
		Project: &ProjectRef{StoreID: "ops"},
	}).Validate())
	require.Error(t, (StoreLocation{StoreID: "ops", Kind: StoreLocationDedicatedRepository, Project: &project}).Validate())
	require.Error(t, (StoreLocation{StoreID: "other", Kind: StoreLocationProjectRepository, Project: &project}).Validate())
}

func TestSharedLayout(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	layout, err := LayoutFor(ref)
	require.NoError(t, err)
	require.Equal(t, "incidents/INC-1", layout.IncidentDir)
	require.Equal(t, "incidents/INC-1/events.jsonl", layout.Events)
	require.Equal(t, "incidents/INC-1/incident.json", layout.Projection)
	require.Equal(t, "incidents/.store/mutations", layout.MutationReceiptsDir)
	require.Equal(t, "incidents/.store/lock", layout.StoreLock)
	require.Equal(t, "incidents/.store/merge-intents", layout.MergeIntentsDir)

	_, err = LayoutFor(IncidentRef{StoreID: "ops", IncidentID: ".."})
	require.Error(t, err)
}

func TestMutationValidate(t *testing.T) {
	ref := IncidentRef{StoreID: "ops", IncidentID: "INC-1"}
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	mutation := Mutation{
		MutationID: "mutation-1", Incident: ref,
		Event: EventDraft{
			At: at, Actor: Actor{Kind: ActorHuman, ID: "alex", Via: ActorViaCLI},
			Type: EventNoteAdded, Assertion: Assertion{Kind: AssertionClaim},
			Payload: mustJSON(t, NoteAddedPayload{Body: "checked retry queue"}),
		},
	}
	require.NoError(t, mutation.Validate())

	badID := mutation
	badID.MutationID = "../escape"
	require.Error(t, badID.Validate())
	badEvent := mutation
	badEvent.Event.Payload = nil
	require.Error(t, badEvent.Validate())
	badIncident := mutation
	badIncident.Incident = IncidentRef{}
	require.Error(t, badIncident.Validate())
}

func TestMergeMutationValidate(t *testing.T) {
	valid := MergeMutation{
		MutationID: "merge-1",
		Source:     IncidentRef{StoreID: "ops", IncidentID: "INC-2"},
		Into:       IncidentRef{StoreID: "ops", IncidentID: "INC-1"},
	}
	require.NoError(t, valid.Validate())

	badMutationID := valid
	badMutationID.MutationID = "../merge"
	require.Error(t, badMutationID.Validate())
	badSource := valid
	badSource.Source = IncidentRef{}
	require.Error(t, badSource.Validate())
	badDestination := valid
	badDestination.Into = IncidentRef{}
	require.Error(t, badDestination.Validate())
	crossStore := valid
	crossStore.Into.StoreID = "other"
	require.Error(t, crossStore.Validate())
	valid.Into = valid.Source
	require.Error(t, valid.Validate())
}

func TestValidSegmentRejectsControlCharacters(t *testing.T) {
	require.False(t, validSegment("incident\x00id"))
}

// Compile-time contract: adapters implement one store interface for all
// three StoreLocation kinds; location changes routing, never the layout.
var _ Store = (*stubStore)(nil)

type stubStore struct{}

func (*stubStore) Append(context.Context, Mutation) (AppendResult, error)       { return AppendResult{}, nil }
func (*stubStore) Events(context.Context, IncidentRef, uint64) ([]Event, error) { return nil, nil }
func (*stubStore) Projection(context.Context, IncidentRef, *time.Time) (Incident, error) {
	return Incident{}, nil
}
func (*stubStore) Merge(context.Context, MergeMutation) (MergeResult, error) {
	return MergeResult{}, nil
}
