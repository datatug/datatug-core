package datatug

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateOptionalCreatedAt(t *testing.T) {
	for _, ok := range []string{"", "2026-10-07T06:18:13Z", "2026-10-21T12:00:00.123456789Z", "2026-10-07T06:18:13+02:00"} {
		assert.NoError(t, ValidateOptionalCreatedAt(ok), ok)
	}
	for _, bad := range []string{"yesterday", "2026-10-07", "2026-10-07 06:18:13"} {
		assert.Error(t, ValidateOptionalCreatedAt(bad), bad)
	}
}

func TestNewCreatedAt(t *testing.T) {
	at := time.Date(2026, 10, 7, 8, 18, 13, 5, time.FixedZone("x", 2*3600))
	assert.Equal(t, "2026-10-07T06:18:13.000000005Z", NewCreatedAt(at))
}

func TestStampCardCreatedAt(t *testing.T) {
	now := time.Date(2026, 10, 25, 1, 2, 3, 0, time.UTC)
	card := &Card{CreatedAt: "2026-01-01T00:00:00Z"}
	StampCardCreatedAt(card, &Card{CreatedAt: "2025-01-01T00:00:00Z"}, now)
	assert.Equal(t, "2026-01-01T00:00:00Z", card.CreatedAt, "own date wins")

	card = &Card{}
	StampCardCreatedAt(card, &Card{CreatedAt: "2025-01-01T00:00:00Z"}, now)
	assert.Equal(t, "2025-01-01T00:00:00Z", card.CreatedAt, "inherited from stored card")

	card = &Card{}
	StampCardCreatedAt(card, &Card{}, now)
	assert.Empty(t, card.CreatedAt, "stored undated card stays undated")

	card = &Card{}
	StampCardCreatedAt(card, nil, now)
	assert.Equal(t, "2026-10-25T01:02:03Z", card.CreatedAt, "new card is stamped")
}

func TestCardCreatedAt(t *testing.T) {
	card := validCard()
	card.CreatedAt = "2026-10-07T06:18:13Z"
	require.NoError(t, card.Validate())
	data, err := json.Marshal(card)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"createdAt":"2026-10-07T06:18:13Z"`)

	var loaded Card // the exact previous failure: unknown field "createdAt"
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, "2026-10-07T06:18:13Z", loaded.CreatedAt)

	card.CreatedAt = ""
	data, err = json.Marshal(card)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "createdAt")
	loaded = Card{}
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Empty(t, loaded.CreatedAt)

	card.CreatedAt = "not-a-date"
	assert.Error(t, card.Validate())
}

func TestCheckCreatedAt(t *testing.T) {
	check := Check{ID: "id", Title: "t", Type: "sql", Data: json.RawMessage(`{"query":"select 1"}`)}
	require.NoError(t, check.Validate())
	check.CreatedAt = "2026-10-07T06:18:13Z"
	require.NoError(t, check.Validate())
	data, err := json.Marshal(check)
	require.NoError(t, err)
	var loaded Check
	require.NoError(t, json.Unmarshal(data, &loaded))
	assert.Equal(t, check.CreatedAt, loaded.CreatedAt)
	check.CreatedAt = "nope"
	assert.Error(t, check.Validate())
}
