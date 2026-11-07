package dataroles

import (
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/reconcile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var at = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

func byName(suggestions []Classification) map[string]Classification {
	out := map[string]Classification{}
	for _, s := range suggestions {
		out[s.Target.Collection] = s
	}
	return out
}

func TestSuggestRoles(t *testing.T) {
	collections := []Collection{
		{Name: "countries", Columns: []Column{{Name: "id", PrimaryKey: true}, {Name: "code", Unique: true}, {Name: "name"}}, ReferencedBy: 3},
		{Name: "currencies", Columns: []Column{{Name: "code", PrimaryKey: true}, {Name: "name"}}},
		{Name: "products", Columns: []Column{{Name: "id", PrimaryKey: true}, {Name: "sku", Unique: true}, {Name: "description"}, {Name: "local_price"}}, ReferencedBy: 2},
		{Name: "factory_settings", Columns: []Column{{Name: "setting_key", PrimaryKey: true}, {Name: "value"}}},
		{Name: "order_lines", Columns: []Column{{Name: "id", PrimaryKey: true}, {Name: "order_id"}, {Name: "created_at"}}, References: 2},
		{Name: "AuditEvents", Columns: []Column{{Name: "occurredAt"}}},
		{Name: "MediaType", Columns: []Column{{Name: "MediaTypeId", PrimaryKey: true}, {Name: "Name"}}, ReferencedBy: 1},
		{Name: "x_ref", Columns: []Column{{Name: "a"}, {Name: "b"}}, ReferencedBy: 1},
		{Name: "stock_moves", Columns: []Column{{Name: "moved_at"}}, References: 2},
		{Name: "plant", Columns: []Column{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"}}, ReferencedBy: 1},
		{Name: "misc", Columns: []Column{{Name: "a"}}},
		{Name: "sensors", Columns: []Column{{Name: "id", PrimaryKey: true}}},
		{Name: "app_log", Columns: []Column{{Name: "message"}}},
	}
	got := byName(Suggest("factory", collections, at))

	countries := got["countries"]
	assert.Equal(t, RoleReference, countries.Role)
	assert.Equal(t, ConfidenceLikely, countries.Confidence)
	assert.Equal(t, StateSuggested, countries.State)
	assert.Equal(t, OriginHeuristic, countries.Origin)
	require.NotNil(t, countries.Consistency)
	assert.Equal(t, []string{"code"}, countries.Consistency.Key)
	assert.Equal(t, reconcile.PresenceRequired, countries.Consistency.Presence)
	assert.Equal(t, reconcile.FieldIgnored, countries.Consistency.Fields["id"], "surrogate id is not compared")

	assert.Equal(t, ConfidencePossible, got["currencies"].Confidence)
	assert.Equal(t, []string{"code"}, got["currencies"].Consistency.Key)
	assert.Nil(t, got["currencies"].Consistency.Fields)

	products := got["products"]
	assert.Equal(t, RoleMaster, products.Role)
	assert.Equal(t, []string{"sku"}, products.Consistency.Key)
	assert.Equal(t, reconcile.PresenceAny, products.Consistency.Presence)

	settings := got["factory_settings"]
	assert.Equal(t, RoleConfiguration, settings.Role)
	assert.Equal(t, reconcile.FieldInstanceSpecific, settings.Consistency.DefaultField)

	assert.Equal(t, RoleTransactional, got["order_lines"].Role)
	assert.Equal(t, ConfidenceLikely, got["order_lines"].Confidence)
	assert.Nil(t, got["order_lines"].Consistency)
	assert.Equal(t, RoleEvent, got["AuditEvents"].Role)
	assert.Equal(t, ConfidenceLikely, got["AuditEvents"].Confidence)
	assert.Equal(t, RoleReference, got["MediaType"].Role)
	assert.Contains(t, got["MediaType"].Evidence[len(got["MediaType"].Evidence)-1], "no natural key")
	assert.Equal(t, RoleReference, got["x_ref"].Role, "small, referenced, references nothing")
	assert.Equal(t, RoleTransactional, got["stock_moves"].Role)
	assert.Equal(t, RoleMaster, got["plant"].Role)
	assert.Equal(t, RoleUnknown, got["misc"].Role)
	assert.Empty(t, got["misc"].Confidence)
	assert.Equal(t, RoleUnknown, got["sensors"].Role)
	assert.Equal(t, RoleEvent, got["app_log"].Role)
	assert.Equal(t, ConfidencePossible, got["app_log"].Confidence)
}

func TestDocumentLifecycle(t *testing.T) {
	doc := Document{Format: Format}
	suggestions := Suggest("factory", []Collection{
		{Name: "countries", Columns: []Column{{Name: "code", PrimaryKey: true}}},
		{Name: "orders", Columns: []Column{{Name: "id"}}},
	}, at)
	assert.Equal(t, 2, doc.Merge(suggestions))
	require.NoError(t, doc.Validate())
	target := Target{Family: "factory", Collection: "countries"}
	_, ok := doc.Effective(target)
	assert.False(t, ok, "suggestions never take effect")

	require.NoError(t, doc.Decide(target, StateConfirmed, "alex", at, "", nil, ""))
	effective, ok := doc.Effective(target)
	require.True(t, ok)
	assert.Equal(t, RoleReference, effective.Role)
	assert.Equal(t, OriginHeuristic, effective.Origin)
	assert.Equal(t, "alex", effective.By)

	// A rescan never overwrites a decision.
	assert.Equal(t, 0, doc.Merge(suggestions))
	effective, _ = doc.Effective(target)
	assert.Equal(t, StateConfirmed, effective.State)

	orders := Target{Family: "factory", Collection: "orders"}
	rules := &reconcile.Rules{Key: []string{"id"}}
	require.NoError(t, doc.Decide(orders, StateConfirmed, "alex", at, RoleMaster, rules, "shared catalogue"))
	effective, _ = doc.Effective(orders)
	assert.Equal(t, OriginUser, effective.Origin, "a changed role is the person's")
	assert.Equal(t, rules, effective.Consistency)
	require.NoError(t, doc.Decide(orders, StateDismissed, "alex", at, RoleMaster, nil, ""))
	_, ok = doc.Effective(orders)
	assert.False(t, ok)

	assert.Error(t, doc.Decide(orders, StateSuggested, "alex", at, "", nil, ""))
	assert.Error(t, doc.Decide(orders, StateConfirmed, "", at, "", nil, ""))
	assert.Error(t, doc.Decide(orders, StateConfirmed, "alex", at, "weird", nil, ""))
	assert.Error(t, doc.Decide(Target{Family: "x", Collection: "y"}, StateConfirmed, "alex", at, "", nil, ""))
}

func TestValidate(t *testing.T) {
	good := Classification{Target: Target{Family: "f", Collection: "c"}, Role: RoleReference, Origin: OriginUser, State: StateConfirmed, At: at}
	assert.NoError(t, Document{Format: Format, Classifications: []Classification{good}}.Validate())
	assert.Error(t, Document{Format: "x"}.Validate())
	assert.ErrorContains(t, Document{Format: Format, Classifications: []Classification{good, good}}.Validate(), "twice")
	for name, mutate := range map[string]func(*Classification){
		"target":     func(c *Classification) { c.Target.Family = "" },
		"role":       func(c *Classification) { c.Role = "x" },
		"origin":     func(c *Classification) { c.Origin = "x" },
		"state":      func(c *Classification) { c.State = "x" },
		"confidence": func(c *Classification) { c.Confidence = "certain" },
		"by":         func(c *Classification) { c.Origin = OriginAI },
	} {
		c := good
		mutate(&c)
		assert.Error(t, Document{Format: Format, Classifications: []Classification{c}}.Validate(), name)
	}
	assert.Equal(t, "f/c", good.Target.String())
}
