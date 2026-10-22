package datatug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/datatug/datatug-core/internal/jsonstrict"
	"gopkg.in/yaml.v3"
)

// QueryExpectedServerIdentity pins descriptive identity, never source authority.
// Adapters must independently match their admitted origin and returned server.
type QueryExpectedServerIdentity struct {
	BaseURL  string `json:"baseUrl" yaml:"baseUrl"`
	ServerID string `json:"serverId" yaml:"serverId"`
}

// QueryRelationshipBinding pins a revision of a source-admitted relationship.
// It is not a foreign-key registry or a grant to execute a join.
type QueryRelationshipBinding struct {
	ID      string                       `json:"id" yaml:"id"`
	Version string                       `json:"version" yaml:"version"`
	From    QueryRelationshipEndpoint    `json:"from" yaml:"from"`
	To      QueryRelationshipEndpoint    `json:"to" yaml:"to"`
	Pairs   []QueryRelationshipFieldPair `json:"pairs" yaml:"pairs"`
}
type QueryRelationshipEndpoint struct {
	Schema string `json:"schema" yaml:"schema"`
	Table  string `json:"table" yaml:"table"`
}
type QueryRelationshipFieldPair struct {
	FromField string `json:"fromField" yaml:"fromField"`
	ToField   string `json:"toField" yaml:"toField"`
}
type QueryRelationshipBindings []QueryRelationshipBinding

