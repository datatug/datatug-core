package policyscope

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/access"
	"github.com/datatug/datatug-core/pkg/datatug"
)

var (
	factoryA     = Target{Environment: "factory-a-prod", Catalog: "factory", Kind: datatug.EnvironmentKindProduction}
	factoryE     = Target{Environment: "factory-e-prod", Catalog: "factory", Kind: datatug.EnvironmentKindProduction}
	factoryATest = Target{Environment: "factory-a-test", Catalog: "factory", Kind: datatug.EnvironmentKindTest}
)

const factoriesYAML = `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata:
  name: factories
default: deny
scopes:
  # countries: everyone but factory E
  - path: /countries
    rules:
      - id: never-factory-e
        effect: deny
        operations: [query]
        instance: factory-e-prod/factory
      - id: read-countries
        effect: allow
        operations: [query]
  - path: /orders
    rules:
      - id: orders-outside-production
        effect: allow
        operations: [query]
      - id: no-production-orders
        effect: deny
        operations: [query]
        environment: production
  - path: /settings
    rules:
      - id: factory-a-settings
        effect: allow
        operations: [query]
        environment: production
        instance: factory-a-prod/factory
`

// decide decodes a projected document with DALgo's strict decoder and
// returns whether it allows a query of path.
func decide(t *testing.T, data []byte, codec access.Codec, path string) bool {
	t.Helper()
	policy, err := access.DecodePolicy(bytes.NewReader(data), codec)
	if err != nil {
		t.Fatalf("DALgo refuses the projected document: %v\n%s", err, data)
	}
	resource := access.CollectionResourceFor(nil, strings.TrimPrefix(path, "/"))
	return policy.Decide(context.Background(), access.Request{Operation: access.Query, Resources: []access.Resource{resource}}).Allowed
}

func project(t *testing.T, data string, codec access.Codec, target Target) []byte {
	t.Helper()
	out, err := Project([]byte(data), codec, target)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	return out
}

func TestProjectYAMLDecisions(t *testing.T) {
	yamlCodec := access.YAMLCodec{}
	cases := []struct {
		target Target
		path   string
		want   bool
	}{
		{factoryA, "/countries", true},
		{factoryE, "/countries", false},
		{factoryATest, "/countries", true},
		{factoryA, "/orders", false},
		{factoryATest, "/orders", true},
		{factoryA, "/settings", true},
		{factoryE, "/settings", false},
		{factoryATest, "/settings", false},
	}
	for _, c := range cases {
		out := project(t, factoriesYAML, yamlCodec, c.target)
		if got := decide(t, out, yamlCodec, c.path); got != c.want {
			t.Errorf("%s %s: allowed=%v, want %v\n%s", c.target.Instance(), c.path, got, c.want, out)
		}
	}
	out := project(t, factoriesYAML, &yamlCodec, factoryE)
	if !bytes.Contains(out, []byte("# countries: everyone but factory E")) {
		t.Errorf("comments must survive the projection:\n%s", out)
	}
	if bytes.Contains(out, []byte("instance:")) || bytes.Contains(out, []byte("environment:")) {
		t.Errorf("selector keys must be removed:\n%s", out)
	}
}

func TestProjectJSONDecisions(t *testing.T) {
	const doc = `{"apiVersion":"dalgo.io/access/v1","kind":"AccessPolicy","metadata":{"name":"factories"},"default":"deny",
"scopes":[{"path":"/countries","rules":[
 {"id":"never-factory-e","effect":"deny","operations":["query"],"instance":"factory-e-prod/factory"},
 {"id":"read-countries","effect":"allow","operations":["query"],"where":{"op":"<","left":{"field":"id"},"right":{"value":9007199254740993}}}]},
 {"path":"/orders","rules":[{"id":"prod","effect":"allow","operations":["query"],"environment":"production"}]}]}`
	jsonCodec := access.JSONCodec{}
	if decide(t, project(t, doc, jsonCodec, factoryE), jsonCodec, "/countries") {
		t.Error("factory E must not read countries")
	}
	out := project(t, doc, &jsonCodec, factoryA)
	if !bytes.Contains(out, []byte("9007199254740993")) {
		t.Errorf("numbers must be kept as written:\n%s", out)
	}
	if !decide(t, out, jsonCodec, "/orders") {
		t.Error("factory A (production) must read orders")
	}
	if decide(t, project(t, doc, jsonCodec, factoryATest), jsonCodec, "/orders") {
		t.Error("factory A test must not read orders")
	}
}

