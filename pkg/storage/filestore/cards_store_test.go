package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testCard(id, title string) *datatug.Card {
	return &datatug.Card{
		ID:    id,
		Title: title,
		Parameters: []datatug.CardParameterDef{{
			ParameterDef: datatug.ParameterDef{ID: "customerId", Type: "string", IsRequired: true},
			Match:        []datatug.CardMatchRule{{Column: "CustomerID"}},
		}},
		Widget: &datatug.BoardWidget{Name: "text", Data: &datatug.TextWidgetDef{Markdown: "hi"}},
	}
}

func newCardsTestStore(t *testing.T) (fsCardsStore, string) {
	t.Helper()
	dir := t.TempDir()
	return newFsCardsStore(dir), filepath.Join(dir, storage.CardsFolder)
}

func writeCardJSON(t *testing.T, p, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o777))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func TestFsCardsStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	store, cardsDir := newCardsTestStore(t)

	cards, err := store.LoadCards(ctx)
	require.NoError(t, err)
	assert.Empty(t, cards)

	require.NoError(t, store.SaveCard(ctx, testCard("b-card", "B")))
	require.NoError(t, store.SaveCard(ctx, testCard("a_card", "A")))
	assert.FileExists(t, filepath.Join(cardsDir, "b-card", "card.json"))

	card, err := store.LoadCard(ctx, "b-card")
	require.NoError(t, err)
	assert.Equal(t, "B", card.Title)
	assert.Equal(t, datatug.CardMatchRule{Column: "CustomerID"}, card.Parameters[0].Match[0])

	cards, err = store.LoadCards(ctx)
	require.NoError(t, err)
	require.Len(t, cards, 2)
	assert.Equal(t, "a_card", cards[0].ID)
	assert.Equal(t, "b-card", cards[1].ID)

	require.NoError(t, store.DeleteCard(ctx, "b-card"))
	assert.NoFileExists(t, filepath.Join(cardsDir, "b-card", "card.json"))
	assert.NoDirExists(t, filepath.Join(cardsDir, "b-card"))
	_, err = store.LoadCard(ctx, "b-card")
	assert.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, store.DeleteCard(ctx, "b-card"), "deleting twice is fine")
}

func TestFsCardsStore_FlatAndFolderLayouts(t *testing.T) {
	ctx := context.Background()
	store, cardsDir := newCardsTestStore(t)
	writeCardJSON(t, filepath.Join(cardsDir, "flat.card.json"), `{"title":"Flat","widget":{"name":"text","data":{"markdown":"x"}}}`)
	writeCardJSON(t, filepath.Join(cardsDir, "both.card.json"), `{"title":"FlatWins?","widget":{"name":"text","data":{"markdown":"x"}}}`)
	writeCardJSON(t, filepath.Join(cardsDir, "both", "card.json"), `{"title":"FolderWins","widget":{"name":"text","data":{"markdown":"x"}}}`)

	flat, err := store.LoadCard(ctx, "flat")
	require.NoError(t, err)
	assert.Equal(t, "Flat", flat.Title)
	assert.Equal(t, "flat", flat.ID, "the id comes from the file name")

	both, err := store.LoadCard(ctx, "both")
	require.NoError(t, err)
	assert.Equal(t, "FolderWins", both.Title)

	cards, err := store.LoadCards(ctx)
	require.NoError(t, err)
	require.Len(t, cards, 2)

	// Saving a flat card writes the folder form and removes the flat file.
	flat.Widget = &datatug.BoardWidget{Name: "text", Data: &datatug.TextWidgetDef{Markdown: "x"}}
	require.NoError(t, store.SaveCard(ctx, flat))
	assert.FileExists(t, filepath.Join(cardsDir, "flat", "card.json"))
	assert.NoFileExists(t, filepath.Join(cardsDir, "flat.card.json"))

	// Delete removes both forms.
	require.NoError(t, store.DeleteCard(ctx, "both"))
	assert.NoFileExists(t, filepath.Join(cardsDir, "both.card.json"))
	assert.NoFileExists(t, filepath.Join(cardsDir, "both", "card.json"))
}

