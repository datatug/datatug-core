package detective

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/datatug/datatug-core/pkg/investigation"
)

var ErrConflict = errors.New("definition or case changed; reload before saving")
var ErrNotFound = errors.New("detective item not found")
var ErrInvalid = errors.New("invalid detective item")
var ErrCapacity = errors.New("Detective catalog capacity reached")
var ErrOutcomeUncertain = errors.New("mutation may have been saved but audit failed; reload before retrying")

// Catalog is the bounded, transitive definition graph for one evaluation. It
// contains no Cases or Runs, and can be loaded through direct repository reads.
type Catalog struct {
	Checks          map[string]Check
	Validators      map[string]PublishedValidator
	QueryValidators map[string]QueryValidators
	verified        bool
	signature       [32]byte
}

// CatalogReader is implemented by local files or a hosted repository. A query
// without local Validators returns (QueryValidators{}, false, nil).
type CatalogReader interface {
	GetCheck(context.Context, string) (Check, error)
	GetPublishedValidator(context.Context, string) (PublishedValidator, error)
	GetQueryValidators(context.Context, string) (QueryValidators, bool, error)
	QueryExists(context.Context, string) (bool, error)
}

// LoadCatalog resolves all transitive Check, query-local and promoted Validator
// dependencies before a query can execute. Every saved-query reference is
// checked by the adapter. The bounded direct reads avoid a whole-project blob.
func LoadCatalog(ctx context.Context, rootCheckID, rootQueryID string, reader CatalogReader) (Catalog, error) {
	if reader == nil || (rootCheckID == "") == (rootQueryID == "") {
		return Catalog{}, ErrInvalid
	}
	catalog := Catalog{Checks: map[string]Check{}, Validators: map[string]PublishedValidator{}, QueryValidators: map[string]QueryValidators{}}
	seenQueries := map[string]bool{}
	visiting := map[string]bool{}
	var visitCheck func(string, int) error
	var visitQuery func(string, []Validator, int) error
	var visitValidators func([]Validator, int) error
	visitValidators = func(items []Validator, depth int) error {
		for _, original := range items {
			v := original
			if v.ValidatorID != "" {
				if _, ok := catalog.Validators[v.ValidatorID]; !ok {
					item, err := reader.GetPublishedValidator(ctx, v.ValidatorID)
					if err != nil {
						return err
					}
					if item.ID != v.ValidatorID || validateValidator(item.Validator) != nil || item.Validator.ValidatorID != "" {
						return ErrInvalid
					}
					owned, err := cloneValue(item)
					if err != nil {
						return err
					}
					catalog.Validators[item.ID] = owned
				}
				v = catalog.Validators[v.ValidatorID].Validator
			}
			if v.Kind == "check" {
				if err := visitCheck(v.CheckID, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	visitQuery = func(id string, inline []Validator, depth int) error {
		if !validQueryRef(id) {
			return ErrInvalid
		}
		if err := visitValidators(inline, depth); err != nil {
			return err
		}
		if seenQueries[id] {
			return nil
		}
		if len(seenQueries) >= maxNodes {
			return ErrCapacity
		}
		seenQueries[id] = true
		exists, err := reader.QueryExists(ctx, id)
		if err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
		local, found, err := reader.GetQueryValidators(ctx, id)
		if err != nil {
			return err
		}
		if found {
			if local.QueryID != id || ValidateQueryValidators(local) != nil {
				return ErrInvalid
			}
			owned, err := cloneValue(local)
			if err != nil {
				return err
			}
			catalog.QueryValidators[id] = owned
			if err := visitValidators(owned.Validators, depth); err != nil {
				return err
			}
		}
		return nil
	}
	visitCheck = func(id string, depth int) error {
		if depth > maxDepth || visiting[id] {
			return fmt.Errorf("%w: check dependency cycle or depth limit", ErrInvalid)
		}
		if _, ok := catalog.Checks[id]; ok {
			return nil
		}
		if len(catalog.Checks) >= maxNodes || !validSlug(id) {
			return ErrCapacity
		}
		check, err := reader.GetCheck(ctx, id)
		if err != nil {
			return err
		}
		if check.ID != id || validateCheck(check) != nil {
			return ErrInvalid
		}
		owned, err := cloneValue(check)
		if err != nil {
			return err
		}
		catalog.Checks[id] = owned
		check = owned
		visiting[id] = true
		defer delete(visiting, id)
		for _, step := range check.Steps {
			if err := ctx.Err(); err != nil {
				return err
			}
			if step.Kind == "check" {
				if err := visitCheck(step.CheckID, depth+1); err != nil {
					return err
				}
			} else if err := visitQuery(step.QueryID, step.Validators, depth); err != nil {
				return err
			}
		}
		return nil
	}
	if rootCheckID != "" {
		if err := visitCheck(rootCheckID, 0); err != nil {
			return Catalog{}, err
		}
	} else if err := visitQuery(rootQueryID, nil, 0); err != nil {
		return Catalog{}, err
	}
	if err := validateGraph(catalog.Checks, catalog.Validators, catalog.QueryValidators); err != nil {
		return Catalog{}, err
	}
	catalog.verified = true
	catalog.signature = catalogSignature(catalog)
	return catalog, nil
}

func catalogSignature(c Catalog) [32]byte {
	data, _ := json.Marshal(struct {
		Checks          map[string]Check
		Validators      map[string]PublishedValidator
		QueryValidators map[string]QueryValidators
	}{c.Checks, c.Validators, c.QueryValidators})
	return sha256.Sum256(data)
}

func (c Catalog) sealed() bool { return c.verified && c.signature == catalogSignature(c) }

func ValidateCheck(check Check) error     { return validateCheck(check) }
func ValidateValidator(v Validator) error { return validateValidator(v) }
func ValidateCatalog(c Catalog) error {
	return validateGraph(c.Checks, c.Validators, c.QueryValidators)
}
func ValidateInputs(schema []Input, values map[string]InputValue) error {
	return validateInputs(schema, values)
}
func ValidSlug(id string) bool                      { return validSlug(id) }
func ValidParameterName(name string) bool           { return validParameterName(name) }
func ValidQueryRef(id string) bool                  { return validQueryRef(id) }
func ValidInputType(t investigation.ValueType) bool { return validInputType(t) }
func ValidTransition(old, next, reason string) bool { return validTransition(old, next, reason) }

func validSlug(id string) bool {
	if len(id) == 0 || len(id) > 128 || id[0] < 'a' || id[0] > 'z' {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-') {
			return false
		}
	}
	return true
}

func validQueryRef(id string) bool {
	if id == "" || len(id) > 512 {
		return false
	}
	for _, segment := range strings.Split(id, "/") {
		if segment == "" || segment == "." || segment == ".." || len(segment) > 128 {
			return false
		}
		for _, r := range segment {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
				return false
			}
		}
	}
	return true
}
