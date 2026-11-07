package reconcile

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func factoryCandidates() []Candidate {
	return []Candidate{
		{Environment: "factory-c-prod", Catalog: "factory", Family: "factory", Kind: datatug.EnvironmentKindProduction, Tenants: []string{"factory-c"}},
		{Environment: "factory-a-prod", Catalog: "factory", Family: "factory", Kind: datatug.EnvironmentKindProduction, Tenants: []string{"factory-a"}},
		{Environment: "factory-a-test", Catalog: "factory", Family: "factory", Kind: datatug.EnvironmentKindTest, Tenants: []string{"factory-a"}},
		{Environment: "factory-b-prod", Catalog: "factory", Family: "factory", Kind: datatug.EnvironmentKindProduction, Tenants: []string{"factory-b", "factory-b2"}},
		{Environment: "factory-a-prod", Catalog: "chinook", Family: "chinook", Kind: datatug.EnvironmentKindProduction},
	}
}

func environments(candidates []Candidate) []string {
	var ids []string
	for _, c := range candidates {
		ids = append(ids, c.Environment)
	}
	return ids
}

func TestSelectByDimensions(t *testing.T) {
	selected, err := InstanceSelector{Family: "factory", Kind: datatug.EnvironmentKindProduction}.Select(factoryCandidates())
	require.NoError(t, err)
	assert.Equal(t, []string{"factory-a-prod", "factory-b-prod", "factory-c-prod"}, environments(selected))

	selected, err = InstanceSelector{Family: "factory", Tenants: []string{"factory-a", "factory-b2"}, Kind: datatug.EnvironmentKindProduction}.Select(factoryCandidates())
	require.NoError(t, err)
	assert.Equal(t, []string{"factory-a-prod", "factory-b-prod"}, environments(selected))

	selected, err = InstanceSelector{Environments: []string{"factory-a-test", "factory-c-prod"}}.Select(factoryCandidates()[:3])
	require.NoError(t, err)
	assert.Equal(t, []string{"factory-a-test", "factory-c-prod"}, environments(selected))
}

func TestSelectErrors(t *testing.T) {
	_, err := InstanceSelector{}.Select(factoryCandidates())
	assert.ErrorIs(t, err, ErrInvalidRules)
	_, err = InstanceSelector{Family: "factory", Kind: "prod"}.Select(factoryCandidates())
	assert.ErrorIs(t, err, ErrInvalidRules)
	_, err = InstanceSelector{Environments: []string{"factory-a-prod", "factory-c-prod"}}.Select(factoryCandidates())
	assert.ErrorContains(t, err, "two matching catalogs")
	_, err = InstanceSelector{Family: "factory", Environments: []string{"nope", "factory-c-prod"}}.Select(factoryCandidates())
	assert.ErrorContains(t, err, `"nope" does not match`)
	_, err = InstanceSelector{Family: "chinook"}.Select(factoryCandidates())
	assert.ErrorContains(t, err, "matches 1 instance")
}

func TestCandidateInstance(t *testing.T) {
	candidates := factoryCandidates()
	assert.Equal(t, Instance{ID: "factory-c-prod", Environment: "production", Family: "factory", Tenant: "factory-c"}, candidates[0].Instance())
	assert.Equal(t, "[factory-b factory-b2]", candidates[3].Instance().Labels["tenants"])
	assert.Equal(t, "", candidates[4].Instance().Tenant)
	SortCandidates(candidates)
	assert.Equal(t, "chinook", candidates[0].Catalog)
	assert.Equal(t, "factory-a-test", candidates[2].Environment)
}

func TestProfileValidate(t *testing.T) {
	valid := Profile{ID: "reference-countries", Title: "Countries", QueryID: "countries", Instances: InstanceSelector{Family: "factory"}, Rules: Rules{Key: []string{"code"}}}
	assert.NoError(t, valid.Validate())
	for name, mutate := range map[string]func(*Profile){
		"bad id":    func(p *Profile) { p.ID = "Bad ID" },
		"no query":  func(p *Profile) { p.QueryID = "" },
		"no select": func(p *Profile) { p.Instances = InstanceSelector{} },
		"bad rules": func(p *Profile) { p.Rules = Rules{} },
	} {
		p := valid
		mutate(&p)
		assert.ErrorIs(t, p.Validate(), ErrInvalidRules, name)
	}
}