func TestSelectorFreeDocumentIsTheSameBytes(t *testing.T) {
	const plain = `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata: {name: plain}
default: deny
scopes:
  - path: /erp/customers
    rules: [{id: read, effect: allow, operations: [query]}]
`
	for _, target := range []Target{factoryA, {}} {
		out, err := Project([]byte(plain), access.YAMLCodec{}, target)
		if err != nil || &out[0] != &[]byte(plain)[0] && string(out) != plain {
			t.Fatalf("Project changed a selector-free document: %v\n%s", err, out)
		}
	}
	data := []byte(plain)
	out, _ := Project(data, access.YAMLCodec{}, Target{})
	if &out[0] != &data[0] {
		t.Error("a selector-free document must be returned as the same slice")
	}
	scoped, err := Scoped(data, access.YAMLCodec{})
	if err != nil || scoped {
		t.Errorf("Scoped = %v, %v; want false, nil", scoped, err)
	}
	scoped, err = Scoped([]byte(factoriesYAML), access.YAMLCodec{})
	if err != nil || !scoped {
		t.Errorf("Scoped = %v, %v; want true, nil", scoped, err)
	}
	if _, err := Scoped([]byte("{"), access.JSONCodec{}); err == nil {
		t.Error("Scoped must refuse a broken document")
	}
}

func TestUnknownTargetIsDenied(t *testing.T) {
	const onlyInstance = `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata: {name: i}
default: deny
scopes:
  - path: /countries
    rules: [{id: a, effect: allow, operations: [query], instance: factory-a-prod/factory}]
`
	for name, c := range map[string]struct {
		doc    string
		target Target
	}{
		"no instance":                 {factoriesYAML, Target{Kind: datatug.EnvironmentKindProduction}},
		"no catalog":                  {onlyInstance, Target{Environment: "factory-a-prod"}},
		"environment rule, no kind":   {factoriesYAML, Target{Environment: "factory-a-prod", Catalog: "factory"}},
		"environment rule, unknown":   {factoriesYAML, Target{Environment: "factory-a-prod", Catalog: "factory", Kind: datatug.EnvironmentKindUnknown}},
		"instance rule needs no kind": {onlyInstance, Target{Environment: "factory-a-prod", Catalog: "factory"}},
	} {
		_, err := Project([]byte(c.doc), access.YAMLCodec{}, c.target)
		want := name != "instance rule needs no kind"
		if got := errors.Is(err, ErrTargetUnknown); got != want {
			t.Errorf("%s: err = %v, want ErrTargetUnknown=%v", name, err, want)
		}
	}
}

