package datatug

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strings"

	"github.com/datatug/datatug-core/internal/jsonstrict"
)

var ErrUnsupportedDataModels = errors.New("unsupported project dataModels metadata")
var modelAddressPattern = regexp.MustCompile(`^modelspec://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/[A-Za-z_][A-Za-z0-9_]*$`)
var modelCommitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// DataModelReference identifies one exact source revision. It performs no I/O.
type DataModelReference struct {
	Address string `json:"address" firestore:"address"`
	Commit  string `json:"commit" firestore:"commit"`
}

func (r DataModelReference) Validate() error {
	if !modelAddressPattern.MatchString(r.Address) || !modelCommitPattern.MatchString(r.Commit) {
		return errors.New("data model requires a supported address and full lowercase source commit")
	}
	for _, p := range strings.Split(strings.TrimPrefix(r.Address, "modelspec://"), "/") {
		if p == "." || p == ".." {
			return errors.New("unsafe data model address")
		}
	}
	return nil
}

// NormalizedAddress preserves the module spelling and normalizes GitHub owner/repository only.
func (r DataModelReference) NormalizedAddress() string {
	p := strings.Split(r.Address, "/")
	if len(p) == 6 {
		p[3] = strings.ToLower(p[3])
		p[4] = strings.ToLower(p[4])
	}
	return strings.Join(p, "/")
}

type ProjectDataModelReference struct {
	ID string `json:"id" firestore:"id"`
	DataModelReference
}
type DataModelCatalogBinding struct {
	Environment string `json:"environment" firestore:"environment"`
	Catalog     string `json:"catalog" firestore:"catalog"`
	Reference   string `json:"reference" firestore:"reference"`
}

// A nil pointer preserves existing metadata on save. An explicit v1 empty
// document replaces/clears it; JSON null is never a clearing instruction.
type ProjectDataModels struct {
	Version         int                         `json:"version" firestore:"version"`
	References      []ProjectDataModelReference `json:"references" firestore:"references"`
	CatalogBindings []DataModelCatalogBinding   `json:"catalogBindings,omitempty" firestore:"catalogBindings,omitempty"`
}

func (d *ProjectDataModels) Validate() error {
	if d == nil {
		return nil
	}
	if d.Version != 1 {
		return fmt.Errorf("%w: version %d", ErrUnsupportedDataModels, d.Version)
	}
	ids := map[string]bool{}
	for _, r := range d.References {
		if !ValidConnectionID(r.ID) || ids[r.ID] {
			return errors.New("invalid or duplicate data model reference alias")
		}
		ids[r.ID] = true
		if err := r.Validate(); err != nil {
			return err
		}
	}
	bindings := map[[2]string]bool{}
	for _, b := range d.CatalogBindings {
		key := [2]string{b.Environment, b.Catalog}
		if !ValidConnectionID(b.Environment) || !ValidConnectionID(b.Catalog) || !ids[b.Reference] || bindings[key] {
			return errors.New("invalid, duplicate or unresolved data model catalog binding")
		}
		bindings[key] = true
	}
	return nil
}
func (d *ProjectDataModels) ReferenceByID(id string) (DataModelReference, error) {
	if err := d.Validate(); err != nil {
		return DataModelReference{}, err
	}
	if d != nil {
		for _, r := range d.References {
			if r.ID == id {
				return r.DataModelReference, nil
			}
		}
	}
	return DataModelReference{}, errors.New("project data model reference not found")
}
func (d *ProjectDataModels) ReferenceForCatalog(environment, catalog string) (DataModelReference, error) {
	if err := d.Validate(); err != nil {
		return DataModelReference{}, err
	}
	if d != nil {
		for _, b := range d.CatalogBindings {
			if b.Environment == environment && b.Catalog == catalog {
				return d.ReferenceByID(b.Reference)
			}
		}
	}
	return DataModelReference{}, errors.New("no data model reference assigned to environment/catalog")
}

type projectDataModelsJSON ProjectDataModels

