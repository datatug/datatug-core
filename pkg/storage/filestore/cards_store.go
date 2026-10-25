package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/datatug/datatug-core/internal/plainfs"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.CardsStore = (*fsCardsStore)(nil)

func newFsCardsStore(projectPath string) fsCardsStore {
	return fsCardsStore{projectDir: projectPath, dirPath: path.Join(projectPath, storage.CardsFolder)}
}

// fsCardsStore stores project cards like boards: the folder form
// "<cardsDir>/<id>/card.json", and the flat form "<cardsDir>/<id>.card.json"
// that is read too. The folder form wins when both exist; writes always use
// the folder form.
type fsCardsStore struct {
	projectDir string
	dirPath    string
}

const cardFileName = storage.CardFileSuffix + ".json"

func (s fsCardsStore) folderCardFilePath(id string) string {
	return path.Join(s.dirPath, id, cardFileName)
}

func (s fsCardsStore) flatCardFilePath(id string) string {
	return path.Join(s.dirPath, storage.JsonFileName(id, storage.CardFileSuffix))
}

func checkCardID(id string) error {
	if !datatug.CardIDPattern.MatchString(id) {
		return fmt.Errorf("invalid card id %q: must match %s", id, datatug.CardIDPattern)
	}
	return nil
}

// errInvalidCardContent marks a card file that was read but is not a valid card:
// listings skip it, while I/O failures propagate.
var errInvalidCardContent = errors.New("invalid card content")

func readCardFile(filePath string) (*datatug.Card, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	card := new(datatug.Card)
	if err = json.Unmarshal(data, card); err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidCardContent, err)
	}
	return card, nil
}

// loadOneCard returns (nil, nil) when the card exists in neither form.
func (s fsCardsStore) loadOneCard(id string) (*datatug.Card, error) {
	if err := checkCardID(id); err != nil {
		return nil, err
	}
	for _, filePath := range []string{s.folderCardFilePath(id), s.flatCardFilePath(id)} {
		card, err := readCardFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load card[%s] from %s: %w", id, filePath, err)
		}
		if card != nil {
			if card.ID != "" && card.ID != id {
				return nil, fmt.Errorf("%w: card[%s] at %s declares id %q", errInvalidCardContent, id, filePath, card.ID)
			}
			card.SetID(id)
			return card, nil
		}
	}
	return nil, nil
}

func (s fsCardsStore) LoadCard(_ context.Context, id string, o ...datatug.StoreOption) (*datatug.Card, error) {
	_ = datatug.GetStoreOptions(o...)
	card, err := s.loadOneCard(id)
	if err != nil {
		return nil, err
	}
	if card == nil {
		return nil, fmt.Errorf("failed to load card[%s] from project: %w", id, os.ErrNotExist)
	}
	return card, nil
}

func (s fsCardsStore) listCardIDs() ([]string, error) {
	dirEntries, err := os.ReadDir(s.dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	suffix := "." + cardFileName
	seen := make(map[string]struct{}, len(dirEntries))
	var ids []string
	add := func(id string) {
		if _, ok := seen[id]; ok || !datatug.CardIDPattern.MatchString(id) {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, de := range dirEntries {
		name := de.Name()
		if de.IsDir() {
			if _, statErr := os.Stat(path.Join(s.dirPath, name, cardFileName)); statErr == nil {
				add(name)
			}
			continue
		}
		if id, ok := strings.CutSuffix(name, suffix); ok {
			add(id)
		}
	}
	return ids, nil
}

func (s fsCardsStore) LoadCards(_ context.Context, o ...datatug.StoreOption) (datatug.Cards, error) {
	_ = datatug.GetStoreOptions(o...)
	ids, err := s.listCardIDs()
	if err != nil {
		return nil, err
	}
	cards := make(datatug.Cards, 0, len(ids))
	for _, id := range ids {
		card, err := s.loadOneCard(id)
		if err == nil && card != nil {
			if err = card.Validate(); err != nil {
				err = fmt.Errorf("%w: %v", errInvalidCardContent, err)
			}
		}
		if errors.Is(err, errInvalidCardContent) {
			// One bad card must not hide the others: the list holds the valid ones.
			log.Printf("filestore: skipping invalid card[%s]: %v", id, err)
			continue
		}
		if err != nil {
			return nil, err // an I/O failure is not a bad card
		}
		if card != nil {
			cards = append(cards, card)
		}
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	return cards, nil
}

// SaveCard always writes the folder form. A flat file of the same card is
// removed so the saved card is not shadowed or duplicated.
func (s fsCardsStore) SaveCard(_ context.Context, card *datatug.Card) error {
	if card == nil {
		return fmt.Errorf("card is nil")
	}
	if err := checkCardID(card.ID); err != nil {
		return err
	}
	if card.CreatedAt == "" {
		existing, err := s.loadOneCard(card.ID)
		if err != nil && !errors.Is(err, errInvalidCardContent) {
			return err
		}
		datatug.StampCardCreatedAt(card, existing, time.Now()) // unreadable content counts as never saved
	}
	if err := saveJSONFile(s.projectDir, path.Join(s.dirPath, card.ID), cardFileName, card); err != nil {
		return fmt.Errorf("failed to save card file: %w", err)
	}
	return plainfs.Remove(s.projectDir, s.flatCardFilePath(card.ID))
}

func (s fsCardsStore) DeleteCard(_ context.Context, id string) error {
	if err := checkCardID(id); err != nil {
		return err
	}
	for _, filePath := range []string{s.folderCardFilePath(id), s.flatCardFilePath(id)} {
		if err := plainfs.Remove(s.projectDir, filePath); err != nil {
			return err
		}
	}
	_ = os.Remove(path.Join(s.dirPath, id)) // only when empty
	return nil
}