func TestEmptiedScopesAndRuleSets(t *testing.T) {
	const doc = `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata: {name: sets}
default: deny
ruleSets:
  analysts:
    - path: /factory
      scopes:
        - path: /factory/countries
          rules: [{id: e-only, effect: allow, operations: [query], instance: factory-e-prod/factory}]
  readers:
    - path: /countries
      rules: [{id: read, effect: allow, operations: [query]}]
bindings:
  roles:
    analyst: [analysts]
    reader: [readers]
`
	out := project(t, doc, access.YAMLCodec{}, factoryA)
	if bytes.Contains(out, []byte("/factory/countries")) || bytes.Contains(out, []byte("path: /factory\n")) {
		t.Errorf("emptied scopes must be removed with their parents:\n%s", out)
	}
	if _, err := access.DecodePolicy(bytes.NewReader(out), access.YAMLCodec{}); err != nil {
		t.Fatalf("DALgo refuses an emptied rule set: %v\n%s", err, out)
	}
	const top = `{"apiVersion":"dalgo.io/access/v1","kind":"AccessPolicy","metadata":{"name":"t"},"default":"deny",
"scopes":[{"path":"/a","scopes":[{"path":"/a/b","rules":[{"id":"x","effect":"allow","operations":["query"],"instance":"factory-e-prod/factory"}]}]},
 {"path":"/kept","rules":[]}, "not-a-scope", {"path":"/raw","rules":["not-a-rule"]}],
"ruleSets":{"s":[{"path":"/x","rules":[{"id":"y","effect":"deny","operations":["query"],"instance":"factory-e-prod/factory"}]}]}}`
	out = project(t, top, access.JSONCodec{}, factoryA)
	if bytes.Contains(out, []byte(`"/a"`)) || !bytes.Contains(out, []byte(`"/kept"`)) || bytes.Contains(out, []byte(`"/x"`)) {
		t.Errorf("JSON scopes: want /a and /x removed and /kept (empty before) kept:\n%s", out)
	}
	const zero = `{"apiVersion":"dalgo.io/access/v1","kind":"AccessPolicy","metadata":{"name":"z"},"default":"deny",
"scopes":[{"path":"/a","rules":[{"id":"x","effect":"allow","operations":["query"],"instance":"factory-e-prod/factory"}]}]}`
	if _, err := Project([]byte(zero), access.JSONCodec{}, factoryA); !errors.Is(err, ErrNoRules) || !Denies(err) {
		t.Errorf("a document emptied by the projection must be ErrNoRules: %v", err)
	}
	if out := project(t, zero, access.JSONCodec{}, factoryE); !decide(t, out, access.JSONCodec{}, "/a") {
		t.Error("factory E keeps its rule")
	}
	yamlZero := "apiVersion: dalgo.io/access/v1\nkind: AccessPolicy\nmetadata: {name: z}\ndefault: deny\nscopes:\n  - path: /a\n    rules: [{id: x, effect: allow, operations: [query], instance: factory-e-prod/factory}]\n"
	if _, err := Project([]byte(yamlZero), access.YAMLCodec{}, factoryA); !errors.Is(err, ErrNoRules) {
		t.Errorf("YAML emptied document: %v", err)
	}
	if !Denies(ErrTargetUnknown) || Denies(ErrInvalidSelector) || Denies(nil) {
		t.Error("Denies")
	}
}

func TestOddShapesPassThrough(t *testing.T) {
	yamlOdd := `apiVersion: dalgo.io/access/v1
kind: AccessPolicy
metadata: {name: odd}
default: deny
scopes:
  - just-a-string
  - path: /a
    rules: [plain, {id: r, effect: allow, operations: [query], instance: factory-a-prod/factory, ? [complex] : key}]
  - path: /b
    rules: not-a-list
    scopes: not-a-list
ruleSets: not-a-mapping
`
	out := project(t, yamlOdd, access.YAMLCodec{}, factoryA)
	if !bytes.Contains(out, []byte("just-a-string")) || !bytes.Contains(out, []byte("not-a-list")) {
		t.Errorf("shapes DALgo refuses must reach it unchanged:\n%s", out)
	}
	for _, doc := range []string{"", "[1, 2]\n", "scopes: 3\n"} {
		if out, err := Project([]byte(doc), access.YAMLCodec{}, factoryA); err != nil || string(out) != doc {
			t.Errorf("YAML %q: got %q, %v", doc, out, err)
		}
	}
	for _, doc := range []string{"[1]", `{"scopes":3,"ruleSets":[]}`} {
		if out, err := Project([]byte(doc), access.JSONCodec{}, factoryA); err != nil || string(out) != doc {
			t.Errorf("JSON %q: got %q, %v", doc, out, err)
		}
	}
}

