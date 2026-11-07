package reconcile

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// ProfilesFolder is the project folder that holds reconciliation profiles,
// one `<id>.json` per profile.
const ProfilesFolder = "reconciliations"

var profileIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Profile is a saved, reviewable reconciliation: which dataset query, which
// instances, which rule. It is a plain project file.
type Profile struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description,omitempty"`
	QueryID     string           `json:"queryId"`
	Instances   InstanceSelector `json:"instances"`
	Rules       Rules            `json:"rules"`
}

// Validate checks the profile's shape and that its rules can be evaluated.
func (p Profile) Validate() error {
	if !profileIDPattern.MatchString(p.ID) {
		return fmt.Errorf("%w: profile id %q must be lower-case kebab", ErrInvalidRules, p.ID)
	}
	if p.QueryID == "" {
		return fmt.Errorf("%w: profile %q needs a queryId", ErrInvalidRules, p.ID)
	}
	if err := p.Instances.Validate(); err != nil {
		return err
	}
	if _, err := normalizeRules(p.Rules); err != nil {
		return err
	}
	return nil
}

// InstanceSelector chooses instances either by dimension (family, optional
// kind and tenants) or by an explicit list of environment ids. Both may be
// combined: an explicit list narrows the dimension match.
type InstanceSelector struct {
	Family       string                  `json:"family,omitempty"`
	Kind         datatug.EnvironmentKind `json:"kind,omitempty"`
	Tenants      []string                `json:"tenants,omitempty"`
	Environments []string                `json:"environments,omitempty"`
}

// Validate requires a family or an explicit environment list.
func (s InstanceSelector) Validate() error {
	if s.Family == "" && len(s.Environments) == 0 {
		return fmt.Errorf("%w: instances need a family or a list of environments", ErrInvalidRules)
	}
	if err := s.Kind.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRules, err)
	}
	return nil
}

// Candidate is one catalog in one environment, as the project records it.
type Candidate struct {
	Environment string                  `json:"environment"`
	Catalog     string                  `json:"catalog"`
	Family      string                  `json:"family"`
	Kind        datatug.EnvironmentKind `json:"kind,omitempty"`
	Tenants     []string                `json:"tenants,omitempty"`
}

// Instance returns the reconciliation identity of the candidate. The
// instance id is the environment id, which is what Perimeter policies and
// execution receipts name.
func (c Candidate) Instance() Instance {
	instance := Instance{ID: c.Environment, Environment: string(c.Kind), Family: c.Family}
	if len(c.Tenants) > 0 {
		instance.Tenant = c.Tenants[0]
		if len(c.Tenants) > 1 {
			instance.Labels = map[string]string{"tenants": fmt.Sprint(c.Tenants)}
		}
	}
	return instance
}

// Select returns the candidates matching the selector, one per environment,
// sorted by environment id. Two catalogs of the same family in one
// environment are ambiguous and refused: name the environment's catalog in
// the project instead of guessing.
func (s InstanceSelector) Select(candidates []Candidate) ([]Candidate, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	explicit := map[string]bool{}
	for _, id := range s.Environments {
		explicit[id] = true
	}
	tenants := map[string]bool{}
	for _, tenant := range s.Tenants {
		tenants[tenant] = true
	}
	byEnvironment := map[string]Candidate{}
	for _, candidate := range candidates {
		if s.Family != "" && candidate.Family != s.Family {
			continue
		}
		if s.Kind != "" && candidate.Kind != s.Kind {
			continue
		}
		if len(explicit) > 0 && !explicit[candidate.Environment] {
			continue
		}
		if len(tenants) > 0 && !anyTenant(candidate.Tenants, tenants) {
			continue
		}
		if prior, ok := byEnvironment[candidate.Environment]; ok {
			return nil, fmt.Errorf("reconcile: environment %q has two matching catalogs (%q, %q); select by family", candidate.Environment, prior.Catalog, candidate.Catalog)
		}
		byEnvironment[candidate.Environment] = candidate
	}
	for id := range explicit {
		if _, ok := byEnvironment[id]; !ok {
			return nil, fmt.Errorf("reconcile: environment %q does not match the selector", id)
		}
	}
	selected := make([]Candidate, 0, len(byEnvironment))
	for _, id := range sortedKeys(byEnvironment) {
		selected = append(selected, byEnvironment[id])
	}
	if len(selected) < 2 {
		return nil, fmt.Errorf("reconcile: the selector matches %d instance(s); at least two are needed", len(selected))
	}
	return selected, nil
}

func anyTenant(have []string, want map[string]bool) bool {
	for _, tenant := range have {
		if want[tenant] {
			return true
		}
	}
	return false
}

// SortCandidates orders candidates by environment then catalog.
func SortCandidates(candidates []Candidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Environment != candidates[j].Environment {
			return candidates[i].Environment < candidates[j].Environment
		}
		return candidates[i].Catalog < candidates[j].Catalog
	})
}
