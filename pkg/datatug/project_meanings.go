package datatug

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

type MeaningIdentity struct {
	DocumentID string `json:"documentId"`
	ConceptID  string `json:"conceptId"`
}
type MeaningScope struct {
	Storage string `json:"storage"`
	Project string `json:"project"`
	SpaceID string `json:"spaceID,omitempty"`
	Branch  string `json:"branch,omitempty"`
}
type MeaningReference struct {
	MeaningIdentity
	Scope MeaningScope `json:"scope"`
}

func (r MeaningReference) Validate() error {
	if r.DocumentID != "project" || !semanticID.MatchString(r.ConceptID) || len(r.ConceptID) > 80 {
		return errors.New("invalid meaning reference identity")
	}
	if r.Scope.Project == "" || len(r.Scope.Project) > 1024 || len(r.Scope.SpaceID) > 256 || len(r.Scope.Branch) > 256 {
		return errors.New("invalid meaning reference scope")
	}
	switch r.Scope.Storage {
	case "local", "github.com":
		if r.Scope.SpaceID != "" {
			return errors.New("invalid meaning reference space")
		}
	case "firestore":
		if r.Scope.Branch != "" {
			return errors.New("invalid meaning reference branch")
		}
	default:
		return errors.New("invalid meaning reference storage")
	}
	return nil
}

