package datatug

import (
	"encoding/json"
	"strings"
)

const maxMeaningCatalogConcepts = 4096
const maxMeaningCatalogResources = 4096
const maxMeaningCatalogBytes = 8 << 20

// Budget the exact serialized structure before appending a projected concept or
// resource. Individual JSON values are bounded before encoding; no oversized
// catalog or resource slice is constructed before refusal.
type meaningCatalogBudget struct{ bytes, concepts int }

func newMeaningCatalogBudget(scope MeaningScope, writable, complete bool) (*meaningCatalogBudget, error) {
	b := &meaningCatalogBudget{}
	c := ProjectMeaningsCatalog{Scope: scope, Version: 1, Revision: strings.Repeat("0", 64), Writable: writable, ReferencesComplete: complete, Entities: []MeaningEntity{}}
	if err := b.charge(c, len(scope.Storage)+len(scope.Project)+len(scope.SpaceID)+len(scope.Branch), false); err != nil {
		return nil, err
	}
	return b, nil
}
func (b *meaningCatalogBudget) charge(v any, rawBytes int, comma bool) error {
	if rawBytes > maxMeaningCatalogBytes-b.bytes {
		return ErrMeaningIncomplete
	}
	data, _ := json.Marshal(v)
	size := len(data)
	if comma {
		size++
	}
	if size > maxMeaningCatalogBytes-b.bytes {
		return ErrMeaningIncomplete
	}
	b.bytes += size
	return nil
}
func (b *meaningCatalogBudget) entity(e MeaningEntity, comma bool) error {
	if b.concepts >= maxMeaningCatalogConcepts {
		return ErrMeaningIncomplete
	}
	e.Fields = []MeaningField{}
	e.Resources = []MeaningResource{}
	if err := b.charge(e, len(e.DocumentID)+len(e.ConceptID)+len(e.Name)+len(e.Description)+len(e.Origin), comma); err != nil {
		return err
	}
	b.concepts++
	return nil
}
func (b *meaningCatalogBudget) field(f MeaningField, comma bool) error {
	if b.concepts >= maxMeaningCatalogConcepts {
		return ErrMeaningIncomplete
	}
	f.Resources = []MeaningResource{}
	if err := b.charge(f, len(f.DocumentID)+len(f.ConceptID)+len(f.Name)+len(f.Description), comma); err != nil {
		return err
	}
	b.concepts++
	return nil
}
func appendBoundedMeaningResource(current []MeaningResource, r MeaningResource) ([]MeaningResource, error) {
	for _, v := range current {
		if v == r {
			return current, nil
		}
	}
	if len(current) >= maxMeaningCatalogResources {
		return nil, ErrMeaningIncomplete
	}
	return append(current, r), nil
}
func (b *meaningCatalogBudget) resources(current *[]MeaningResource, incoming []MeaningResource) error {
	for _, r := range incoming {
		duplicate := false
		for _, v := range *current {
			if v == r {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		if len(*current) >= maxMeaningCatalogResources {
			return ErrMeaningIncomplete
		}
		if err := b.charge(r, len(r.Kind)+len(r.ID)+len(r.Title)+len(r.Environment), len(*current) > 0); err != nil {
			return err
		}
		*current = append(*current, r)
	}
	return nil
}
