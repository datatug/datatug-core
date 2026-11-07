// Package schemafamily identifies which database family (DbModel) a catalog
// belongs to from its schema shape alone, and reconciles schemas across N
// instances of one family.
//
// It implements the fingerprint and structural classification stages of the
// semantic-discovery "database identity" spec: fp1 fingerprints at the
// `types` and `keys` levels, and deterministic bands with measured evidence.
// Fingerprints identify structure, not systems: a match is evidence, never
// proof that two databases are the same logical system. The package reads no
// records and knows no database engine; callers convert their scanner output
// into Catalog.
package schemafamily

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Field is one column or document field with its portable type.
type Field struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable,omitempty"`
	PrimaryKey bool   `json:"primaryKey,omitempty"`
}

// ForeignKey is `(columns) → collection(columns)`, without constraint names.
type ForeignKey struct {
	Columns    []string `json:"columns"`
	Collection string   `json:"collection"`
	RefColumns []string `json:"refColumns"`
}

// Collection is one table, view or document collection.
type Collection struct {
	Schema      string       `json:"schema,omitempty"`
	Name        string       `json:"name"`
	Kind        string       `json:"kind"` // table | view | collection
	Fields      []Field      `json:"fields"`
	UniqueKeys  [][]string   `json:"uniqueKeys,omitempty"`
	ForeignKeys []ForeignKey `json:"foreignKeys,omitempty"`
}

// QualifiedName is `schema.name`, or `name` without a schema.
func (c Collection) QualifiedName() string {
	if c.Schema == "" {
		return c.Name
	}
	return c.Schema + "." + c.Name
}

// Catalog is one database's shape snapshot. KeysUnavailable names why keys
// could not be read (for example an adapter that does not read foreign
// keys); the keys fingerprint is then absent.
type Catalog struct {
	ID              string       `json:"id"`
	Collections     []Collection `json:"collections"`
	KeysUnavailable string       `json:"keysUnavailable,omitempty"`
}

// Fingerprint holds the fp1 levels of one catalog.
type Fingerprint struct {
	Types           string  `json:"types"`
	Keys            *string `json:"keys"`
	KeysUnavailable string  `json:"keysUnavailable,omitempty"`
}

type canonicalField struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	PrimaryKey bool   `json:"primaryKey,omitempty"`
}

type canonicalCollection struct {
	Name        string           `json:"name"`
	Kind        string           `json:"kind"`
	Fields      []canonicalField `json:"fields"`
	PrimaryKey  []string         `json:"primaryKey,omitempty"`
	UniqueKeys  []string         `json:"uniqueKeys,omitempty"`
	ForeignKeys []string         `json:"foreignKeys,omitempty"`
}

// Compute returns the catalog's fp1 fingerprints. Excluded by design: row
// counts, statistics, comments, index and constraint names, column ordinal,
// defaults, collation, owner, grants, timestamps and engine version.
func Compute(catalog Catalog) Fingerprint {
	fp := Fingerprint{Types: level("types", canonical(catalog, false))}
	if catalog.KeysUnavailable != "" {
		fp.KeysUnavailable = catalog.KeysUnavailable
		return fp
	}
	keys := level("keys", canonical(catalog, true))
	fp.Keys = &keys
	return fp
}

func level(name string, value any) string {
	data, _ := json.Marshal(value) // plain structs of strings and bools
	sum := sha256.Sum256(data)
	return "fp1:" + name + ":" + hex.EncodeToString(sum[:])
}

