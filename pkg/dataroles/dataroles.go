// Package dataroles classifies a database family's collections by data role
// (reference, master, configuration, transactional, event) and suggests a
// default consistency rule for each, with provenance and review state.
//
// Classification is separate from consistency: a role describes what a
// dataset is, a reconcile.Rules describes where it must agree. Suggestions
// never take effect until a person confirms them. Suggest reads schema only,
// never record values.
package dataroles

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/reconcile"
)

// Format identifies the document stored as FileName in a project.
const Format = "datatug-data-roles/v1"

// FileName is the project file beside datatug-labels.json.
const FileName = "datatug-data-roles.json"

// HeuristicVersion names the deterministic suggester recorded in provenance.
const HeuristicVersion = "dataroles-heuristic/1"

// Role is a dataset's semantic data role.
type Role string

const (
	RoleReference     Role = "reference"
	RoleMaster        Role = "master"
	RoleConfiguration Role = "configuration"
	RoleTransactional Role = "transactional"
	RoleEvent         Role = "event"
	RoleUnknown       Role = "unknown"
)

// Roles lists every role.
var Roles = []Role{RoleReference, RoleMaster, RoleConfiguration, RoleTransactional, RoleEvent, RoleUnknown}

// Origin records where a classification came from.
type Origin string

const (
	OriginUser      Origin = "user"
	OriginImported  Origin = "imported" // from an authoritative specification, e.g. Meaning Graph
	OriginObserved  Origin = "observed" // from permitted data inspection
	OriginAI        Origin = "ai"
	OriginHeuristic Origin = "heuristic"
)

// State is the review state.
type State string

const (
	StateSuggested State = "suggested"
	StateConfirmed State = "confirmed"
	StateDismissed State = "dismissed"
)

// Confidence is a word tied to evidence, never a probability.
type Confidence string

const (
	ConfidenceLikely   Confidence = "likely"
	ConfidencePossible Confidence = "possible"
)

// Target names one collection of one database family (DbModel).
type Target struct {
	Family     string `json:"family"`
	Collection string `json:"collection"`
}

func (t Target) String() string { return t.Family + "/" + t.Collection }

// Classification is one role assignment with provenance.
type Classification struct {
	Target      Target           `json:"target"`
	Role        Role             `json:"role"`
	Consistency *reconcile.Rules `json:"consistency,omitempty"`
	Origin      Origin           `json:"origin"`
	Version     string           `json:"version,omitempty"` // suggester or model version
	State       State            `json:"state"`
	Confidence  Confidence       `json:"confidence,omitempty"`
	Evidence    []string         `json:"evidence,omitempty"`
	By          string           `json:"by,omitempty"`
	At          time.Time        `json:"at"`
	Reason      string           `json:"reason,omitempty"`
}

// Document is the stored file.
type Document struct {
	Format          string           `json:"format"`
	Classifications []Classification `json:"classifications"`
}

// Validate checks every entry and that each target has at most one entry.
func (d Document) Validate() error {
	if d.Format != Format {
		return fmt.Errorf("dataroles: format must be %q", Format)
	}
	seen := map[Target]bool{}
	for i, c := range d.Classifications {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("dataroles: classification %d: %w", i, err)
		}
		if seen[c.Target] {
			return fmt.Errorf("dataroles: %s is classified twice", c.Target)
		}
		seen[c.Target] = true
	}
	return nil
}

// Validate checks one entry.
func (c Classification) Validate() error {
	if c.Target.Family == "" || c.Target.Collection == "" {
		return fmt.Errorf("target needs family and collection")
	}
	if !oneOf(c.Role, Roles) {
		return fmt.Errorf("unknown role %q", c.Role)
	}
	if !oneOf(c.Origin, []Origin{OriginUser, OriginImported, OriginObserved, OriginAI, OriginHeuristic}) {
		return fmt.Errorf("unknown origin %q", c.Origin)
	}
	if !oneOf(c.State, []State{StateSuggested, StateConfirmed, StateDismissed}) {
		return fmt.Errorf("unknown state %q", c.State)
	}
	if c.Confidence != "" && c.Confidence != ConfidenceLikely && c.Confidence != ConfidencePossible {
		return fmt.Errorf("unknown confidence %q", c.Confidence)
	}
	if c.State == StateConfirmed && c.Origin != OriginUser && c.Origin != OriginImported && c.By == "" {
		return fmt.Errorf("a confirmed %s suggestion must name who confirmed it", c.Origin)
	}
	return nil
}

func oneOf[T comparable](v T, set []T) bool {
	for _, s := range set {
		if v == s {
			return true
		}
	}
	return false
}