func TestFsCardsStore_Rejections(t *testing.T) {
	ctx := context.Background()
	store, cardsDir := newCardsTestStore(t)

	for _, id := range []string{"", "../x", "a/b", "a b", "a.b"} {
		assert.Error(t, store.SaveCard(ctx, testCard(id, "T")), id)
		_, err := store.LoadCard(ctx, id)
		assert.Error(t, err, id)
		assert.Error(t, store.DeleteCard(ctx, id), id)
	}
	assert.Error(t, store.SaveCard(ctx, nil))
	invalid := testCard("ok", "")
	assert.Error(t, store.SaveCard(ctx, invalid), "invalid card is not saved")
	assert.NoFileExists(t, filepath.Join(cardsDir, "ok", "card.json"))

	// Unreadable and unknown-key files fail the load with the path.
	writeCardJSON(t, filepath.Join(cardsDir, "broken", "card.json"), `{not json`)
	_, err := store.LoadCard(ctx, "broken")
	assert.ErrorContains(t, err, "broken")
	// One bad card does not hide the others.
	require.NoError(t, store.SaveCard(ctx, testCard("good", "Good")))
	cards, err := store.LoadCards(ctx)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "good", cards[0].ID)
	require.NoError(t, os.RemoveAll(filepath.Join(cardsDir, "broken")))

	writeCardJSON(t, filepath.Join(cardsDir, "extra", "card.json"), `{"title":"T","unknown":1}`)
	_, err = store.LoadCard(ctx, "extra")
	assert.Error(t, err)
}

func TestFsCardsStore_IDMismatchAndInvalidCardsAreRefusedOrSkipped(t *testing.T) {
	ctx := context.Background()
	store, cardsDir := newCardsTestStore(t)
	writeCardJSON(t, filepath.Join(cardsDir, "mine", "card.json"), `{"id":"other","title":"T","widget":{"name":"text","data":{"markdown":"x"}}}`)
	_, err := store.LoadCard(ctx, "mine")
	assert.ErrorContains(t, err, `declares id "other"`)
	writeCardJSON(t, filepath.Join(cardsDir, "same", "card.json"), `{"id":"same","title":"T","widget":{"name":"text","data":{"markdown":"x"}}}`)
	card, err := store.LoadCard(ctx, "same")
	require.NoError(t, err)
	assert.Equal(t, "same", card.ID)
	// A card that parses but does not validate is left out of the list.
	writeCardJSON(t, filepath.Join(cardsDir, "untitled", "card.json"), `{"title":"","widget":{"name":"text"}}`)
	cards, err := store.LoadCards(ctx)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "same", cards[0].ID)
}

func TestFsCardsStore_ListIgnoresForeignEntries(t *testing.T) {
	ctx := context.Background()
	store, cardsDir := newCardsTestStore(t)
	require.NoError(t, store.SaveCard(ctx, testCard("keep", "Keep")))
	require.NoError(t, store.SaveCard(ctx, testCard("keep", "Keep again")), "overwrite")
	require.NoError(t, os.MkdirAll(filepath.Join(cardsDir, "empty-dir"), 0o777))
	writeCardJSON(t, filepath.Join(cardsDir, "readme.md"), "x")
	writeCardJSON(t, filepath.Join(cardsDir, "bad id.card.json"), "{}")
	writeCardJSON(t, filepath.Join(cardsDir, "bad id", "card.json"), "{}")

	cards, err := store.LoadCards(ctx)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	assert.Equal(t, "Keep again", cards[0].Title)
}

func TestFsCardsStore_ListErrors(t *testing.T) {
	dir := t.TempDir()
	store := newFsCardsStore(dir)
	// "cards" is a file, not a folder: the directory read fails.
	require.NoError(t, os.WriteFile(filepath.Join(dir, storage.CardsFolder), []byte("x"), 0o644))
	_, err := store.LoadCards(context.Background())
	assert.Error(t, err)
}

func TestFsCardsStore_ListSkipsVanishedCard(t *testing.T) {
	ctx := context.Background()
	store, cardsDir := newCardsTestStore(t)
	// A dangling symlink is listed by the directory scan but cannot be read.
	require.NoError(t, os.MkdirAll(cardsDir, 0o777))
	require.NoError(t, os.Symlink(filepath.Join(cardsDir, "nowhere"), filepath.Join(cardsDir, "ghost.card.json")))
	cards, err := store.LoadCards(ctx)
	require.NoError(t, err)
	assert.Empty(t, cards)
}