func TestProjectRefusals(t *testing.T) {
	rule := func(extra string) string {
		return "apiVersion: dalgo.io/access/v1\nkind: AccessPolicy\nmetadata: {name: r}\ndefault: deny\nscopes:\n  - path: /a\n    rules:\n      - {id: r, effect: allow, operations: [query], " + extra + "}\n"
	}
	jsonRule := func(extra string) string {
		return `{"scopes":[{"path":"/a","rules":[{"id":"r",` + extra + `}]}]}`
	}
	yamlCases := map[string]string{
		"unknown kind":       rule("environment: prod"),
		"kind unknown":       rule("environment: unknown"),
		"bare instance":      rule("instance: factory-e-prod"),
		"deep instance":      rule("instance: a/b/c"),
		"space in instance":  rule("instance: a /b"),
		"empty catalog":      rule("instance: 'a/'"),
		"not a string":       rule("instance: [a/b]"),
		"number":             rule("environment: 3"),
		"empty":              rule("environment: ''"),
		"duplicate":          rule("instance: a/b, instance: a/c"),
		"anchor":             rule("instance: a/b, operations: &ops [query]") + "  - path: /b\n    rules: [{id: s, effect: allow, operations: *ops}]\n",
		"alias selector":     rule("instance: &i a/b") + "  - path: /b\n    rules: [{id: s, effect: allow, operations: [query], instance: *i}]\n",
		"two documents":      rule("instance: a/b") + "---\nkind: x\n",
		"broken":             "scopes: [\n",
		"second broken":      rule("instance: a/b") + "---\n[\n",
		"rule set selectors": "ruleSets:\n  s:\n    - path: /a\n      rules: [{id: r, instance: nope}]\n",
		"nested selectors":   "scopes:\n  - path: /a\n    scopes:\n      - path: /a/b\n        rules: [{id: r, instance: nope}]\n",
	}
	for name, doc := range yamlCases {
		if _, err := Project([]byte(doc), access.YAMLCodec{}, factoryA); err == nil {
			t.Errorf("YAML %s: want a refusal", name)
		}
	}
	jsonCases := map[string]string{
		"not a string":  jsonRule(`"instance":["a/b"]`),
		"empty":         jsonRule(`"environment":""`),
		"unknown kind":  jsonRule(`"environment":"prod"`),
		"broken":        `{`,
		"two documents": `{} {}`,
		"rule set":      `{"ruleSets":{"s":[{"path":"/a","rules":[{"instance":"x"}]}]}}`,
		"nested":        `{"scopes":[{"path":"/a","scopes":[{"path":"/a/b","rules":[{"instance":"x"}]}]}]}`,
	}
	for name, doc := range jsonCases {
		if _, err := Project([]byte(doc), access.JSONCodec{}, factoryA); err == nil {
			t.Errorf("JSON %s: want a refusal", name)
		}
	}
	if _, err := Project([]byte("x"), nil, factoryA); !errors.Is(err, ErrUnsupportedCodec) {
		t.Errorf("unsupported codec: %v", err)
	}
	if _, err := Scoped([]byte("x"), nil); !errors.Is(err, ErrUnsupportedCodec) {
		t.Errorf("Scoped, unsupported codec: %v", err)
	}
}

func TestSelector(t *testing.T) {
	if (Target{Environment: "e"}).Instance() != "" || (Target{Catalog: "c"}).Instance() != "" || factoryA.Instance() != "factory-a-prod/factory" {
		t.Error("Target.Instance")
	}
	if !(Selector{}).IsZero() || (Selector{Instance: "a/b"}).IsZero() {
		t.Error("Selector.IsZero")
	}
	for _, s := range []Selector{{}, {Environment: datatug.EnvironmentKindProduction}, {Instance: "a/b"}} {
		if err := s.Validate(); err != nil {
			t.Errorf("Validate(%+v): %v", s, err)
		}
	}
	if err := (Selector{Environment: "prod"}).Validate(); !errors.Is(err, ErrInvalidSelector) || !strings.Contains(err.Error(), "production") || strings.Contains(err.Error(), "unknown,") {
		t.Errorf("Validate lists the kinds except unknown: %v", err)
	}
	match, err := (Selector{Environment: datatug.EnvironmentKindProduction, Instance: "factory-a-prod/factory"}).Matches(factoryA)
	if !match || err != nil {
		t.Errorf("Matches = %v, %v", match, err)
	}
	if match, _ := (Selector{Environment: datatug.EnvironmentKindTest}).Matches(factoryA); match {
		t.Error("kind mismatch must not match")
	}
	if _, err := (Selector{Instance: "a/b"}).Matches(Target{}); !errors.Is(err, ErrTargetUnknown) {
		t.Error("unknown instance must be ErrTargetUnknown")
	}
}