// Effective returns the confirmed classification for a target, if any.
// Suggested and dismissed entries never take effect.
func (d Document) Effective(target Target) (Classification, bool) {
	for _, c := range d.Classifications {
		if c.Target == target && c.State == StateConfirmed {
			return c, true
		}
	}
	return Classification{}, false
}

// Merge adds suggestions for targets the document does not classify yet.
// Existing entries — confirmed, dismissed or suggested — are never replaced,
// so a person's decision survives every rescan.
func (d *Document) Merge(suggestions []Classification) (added int) {
	have := map[Target]bool{}
	for _, c := range d.Classifications {
		have[c.Target] = true
	}
	for _, s := range suggestions {
		if have[s.Target] {
			continue
		}
		d.Classifications = append(d.Classifications, s)
		have[s.Target] = true
		added++
	}
	sort.SliceStable(d.Classifications, func(i, j int) bool {
		return d.Classifications[i].Target.String() < d.Classifications[j].Target.String()
	})
	return added
}

// Decide confirms or dismisses an entry on a person's behalf. Confirming may
// replace the role or the consistency rule the person reviewed.
func (d *Document) Decide(target Target, state State, by string, at time.Time, role Role, consistency *reconcile.Rules, reason string) error {
	if state != StateConfirmed && state != StateDismissed {
		return fmt.Errorf("dataroles: a decision confirms or dismisses")
	}
	if by == "" {
		return fmt.Errorf("dataroles: a decision names who made it")
	}
	for i := range d.Classifications {
		c := &d.Classifications[i]
		if c.Target != target {
			continue
		}
		if role != "" {
			if !oneOf(role, Roles) {
				return fmt.Errorf("dataroles: unknown role %q", role)
			}
			if role != c.Role {
				c.Origin = OriginUser
			}
			c.Role = role
		}
		if consistency != nil {
			c.Consistency = consistency
		}
		c.State, c.By, c.At, c.Reason = state, by, at, reason
		return nil
	}
	return fmt.Errorf("dataroles: %s is not classified", target)
}

// Column describes a column for schema-only classification.
type Column struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	PrimaryKey bool   `json:"primaryKey,omitempty"`
	Unique     bool   `json:"unique,omitempty"`
}

// Collection is the schema-only shape of one collection. ReferencedBy counts
// foreign keys from other collections pointing at it; References counts its
// own foreign keys.
type Collection struct {
	Name         string   `json:"name"`
	Columns      []Column `json:"columns"`
	ReferencedBy int      `json:"referencedBy"`
	References   int      `json:"references"`
}

type pattern struct {
	role Role
	re   *regexp.Regexp
}

// Name patterns match whole words of a snake/camel/kebab-split name.
var patterns = []pattern{
	{RoleEvent, regexp.MustCompile(`^(audit|log|logs|event|events|history|histories|activity|activities|journal|trace|traces)$`)},
	{RoleConfiguration, regexp.MustCompile(`^(config|configs|configuration|configurations|setting|settings|parameter|parameters|option|options|preference|preferences|flag|flags|feature)$`)},
	{RoleReference, regexp.MustCompile(`^(country|countries|currency|currencies|unit|units|uom|language|languages|locale|locales|region|regions|timezone|timezones|status|statuses|type|types|category|categories|genre|genres|code|codes|lookup|lookups|classification|classifications)$`)},
	{RoleTransactional, regexp.MustCompile(`^(order|orders|invoice|invoices|payment|payments|shipment|shipments|transaction|transactions|movement|movements|line|lines|item|items|run|runs|receipt|receipts|booking|bookings|sale|sales|transfer|transfers)$`)},
	{RoleMaster, regexp.MustCompile(`^(product|products|customer|customers|supplier|suppliers|vendor|vendors|employee|employees|equipment|machine|machines|account|accounts|artist|artists|album|albums|track|tracks|partner|partners|site|sites|warehouse|warehouses|material|materials)$`)},
}

var wordSplit = regexp.MustCompile(`[A-Z]?[a-z0-9]+|[A-Z]+`)

func words(name string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == '.' || r == ' ' }) {
		for _, w := range wordSplit.FindAllString(part, -1) {
			out = append(out, strings.ToLower(w))
		}
	}
	return out
}

var temporal = regexp.MustCompile(`(^|_)(date|time|at|timestamp|created|occurred)($|_)|(Date|At|Time)$`)

