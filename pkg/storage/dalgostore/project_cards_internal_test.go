package dalgostore

import (
	"context"
	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
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