func TestUnscopedKeepsEveryRule(t *testing.T) {
	out, err := Unscoped([]byte(factoriesYAML), access.YAMLCodec{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"never-factory-e", "no-production-orders", "factory-a-settings"} {
		if !bytes.Contains(out, []byte(id)) {
			t.Errorf("Unscoped dropped %s:\n%s", id, out)
		}
	}
	if bytes.Contains(out, []byte("instance:")) {
		t.Errorf("Unscoped kept a selector key:\n%s", out)
	}
	if _, err := access.DecodePolicy(bytes.NewReader(out), access.YAMLCodec{}); err != nil {
		t.Errorf("DALgo refuses the unscoped view: %v", err)
	}
	plain := []byte(`{"scopes":[]}`)
	if out, err := Unscoped(plain, access.JSONCodec{}); err != nil || &out[0] != &plain[0] {
		t.Errorf("a selector-free document must be returned as is: %v", err)
	}
	if _, err := Unscoped([]byte("x"), nil); !errors.Is(err, ErrUnsupportedCodec) {
		t.Errorf("Unscoped, unsupported codec: %v", err)
	}
}

func TestMalformedKindIsUnknown(t *testing.T) {
	// An environment file is not checked when it is loaded: a kind outside
	// the closed set must not count as "not production".
	for _, kind := range []datatug.EnvironmentKind{"Production", "prod"} {
		target := Target{Environment: "factory-a-prod", Catalog: "factory", Kind: kind}
		if _, err := Project([]byte(factoriesYAML), access.YAMLCodec{}, target); !errors.Is(err, ErrTargetUnknown) {
			t.Errorf("kind %q: err = %v, want ErrTargetUnknown", kind, err)
		}
	}
}

func TestJSONKeysDifferingOnlyByCaseAreRefused(t *testing.T) {
	doc := `{"scopes":[{"path":"/a","rules":[{"id":"r","effect":"allow","Effect":"deny","operations":["query"],"instance":"factory-a-prod/factory"}]}],"list":["x",{"k":1},[2]],"n":null}`
	if _, err := Project([]byte(doc), access.JSONCodec{}, factoryA); !errors.Is(err, ErrInvalidSelector) || !strings.Contains(err.Error(), "differ only by case") {
		t.Errorf("err = %v", err)
	}
	ok := `{"scopes":[{"path":"/a","rules":[{"id":"r","effect":"allow","operations":["query"],"instance":"factory-a-prod/factory","where":{"left":{"field":"x"},"right":{"value":"Effect"}}}]}],"list":["effect",{"k":1},[2]]}`
	if _, err := Project([]byte(ok), access.JSONCodec{}, factoryA); err != nil {
		t.Errorf("a string value equal to a key is no duplicate: %v", err)
	}
	plain := []byte(`{"scopes":[{"path":"/a","rules":[{"id":"r","effect":"allow","Effect":"deny","operations":["query"]}]}]}`)
	if out, err := Project(plain, access.JSONCodec{}, factoryA); err != nil || string(out) != string(plain) {
		t.Errorf("a selector-free document is not checked or changed: %v", err)
	}
	if foldedDuplicate([]byte(`{"a":`)) != "" {
		t.Error("a truncated document has no duplicate")
	}
}