func queryOpaqueSourceValue(value string, asciiControls bool) error {
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || len(value) > 4096 {
		return fmt.Errorf("source pin must be nonblank valid UTF-8 within4096 bytes")
	}
	for _, r := range value {
		if r < 32 || r == 127 || !asciiControls && unicode.IsControl(r) {
			return fmt.Errorf("source pin contains controls")
		}
	}
	if reason, bad := EmbeddedCredentialReason(value); bad {
		return fmt.Errorf("source pin: %s", reason)
	}
	return nil
}
func (v QueryExpectedServerIdentity) Validate() error {
	if err := queryOpaqueSourceValue(v.ServerID, true); err != nil {
		return err
	}
	if err := queryOpaqueSourceValue(v.BaseURL, true); err != nil {
		return err
	}
	u, err := url.Parse(v.BaseURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(v.BaseURL, "#") || (u.Scheme != "https" && (u.Scheme != "http" || u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1")) {
		return fmt.Errorf("expected server base must be HTTPS or loopback HTTP without credentials/query/fragment")
	}
	return nil
}
func (v QueryRelationshipBinding) Validate() error {
	if v.ID == "" || v.From.Table == "" || v.To.Table == "" || len(v.Pairs) == 0 {
		return fmt.Errorf("relationship id/endpoints/pairs are required")
	}
	if err := queryOpaqueSourceValue(v.Version, false); err != nil {
		return err
	}
	for _, value := range []string{v.ID, v.From.Schema, v.From.Table, v.To.Schema, v.To.Table} {
		if reason, bad := identifierEnvelopeReason(value); bad {
			return fmt.Errorf("relationship identifier: %s", reason)
		}
		if reason, bad := EmbeddedCredentialReason(value); bad {
			return fmt.Errorf("relationship identifier: %s", reason)
		}
	}
	from, to := map[string]bool{}, map[string]bool{}
	for _, pair := range v.Pairs {
		if pair.FromField == "" || pair.ToField == "" || from[pair.FromField] || to[pair.ToField] {
			return fmt.Errorf("relationship field pairs must be nonempty and unambiguous")
		}
		for _, value := range []string{pair.FromField, pair.ToField} {
			if reason, bad := identifierEnvelopeReason(value); bad {
				return fmt.Errorf("relationship field: %s", reason)
			}
			if reason, bad := EmbeddedCredentialReason(value); bad {
				return fmt.Errorf("relationship field: %s", reason)
			}
		}
		from[pair.FromField], to[pair.ToField] = true, true
	}
	return nil
}
func validateQuerySourceBindings(q QueryDef) error {
	if q.Federation != nil && q.Federation.ExpectedServerIdentity != nil {
		if err := q.Federation.ExpectedServerIdentity.Validate(); err != nil {
			return fmt.Errorf("federation.expectedServerIdentity: %w", err)
		}
	}
	seen := map[string]bool{}
	for _, binding := range q.RelationshipBindings {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("relationshipBindings: %w", err)
		}
		key, _ := json.Marshal(struct {
			ID       string
			From, To QueryRelationshipEndpoint
		}{binding.ID, binding.From, binding.To})
		if seen[string(key)] {
			return fmt.Errorf("duplicate relationship binding")
		}
		seen[string(key)] = true
	}
	return nil
}
func (s *queryStorageScreen) relationshipBindings(bindings QueryRelationshipBindings) {
	for i, v := range bindings {
		at := fmt.Sprintf("relationshipBindings[%d]", i)
		s.metadataIdentifier(at+".id", v.ID)
		s.prose(at+".version", v.Version)
		s.metadataIdentifier(at+".from.schema", v.From.Schema)
		s.metadataIdentifier(at+".from.table", v.From.Table)
		s.metadataIdentifier(at+".to.schema", v.To.Schema)
		s.metadataIdentifier(at+".to.table", v.To.Table)
		for j, pair := range v.Pairs {
			s.metadataIdentifier(fmt.Sprintf("%s.pairs[%d].fromField", at, j), pair.FromField)
			s.metadataIdentifier(fmt.Sprintf("%s.pairs[%d].toField", at, j), pair.ToField)
		}
	}
}
func checkOptionalSourceMembers(fields map[string]any, name string) error {
	for key, value := range fields {
		if strings.EqualFold(key, name) && (key != name || value == nil) {
			return fmt.Errorf("%s must be canonical and nonnull", name)
		}
	}
	return nil
}
func checkOptionalSourceJSON(data []byte, name string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key, value := range fields {
		if strings.EqualFold(key, name) && (key != name || bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
			return fmt.Errorf("%s must be canonical and nonnull", name)
		}
	}
	return nil
}

// Only inspect resolved shape here; decode original typed nodes below so 001,
// date-looking strings and aliases retain their original scalar/anchor identity.
func decodeClosedSourceYAML(node *yaml.Node, target any) error {
	var fields map[string]any
	if err := node.Decode(&fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("source binding must be a mapping")
	}
	typ := reflect.TypeOf(target).Elem()
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		value, ok := fields[name]
		if !ok || value == nil {
			return fmt.Errorf("missing/null source binding field %s", name)
		}
		if values, ok := value.([]any); ok {
			for _, item := range values {
				if item == nil {
					return fmt.Errorf("null source binding array member")
				}
			}
		}
		delete(fields, name)
	}
	if len(fields) != 0 {
		return fmt.Errorf("unknown/noncanonical source binding field")
	}
	return decodeKnownQueryYAML(node, target)
}
func (v *QueryExpectedServerIdentity) UnmarshalJSON(data []byte) error {
	type wire QueryExpectedServerIdentity
	var w wire
	if err := decodeClosedMapping(data, &w); err != nil {
		return err
	}
	*v = QueryExpectedServerIdentity(w)
	return nil
}
func (v *QueryRelationshipBinding) UnmarshalJSON(data []byte) error {
	type wire QueryRelationshipBinding
	var w wire
	if err := decodeClosedMapping(data, &w); err != nil {
		return err
	}
	*v = QueryRelationshipBinding(w)
	return nil
}
func (v *QueryRelationshipEndpoint) UnmarshalJSON(data []byte) error {
	type wire QueryRelationshipEndpoint
	var w wire
	if err := decodeClosedMapping(data, &w); err != nil {
		return err
	}
	*v = QueryRelationshipEndpoint(w)
	return nil
}
func (v *QueryRelationshipFieldPair) UnmarshalJSON(data []byte) error {
	type wire QueryRelationshipFieldPair
	var w wire
	if err := decodeClosedMapping(data, &w); err != nil {
		return err
	}
	*v = QueryRelationshipFieldPair(w)
	return nil
}
func (v *QueryExpectedServerIdentity) UnmarshalYAML(n *yaml.Node) error {
	type wire QueryExpectedServerIdentity
	var w wire
	if err := decodeClosedSourceYAML(n, &w); err != nil {
		return err
	}
	*v = QueryExpectedServerIdentity(w)
	return nil
}
func (v *QueryRelationshipBinding) UnmarshalYAML(n *yaml.Node) error {
	type wire QueryRelationshipBinding
	var w wire
	if err := decodeClosedSourceYAML(n, &w); err != nil {
		return err
	}
	*v = QueryRelationshipBinding(w)
	return nil
}
func (v *QueryRelationshipEndpoint) UnmarshalYAML(n *yaml.Node) error {
	type wire QueryRelationshipEndpoint
	var w wire
	if err := decodeClosedSourceYAML(n, &w); err != nil {
		return err
	}
	*v = QueryRelationshipEndpoint(w)
	return nil
}
func (v *QueryRelationshipFieldPair) UnmarshalYAML(n *yaml.Node) error {
	type wire QueryRelationshipFieldPair
	var w wire
	if err := decodeClosedSourceYAML(n, &w); err != nil {
		return err
	}
	*v = QueryRelationshipFieldPair(w)
	return nil
}
func (v *QueryRelationshipBindings) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return fmt.Errorf("null relationship bindings")
	}
	type wire QueryRelationshipBindings
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	*v = QueryRelationshipBindings(w)
	return nil
}
func (v *QueryRelationshipBindings) UnmarshalYAML(n *yaml.Node) error {
	var values []any
	if err := n.Decode(&values); err != nil {
		return err
	}
	if values == nil {
		return fmt.Errorf("null relationship bindings")
	}
	for _, value := range values {
		if value == nil {
			return fmt.Errorf("null relationship binding")
		}
	}
	type wire QueryRelationshipBindings
	var w wire
	if err := decodeKnownQueryYAML(n, &w); err != nil {
		return err
	}
	*v = QueryRelationshipBindings(w)
	return nil
}
func (v *QueryFederation) UnmarshalJSON(data []byte) error {
	type wire QueryFederation
	if err := checkOptionalSourceJSON(data, "expectedServerIdentity"); err != nil {
		return err
	}
	if err := jsonstrict.CheckNoDuplicateKeysForSchema(data, reflect.TypeOf(wire{})); err != nil {
		return err
	}
	var w wire
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err := d.Decode(&w); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing federation JSON")
	}
	*v = QueryFederation(w)
	return nil
}
func (v *QueryFederation) UnmarshalYAML(n *yaml.Node) error {
	var fields map[string]any
	if err := n.Decode(&fields); err != nil {
		return err
	}
	if err := checkOptionalSourceMembers(fields, "expectedServerIdentity"); err != nil {
		return err
	}
	type wire QueryFederation
	var w wire
	if err := decodeKnownQueryYAML(n, &w); err != nil {
		return err
	}
	*v = QueryFederation(w)
	return nil
}
