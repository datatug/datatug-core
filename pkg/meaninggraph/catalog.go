package meaninggraph

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"gopkg.in/yaml.v3"
)

type revisionRow struct{ ID, Kind, Address, Commit, Path string }
type selection struct {
	RevisionID, RegistryID, ModuleID, ModuleVersion string
	Definition                                      map[string]any
}
type nativeCatalog struct{ Models []selection }
type loadedModel struct {
	result        ModelRevision
	ast           map[string]any
	alias         string
	definitionRef map[string]any
}

func normalizeAddress(kind, address string) (string, error) {
	if kind == "model" {
		r := datatug.DataModelReference{Address: address, Commit: strings.Repeat("0", 40)}
		if err := r.Validate(); err != nil {
			return "", err
		}
		return r.NormalizedAddress(), nil
	}
	if kind != "meaning" || !strings.HasPrefix(address, "meaning://github.com/") {
		return "", errors.New("invalid revision kind/address")
	}
	repo := "https://" + strings.TrimPrefix(address, "meaning://")
	if !repositoryPattern.MatchString(repo) {
		return "", errors.New("invalid meaning address")
	}
	for _, p := range strings.Split(repo, "/") {
		if p == "." || p == ".." {
			return "", errors.New("unsafe address")
		}
	}
	return strings.ToLower(address), nil
}
func revisionID(kind, address, commit string) string {
	body, _ := json.Marshal([]string{"meaninggraph-revision/v1", kind, address, commit})
	return digest(body)
}
func publisher(address string) string {
	parts := strings.Split(address, "/")
	return "https://" + strings.Join(parts[2:5], "/")
}
func (s *session) catalog(ctx context.Context) (nativeCatalog, map[string]revisionRow, error) {
	var result nativeCatalog
	rows := map[string]revisionRow{}
	raw, err := s.read(ctx, "catalog.json", s.resolver.config.CatalogSHA256)
	if err != nil {
		return result, nil, err
	}
	c, err := parseJSON(raw)
	if err != nil {
		return result, nil, err
	}
	if c["format"] != "meaninggraph-catalog/draft-1" {
		return result, nil, errors.New("unsupported native catalog format")
	}
	body := map[string]any{}
	for k, v := range c {
		if k != "checksum" {
			body[k] = v
		}
	}
	// parseJSON emits only canonical-supported JSON types.
	canonicalBody, _ := canonical(body)
	if c["checksum"] != "sha256:"+digest(canonicalBody) {
		return result, nil, errors.New("native catalog checksum mismatch")
	}
	indexRef, err := obj(c["revision_index"])
	if err != nil {
		return result, nil, err
	}
	if indexRef["path"] != "revisions/index.json" {
		return result, nil, errors.New("unsafe revision-index path")
	}
	indexHash, err := text(indexRef, "sha256")
	if err != nil || !sha64.MatchString(indexHash) {
		return result, nil, errors.New("invalid revision-index digest")
	}
	raw, err = s.read(ctx, "revisions/index.json", indexHash)
	if err != nil {
		return result, nil, err
	}
	index, err := parseJSON(raw)
	if err != nil {
		return result, nil, err
	}
	if index["format"] != "meaninggraph-revision-index/draft-1" {
		return result, nil, errors.New("unsupported revision-index format")
	}
	values, err := list(index["revisions"])
	if err != nil {
		return result, nil, err
	}
	if len(values) > 10000 {
		return result, nil, errors.New("revision-index traversal bound exceeded")
	}
	for _, value := range values {
		row, e := obj(value)
		if e != nil {
			return result, nil, e
		}
		id, e := text(row, "id")
		if e != nil || !sha64.MatchString(id) {
			return result, nil, errors.New("invalid revision ID")
		}
		kind, _ := text(row, "kind")
		address, _ := text(row, "address")
		address, e = normalizeAddress(kind, address)
		if e != nil {
			return result, nil, e
		}
		commit, _ := text(row, "commit")
		path, _ := text(row, "path")
		if !sha40.MatchString(commit) || id != revisionID(kind, address, commit) || path != "revisions/$records/"+id+".yaml" {
			return result, nil, errors.New("revision identity/path mismatch")
		}
		if _, ok := rows[id]; ok {
			return result, nil, errors.New("duplicate revision ID")
		}
		rows[id] = revisionRow{id, kind, address, commit, path}
	}
	pins, err := obj(c["source_revisions"])
	if err != nil {
		return result, nil, err
	}
	identities := map[string]bool{}
	names := map[string]bool{}
	for _, group := range []struct{ key, kind string }{{"models", "model"}, {"graphs", "meaning"}} {
		entries, err := list(c[group.key])
		if err != nil {
			return result, nil, err
		}
		for _, value := range entries {
			m, e := obj(value)
			if e != nil {
				return result, nil, e
			}
			id, _ := text(m, "revision_id")
			name, _ := text(m, "id")
			address, _ := text(m, "address")
			address, e = normalizeAddress(group.kind, address)
			if e != nil {
				return result, nil, e
			}
			commit, _ := text(m, "commit")
			repo, _ := text(m, "repository")
			row, ok := rows[id]
			canonicalRepo := strings.ToLower(repo)
			identity := group.kind + ":" + address
			qualifiedName := group.kind + ":" + name
			if !registryID.MatchString(name) || !ok || row.Kind != group.kind || row.Address != address || row.Commit != commit || !repositoryPattern.MatchString(repo) || publisher(address) != canonicalRepo || identities[identity] || names[qualifiedName] {
				return result, nil, errors.New("invalid native selection identity")
			}
			sourcePins := 0
			for pinRepo, pin := range pins {
				if strings.EqualFold(pinRepo, repo) {
					sourcePins++
					if pin != commit {
						return result, nil, errors.New("selected source pin mismatch")
					}
				}
			}
			if sourcePins != 1 {
				return result, nil, errors.New("selected source pin missing/ambiguous")
			}
			identities[identity] = true
			names[qualifiedName] = true
			def, e := obj(m["definition"])
			if e != nil {
				return result, nil, e
			}
			hash, _ := text(def, "sha256")
			if def["path"] != "definitions/"+id+".json" || !sha64.MatchString(hash) {
				return result, nil, errors.New("invalid selected definition reference")
			}
			if group.kind == "model" {
				moduleID, _ := text(m, "module_id")
				moduleVersion, _ := text(m, "module_version")
				result.Models = append(result.Models, selection{id, name, moduleID, moduleVersion, def})
			}
		}
	}
	return result, rows, nil
}
func artifactPath(hash string) string { return "artifacts/sha256/" + hash[:2] + "/" + hash }
func (s *session) evidence(ctx context.Context, value any) ([]byte, map[string]any, error) {
	e, err := obj(value)
	if err != nil {
		return nil, nil, err
	}
	hash, _ := text(e, "sha256")
	commit, _ := text(e, "commit")
	path, _ := text(e, "path")
	repo, _ := text(e, "repository")
	if !sha64.MatchString(hash) || !sha40.MatchString(commit) || safePath(path) != nil || (repo != RegistryRepository && repo != LegacyRegistryRepository) || e["artifactPath"] != artifactPath(hash) {
		return nil, nil, errors.New("invalid archived registration provenance")
	}
	body, err := s.read(ctx, artifactPath(hash), hash)
	return body, e, err
}
func parseRegistration(raw []byte) (map[string]any, error) {
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "{") {
		return parseJSON(raw)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("registration mapping required")
	}
	node := document.Content[0]
	result := map[string]any{}
	seen := map[string]bool{}
	// yaml.v3 aliases refer to anchored scalar/sequence/mapping nodes;
	// YAML alias nodes cannot themselves declare an anchor. Dereference once,
	// then require a string, so recursive container aliases cannot expand.
	scalar := func(n *yaml.Node) (string, error) {
		if n.Kind == yaml.AliasNode {
			n = n.Alias
		}
		if n.Kind != yaml.ScalarNode || n.Tag != "!!str" {
			return "", errors.New("registration string required")
		}
		return n.Value, nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		key, err := scalar(node.Content[i])
		if err != nil {
			return nil, err
		}
		if seen[key] {
			return nil, errors.New("duplicate registration key")
		}
		seen[key] = true
		v := node.Content[i+1]
		switch key {
		case "format", "module", "commit", "address", "repository", "source_file", "json_file", "licence", "title":
			value, e := scalar(v)
			if e != nil {
				return nil, e
			}
			result[key] = value
		case "maintainers":
			if v.Kind == yaml.AliasNode {
				v = v.Alias
			}
			if v.Kind != yaml.SequenceNode {
				return nil, errors.New("registration maintainers array required")
			}
			values := []any{}
			for _, n := range v.Content {
				value, e := scalar(n)
				if e != nil {
					return nil, e
				}
				values = append(values, value)
			}
			result[key] = values
		}
	}
	return result, nil
}
func (s *session) load(ctx context.Context, row revisionRow) (*loadedModel, error) {
	raw, err := s.read(ctx, row.Path, "")
	if err != nil {
		return nil, err
	}
	receipt, err := parseJSON(raw)
	if err != nil {
		return nil, err
	}
	address, _ := text(receipt, "address")
	address, err = normalizeAddress("model", address)
	if err != nil {
		return nil, err
	}
	id, _ := text(receipt, "registry_id")
	repo, _ := text(receipt, "repository")
	canonicalRepo := publisher(row.Address)
	if receipt["format"] != "meaninggraph-revision/draft-1" || receipt["kind"] != "model" || receipt["id"] != row.ID || address != row.Address || receipt["commit"] != row.Commit || !registryID.MatchString(id) || !repositoryPattern.MatchString(repo) || strings.ToLower(repo) != canonicalRepo {
		return nil, errors.New("retained model receipt identity mismatch")
	}
	registrationRaw, sourceRef, err := s.evidence(ctx, receipt["source_record"])
	if err != nil {
		return nil, err
	}
	if sourceRef["path"] != "models/$records/"+id+".yaml" {
		return nil, errors.New("registration path/registry ID mismatch")
	}
	registration, err := parseRegistration(registrationRaw)
	if err != nil {
		return nil, err
	}
	if acquisition, ok := receipt["acquisition_record"]; ok {
		if _, _, err = s.evidence(ctx, acquisition); err != nil {
			return nil, err
		}
	}
	fileValues, err := list(receipt["files"])
	if err != nil || len(fileValues) == 0 {
		return nil, errors.New("retained model files required")
	}
	files := []ArchivedSource{}
	texts := map[string][]byte{}
	for _, value := range fileValues {
		f, e := obj(value)
		if e != nil {
			return nil, e
		}
		path, _ := text(f, "path")
		hash, _ := text(f, "sha256")
		media, _ := text(f, "mediaType")
		if safePath(path) != nil || !sha64.MatchString(hash) || media == "" || f["artifactPath"] != artifactPath(hash) {
			return nil, errors.New("invalid retained artifact identity")
		}
		if _, exists := texts[path]; exists {
			return nil, errors.New("duplicate retained source path")
		}
		body, e := s.read(ctx, artifactPath(hash), hash)
		if e != nil {
			return nil, e
		}
		texts[path] = body
		files = append(files, ArchivedSource{path, hash, media, append([]byte(nil), body...)})
	}
	paths, err := obj(receipt["source_paths"])
	if err != nil {
		return nil, err
	}
	hcl, _ := text(paths, "hcl")
	jsonPath, _ := text(paths, "json")
	if hcl == jsonPath || safePath(hcl) != nil || safePath(jsonPath) != nil || texts[hcl] == nil || texts[jsonPath] == nil {
		return nil, errors.New("incomplete archived model source pair")
	}
	original, err := parseJSON(texts[jsonPath])
	if err != nil {
		return nil, err
	}
	module, err := obj(receipt["module"])
	if err != nil {
		return nil, err
	}
	same, err := equal(module, original["module"])
	if err != nil {
		return nil, err
	}
	alias, _ := text(registration, "module")
	moduleID, _ := text(module, "id")
	moduleVersion, _ := text(module, "version")
	sourceAddress, _ := text(registration, "address")
	sourceAddress, err = normalizeAddress("model", sourceAddress)
	if err != nil {
		return nil, err
	}
	sourceRepo, _ := text(registration, "repository")
	licences, err := obj(receipt["licences"])
	if err != nil {
		return nil, err
	}
	maintainers, err := list(receipt["maintainers"])
	if err != nil || len(maintainers) == 0 {
		return nil, errors.New("model attribution required")
	}
	for _, name := range maintainers {
		if _, e := str(name); e != nil {
			return nil, e
		}
	}
	licence, _ := text(licences, "model")
	sourceLicence, _ := text(registration, "licence")
	// Every receipt maintainer was checked as a string above, so this
	// comparison cannot enter numeric normalization or return its error.
	attribution, _ := equal(maintainers, registration["maintainers"])
	if !same || !aliasName.MatchString(alias) || strings.Split(row.Address, "/")[5] != alias || registration["format"] != "modelspec-registry/draft-1" || registration["commit"] != row.Commit || sourceAddress != row.Address || strings.ToLower(sourceRepo) != canonicalRepo || registration["source_file"] != hcl || registration["json_file"] != jsonPath || original["modelspec"] != receipt["format_identifier"] || moduleID == "" || moduleVersion == "" || (module["name"] != nil && module["name"] != alias) || !attribution || licence == "" || licence != sourceLicence {
		return nil, errors.New("archived source/module/attribution identity mismatch")
	}
	logical, err := obj(receipt["logical_digest"])
	if err != nil {
		return nil, err
	}
	logicalHash, _ := text(logical, "sha256")
	if logical["algorithm"] != "meaninggraph-logical-digest/v1" || !sha64.MatchString(logicalHash) {
		return nil, errors.New("invalid logical digest metadata")
	}
	defRef, err := obj(receipt["definition"])
	if err != nil {
		return nil, err
	}
	defHash, _ := text(defRef, "sha256")
	if defRef["path"] != "definitions/"+row.ID+".json" || !sha64.MatchString(defHash) {
		return nil, errors.New("unsafe model definition reference")
	}
	definitionRaw, err := s.read(ctx, "definitions/"+row.ID+".json", defHash)
	if err != nil {
		return nil, err
	}
	definition, err := parseJSON(definitionRaw)
	if err != nil {
		return nil, err
	}
	ast, err := normalizeAST(original)
	if err != nil {
		return nil, err
	}
	same, err = equal(ast, definition["ast"])
	if err != nil {
		return nil, err
	}
	if definition["format"] != "meaninggraph-definition/draft-1" || definition["kind"] != "model" || definition["revision_id"] != row.ID || !same {
		return nil, errors.New("definition differs from archived source AST")
	}
	pins, err := list(receipt["dependency_pins"])
	if err != nil {
		return nil, err
	}
	dependencies := []DependencyPin{}
	seen := map[string]bool{}
	for _, value := range pins {
		pin, e := obj(value)
		if e != nil {
			return nil, e
		}
		alias, _ := text(pin, "alias")
		id, _ := text(pin, "revision_id")
		if !aliasName.MatchString(alias) || !sha64.MatchString(id) || seen[alias] {
			return nil, errors.New("invalid/ambiguous exact module lock")
		}
		seen[alias] = true
		dependencies = append(dependencies, DependencyPin{alias, id})
	}
	var receiptParts map[string]json.RawMessage
	// The same receipt bytes already passed parseJSON's object validation.
	_ = json.Unmarshal(raw, &receiptParts)
	moduleRaw := append(json.RawMessage(nil), receiptParts["module"]...)
	title, _ := text(registration, "title")
	return &loadedModel{result: ModelRevision{DataModelReference: datatug.DataModelReference{Address: receipt["address"].(string), Commit: row.Commit}, RevisionID: row.ID, RegistryID: id, Title: title, Module: moduleRaw, Receipt: append([]byte(nil), raw...), Definition: append([]byte(nil), definitionRaw...), SourceJSON: append([]byte(nil), texts[jsonPath]...), SourceHCL: append([]byte(nil), texts[hcl]...), Sources: files, Dependencies: dependencies}, ast: ast, alias: alias, definitionRef: defRef}, nil
}
func verifySelection(selected selection, model *loadedModel) error {
	same, err := equal(selected.Definition, model.definitionRef)
	if err != nil {
		return err
	}
	// Module is copied from the receipt object checked by load.
	module, _ := parseJSON(model.result.Module)
	if !same || selected.RegistryID != model.result.RegistryID || selected.ModuleID != module["id"] || selected.ModuleVersion != module["version"] {
		return errors.New("selection differs from exact retained definition")
	}
	return nil
}
func closedObject(value map[string]any, allowed ...string) error {
	for key := range value {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
				break
			}
		}
		if !found {
			return errors.New("unsupported archived ModelSpec field")
		}
	}
	return nil
}
func normalizeAST(original map[string]any) (map[string]any, error) {
	old := original["modelspec"] == "1.0-draft"
	if !old && original["modelspec"] != "1.0-draft-2" {
		return nil, errors.New("unsupported archived ModelSpec format")
	}
	recordKey := "records"
	if old {
		recordKey = "entities"
	}
	if err := closedObject(original, "modelspec", "module", "components", "enums", recordKey); err != nil {
		return nil, err
	}
	result := map[string]any{"modelspec": "1.0-draft-2", "module": original["module"]}
	for _, group := range []struct{ source, target string }{{"components", "components"}, {"enums", "enums"}, {recordKey, "records"}} {
		values, exists := original[group.source]
		if !exists {
			continue
		}
		declarations, err := obj(values)
		if err != nil {
			return nil, err
		}
		normalized := map[string]any{}
		for name, value := range declarations {
			declaration, err := obj(value)
			if err != nil {
				return nil, err
			}
			if group.target == "enums" {
				if err := closedObject(declaration, "values"); err != nil {
					return nil, err
				}
				normalized[name] = declaration
				continue
			}
			fieldKey := "fields"
			if old && group.target == "records" {
				fieldKey = "properties"
			}
			allowed := []string{fieldKey}
			if group.target == "records" {
				allowed = append(allowed, "key", "use")
			}
			if err := closedObject(declaration, allowed...); err != nil {
				return nil, err
			}
			fields := map[string]any{}
			if raw, ok := declaration[fieldKey]; ok {
				source, err := obj(raw)
				if err != nil {
					return nil, err
				}
				for fieldName, value := range source {
					field, err := obj(value)
					if err != nil {
						return nil, err
					}
					referenceKey := "record"
					if old {
						referenceKey = "entity"
					}
					if err := closedObject(field, "type", referenceKey, "component", "enum", "required", "unique", "min_len", "max_len", "pattern", "format"); err != nil {
						return nil, err
					}
					copy := map[string]any{}
					for key, value := range field {
						if old && key == "entity" {
							key = "record"
						}
						copy[key] = value
					}
					fields[fieldName] = copy
				}
			}
			next := map[string]any{"fields": fields}
			for key, value := range declaration {
				if key != fieldKey {
					next[key] = value
				}
			}
			normalized[name] = next
		}
		result[group.target] = normalized
	}
	return result, nil
}
func verifyClosure(model *loadedModel, closure map[string]*loadedModel) error {
	locks := map[string]*loadedModel{}
	aliases := map[string]string{}
	for _, target := range closure {
		if prior, ok := aliases[target.alias]; ok && prior != target.result.RevisionID {
			return errors.New("conflicting exact revisions for module alias")
		}
		aliases[target.alias] = target.result.RevisionID
	}
	for _, pin := range model.result.Dependencies {
		target := closure[pin.RevisionID]
		if target == nil || target.alias != pin.Alias {
			return errors.New("exact dependency alias mismatch")
		}
		locks[pin.Alias] = target
	}
	resolve := func(reference, kind string) error {
		parts := strings.Split(reference, ".")
		if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && parts[1] == "" {
			return errors.New("malformed model reference")
		}
		target := model
		if len(parts) == 2 && parts[0] != model.alias {
			target = locks[parts[0]]
			if target == nil {
				return errors.New("unresolved exact model reference; no latest fallback")
			}
		}
		group, err := obj(target.ast[kind])
		if err != nil {
			return err
		}
		if _, ok := group[parts[len(parts)-1]]; !ok {
			return errors.New("missing exact model declaration")
		}
		return nil
	}
	for _, kind := range []string{"records", "components"} {
		raw, ok := model.ast[kind]
		if !ok {
			continue
		}
		// normalizeAST constructs these exact map types before closure proof.
		group := raw.(map[string]any)
		for _, value := range group {
			declaration := value.(map[string]any)
			if kind == "records" {
				if raw, ok := declaration["use"]; ok {
					uses, err := list(raw)
					if err != nil {
						return err
					}
					for _, value := range uses {
						reference, e := str(value)
						if e != nil {
							return e
						}
						if e = resolve(reference, "components"); e != nil {
							return e
						}
					}
				}
			}
			fields := declaration["fields"].(map[string]any)
			for _, value := range fields {
				field := value.(map[string]any)
				for _, ref := range []struct{ key, kind string }{{"record", "records"}, {"component", "components"}, {"enum", "enums"}} {
					if value, ok := field[ref.key]; ok {
						reference, isString := value.(string)
						if ref.key == "enum" && !isString {
							continue
						}
						if !isString {
							return errors.New("model reference must be a string")
						}
						if err := resolve(reference, ref.kind); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}
