package dalgostore_test

import (
	"context"
	"testing"

	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
	"github.com/stretchr/testify/require"
)

func assetCard(id string) *datatug.Card {
	return &datatug.Card{
		ID:    id,
		Title: "Customer",
		Parameters: []datatug.CardParameterDef{{
			ParameterDef: datatug.ParameterDef{ID: "customerId", Type: "string", IsRequired: true},
			Match:        []datatug.CardMatchRule{{Table: "Orders", Column: "CustomerID"}},
		}},
		Widget: &datatug.BoardWidget{Name: "text", Data: &datatug.TextWidgetDef{Markdown: "hi"}},
	}
}

func TestProjectCardsRoundTripAndIsolation(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	root := record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "one"), "projects", "project")
	store, err := dalgostore.NewProjectItemsStore(db, "project", root)
	require.NoError(t, err)
	require.NoError(t, store.SaveCard(ctx, assetCard("customer")))
	require.NoError(t, store.SaveCard(ctx, assetCard("customer")), "overwrite")
	require.NoError(t, store.SaveCard(ctx, assetCard("second")), "a new card lists the existing ones")

	got, err := store.LoadCard(ctx, "customer")
	require.NoError(t, err)
	require.Equal(t, "Orders", got.Parameters[0].Match[0].Table)
	cards, err := store.LoadCards(ctx)
	require.NoError(t, err)
	require.Len(t, cards, 2)

	other, err := dalgostore.NewProjectItemsStore(db, "project", record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "two"), "projects", "project"))
	require.NoError(t, err)
	otherCards, err := other.LoadCards(ctx)
	require.NoError(t, err)
	require.Empty(t, otherCards)

	require.NoError(t, store.DeleteCard(ctx, "customer"))
	require.NoError(t, store.DeleteCard(ctx, "second"))
	cards, err = store.LoadCards(ctx)
	require.NoError(t, err)
	require.Empty(t, cards)
}

func TestProjectCardsRejections(t *testing.T) {
	ctx := context.Background()
	store, err := dalgostore.NewProjectItemsStore(newTestDB(), "project", record.NewKeyWithID("projects", "project"))
	require.NoError(t, err)
	require.ErrorIs(t, store.SaveCard(ctx, nil), dalgostore.ErrAssetInvalid)
	require.Error(t, store.SaveCard(ctx, assetCard("bad/id")))
	invalid := assetCard("ok")
	invalid.Title = ""
	require.ErrorIs(t, store.SaveCard(ctx, invalid), dalgostore.ErrAssetInvalid)
}

func TestProjectCardsCreatedAt(t *testing.T) {
	ctx := context.Background()
	root := record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "one"), "projects", "project")
	store, err := dalgostore.NewProjectItemsStore(newTestDB(), "project", root)
	require.NoError(t, err)

	fresh := assetCard("fresh")
	require.NoError(t, store.SaveCard(ctx, fresh))
	require.NotEmpty(t, fresh.CreatedAt, "new card is stamped")

	again := assetCard("fresh")
	require.NoError(t, store.SaveCard(ctx, again))
	require.Equal(t, fresh.CreatedAt, again.CreatedAt, "overwrite keeps the stored date")
	got, err := store.LoadCard(ctx, "fresh")
	require.NoError(t, err)
	require.Equal(t, fresh.CreatedAt, got.CreatedAt)

	explicit := assetCard("explicit")
	explicit.CreatedAt = "2026-10-07T06:18:13Z"
	require.NoError(t, store.SaveCard(ctx, explicit))
	got, err = store.LoadCard(ctx, "explicit")
	require.NoError(t, err)
	require.Equal(t, "2026-10-07T06:18:13Z", got.CreatedAt)

	bad := assetCard("bad")
	bad.CreatedAt = "nope"
	require.ErrorIs(t, store.SaveCard(ctx, bad), dalgostore.ErrAssetInvalid)
}