func TestFsCardsStore_DeleteAndSaveFaults(t *testing.T) {
	ctx := context.Background()
	store, cardsDir := newCardsTestStore(t)
	// A folder where the card file should be makes Remove refuse.
	require.NoError(t, os.MkdirAll(filepath.Join(cardsDir, "x", "card.json"), 0o777))
	assert.Error(t, store.DeleteCard(ctx, "x"))
	assert.Error(t, store.SaveCard(ctx, testCard("x", "T")))

	// A folder at the flat path makes the post-save flat cleanup fail.
	require.NoError(t, os.MkdirAll(filepath.Join(cardsDir, "y.card.json"), 0o777))
	assert.Error(t, store.SaveCard(ctx, testCard("y", "T")))
}

func TestFsProjectStore_HasCards(t *testing.T) {
	dir := t.TempDir()
	var store datatug.CardsStore = newFsProjectStore("p", dir)
	require.NoError(t, store.SaveCard(context.Background(), testCard("c", "C")))
	assert.FileExists(t, filepath.Join(dir, "cards", "c", "card.json"))
}

func TestFsCardsStore_ListPropagatesIOFailuresButSkipsBadContent(t *testing.T) {
	ctx := context.Background()
	store, cardsDir := newCardsTestStore(t)
	require.NoError(t, store.SaveCard(ctx, testCard("good", "Good")))
	writeCardJSON(t, filepath.Join(cardsDir, "broken", "card.json"), `{not json`)
	cards, err := store.LoadCards(ctx)
	require.NoError(t, err)
	require.Len(t, cards, 1)

	// A card file that cannot be read (here: a folder in its place) is an I/O
	// failure, not a bad card: the list fails instead of serving a partial result.
	require.NoError(t, os.MkdirAll(filepath.Join(cardsDir, "unreadable", "card.json"), 0o777))
	_, err = store.LoadCards(ctx)
	require.Error(t, err)
	assert.NotErrorIs(t, err, errInvalidCardContent)
}

func TestFsCardsStore_CreatedAt(t *testing.T) {
	ctx := context.Background()
	store, cardsDir := newCardsTestStore(t)

	fresh := testCard("fresh", "Fresh")
	require.NoError(t, store.SaveCard(ctx, fresh))
	require.NotEmpty(t, fresh.CreatedAt, "new card is stamped")
	_, err := time.Parse(time.RFC3339Nano, fresh.CreatedAt)
	require.NoError(t, err)

	again := testCard("fresh", "Fresh v2") // overwrite without a date keeps the stored one
	require.NoError(t, store.SaveCard(ctx, again))
	assert.Equal(t, fresh.CreatedAt, again.CreatedAt)
	loaded, err := store.LoadCard(ctx, "fresh")
	require.NoError(t, err)
	assert.Equal(t, fresh.CreatedAt, loaded.CreatedAt)

	explicit := testCard("explicit", "E")
	explicit.CreatedAt = "2026-10-07T06:18:13Z"
	require.NoError(t, store.SaveCard(ctx, explicit))
	loaded, err = store.LoadCard(ctx, "explicit")
	require.NoError(t, err)
	assert.Equal(t, "2026-10-07T06:18:13Z", loaded.CreatedAt)

	// A card file written by other tools with createdAt loads (it used to fail).
	writeCardJSON(t, filepath.Join(cardsDir, "foreign", "card.json"),
		`{"id":"foreign","title":"F","createdAt":"2026-10-07T06:18:13Z","widget":{"name":"text","data":{"markdown":"hi"}}}`)
	loaded, err = store.LoadCard(ctx, "foreign")
	require.NoError(t, err)
	assert.Equal(t, "2026-10-07T06:18:13Z", loaded.CreatedAt)

	// An undated stored card stays undated on save.
	writeCardJSON(t, filepath.Join(cardsDir, "old", "card.json"),
		`{"id":"old","title":"O","widget":{"name":"text","data":{"markdown":"hi"}}}`)
	old := testCard("old", "O")
	require.NoError(t, store.SaveCard(ctx, old))
	assert.Empty(t, old.CreatedAt)

	// Unreadable stored content counts as never saved.
	writeCardJSON(t, filepath.Join(cardsDir, "broken", "card.json"), `{`)
	broken := testCard("broken", "B")
	require.NoError(t, store.SaveCard(ctx, broken))
	assert.NotEmpty(t, broken.CreatedAt)

	bad := testCard("bad", "B")
	bad.CreatedAt = "nope"
	require.Error(t, bad.Validate())
}