func exactFields(body []byte, allowed ...string) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' {
		return nil, errors.New("dataModels object required")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	for key := range fields {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown or non-exact dataModels key %q", key)
		}
	}
	return fields, nil
}
func (d *ProjectDataModels) UnmarshalJSON(body []byte) error {
	if err := jsonstrict.ValidateUnicode(body); err != nil {
		return err
	}
	if err := jsonstrict.CheckNoDuplicateKeysForSchema(body, reflect.TypeOf(projectDataModelsJSON{})); err != nil {
		return err
	}
	fields, err := exactFields(body, "version", "references", "catalogBindings")
	if err != nil {
		return err
	}
	if _, ok := fields["version"]; !ok {
		return errors.New("dataModels.version required")
	}
	if raw := bytes.TrimSpace(fields["references"]); len(raw) == 0 || raw[0] != '[' {
		return errors.New("dataModels.references array required")
	}
	var refs []json.RawMessage
	// The duplicate-key scanner established valid JSON, and the leading
	// delimiter check established an array; RawMessages accept every element.
	_ = json.Unmarshal(fields["references"], &refs)
	for _, raw := range refs {
		if _, err = exactFields(raw, "id", "address", "commit"); err != nil {
			return err
		}
	}
	if raw, ok := fields["catalogBindings"]; ok {
		if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '[' {
			return errors.New("dataModels.catalogBindings must be an array")
		}
		var rows []json.RawMessage
		_ = json.Unmarshal(raw, &rows) // Validated JSON array of RawMessages.
		for _, row := range rows {
			if _, err = exactFields(row, "environment", "catalog", "reference"); err != nil {
				return err
			}
		}
	}
	var typed projectDataModelsJSON
	if err = json.Unmarshal(body, &typed); err != nil {
		return err
	}
	next := ProjectDataModels(typed)
	if err = next.Validate(); err != nil {
		return err
	}
	*d = next
	return nil
}
func (d ProjectDataModels) MarshalJSON() ([]byte, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}
	v := projectDataModelsJSON(d)
	if v.References == nil {
		v.References = []ProjectDataModelReference{}
	}
	return json.Marshal(v)
}

// Validate the recognized namespace before ordinary decoding can fold keys,
// discard duplicate keys or bypass a pointed UnmarshalJSON for JSON null.
// Unrelated opaque metadata keeps its existing decoding/preservation policy.
func validateProjectDataModelsJSON(body []byte) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return nil
	}
	if token != json.Delim('{') {
		return errors.New("project must be an object")
	}
	seen := false
	for dec.More() {
		t, e := dec.Token()
		if e != nil {
			return e
		}
		key, _ := t.(string)
		var raw json.RawMessage
		if e = dec.Decode(&raw); e != nil {
			return e
		}
		if strings.EqualFold(key, "dataModels") {
			if key != "dataModels" || seen {
				return errors.New("duplicate or non-exact dataModels project key")
			}
			seen = true
			var value ProjectDataModels
			if e = value.UnmarshalJSON(raw); e != nil {
				return e
			}
		}
	}
	if _, err = dec.Token(); err != nil {
		return err
	}
	if _, err = dec.Token(); err != io.EOF {
		return errors.New("unexpected project JSON suffix")
	}
	return nil
}

type projectJSON Project

func (p *Project) UnmarshalJSON(body []byte) error {
	if err := validateProjectEnvironmentOrderJSON(body); err != nil {
		return err
	}
	if err := validateProjectDataModelsJSON(body); err != nil {
		return err
	}
	v := projectJSON(*p)
	if err := json.Unmarshal(body, &v); err != nil {
		return err
	}
	*p = Project(v)
	return nil
}

// MergePreservedMetadata validates existing recognized metadata even when an
// explicit replacement was submitted: an unsupported version is never downgraded.
func (p *ProjectFile) MergePreservedMetadata(existing ProjectFile) error {
	if err := existing.Databases.Validate(); err != nil {
		return err
	}
	if err := p.Databases.Validate(); err != nil {
		return err
	}
	if p.Databases == nil {
		p.Databases = existing.Databases
	}
	if err := existing.Coordination.Validate(); err != nil {
		return err
	}
	if err := p.Coordination.Validate(); err != nil {
		return err
	}
	if p.Coordination == nil {
		p.Coordination = existing.Coordination
	}
	if err := existing.DataModels.Validate(); err != nil {
		return err
	}
	if err := p.DataModels.Validate(); err != nil {
		return err
	}
	if p.DataModels == nil {
		p.DataModels = existing.DataModels
	}
	if len(existing.jsonMetadata) > 0 {
		if p.jsonMetadata == nil {
			p.jsonMetadata = map[string]json.RawMessage{}
		}
		for k, v := range existing.jsonMetadata {
			if _, ok := p.jsonMetadata[k]; !ok {
				p.jsonMetadata[k] = append(json.RawMessage(nil), v...)
			}
		}
	}
	return nil
}
