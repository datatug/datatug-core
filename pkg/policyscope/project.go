package policyscope

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/dal-go/dalgo/access"
	"github.com/datatug/datatug-core/pkg/datatug"
	"gopkg.in/yaml.v3"
)

// ErrUnsupportedCodec is returned for a codec other than DALgo's YAML and
// JSON codecs: its documents cannot be inspected for selectors.
var ErrUnsupportedCodec = errors.New("policyscope: only YAML and JSON policy documents can be inspected for rule selectors")

// ErrNoRules is returned when no rule of the document's top-level scopes
// applies to the target. DALgo cannot decode a document without a scope, so
// the caller stands a policy that denies every read in its place.
var ErrNoRules = errors.New("policyscope: no rule of the policy applies to this instance, so it denies every read")

// Denies reports whether err from Project means the document denies every
// read of the target.
func Denies(err error) bool { return errors.Is(err, ErrTargetUnknown) || errors.Is(err, ErrNoRules) }

// Project returns the plain DALgo document data stands for when read for t.
// A document with no selector is returned unchanged, as the same slice.
// Otherwise every rule whose selectors do not all match t is removed, the
// selector keys are removed from the rules that stay, and a scope left with
// no rule or child scope by that removal is removed with it. A document that
// uses a selector returns ErrTargetUnknown when t's instance is not known,
// or when a rule names an environment and t's kind is not known, and
// ErrNoRules when the projection leaves it no top-level scope.
func Project(data []byte, codec access.Codec, t Target) ([]byte, error) {
	p, err := newProjection(data, codec, t)
	if err != nil {
		return nil, err
	}
	if !p.scoped {
		return data, nil
	}
	if t.Instance() == "" || p.environments && !t.kindKnown() {
		return nil, ErrTargetUnknown
	}
	if p.noRules {
		return nil, ErrNoRules
	}
	return p.encode()
}

// Scoped reports whether any rule of the document names a selector. It
// refuses a document whose selectors cannot be applied.
func Scoped(data []byte, codec access.Codec) (bool, error) {
	p, err := newProjection(data, codec, Target{})
	if err != nil {
		return false, err
	}
	return p.scoped, nil
}

// projection is one document walked for one target. Decisions taken for a
// target that is not known are discarded by Project.
type projection struct {
	target       Target
	scoped       bool
	environments bool
	noRules      bool
	encode       func() ([]byte, error)
}

func newProjection(data []byte, codec access.Codec, t Target) (*projection, error) {
	p := &projection{target: t}
	var err error
	switch codec.(type) {
	case access.YAMLCodec, *access.YAMLCodec:
		err = p.yaml(data)
	case access.JSONCodec, *access.JSONCodec:
		err = p.json(data)
	default:
		return nil, ErrUnsupportedCodec
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// keep records rule's selector and decides whether the rule stays.
func (p *projection) keep(s Selector) (bool, error) {
	if s.IsZero() {
		return true, nil
	}
	if err := s.Validate(); err != nil {
		return false, err
	}
	p.scoped = true
	p.environments = p.environments || s.Environment != ""
	match, err := s.Matches(p.target)
	return match && err == nil, nil
}

func duplicate(key string) error {
	return fmt.Errorf("%w: a rule names %s more than once", ErrInvalidSelector, key)
}

func notString(key string) error {
	return fmt.Errorf("%w: a rule's %s must be a non-empty string", ErrInvalidSelector, key)
}

// YAML: the document is edited as a node tree, so every other key, value,
// order and comment reaches DALgo exactly as written.

func (p *projection) yaml(data []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var root yaml.Node
	if err := decoder.Decode(&root); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // empty: DALgo refuses it as before
		}
		return fmt.Errorf("policyscope: decode YAML policy: %w", err)
	}
	if err := decoder.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		// A second document, or a broken one: DALgo refuses the stream, and
		// so does a projection, which could otherwise drop the second.
		return fmt.Errorf("policyscope: a policy stream must contain exactly one document")
	}
	document := root.Content[0]
	if document.Kind != yaml.MappingNode {
		return nil // not a policy document: DALgo refuses it as before
	}
	anchored := false
	walkYAML(&root, func(n *yaml.Node) { anchored = anchored || n.Anchor != "" || n.Kind == yaml.AliasNode })
	scopes := yamlValue(document, "scopes")
	before := yamlLen(scopes)
	if err := p.yamlScopes(scopes); err != nil {
		return err
	}
	p.noRules = before > 0 && yamlLen(scopes) == 0
	if sets := yamlValue(document, "ruleSets"); sets != nil && sets.Kind == yaml.MappingNode {
		for i := 1; i < len(sets.Content); i += 2 {
			if err := p.yamlScopes(sets.Content[i]); err != nil {
				return err
			}
		}
	}
	if p.scoped && anchored {
		return fmt.Errorf("%w: a document with rule selectors must not use YAML anchors or aliases", ErrInvalidSelector)
	}
	p.encode = func() ([]byte, error) {
		var out bytes.Buffer
		encoder := yaml.NewEncoder(&out)
		encoder.SetIndent(2)
		err := encoder.Encode(&root)
		return out.Bytes(), errors.Join(err, encoder.Close())
	}
	return nil
}

func walkYAML(n *yaml.Node, visit func(*yaml.Node)) {
	visit(n)
	for _, child := range n.Content {
		walkYAML(child, visit)
	}
}

func yamlValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// yamlScopes projects a scope list in place; a scope emptied by the
// projection is removed.
func (p *projection) yamlScopes(scopes *yaml.Node) error {
	if scopes == nil || scopes.Kind != yaml.SequenceNode {
		return nil
	}
	kept := scopes.Content[:0:0]
	for _, scope := range scopes.Content {
		emptied, err := p.yamlScope(scope)
		if err != nil {
			return err
		}
		if !emptied {
			kept = append(kept, scope)
		}
	}
	scopes.Content = kept
	return nil
}

func (p *projection) yamlScope(scope *yaml.Node) (emptied bool, err error) {
	if scope.Kind != yaml.MappingNode {
		return false, nil
	}
	rules, children := yamlValue(scope, "rules"), yamlValue(scope, "scopes")
	before := yamlLen(rules) + yamlLen(children)
	if rules != nil && rules.Kind == yaml.SequenceNode {
		kept := rules.Content[:0:0]
		for _, rule := range rules.Content {
			keep, err := p.yamlRule(rule)
			if err != nil {
				return false, err
			}
			if keep {
				kept = append(kept, rule)
			}
		}
		rules.Content = kept
	}
	if err := p.yamlScopes(children); err != nil {
		return false, err
	}
	return before > 0 && yamlLen(rules)+yamlLen(children) == 0, nil
}

func yamlLen(n *yaml.Node) int {
	if n == nil || n.Kind != yaml.SequenceNode {
		return 0
	}
	return len(n.Content)
}

func (p *projection) yamlRule(rule *yaml.Node) (bool, error) {
	if rule.Kind != yaml.MappingNode {
		return true, nil
	}
	var s Selector
	seen := map[string]bool{}
	kept := rule.Content[:0:0]
	for i := 0; i+1 < len(rule.Content); i += 2 {
		key, value := rule.Content[i], rule.Content[i+1]
		if key.Value != KeyEnvironment && key.Value != KeyInstance {
			kept = append(kept, key, value)
			continue
		}
		if seen[key.Value] {
			return false, duplicate(key.Value)
		}
		seen[key.Value] = true
		if value.Kind != yaml.ScalarNode || value.ShortTag() != "!!str" || value.Value == "" {
			return false, notString(key.Value)
		}
		if key.Value == KeyEnvironment {
			s.Environment = datatug.EnvironmentKind(value.Value)
		} else {
			s.Instance = value.Value
		}
	}
	rule.Content = kept
	return p.keep(s)
}

// JSON: numbers are kept as written (json.Number), so no value changes.

func (p *projection) json(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return fmt.Errorf("policyscope: decode JSON policy: %w", err)
	}
	if decoder.More() {
		return fmt.Errorf("policyscope: a policy stream must contain exactly one document")
	}
	document, ok := root.(map[string]any)
	if !ok {
		return nil // not a policy document: DALgo refuses it as before
	}
	before := jsonLen(document["scopes"])
	if err := p.jsonScopes(document, "scopes"); err != nil {
		return err
	}
	p.noRules = before > 0 && jsonLen(document["scopes"]) == 0
	if sets, ok := document["ruleSets"].(map[string]any); ok {
		for name := range sets {
			if err := p.jsonScopes(sets, name); err != nil {
				return err
			}
		}
	}
	p.encode = func() ([]byte, error) {
		var out bytes.Buffer
		encoder := json.NewEncoder(&out)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		err := encoder.Encode(root)
		return out.Bytes(), err
	}
	return nil
}

// jsonScopes projects parent[key], a scope list; a scope emptied by the
// projection is removed.
func (p *projection) jsonScopes(parent map[string]any, key string) error {
	scopes, ok := parent[key].([]any)
	if !ok {
		return nil
	}
	kept := []any{}
	for _, item := range scopes {
		emptied, err := p.jsonScope(item)
		if err != nil {
			return err
		}
		if !emptied {
			kept = append(kept, item)
		}
	}
	parent[key] = kept
	return nil
}

func (p *projection) jsonScope(item any) (emptied bool, err error) {
	scope, ok := item.(map[string]any)
	if !ok {
		return false, nil
	}
	before := jsonLen(scope["rules"]) + jsonLen(scope["scopes"])
	if rules, ok := scope["rules"].([]any); ok {
		kept := []any{}
		for _, rule := range rules {
			keep, err := p.jsonRule(rule)
			if err != nil {
				return false, err
			}
			if keep {
				kept = append(kept, rule)
			}
		}
		scope["rules"] = kept
	}
	if err := p.jsonScopes(scope, "scopes"); err != nil {
		return false, err
	}
	return before > 0 && jsonLen(scope["rules"])+jsonLen(scope["scopes"]) == 0, nil
}

func jsonLen(v any) int {
	list, _ := v.([]any)
	return len(list)
}

func (p *projection) jsonRule(item any) (bool, error) {
	rule, ok := item.(map[string]any)
	if !ok {
		return true, nil
	}
	var s Selector
	for _, key := range []string{KeyEnvironment, KeyInstance} {
		value, present := rule[key]
		if !present {
			continue
		}
		text, ok := value.(string)
		if !ok || text == "" {
			return false, notString(key)
		}
		if key == KeyEnvironment {
			s.Environment = datatug.EnvironmentKind(text)
		} else {
			s.Instance = text
		}
		delete(rule, key)
	}
	return p.keep(s)
}
