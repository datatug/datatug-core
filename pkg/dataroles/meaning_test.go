package dataroles

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/reconcile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromMeaning(t *testing.T) {
	countries := Target{Family: "factory", Collection: "countries"}
	c, err := FromMeaning(countries, MeaningDefault{Concept: "meaninggraph.org/geo#country", Role: RoleReference, Scope: "global", Expectation: ExpectIdentical, Key: []string{"code"}}, at)
	require.NoError(t, err)
	assert.Equal(t, OriginImported, c.Origin)
	assert.Equal(t, StateConfirmed, c.State)
	assert.Equal(t, "meaninggraph.org/geo#country", c.Version)
	require.NotNil(t, c.Consistency)
	assert.Equal(t, reconcile.PresenceRequired, c.Consistency.Presence)
	assert.Equal(t, reconcile.FieldIdentical, c.Consistency.DefaultField)
	assert.Contains(t, c.Evidence, "expected identical at global scope")
	assert.NoError(t, Document{Format: Format, Classifications: []Classification{c}}.Validate())

	products := Target{Family: "factory", Collection: "products"}
	c, err = FromMeaning(products, MeaningDefault{Concept: "c", Role: RoleMaster, Expectation: ExpectPartial, Key: []string{"sku"},
		Properties: map[string]Expectation{"price": ExpectInstanceSpecific, "notes": ExpectUnrestricted, "name": ExpectNormalized, "description": ExpectIdentical}}, at)
	require.NoError(t, err)
	assert.Equal(t, reconcile.PresenceAny, c.Consistency.Presence)
	assert.Equal(t, map[string]reconcile.FieldPolicy{"price": reconcile.FieldInstanceSpecific, "notes": reconcile.FieldIgnored, "name": reconcile.FieldNormalized, "description": reconcile.FieldIdentical}, c.Consistency.Fields)

	for expectation, want := range map[Expectation][2]string{
		ExpectNormalized:       {string(reconcile.PresenceRequired), string(reconcile.FieldNormalized)},
		ExpectInstanceSpecific: {string(reconcile.PresenceAny), string(reconcile.FieldInstanceSpecific)},
	} {
		c, err = FromMeaning(products, MeaningDefault{Concept: "c", Role: RoleReference, Expectation: expectation, Key: []string{"sku"}}, at)
		require.NoError(t, err)
		assert.Equal(t, want, [2]string{string(c.Consistency.Presence), string(c.Consistency.DefaultField)})
	}

	c, err = FromMeaning(products, MeaningDefault{Concept: "c", Role: RoleEvent, Expectation: ExpectUnrestricted}, at)
	require.NoError(t, err)
	assert.Nil(t, c.Consistency)
	c, err = FromMeaning(products, MeaningDefault{Concept: "c"}, at)
	require.NoError(t, err)
	assert.Equal(t, RoleUnknown, c.Role)

	c, err = FromMeaning(products, MeaningDefault{Concept: "c", Role: RoleMaster, Expectation: ExpectIdentical}, at)
	require.NoError(t, err)
	assert.Nil(t, c.Consistency, "no key, no rule")
	assert.Contains(t, c.Evidence[1], "no identifier binding")

	for name, d := range map[string]MeaningDefault{
		"bad role":          {Role: "weird"},
		"bad scope":         {Role: RoleMaster, Scope: "tenant"},
		"bad expectation":   {Role: RoleMaster, Expectation: "mostly", Key: []string{"sku"}},
		"partial property":  {Role: RoleMaster, Expectation: ExpectPartial, Key: []string{"sku"}, Properties: map[string]Expectation{"x": ExpectPartial}},
		"orphan properties": {Role: RoleMaster, Properties: map[string]Expectation{"x": ExpectIdentical}},
	} {
		_, err := FromMeaning(products, d, at)
		assert.Error(t, err, name)
	}
}

func TestResolvePrecedence(t *testing.T) {
	target := Target{Family: "factory", Collection: "countries"}
	meaning, err := FromMeaning(target, MeaningDefault{Concept: "geo#country", Role: RoleReference, Expectation: ExpectIdentical, Key: []string{"code"}}, at)
	require.NoError(t, err)

	doc := Document{Format: Format}
	got, ok := Resolve(doc, target, &meaning)
	require.True(t, ok)
	assert.Equal(t, OriginImported, got.Origin, "meaning default applies when the project has no decision")

	doc.Merge([]Classification{{Target: target, Role: RoleReference, Origin: OriginHeuristic, State: StateSuggested, At: at}})
	got, _ = Resolve(doc, target, &meaning)
	assert.Equal(t, OriginImported, got.Origin, "a suggestion never outranks the meaning default")

	require.NoError(t, doc.Decide(target, StateConfirmed, "alex", at, RoleConfiguration, nil, "regional list"))
	got, _ = Resolve(doc, target, &meaning)
	assert.Equal(t, RoleConfiguration, got.Role, "the project's confirmed decision wins")

	_, ok = Resolve(Document{Format: Format}, target, nil)
	assert.False(t, ok)
	other := meaning
	other.Target.Collection = "elsewhere"
	_, ok = Resolve(Document{Format: Format}, target, &other)
	assert.False(t, ok)
}
