package dto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
)

var ErrAssetRevisionConflict = errors.New("project asset revision conflict")

type AssetWriteCondition struct {
	ProjectRef
	Environment string `json:"environment"`
	OperationID string `json:"operationId"`
	IfNoneMatch bool   `json:"ifNoneMatch,omitempty"`
	IfMatch     string `json:"ifMatch,omitempty"`
}

func (v AssetWriteCondition) Validate() error {
	if v.ProjectRef.Validate() != nil || !operationIDPattern.MatchString(v.OperationID) || v.Environment == "" || strings.TrimSpace(v.Environment) != v.Environment || v.IfNoneMatch == (v.IfMatch != "") {
		return errors.New("invalid asset write condition")
	}
	return nil
}

type SaveBoardRequest struct {
	AssetWriteCondition
	Board datatug.Board `json:"board"`
}

func (v SaveBoardRequest) Validate() error {
	if err := v.AssetWriteCondition.Validate(); err != nil {
		return err
	}
	return v.Board.Validate()
}

type SaveEntityRequest struct {
	AssetWriteCondition
	Entity datatug.Entity `json:"entity"`
}

func (v SaveEntityRequest) Validate() error {
	if err := v.AssetWriteCondition.Validate(); err != nil {
		return err
	}
	return v.Entity.Validate()
}

// SaveCardRequest saves one standalone project card.
type SaveCardRequest struct {
	AssetWriteCondition
	Card datatug.Card `json:"card"`
}

func (v SaveCardRequest) Validate() error {
	if err := v.AssetWriteCondition.Validate(); err != nil {
		return err
	}
	return v.Card.Validate()
}

type SaveCardResponse struct {
	Card     datatug.Card `json:"card"`
	Revision string       `json:"revision"`
	Receipt  string       `json:"receipt"`
}

type SaveBoardResponse struct {
	Board    datatug.Board `json:"board"`
	Revision string        `json:"revision"`
	Receipt  string        `json:"receipt"`
}
type SaveEntityResponse struct {
	Entity   datatug.Entity `json:"entity"`
	Revision string         `json:"revision"`
	Receipt  string         `json:"receipt"`
}

// ProjectAssetRevision qualifies a content digest by its owning wire type.
// Callers validate the actual asset before deriving its revision.
func ProjectAssetRevision(kind string, asset any) (string, error) {
	if kind != "board" && kind != "entity" && kind != "card" {
		return "", ErrUnsupportedCapability
	}
	encoded, err := json.Marshal(asset)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return kind + "1-" + hex.EncodeToString(digest[:]), nil
}