type MeaningResource struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Environment string `json:"environment,omitempty"`
}
type MeaningField struct {
	MeaningIdentity
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Resources   []MeaningResource `json:"resources"`
}
type MeaningEntity struct {
	MeaningIdentity
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Origin      string            `json:"origin"`
	Fields      []MeaningField    `json:"fields"`
	Resources   []MeaningResource `json:"resources"`
}
type ProjectMeaningsCatalog struct {
	Scope              MeaningScope    `json:"scope"`
	Version            int             `json:"version"`
	Revision           string          `json:"revision"`
	Writable           bool            `json:"writable"`
	ReferencesComplete bool            `json:"referencesComplete"`
	Entities           []MeaningEntity `json:"entities"`
}
type ProjectMeaningAction struct {
	Action      string          `json:"action"`
	ConceptID   string          `json:"conceptId,omitempty"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Entity      MeaningIdentity `json:"entity,omitempty"`
	Field       MeaningIdentity `json:"field,omitempty"`
}
type ProjectMeaningWrite struct {
	OperationID string               `json:"operationId"`
	IfMatch     string               `json:"ifMatch"`
	Change      ProjectMeaningAction `json:"change"`
}
type ProjectMeaningAck struct {
	Receipt string                 `json:"receipt"`
	Catalog ProjectMeaningsCatalog `json:"catalog"`
}

// ProjectMeaningsDocument is persisted atomically with its operation receipts.
// Resources are recomputed; persisted definitions must contain none.
type ProjectMeaningsDocument struct {
	Version  int                                `json:"version"`
	Entities []MeaningEntity                    `json:"entities"`
	Receipts map[string]MeaningOperationReceipt `json:"receipts,omitempty"`
}
type MeaningOperationReceipt struct {
	Digest  string `json:"digest"`
	Receipt string `json:"receipt"`
}

// Sources must be a complete authorized snapshot read in the provider transaction.
// Database must come from an exact resolved query connection, never SQL inference.
type MeaningQuerySource struct {
	ID            string
	Title         string
	SemanticModel *QuerySemanticModel
	Databases     []MeaningResource
	Database      *MeaningResource
}
type MeaningBoardSource struct {
	ID       string
	Title    string
	QueryIDs []string
}

var ErrMeaningConflict = errors.New("meaning catalog revision conflict")
var ErrMeaningReferenced = errors.New("meaning has inbound references")
var ErrMeaningIncomplete = errors.New("meaning references are incomplete")
var ErrMeaningOperationConflict = errors.New("meaning operation ID conflict")
var ErrMeaningReadOnly = errors.New("meaning catalog is read-only")

func meaningDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func QueryMeaningDocumentID(queryID, documentID string) string {
	b, _ := json.Marshal([]string{queryID, documentID})
	return "query:" + string(b)
}
func (d ProjectMeaningsDocument) Validate() error {
	if d.Version != 1 {
		return errors.New("invalid meanings document version")
	}
	seen := map[string]bool{}
	for _, e := range d.Entities {
		if e.DocumentID != "project" || e.Origin != "project" || len(e.Resources) > 0 {
			return errors.New("invalid standalone entity")
		}
		if err := validateMeaningText(e.ConceptID, e.Name, e.Description); err != nil {
			return err
		}
		if seen[e.ConceptID] {
			return errors.New("duplicate meaning identity")
		}
		seen[e.ConceptID] = true
		for _, f := range e.Fields {
			if f.DocumentID != "project" || len(f.Resources) > 0 {
				return errors.New("invalid standalone field")
			}
			if err := validateMeaningText(f.ConceptID, f.Name, f.Description); err != nil {
				return err
			}
			if seen[f.ConceptID] {
				return errors.New("duplicate meaning identity")
			}
			seen[f.ConceptID] = true
		}
	}
	if len(seen) > 4096 || len(d.Receipts) > 10000 {
		return errors.New("meanings document limit exceeded")
	}
	return nil
}
func validateMeaningText(id, name, description string) error {
	if len(id) > 80 || !semanticID.MatchString(id) || strings.TrimSpace(name) == "" || len(name) > 512 || len(description) > 4096 {
		return errors.New("invalid meaning text or ID")
	}
	return screenSemanticStrings(reflect.ValueOf([]string{name, description}))
}
func appendMeaningResource(resources []MeaningResource, r MeaningResource) []MeaningResource {
	for _, v := range resources {
		if v == r {
			return resources
		}
	}
	return append(resources, r)
}

func BuildProjectMeaningsCatalog(d ProjectMeaningsDocument, queries []MeaningQuerySource, boards []MeaningBoardSource, scope MeaningScope, writable, complete bool) (ProjectMeaningsCatalog, error) {
	if err := d.Validate(); err != nil {
		return ProjectMeaningsCatalog{}, err
	}
	budget, err := newMeaningCatalogBudget(scope, writable, complete)
	if err != nil {
		return ProjectMeaningsCatalog{}, err
	}
	for i, e := range d.Entities {
		if err := budget.entity(e, i > 0); err != nil {
			return ProjectMeaningsCatalog{}, err
		}
		for j, f := range e.Fields {
			if err := budget.field(f, j > 0); err != nil {
				return ProjectMeaningsCatalog{}, err
			}
		}
	}
	b, _ := json.Marshal(d.Entities)
	var entities []MeaningEntity
	_ = json.Unmarshal(b, &entities)
	if entities == nil {
		entities = []MeaningEntity{}
	}
	for i := range entities {
		entities[i].Resources = []MeaningResource{}
		if entities[i].Fields == nil {
			entities[i].Fields = []MeaningField{}
		}
		for j := range entities[i].Fields {
			entities[i].Fields[j].Resources = []MeaningResource{}
		}
	}
	for _, q := range queries {
		if q.Title == "" {
			q.Title = q.ID
		}
		if q.SemanticModel == nil {
			continue
		}
		if err := q.SemanticModel.Validate(); err != nil {
			return ProjectMeaningsCatalog{}, fmt.Errorf("query semantic model: %w", err)
		}
		if len(q.ID) > maxMeaningCatalogBytes {
			return ProjectMeaningsCatalog{}, ErrMeaningIncomplete
		}
		docID := QueryMeaningDocumentID(q.ID, q.SemanticModel.Meaning.ID)
		index := map[string]int{}
		start := len(entities)
		for _, c := range q.SemanticModel.Meaning.Concepts {
			if c.Kind == "entity" {
				index[c.ID] = len(entities)
				entity := MeaningEntity{MeaningIdentity: MeaningIdentity{docID, c.ID}, Name: c.Labels.En, Description: c.Description, Origin: "query", Fields: []MeaningField{}, Resources: []MeaningResource{}}
				if err := budget.entity(entity, len(entities) > 0); err != nil {
					return ProjectMeaningsCatalog{}, err
				}
				entities = append(entities, entity)
			}
		}
		for _, c := range q.SemanticModel.Meaning.Concepts {
			if c.Kind == "property" {
				i := index[c.Of]
				field := MeaningField{MeaningIdentity: MeaningIdentity{docID, c.ID}, Name: c.Labels.En, Description: c.Description, Resources: []MeaningResource{}}
				if err := budget.field(field, len(entities[i].Fields) > 0); err != nil {
					return ProjectMeaningsCatalog{}, err
				}
				entities[i].Fields = append(entities[i].Fields, field)
			}
		}
		resources := []MeaningResource{{Kind: "query", ID: q.ID, Title: q.Title}}
		for _, database := range q.Databases {
			if database.Kind != "database" || database.ID == "" || database.Environment == "" {
				return ProjectMeaningsCatalog{}, errors.New("invalid database provenance")
			}
			if database.Title == "" {
				database.Title = database.ID
			}
			resources, err = appendBoundedMeaningResource(resources, database)
			if err != nil {
				return ProjectMeaningsCatalog{}, err
			}
		}
		if q.Database != nil {
			if q.Database.Kind != "database" || q.Database.ID == "" || q.Database.Environment == "" {
				return ProjectMeaningsCatalog{}, errors.New("invalid database provenance")
			}
			resources, err = appendBoundedMeaningResource(resources, *q.Database)
			if err != nil {
				return ProjectMeaningsCatalog{}, err
			}
		}
		for _, b := range boards {
			if b.Title == "" {
				b.Title = b.ID
			}
			for _, id := range b.QueryIDs {
				if id == q.ID {
					resources, err = appendBoundedMeaningResource(resources, MeaningResource{Kind: "board", ID: b.ID, Title: b.Title})
					if err != nil {
						return ProjectMeaningsCatalog{}, err
					}
				}
			}
		}
		for i := start; i < len(entities); i++ {
			if err := budget.resources(&entities[i].Resources, resources); err != nil {
				return ProjectMeaningsCatalog{}, err
			}
			for j := range entities[i].Fields {
				if err := budget.resources(&entities[i].Fields[j].Resources, resources); err != nil {
					return ProjectMeaningsCatalog{}, err
				}
			}
		}
		for _, c := range q.SemanticModel.Meaning.Concepts {
			if c.MeaningRef == nil || c.MeaningRef.Scope != scope {
				continue
			}
			found := false
			for i := 0; i < start; i++ {
				if entities[i].Origin != "project" {
					continue
				}
				if c.Kind == "entity" && entities[i].ConceptID == c.MeaningRef.ConceptID {
					found = true
					for _, r := range resources {
						if err := budget.resources(&entities[i].Resources, []MeaningResource{r}); err != nil {
							return ProjectMeaningsCatalog{}, err
						}
					}
				}
				if c.Kind == "property" {
					for j := range entities[i].Fields {
						if entities[i].Fields[j].ConceptID == c.MeaningRef.ConceptID {
							parent := q.SemanticModel.Meaning.Concepts
							ownerRef := ""
							for _, o := range parent {
								if o.ID == c.Of && o.MeaningRef != nil && o.MeaningRef.Scope == scope {
									ownerRef = o.MeaningRef.ConceptID
								}
							}
							if ownerRef != entities[i].ConceptID {
								return ProjectMeaningsCatalog{}, errors.New("meaning property reference owner mismatch")
							}
							found = true
							for _, r := range resources {
								if err := budget.resources(&entities[i].Fields[j].Resources, []MeaningResource{r}); err != nil {
									return ProjectMeaningsCatalog{}, err
								}
								if err := budget.resources(&entities[i].Resources, []MeaningResource{r}); err != nil {
									return ProjectMeaningsCatalog{}, err
								}
							}
						}
					}
				}
			}
			if !found {
				return ProjectMeaningsCatalog{}, errors.New("unresolved project meaning reference")
			}
		}
	}
	sort.Slice(entities, func(i, j int) bool {
		if entities[i].Name == entities[j].Name {
			return entities[i].DocumentID+entities[i].ConceptID < entities[j].DocumentID+entities[j].ConceptID
		}
		return entities[i].Name < entities[j].Name
	})
	for i := range entities {
		sort.Slice(entities[i].Fields, func(a, b int) bool { return entities[i].Fields[a].ConceptID < entities[i].Fields[b].ConceptID })
		sortMeaningResources(entities[i].Resources)
		for j := range entities[i].Fields {
			sortMeaningResources(entities[i].Fields[j].Resources)
		}
	}
	catalog := ProjectMeaningsCatalog{Scope: scope, Version: 1, Writable: writable, ReferencesComplete: complete, Entities: entities}
	catalog.Revision = meaningDigest(struct {
		Scope    MeaningScope
		Entities []MeaningEntity
		Complete bool
	}{scope, entities, complete})
	return catalog, nil
}
func sortMeaningResources(v []MeaningResource) {
	sort.Slice(v, func(i, j int) bool { return v[i].Kind+v[i].Environment+v[i].ID < v[j].Kind+v[j].Environment+v[j].ID })
}

// Apply requires a freshly built catalog from the same locked snapshot as d.
// Provider persists next atomically, then rebuilds the returned catalog.
func ApplyProjectMeaningChange(d ProjectMeaningsDocument, c ProjectMeaningsCatalog, w ProjectMeaningWrite) (ProjectMeaningsDocument, ProjectMeaningAck, error) {
	fail := func(e error) (ProjectMeaningsDocument, ProjectMeaningAck, error) { return d, ProjectMeaningAck{}, e }
	if !c.Writable {
		return fail(ErrMeaningReadOnly)
	}
	if len(w.OperationID) < 8 || len(w.OperationID) > 128 {
		return fail(errors.New("invalid operation ID"))
	}
	digest := meaningDigest(w)
	if receipt, ok := d.Receipts[w.OperationID]; ok {
		if receipt.Digest != digest {
			return fail(ErrMeaningOperationConflict)
		}
		return d, ProjectMeaningAck{Receipt: receipt.Receipt, Catalog: c}, nil
	}
	if !c.ReferencesComplete {
		return fail(ErrMeaningIncomplete)
	}
	if w.IfMatch == "" || w.IfMatch != c.Revision {
		return fail(ErrMeaningConflict)
	}
	if err := d.Validate(); err != nil {
		return fail(err)
	}
	data, _ := json.Marshal(d)
	var next ProjectMeaningsDocument
	_ = json.Unmarshal(data, &next)
	a := w.Change
	find := func(id MeaningIdentity) int {
		for i, e := range next.Entities {
			if e.MeaningIdentity == id {
				return i
			}
		}
		return -1
	}
	i := find(a.Entity)
	switch a.Action {
	case "add-entity":
		if err := validateMeaningText(a.ConceptID, a.Name, a.Description); err != nil {
			return fail(err)
		}
		next.Entities = append(next.Entities, MeaningEntity{MeaningIdentity: MeaningIdentity{"project", a.ConceptID}, Name: a.Name, Description: a.Description, Origin: "project", Fields: []MeaningField{}, Resources: []MeaningResource{}})
	case "add-field":
		if i < 0 {
			return fail(errors.New("entity not found"))
		}
		if err := validateMeaningText(a.ConceptID, a.Name, a.Description); err != nil {
			return fail(err)
		}
		next.Entities[i].Fields = append(next.Entities[i].Fields, MeaningField{MeaningIdentity: MeaningIdentity{"project", a.ConceptID}, Name: a.Name, Description: a.Description, Resources: []MeaningResource{}})
	case "delete-entity", "delete-field":
		if i < 0 {
			return fail(errors.New("entity not found"))
		}
		var current *MeaningEntity
		for j := range c.Entities {
			if c.Entities[j].MeaningIdentity == a.Entity {
				current = &c.Entities[j]
			}
		}
		if current == nil {
			return fail(ErrMeaningIncomplete)
		}
		if a.Action == "delete-entity" {
			if len(current.Resources) > 0 {
				return fail(ErrMeaningReferenced)
			}
			for _, f := range current.Fields {
				if len(f.Resources) > 0 {
					return fail(ErrMeaningReferenced)
				}
			}
			next.Entities = append(next.Entities[:i], next.Entities[i+1:]...)
		} else {
			j := -1
			for k, f := range next.Entities[i].Fields {
				if f.MeaningIdentity == a.Field {
					j = k
				}
			}
			if j < 0 {
				return fail(errors.New("field not found"))
			}
			for _, f := range current.Fields {
				if f.MeaningIdentity == a.Field && len(f.Resources) > 0 {
					return fail(ErrMeaningReferenced)
				}
			}
			next.Entities[i].Fields = append(next.Entities[i].Fields[:j], next.Entities[i].Fields[j+1:]...)
		}
	default:
		return fail(errors.New("unknown meaning action"))
	}
	if err := next.Validate(); err != nil {
		return fail(err)
	}
	if next.Receipts == nil {
		next.Receipts = map[string]MeaningOperationReceipt{}
	}
	receipt := w.OperationID
	next.Receipts[w.OperationID] = MeaningOperationReceipt{Digest: digest, Receipt: receipt}
	if err := next.Validate(); err != nil {
		return fail(err)
	}
	return next, ProjectMeaningAck{Receipt: receipt}, nil
}

// ValidateProjectMeaningReferences validates canonical targets before a query save.
// Call under the same transaction/lock used to change definitions.
func ValidateProjectMeaningReferences(model *QuerySemanticModel, d ProjectMeaningsDocument, scope MeaningScope) error {
	if model == nil {
		return nil
	}
	if err := d.Validate(); err != nil {
		return err
	}
	owners := map[string]string{}
	entities := map[string]bool{}
	concepts := map[string]QueryMeaningConcept{}
	for _, e := range d.Entities {
		entities[e.ConceptID] = true
		for _, f := range e.Fields {
			owners[f.ConceptID] = e.ConceptID
		}
	}
	for _, c := range model.Meaning.Concepts {
		concepts[c.ID] = c
	}
	for _, c := range model.Meaning.Concepts {
		if c.MeaningRef == nil {
			continue
		}
		r := c.MeaningRef
		if err := r.Validate(); err != nil {
			return err
		}
		if r.Scope != scope {
			return errors.New("meaning reference scope mismatch")
		}
		switch c.Kind {
		case "entity":
			if !entities[r.ConceptID] {
				return errors.New("meaning entity target not found")
			}
		case "property":
			owner, ok := owners[r.ConceptID]
			if !ok {
				return errors.New("meaning property target not found")
			}
			parent := concepts[c.Of]
			if parent.MeaningRef == nil || parent.MeaningRef.Scope != scope || parent.MeaningRef.ConceptID != owner {
				return errors.New("meaning property reference owner mismatch")
			}
		default:
			return errors.New("invalid referenced concept kind")
		}
	}
	return nil
}

// MeaningBoardSourceFromBoard extracts only the supported saved-query widget
// contracts. A malformed or unsupported widget prevents a complete scan.
func MeaningBoardSourceFromBoard(b Board) (MeaningBoardSource, error) {
	result := MeaningBoardSource{ID: b.ID, Title: b.Title, QueryIDs: []string{}}
	var visit func(BoardWidget, int) error
	add := func(id string) {
		for _, v := range result.QueryIDs {
			if v == id {
				return
			}
		}
		if id != "" {
			result.QueryIDs = append(result.QueryIDs, id)
		}
	}
	visit = func(w BoardWidget, depth int) error {
		if depth > 32 {
			return errors.New("board widget depth exceeded")
		}
		raw, err := json.Marshal(w.Data)
		if err != nil {
			return err
		}
		switch w.Name {
		case "SQL":
			var d SQLWidgetDef
			if err = json.Unmarshal(raw, &d); err != nil {
				return err
			}
			add(d.SQL.QueryID)
		case "chart":
			var d ChartWidgetDef
			if err = json.Unmarshal(raw, &d); err != nil {
				return err
			}
			for _, s := range d.Sources {
				add(s.QueryID)
			}
		case "tabs":
			var d TabsWidgetDef
			if err = json.Unmarshal(raw, &d); err != nil {
				return err
			}
			for _, t := range d.Tabs {
				if t.Widget.Name == "" && t.Widget.Data == nil {
					continue
				}
				if err := visit(t.Widget, depth+1); err != nil {
					return err
				}
			}
		case "HTTP":
		default:
			return errors.New("unsupported board widget")
		}
		return nil
	}
	for _, row := range b.Rows {
		for _, card := range row.Cards {
			if card.Widget != nil {
				if err := visit(*card.Widget, 0); err != nil {
					return result, err
				}
			}
		}
	}
	sort.Strings(result.QueryIDs)
	return result, nil
}

// MeaningDatabasesForQueryTargets resolves exact declared server+catalog
// references. Connection labels, SQL text, and dataset names are not matches.
func MeaningDatabasesForQueryTargets(q QueryDef, envs Environments) []MeaningResource {
	result := []MeaningResource{}
	for _, target := range q.Targets {
		if target.Catalog == "" || target.Driver == "" || target.Host == "" {
			continue
		}
		for _, env := range envs {
			if env == nil {
				continue
			}
			for _, server := range env.DbServers {
				if server == nil || server.Driver != target.Driver || server.Host != target.Host || server.Port != target.Port {
					continue
				}
				for _, catalogID := range server.Catalogs {
					if catalogID == target.Catalog {
						result = appendMeaningResource(result, MeaningResource{Kind: "database", ID: catalogID, Title: catalogID, Environment: env.ID})
					}
				}
			}
		}
	}
	sortMeaningResources(result)
	return result
}
