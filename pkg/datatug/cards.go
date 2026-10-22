package datatug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	"github.com/strongo/validation"
)

// CardIDPattern is the shape of a project card id. The id is also the name of
// the card's `cards/<id>/` folder, so it never contains a path separator.
var CardIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// Card display modes.
const (
	// CardDisplayPanel shows a matched card in the side panel (the default).
	CardDisplayPanel = "panel"
	// CardDisplayTooltip shows a matched card as a compact popover.
	CardDisplayTooltip = "tooltip"
)

// MaxCardDescriptionLength bounds Card.Description.
const MaxCardDescriptionLength = 1000

// CardMatchRule tells which grid cells feed a card parameter. Exactly one
// shape is valid: meaning-graph {entity, field}; {table, column}; or {column}.
type CardMatchRule struct {
	Entity string `json:"entity,omitempty"`
	Field  string `json:"field,omitempty"`
	Table  string `json:"table,omitempty"`
	Column string `json:"column,omitempty"`
}

// Validate returns error if the rule is not exactly one of the three shapes.
func (v CardMatchRule) Validate() error {
	entity := v.Entity != "" || v.Field != ""
	switch {
	case entity:
		if v.Entity == "" || v.Field == "" || v.Table != "" || v.Column != "" {
			return validation.NewErrBadRecordFieldValue("match", "entity rule requires entity and field only")
		}
	case v.Table != "":
		if v.Column == "" {
			return validation.NewErrBadRecordFieldValue("match", "table rule requires table and column")
		}
	case v.Column == "":
		return validation.NewErrBadRecordFieldValue("match", "rule is empty: use entity+field, table+column or column")
	}
	return nil
}

// CardParameterDef is a card parameter: the existing parameter definition plus
// optional match rules.
type CardParameterDef struct {
	ParameterDef
	Match []CardMatchRule `json:"match,omitempty"`
}

// Validate returns error if not valid.
func (v CardParameterDef) Validate() error {
	if err := v.ParameterDef.Validate(); err != nil {
		return err
	}
	for i, rule := range v.Match {
		if err := rule.Validate(); err != nil {
			return fmt.Errorf("invalid match rule at index %v: %w", i, err)
		}
	}
	return nil
}

// BoardCardRef is a dashboard cell's reference to a project card in place of
// an inline widget. It binds the card's parameters to board parameters or
// constants.
type BoardCardRef struct {
	CardID     string                  `json:"cardId"`
	Parameters WidgetParameterBindings `json:"parameters,omitempty"`
}

// Validate returns error if not valid.
func (v BoardCardRef) Validate() error {
	if v.CardID == "" {
		return validation.NewErrRecordIsMissingRequiredField("cardId")
	}
	if !CardIDPattern.MatchString(v.CardID) {
		return validation.NewErrBadRecordFieldValue("cardId", "invalid card id")
	}
	if err := v.Parameters.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("parameters", err.Error())
	}
	return nil
}

// Card is a standalone project card stored at `cards/<id>/card.json`: the
// shape of a dashboard BoardCard plus its own parameters and default display.
// A card never holds a ref, so cards cannot cycle.
type Card struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description,omitempty"`
	Parameters  []CardParameterDef `json:"parameters,omitempty"`
	Widget      *BoardWidget       `json:"widget,omitempty"`
	Display     string             `json:"display,omitempty"`
}

// GetID returns the card id.
func (v *Card) GetID() string { return v.ID }

// SetID sets the card id.
func (v *Card) SetID(id string) { v.ID = id }

// UnmarshalJSON rejects unknown keys, so a card cannot carry a ref or any
// other field outside the contract. Numbers keep their exact text (json.Number),
// so a saved constant or default is never rounded.
func (v *Card) UnmarshalJSON(data []byte) error {
	type plain Card
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	var decoded plain
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("unexpected data after card JSON")
	}
	*v = Card(decoded)
	return nil
}

// Validate returns error if not valid.
func (v *Card) Validate() error {
	if v.ID == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	if !CardIDPattern.MatchString(v.ID) {
		return validation.NewErrBadRecordFieldValue("id", "must match "+CardIDPattern.String())
	}
	if err := validateStringField("title", v.Title, true, MaxTitleLength); err != nil {
		return err
	}
	if err := validateStringField("description", v.Description, false, MaxCardDescriptionLength); err != nil {
		return err
	}
	switch v.Display {
	case "", CardDisplayPanel, CardDisplayTooltip:
	default:
		return validation.NewErrBadRecordFieldValue("display", "must be panel or tooltip")
	}
	seen := make(map[string]bool, len(v.Parameters))
	for i, p := range v.Parameters {
		if err := p.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue("parameters", fmt.Sprintf("invalid parameter #%v: %v", i+1, err))
		}
		if seen[p.ID] {
			return validation.NewErrBadRecordFieldValue("parameters", fmt.Sprintf("duplicate parameter id=%v", p.ID))
		}
		seen[p.ID] = true
	}
	if v.Widget == nil {
		return validation.NewErrRecordIsMissingRequiredField("widget")
	}
	if err := v.Widget.Validate(); err != nil {
		return fmt.Errorf("invalid card widget with id=%v: %w", v.ID, err)
	}
	return nil
}

