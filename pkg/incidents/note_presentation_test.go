package incidents

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNotePresentationStrictValidation(t *testing.T) {
	created := task3CreatedEvent(task3CreateMutation())
	for _, raw := range []string{
		`{"body":"ordinary"}`, `{"body":"ordinary","presentation":null}`,
		`{"body":"fictional","presentation":{"kind":"demo-character","characterId":"sarah"}}`,
		`{"body":"fictional","presentation":{"kind":"demo-character","characterId":"maya"}}`,
		`{"body":"fictional","presentation":{"kind":"demo-character","characterId":"daniel"}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			note := eventWithPayload(t, created.Incident, 2, created.At.Add(time.Minute), EventNoteAdded, NoteAddedPayload{Body: "placeholder"})
			note.Payload = json.RawMessage(raw)
			require.NoError(t, note.Validate())
			folded, err := Fold([]Event{created, note}, nil)
			require.NoError(t, err)
			require.Len(t, folded.Notes, 1)
		})
	}
	for _, presentation := range []string{
		`{}`, `[]`, `true`, `"maya"`, `42`,
		`{"kind":"human","characterId":"maya"}`,
		`{"kind":"demo-character","characterId":"alex"}`,
		`{"kind":"demo-character"}`, `{"characterId":"maya"}`,
		`{"kind":null,"characterId":"maya"}`, `{"kind":"demo-character","characterId":null}`,
		`{"kind":1,"characterId":"maya"}`, `{"kind":"demo-character","characterId":true}`,
		`{"kind":"demo-character","characterId":"Maya"}`,
		`{"kind":"demo-character","characterId":"maya","actor":"forged"}`,
		`{"kind":"demo-character","characterId":"maya","at":"forged"}`,
		`{"kind":"demo-character","characterId":"maya","role":"admin"}`,
		`{"kind":"demo-character","characterId":"maya","extra":null}`,
		`{"kind":"demo-character","characterId":"maya","characterId":"sarah"}`,
	} {
		t.Run(presentation, func(t *testing.T) {
			note := eventWithPayload(t, created.Incident, 2, created.At.Add(time.Minute), EventNoteAdded, NoteAddedPayload{Body: "fictional"})
			note.Payload = json.RawMessage(fmt.Sprintf(`{"body":"fictional","presentation":%s}`, presentation))
			require.Error(t, note.Validate())
			_, err := Fold([]Event{created, note}, nil)
			require.Error(t, err)
		})
	}
}

func TestDemoCharactersRetainRealActorAndEventVisibility(t *testing.T) {
	created := task3CreatedEvent(task3CreateMutation())
	events := []Event{created}
	for i, id := range []string{"sarah", "maya", "daniel"} {
		note := eventWithPayload(t, created.Incident, uint64(i+2), created.At.Add(time.Duration(i+1)*time.Minute), EventNoteAdded, NoteAddedPayload{Body: "simulated dialogue", Presentation: &NotePresentation{Kind: "demo-character", CharacterID: id}})
		note.Actor = created.Actor
		events = append(events, note)
		view, visible, err := ApplyEventView(note, ViewPolicy{})
		require.NoError(t, err)
		require.True(t, visible)
		require.Equal(t, created.Actor, view.Actor)
		require.Equal(t, note.At, view.At)
		var payload NoteAddedPayload
		require.NoError(t, json.Unmarshal(view.Payload, &payload))
		require.Equal(t, id, payload.Presentation.CharacterID)
		view.Payload[0] = 'X'
		require.True(t, json.Valid(note.Payload))
		policy := ViewPolicy{WithheldEvents: map[string]bool{note.ID: true}}
		hidden, visible, err := ApplyEventView(note, policy)
		require.NoError(t, err)
		require.False(t, visible)
		require.Equal(t, Event{}, hidden)
		item, visible, err := ApplyStreamItemView(StreamItem{Event: note}, policy)
		require.NoError(t, err)
		require.False(t, visible)
		require.Empty(t, item.Event.Payload)
	}
	folded, err := Fold(events, nil)
	require.NoError(t, err)
	ordinary := append([]Event(nil), events...)
	for i := 1; i < len(ordinary); i++ {
		ordinary[i].Payload = mustTask3JSON(NoteAddedPayload{Body: "simulated dialogue"})
	}
	expected, err := Fold(ordinary, nil)
	require.NoError(t, err)
	require.Equal(t, expected, folded)
	hidden := ApplyIncidentView(folded, ViewPolicy{WithheldEvents: map[string]bool{events[1].ID: true, events[2].ID: true, events[3].ID: true}})
	require.Empty(t, hidden.Notes)
}