// Suggest classifies collections of one family from schema alone and
// returns suggestions with evidence. It reads no record values.
func Suggest(family string, collections []Collection, at time.Time) []Classification {
	out := make([]Classification, 0, len(collections))
	for _, collection := range collections {
		out = append(out, suggestOne(family, collection, at))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Target.Collection < out[j].Target.Collection })
	return out
}

func suggestOne(family string, collection Collection, at time.Time) Classification {
	c := Classification{
		Target: Target{Family: family, Collection: collection.Name},
		Role:   RoleUnknown, Origin: OriginHeuristic, Version: HeuristicVersion, State: StateSuggested, At: at,
	}
	nameWords := words(collection.Name)
	// The last word names the thing (order_lines → lines); earlier words qualify it.
	for wi := len(nameWords) - 1; wi >= 0 && c.Role == RoleUnknown; wi-- {
		for _, p := range patterns {
			if p.re.MatchString(nameWords[wi]) {
				c.Role = p.role
				c.Evidence = append(c.Evidence, fmt.Sprintf("name %q contains %q", collection.Name, nameWords[wi]))
				break
			}
		}
	}
	hasTemporal := false
	for _, column := range collection.Columns {
		if temporal.MatchString(column.Name) {
			hasTemporal = true
		}
	}
	signals := len(c.Evidence)
	switch {
	case c.Role == RoleUnknown && collection.ReferencedBy > 0 && len(collection.Columns) <= 4 && collection.References == 0:
		c.Role = RoleReference
		c.Evidence = append(c.Evidence, fmt.Sprintf("%d column(s), referenced by %d foreign key(s), references nothing", len(collection.Columns), collection.ReferencedBy))
	case c.Role == RoleUnknown && collection.References >= 2 && hasTemporal:
		c.Role = RoleTransactional
		c.Evidence = append(c.Evidence, fmt.Sprintf("references %d collections and has a date/time column", collection.References))
	case c.Role == RoleUnknown && collection.ReferencedBy > 0:
		c.Role = RoleMaster
		c.Evidence = append(c.Evidence, fmt.Sprintf("referenced by %d foreign key(s)", collection.ReferencedBy))
	case c.Role == RoleReference && collection.ReferencedBy > 0:
		c.Evidence = append(c.Evidence, fmt.Sprintf("referenced by %d foreign key(s)", collection.ReferencedBy))
		signals++
	case c.Role == RoleTransactional && hasTemporal:
		c.Evidence = append(c.Evidence, "has a date/time column")
		signals++
	case c.Role == RoleEvent && hasTemporal:
		c.Evidence = append(c.Evidence, "has a date/time column")
		signals++
	}
	if c.Role == RoleUnknown {
		return c
	}
	c.Confidence = ConfidencePossible
	if signals >= 2 {
		c.Confidence = ConfidenceLikely
	}
	c.Consistency = suggestRules(c.Role, collection)
	if c.Consistency == nil && (c.Role == RoleReference || c.Role == RoleMaster || c.Role == RoleConfiguration) {
		c.Evidence = append(c.Evidence, "no natural key found: choose one before reconciling (surrogate ids rarely match across databases)")
	}
	return c
}

var naturalKey = regexp.MustCompile(`^(code|iso|iso2|iso3|iso_code|alpha2|alpha3|sku|key|name|slug|abbreviation|symbol)$|(_code|_key|_sku|Code|Key|Sku)$`)

// suggestRules proposes a consistency rule for roles that usually agree
// across instances, keyed by a natural (non-surrogate) unique column.
func suggestRules(role Role, collection Collection) *reconcile.Rules {
	if role != RoleReference && role != RoleMaster && role != RoleConfiguration {
		return nil
	}
	key := ""
	for _, column := range collection.Columns {
		if naturalKey.MatchString(column.Name) && (column.Unique || column.PrimaryKey) {
			key = column.Name
			break
		}
	}
	if key == "" {
		return nil
	}
	rules := &reconcile.Rules{Key: []string{key}, Mode: reconcile.ModeConsensus, DefaultField: reconcile.FieldIdentical}
	fields := map[string]reconcile.FieldPolicy{}
	for _, column := range collection.Columns {
		if column.Name != key && column.PrimaryKey {
			fields[column.Name] = reconcile.FieldIgnored // surrogate id
		}
	}
	switch role {
	case RoleReference:
		rules.Presence = reconcile.PresenceRequired
	case RoleMaster:
		rules.Presence = reconcile.PresenceAny
	case RoleConfiguration:
		rules.Presence = reconcile.PresenceAny
		rules.DefaultField = reconcile.FieldInstanceSpecific
	}
	if len(fields) > 0 {
		rules.Fields = fields
	}
	return rules
}