// Brief returns the list row of the card.
func (v *Card) Brief() CardBrief {
	brief := CardBrief{ID: v.ID, Title: v.Title, Description: v.Description, Display: v.Display}
	for _, p := range v.Parameters {
		brief.Parameters = append(brief.Parameters, CardBriefParameter{
			ID: p.ID, Type: p.Type, IsRequired: p.IsRequired, Match: p.Match,
		})
	}
	return brief
}

// Cards is a list of project cards.
type Cards []*Card

// Validate returns error if any card is not valid.
func (v Cards) Validate() error {
	for i, card := range v {
		if err := card.Validate(); err != nil {
			return fmt.Errorf("invalid card at index %v: %w", i, err)
		}
	}
	return nil
}

// Briefs returns the list rows of the cards.
func (v Cards) Briefs() []CardBrief {
	briefs := make([]CardBrief, len(v))
	for i, card := range v {
		briefs[i] = card.Brief()
	}
	return briefs
}

// CardBriefParameter is the part of a card parameter that cell matching needs.
type CardBriefParameter struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	IsRequired bool            `json:"isRequired,omitempty"`
	Match      []CardMatchRule `json:"match,omitempty"`
}

// CardBrief is a row of `all_cards`, enough to match cells without loading
// every card.
type CardBrief struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Description string               `json:"description,omitempty"`
	Parameters  []CardBriefParameter `json:"parameters,omitempty"`
	Display     string               `json:"display,omitempty"`
}

// ProjectCardBrief is the wire name of CardBrief used by the web client.
type ProjectCardBrief = CardBrief

const maxTextWidgetMarkdownLength = 64 * 1024

// TextWidgetDef is the `text` widget: markdown with {{parameterId}} placeholders.
type TextWidgetDef struct {
	WidgetBase
	Markdown string `json:"markdown"`
}

// Validate returns error if not valid.
func (v *TextWidgetDef) Validate() error {
	if err := v.WidgetBase.Validate(false); err != nil {
		return err
	}
	return validateStringField("markdown", v.Markdown, false, maxTextWidgetMarkdownLength)
}

// FieldsWidgetSource tells where a fields widget takes its record from:
// exactly one of a saved query (first row) or a record parameter.
type FieldsWidgetSource struct {
	QueryID    string                  `json:"queryId,omitempty"`
	Parameters WidgetParameterBindings `json:"parameters,omitempty"`
	Parameter  string                  `json:"parameter,omitempty"`
}

// Validate returns error if not valid.
func (v FieldsWidgetSource) Validate() error {
	switch {
	case v.QueryID != "" && v.Parameter != "":
		return validation.NewErrBadRecordFieldValue("source", "queryId and parameter are mutually exclusive")
	case v.QueryID != "":
		if err := v.Parameters.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue("source.parameters", err.Error())
		}
	case v.Parameter != "":
		if len(v.Parameters) > 0 {
			return validation.NewErrBadRecordFieldValue("source.parameters", "only allowed with queryId")
		}
	default:
		return validation.NewErrRecordIsMissingRequiredField("source")
	}
	return nil
}

// FieldsWidgetField selects one column to show, with an optional label.
type FieldsWidgetField struct {
	Column string `json:"column"`
	Label  string `json:"label,omitempty"`
}

// FieldsWidgetDef is the `fields` widget: one record as label/value pairs.
type FieldsWidgetDef struct {
	WidgetBase
	Source FieldsWidgetSource  `json:"source"`
	Fields []FieldsWidgetField `json:"fields,omitempty"`
}

// Validate returns error if not valid.
func (v *FieldsWidgetDef) Validate() error {
	if err := v.WidgetBase.Validate(false); err != nil {
		return err
	}
	if err := v.Source.Validate(); err != nil {
		return err
	}
	for i, f := range v.Fields {
		if f.Column == "" {
			return validation.NewErrBadRecordFieldValue("fields", fmt.Sprintf("field #%v is missing column", i+1))
		}
	}
	return nil
}