func canonical(catalog Catalog, keys bool) []canonicalCollection {
	out := make([]canonicalCollection, 0, len(catalog.Collections))
	for _, collection := range catalog.Collections {
		c := canonicalCollection{Name: collection.QualifiedName(), Kind: collection.Kind}
		for _, field := range collection.Fields {
			f := canonicalField{Name: field.Name, Type: field.Type, Nullable: field.Nullable}
			if keys && field.PrimaryKey {
				f.PrimaryKey = true
				c.PrimaryKey = append(c.PrimaryKey, field.Name)
			}
			c.Fields = append(c.Fields, f)
		}
		sort.Slice(c.Fields, func(i, j int) bool { return c.Fields[i].Name < c.Fields[j].Name })
		if keys {
			sort.Strings(c.PrimaryKey)
			for _, unique := range collection.UniqueKeys {
				c.UniqueKeys = append(c.UniqueKeys, joinSorted(unique))
			}
			sort.Strings(c.UniqueKeys)
			for _, fk := range collection.ForeignKeys {
				c.ForeignKeys = append(c.ForeignKeys, fmt.Sprintf("(%s)->%s(%s)", strings.Join(fk.Columns, ","), fk.Collection, strings.Join(fk.RefColumns, ",")))
			}
			sort.Strings(c.ForeignKeys)
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func joinSorted(values []string) string {
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// Band is a deterministic classification of a catalog against one family.
type Band string

const (
	BandIdentical                  Band = "identical"
	BandSameSystemDifferentVersion Band = "same-system-different-version"
	BandRelatedDistinct            Band = "related-distinct"
	BandNew                        Band = "new"
	BandInsufficientEvidence       Band = "insufficient-evidence"
)

// Thresholds are configuration; Default carries the spec's defaults.
type Thresholds struct {
	CollectionOverlap float64 `json:"collectionOverlap"`
	FieldOverlap      float64 `json:"fieldOverlap"`
	MinCollections    int     `json:"minCollections"`
	IdentifierMatches int     `json:"identifierMatches"`
}

// Default is the spec's threshold set.
var Default = Thresholds{CollectionOverlap: 0.8, FieldOverlap: 0.8, MinCollections: 3, IdentifierMatches: 3}

// Family is a known database family: its id (the DbModel id) and the shape
// of one representative catalog (the most recent attached scan).
type Family struct {
	ID      string  `json:"id"`
	Catalog Catalog `json:"catalog"`
}

// Evidence is reported instead of a single score.
type Evidence struct {
	SharedCollections   int      `json:"sharedCollections"`
	UnionCollections    int      `json:"unionCollections"`
	SharedFields        int      `json:"sharedFields"`
	UnionFields         int      `json:"unionFields"`
	TypeCompatible      int      `json:"typeCompatibleFields"`
	IdentifierMatches   int      `json:"identifierMatches"`
	SamePrimaryKeys     bool     `json:"samePrimaryKeys"`
	OnlyHere            []string `json:"onlyHere,omitempty"`
	OnlyInFamily        []string `json:"onlyInFamily,omitempty"`
	TypeChanges         []string `json:"typeChanges,omitempty"`
	FingerprintLevel    string   `json:"fingerprintLevel,omitempty"`
	CollectionOverlapPc int      `json:"collectionOverlapPercent"`
	FieldOverlapPc      int      `json:"fieldOverlapPercent"`
}

// Match is the classification of a catalog against one family.
type Match struct {
	Family     string   `json:"family"`
	Band       Band     `json:"band"`
	Confidence string   `json:"confidence,omitempty"` // likely | possible
	Evidence   Evidence `json:"evidence"`
}

// Classify compares a catalog with every known family and returns matches
// ordered best band first. A family whose band is `new` is omitted; when no
// family matches, the single result has band `new` (or
// `insufficient-evidence`) and no family.
func Classify(catalog Catalog, families []Family, thresholds Thresholds) []Match {
	if len(catalog.Collections) < thresholds.MinCollections {
		return []Match{{Band: BandInsufficientEvidence, Evidence: Evidence{UnionCollections: len(catalog.Collections)}}}
	}
	fp := Compute(catalog)
	var matches []Match
	for _, family := range families {
		m := classifyOne(catalog, fp, family, thresholds)
		if m.Band != BandNew {
			matches = append(matches, m)
		}
	}
	if len(matches) == 0 {
		return []Match{{Band: BandNew}}
	}
	rank := map[Band]int{BandIdentical: 0, BandSameSystemDifferentVersion: 1, BandRelatedDistinct: 2}
	sort.SliceStable(matches, func(i, j int) bool {
		if rank[matches[i].Band] != rank[matches[j].Band] {
			return rank[matches[i].Band] < rank[matches[j].Band]
		}
		if matches[i].Evidence.FieldOverlapPc != matches[j].Evidence.FieldOverlapPc {
			return matches[i].Evidence.FieldOverlapPc > matches[j].Evidence.FieldOverlapPc
		}
		return matches[i].Family < matches[j].Family
	})
	return matches
}

func classifyOne(catalog Catalog, fp Fingerprint, family Family, thresholds Thresholds) Match {
	m := Match{Family: family.ID, Evidence: compareShapes(catalog, family.Catalog)}
	other := Compute(family.Catalog)
	switch {
	case fp.Keys != nil && other.Keys != nil:
		if *fp.Keys == *other.Keys {
			m.Band, m.Confidence, m.Evidence.FingerprintLevel = BandIdentical, "likely", "keys"
			return m
		}
	case fp.Types == other.Types:
		m.Band, m.Confidence, m.Evidence.FingerprintLevel = BandIdentical, "possible", "types"
		return m
	}
	e := m.Evidence
	collectionOverlap := ratio(e.SharedCollections, e.UnionCollections)
	fieldOverlap := ratio(e.SharedFields, e.UnionFields)
	switch {
	case collectionOverlap >= thresholds.CollectionOverlap && fieldOverlap >= thresholds.FieldOverlap:
		m.Band = BandSameSystemDifferentVersion
		m.Confidence = "possible"
		if e.SamePrimaryKeys {
			m.Confidence = "likely"
		}
	case e.IdentifierMatches >= thresholds.IdentifierMatches:
		m.Band, m.Confidence = BandRelatedDistinct, "possible"
	default:
		m.Band = BandNew
	}
	return m
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func percent(a, b int) int { return int(ratio(a, b)*100 + 0.5) }

func index(catalog Catalog) map[string]Collection {
	out := make(map[string]Collection, len(catalog.Collections))
	for _, c := range catalog.Collections {
		out[strings.ToLower(c.QualifiedName())] = c
	}
	return out
}

func fieldsOf(c Collection) map[string]Field {
	out := make(map[string]Field, len(c.Fields))
	for _, f := range c.Fields {
		out[strings.ToLower(f.Name)] = f
	}
	return out
}

func primaryKey(c Collection) string {
	var pk []string
	for _, f := range c.Fields {
		if f.PrimaryKey {
			pk = append(pk, strings.ToLower(f.Name))
		}
	}
	return joinSorted(pk)
}

// isIdentifier reports whether a field name looks like an identifier.
func isIdentifier(name string) bool {
	lower := strings.ToLower(name)
	return lower == "id" || lower == "code" || lower == "sku" || strings.HasSuffix(lower, "_id") || strings.HasSuffix(lower, "id") && len(lower) > 2 && name[len(name)-2] == 'I' || strings.HasSuffix(lower, "_code")
}

func compareShapes(here, family Catalog) Evidence {
	a, b := index(here), index(family)
	var e Evidence
	e.SamePrimaryKeys = true
	names := map[string]bool{}
	for name := range a {
		names[name] = true
	}
	for name := range b {
		names[name] = true
	}
	e.UnionCollections = len(names)
	for _, name := range sortedNames(names) {
		left, inHere := a[name]
		right, inFamily := b[name]
		switch {
		case !inFamily:
			e.OnlyHere = append(e.OnlyHere, left.QualifiedName())
			continue
		case !inHere:
			e.OnlyInFamily = append(e.OnlyInFamily, right.QualifiedName())
			continue
		}
		e.SharedCollections++
		if primaryKey(left) != primaryKey(right) {
			e.SamePrimaryKeys = false
		}
		lf, rf := fieldsOf(left), fieldsOf(right)
		fieldNames := map[string]bool{}
		for n := range lf {
			fieldNames[n] = true
		}
		for n := range rf {
			fieldNames[n] = true
		}
		e.UnionFields += len(fieldNames)
		for _, n := range sortedNames(fieldNames) {
			l, okL := lf[n]
			r, okR := rf[n]
			if !okL || !okR {
				continue
			}
			e.SharedFields++
			if l.Type == r.Type {
				e.TypeCompatible++
				if isIdentifier(l.Name) {
					e.IdentifierMatches++
				}
			} else {
				e.TypeChanges = append(e.TypeChanges, fmt.Sprintf("%s.%s: %s → %s", right.QualifiedName(), r.Name, r.Type, l.Type))
			}
		}
	}
	e.CollectionOverlapPc = percent(e.SharedCollections, e.UnionCollections)
	e.FieldOverlapPc = percent(e.SharedFields, e.UnionFields)
	return e
}

func sortedNames(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SchemaDrift is one collection or field that is not the same across all
// instances of a family.
type SchemaDrift struct {
	Collection string            `json:"collection"`
	Field      string            `json:"field,omitempty"`
	MissingIn  []string          `json:"missingIn,omitempty"`
	Types      map[string]string `json:"types,omitempty"` // set when types differ
}

// SchemaReconciliation is the N-way schema comparison of one family.
type SchemaReconciliation struct {
	Instances []string `json:"instances"`
	// Groups lists instances sharing one keys-level (else types-level)
	// fingerprint, largest group first: one group means no drift.
	Groups []FingerprintGroup `json:"groups"`
	Drift  []SchemaDrift      `json:"drift"`
}

// FingerprintGroup is a set of instances with the same fingerprint.
type FingerprintGroup struct {
	Fingerprint string   `json:"fingerprint"`
	Instances   []string `json:"instances"`
}

// ReconcileSchemas compares N catalogs of one family in one pass over the
// union of their collections and fields. Majority is never treated as
// correct: drift lists where each element is missing or typed differently.
func ReconcileSchemas(catalogs []Catalog) SchemaReconciliation {
	result := SchemaReconciliation{Instances: []string{}, Groups: []FingerprintGroup{}, Drift: []SchemaDrift{}}
	byFingerprint := map[string][]string{}
	indexes := make([]map[string]Collection, len(catalogs))
	collections := map[string]string{} // lower-case → display name
	keysEverywhere := true
	for _, c := range catalogs {
		if c.KeysUnavailable != "" {
			keysEverywhere = false
		}
	}
	for i, catalog := range catalogs {
		result.Instances = append(result.Instances, catalog.ID)
		fp := Compute(catalog)
		key := fp.Types
		if keysEverywhere {
			key = *fp.Keys
		}
		byFingerprint[key] = append(byFingerprint[key], catalog.ID)
		indexes[i] = index(catalog)
		for name, collection := range indexes[i] {
			collections[name] = collection.QualifiedName()
		}
	}
	for fingerprint, ids := range byFingerprint {
		result.Groups = append(result.Groups, FingerprintGroup{Fingerprint: fingerprint, Instances: ids})
	}
	sort.Slice(result.Groups, func(i, j int) bool {
		if len(result.Groups[i].Instances) != len(result.Groups[j].Instances) {
			return len(result.Groups[i].Instances) > len(result.Groups[j].Instances)
		}
		return result.Groups[i].Fingerprint < result.Groups[j].Fingerprint
	})
	names := make(map[string]bool, len(collections))
	for name := range collections {
		names[name] = true
	}
	for _, name := range sortedNames(names) {
		drift := SchemaDrift{Collection: collections[name]}
		fields := map[string]string{}
		for i, idx := range indexes {
			collection, ok := idx[name]
			if !ok {
				drift.MissingIn = append(drift.MissingIn, catalogs[i].ID)
				continue
			}
			for n, f := range fieldsOf(collection) {
				fields[n] = f.Name
			}
		}
		if len(drift.MissingIn) > 0 {
			result.Drift = append(result.Drift, drift)
		}
		fieldNames := make(map[string]bool, len(fields))
		for n := range fields {
			fieldNames[n] = true
		}
		for _, n := range sortedNames(fieldNames) {
			fd := SchemaDrift{Collection: collections[name], Field: fields[n]}
			types := map[string]string{}
			distinct := map[string]bool{}
			for i, idx := range indexes {
				collection, ok := idx[name]
				if !ok {
					continue
				}
				f, ok := fieldsOf(collection)[n]
				if !ok {
					fd.MissingIn = append(fd.MissingIn, catalogs[i].ID)
					continue
				}
				types[catalogs[i].ID] = f.Type
				distinct[f.Type] = true
			}
			if len(distinct) > 1 {
				fd.Types = types
			}
			if len(fd.MissingIn) > 0 || fd.Types != nil {
				result.Drift = append(result.Drift, fd)
			}
		}
	}
	return result
}
