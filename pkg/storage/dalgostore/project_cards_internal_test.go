package dalgostore

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

func TestCardAssetCodec(t *testing.T) {
	_, err := encodeAsset((*datatug.Card)(nil))
	require.ErrorIs(t, err, ErrAssetInvalid)
	require.ErrorIs(t, decodeAsset([]byte(`{"id":"other","title":"T"}`), new(datatug.Card), "mine"), ErrAssetInvalid)
	require.ErrorIs(t, decodeAsset([]byte(`{"id":"mine","title":"T","ref":{"cardId":"x"}}`), new(datatug.Card), "mine"), ErrAssetInvalid)
}

func TestCardAssetProviderFailures(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	for _, fault := range []assetFailureSession{
		{ReadwriteSession: db.(dal.ReadwriteSession), DB: db, getErr: true},
		{ReadwriteSession: db.(dal.ReadwriteSession), DB: db, queryErr: true},
	} {
		s, err := NewProjectItemsStore(fault, "p", projectKey("p"))
		require.NoError(t, err)
		_, err = s.LoadCard(ctx, "c")
		if fault.getErr {
			require.Error(t, err)
		}
		_, err = s.LoadCards(ctx)
		if fault.queryErr {
			require.Error(t, err)
		}
	}
}

func createdAtTestCard(id string) *datatug.Card {
	return &datatug.Card{ID: id, Title: "T", Widget: &datatug.BoardWidget{Name: "text", Data: &datatug.TextWidgetDef{Markdown: "hi"}}}
}

func TestSaveCardStampsOverUnreadableContent(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	s, err := NewProjectItemsStore(db, "p", projectKey("p"))
	require.NoError(t, err)
	key, err := assetKey(s.assetRoot(), "cards", "c")
	require.NoError(t, err)
	require.NoError(t, db.(dal.ReadwriteSession).Set(ctx, record.NewRecordWithData(key, &assetDocument{Content: []byte(`{`)})))
	card := createdAtTestCard("c")
	require.NoError(t, s.SaveCard(ctx, card))
	require.NotEmpty(t, card.CreatedAt, "unreadable stored content counts as never saved")
}

func TestSaveCardStampPushesPastSizeLimit(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	s, err := NewProjectItemsStore(db, "p", projectKey("p"))
	require.NoError(t, err)
	card := createdAtTestCard("big")
	rule := datatug.CardMatchRule{Column: "c"}
	param := datatug.CardParameterDef{ParameterDef: datatug.ParameterDef{ID: "p", Type: "string"}, Match: []datatug.CardMatchRule{rule}}
	card.Parameters = []datatug.CardParameterDef{param}
	// Grow the card to fit the limit without room for a createdAt stamp.
	for i := 0; ; i++ {
		card.Parameters[0].Match[0].Column = strings.Repeat("c", i)
		body, err := json.Marshal(card)
		require.NoError(t, err)
		if len(body) > MaxProjectAssetBytes-10 {
			break
		}
		if i == 0 {
			card.Parameters = make([]datatug.CardParameterDef, 0, 60)
			for j := 0; j < 60; j++ {
				p := param
				p.ID = fmt.Sprintf("p%02d", j)
				p.Match = []datatug.CardMatchRule{{Column: strings.Repeat("c", 4000)}}
				card.Parameters = append(card.Parameters, p)
			}
		}
		if len(card.Parameters) == 60 {
			card.Parameters[59].Match[0].Column = strings.Repeat("c", i+1)
		}
	}
	require.ErrorIs(t, s.SaveCard(ctx, card), ErrAssetLimit)
}
