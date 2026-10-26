package walkthrough

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type localeSourceFile struct {
	URL      string `json:"url"`
	FileDate string `json:"fileDate"`
	SHA256   string `json:"sha256"`
}

type localeWholeTagExpected struct {
	Tag             string `json:"tag"`
	Canonical       string `json:"canonical"`
	PrimaryLanguage string `json:"primaryLanguage"`
}

type localeWholeTagTable struct {
	Tag       string `json:"tag"`
	Preferred string `json:"preferred"`
}

type localeTablesFile struct {
	Profile             string                `json:"profile"`
	LanguageRegistry    localeSourceFile      `json:"languageRegistry"`
	ExtensionsRegistry  localeSourceFile      `json:"extensionsRegistry"`
	Languages           []string              `json:"languages"`
	Scripts             []string              `json:"scripts"`
	Regions             []string              `json:"regions"`
	Variants            []string              `json:"variants"`
	Extlangs            map[string]string     `json:"extlangs"`
	PreferredLanguages  map[string]string     `json:"preferredLanguages"`
	PreferredScripts    map[string]string     `json:"preferredScripts"`
	PreferredRegions    map[string]string     `json:"preferredRegions"`
	PreferredVariants   map[string]string     `json:"preferredVariants"`
	Grandfathered       []localeWholeTagTable `json:"grandfathered"`
	Redundant           []localeWholeTagTable `json:"redundant"`
	ExtensionSingletons []string              `json:"extensionSingletons"`
	PrivateUseRanges    map[string][][]string `json:"privateUseRanges"`
}

type localeProvenanceSource struct {
	URL       string `json:"url"`
	FileDate  string `json:"fileDate"`
	SHA256    string `json:"sha256"`
	Bytes     int    `json:"bytes"`
	Retrieved string `json:"retrieved"`
}

type localeProvenanceFile struct {
	Profile     string                            `json:"profile"`
	GeneratedBy string                            `json:"generatedBy"`
	Sources     map[string]localeProvenanceSource `json:"sources"`
	TablesFile  string                            `json:"tablesFile"`
	TablesSHA   string                            `json:"tablesSha256"`
	Inventory   map[string]int                    `json:"inventory"`
}

type localeCorpusIssue struct {
	Code     LocaleCode `json:"code"`
	Subtag   string     `json:"subtag"`
	Snapshot string     `json:"snapshot"`
}

type localeCorpusWant struct {
	Accepted        bool                `json:"accepted"`
	Tag             string              `json:"tag"`
	PrimaryLanguage string              `json:"primaryLanguage"`
	Issues          []localeCorpusIssue `json:"issues"`
}

type localeCorpusVector struct {
	ID        string           `json:"id"`
	Operation string           `json:"operation"`
	Input     string           `json:"input"`
	Want      localeCorpusWant `json:"want"`
}

type localeCorpus struct {
	CorpusVersion      string                   `json:"corpusVersion"`
	Profile            string                   `json:"profile"`
	LanguageRegistry   localeSourceFile         `json:"languageRegistry"`
	ExtensionsRegistry localeSourceFile         `json:"extensionsRegistry"`
	Note               string                   `json:"note"`
	Grandfathered      []localeWholeTagExpected `json:"grandfathered"`
	Redundant          []localeWholeTagExpected `json:"redundant"`
	Vectors            []localeCorpusVector     `json:"vectors"`
}

func loadLocaleCorpus(t *testing.T) localeCorpus {
	t.Helper()
	var corpus localeCorpus
	raw, err := os.ReadFile(filepath.Join("testdata", "locale", "locale-corpus.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &corpus))
	return corpus
}

func loadLocaleTables(t *testing.T) (localeTablesFile, []byte) {
	t.Helper()
	var tables localeTablesFile
	raw, err := os.ReadFile(filepath.Join("testdata", "locale", "locale-tables.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &tables))
	return tables, raw
}

func localeIssueView(issues []LocaleIssue) []localeCorpusIssue {
	view := make([]localeCorpusIssue, len(issues))
	for index, issue := range issues {
		view[index] = localeCorpusIssue{
			Code:     issue.Code,
			Subtag:   issue.Subtag,
			Snapshot: issue.Snapshot,
		}
	}
	return view
}

func TestLocaleConformanceCorpus(t *testing.T) {
	corpus := loadLocaleCorpus(t)
	require.Equal(t, "walkthrough-locale-v1", corpus.CorpusVersion)
	require.Equal(t, LocaleProfile, corpus.Profile)
	require.Equal(t, LocaleRegistryFileDate, corpus.LanguageRegistry.FileDate)
	require.Equal(t, LocaleRegistrySHA256, corpus.LanguageRegistry.SHA256)
	require.Equal(t, LocaleExtensionsRegistryFileDate, corpus.ExtensionsRegistry.FileDate)
	require.Equal(t, LocaleExtensionsRegistrySHA256, corpus.ExtensionsRegistry.SHA256)
	require.Contains(t, corpus.Note, "RFC 5646")

	ids := make(map[string]bool)
	for _, vector := range corpus.Vectors {
		require.False(t, ids[vector.ID], "duplicate vector id %s", vector.ID)
		ids[vector.ID] = true
		var result LocaleResult
		switch vector.Operation {
		case "authored-admission":
			result = AdmitAuthoredLocale(vector.Input)
		case "visitor-normalization":
			result = NormalizeVisitorLocale(vector.Input)
		default:
			t.Fatalf("%s: unknown locale operation %q", vector.ID, vector.Operation)
		}
		require.Equal(t, vector.Want.Accepted, result.Accepted, vector.ID)
		if vector.Want.Accepted {
			require.Equal(t, vector.Want.Tag, result.Tag, vector.ID)
			require.Equal(t, vector.Want.PrimaryLanguage, result.PrimaryLanguage, vector.ID)
			require.Empty(t, result.Issues, vector.ID)
			require.NoError(t, result.Err(), vector.ID)
			continue
		}
		require.Empty(t, result.Tag, vector.ID)
		require.Empty(t, result.PrimaryLanguage, vector.ID)
		require.Equal(t, vector.Want.Issues, localeIssueView(result.Issues), vector.ID)
		refusal := result.Err()
		require.Error(t, refusal, vector.ID)
		var localeRefusalError *LocaleError
		require.ErrorAs(t, refusal, &localeRefusalError)
		require.True(t, localeRefusalError.Recoverable(), vector.ID)
		require.Equal(t, vector.Want.Issues[0].Code, LocaleErrorCode(refusal), vector.ID)
	}
}

// TestLocaleRegisteredWholeTags exercises every grandfathered and redundant
// record through both entry points. The expected table is committed in the
// corpus, independent of the generated Go table and of either implementation.
func TestLocaleRegisteredWholeTags(t *testing.T) {
	corpus := loadLocaleCorpus(t)
	require.Len(t, corpus.Grandfathered, 26)
	require.Len(t, corpus.Redundant, 67)
	require.Len(t, localeGrandfatheredRecords, 26)
	require.Len(t, localeRedundantRecords, 67)

	expected := make(map[string]localeWholeTagExpected)
	for _, record := range append(append(
		[]localeWholeTagExpected{},
		corpus.Grandfathered...,
	), corpus.Redundant...) {
		expected[strings.ToLower(record.Tag)] = record
	}
	require.Len(t, expected, 93)

	generated := make(map[string]bool)
	for _, records := range [][]localeWholeTagRecord{
		localeGrandfatheredRecords,
		localeRedundantRecords,
	} {
		for _, record := range records {
			lower := strings.ToLower(record.tag)
			generated[lower] = true
			want, ok := expected[lower]
			require.True(t, ok, "missing corpus expectation for %q", record.tag)
			for name, operation := range map[string]func(string) LocaleResult{
				"authored": AdmitAuthoredLocale,
				"visitor":  NormalizeVisitorLocale,
			} {
				result := operation(record.tag)
				require.True(t, result.Accepted, "%s %s", name, record.tag)
				require.Equal(t, want.Canonical, result.Tag, "%s %s", name, record.tag)
				require.Equal(t, want.PrimaryLanguage, result.PrimaryLanguage, "%s %s", name, record.tag)
			}
		}
	}
	for lower := range expected {
		require.True(t, generated[lower], "corpus has no generated record for %q", lower)
	}
}

func TestLocaleTablesAndProvenance(t *testing.T) {
	tables, tablesRaw := loadLocaleTables(t)
	provenanceRaw, err := os.ReadFile(filepath.Join("testdata", "locale", "locale-provenance.json"))
	require.NoError(t, err)
	var provenance localeProvenanceFile
	require.NoError(t, json.Unmarshal(provenanceRaw, &provenance))

	require.Equal(t, LocaleProfile, tables.Profile)
	require.Equal(t, LocaleTablesSHA256, sha256Hex(tablesRaw))
	require.Equal(t, LocaleTablesSHA256, provenance.TablesSHA)
	require.Equal(t, "locale-tables.json", provenance.TablesFile)
	require.Equal(t, "pkg/walkthrough/testdata/locale/generate_locale_tables.py", provenance.GeneratedBy)
	require.Equal(t, LocaleRegistryFileDate, provenance.Sources["languageSubtagRegistry"].FileDate)
	require.Equal(t, LocaleRegistrySHA256, provenance.Sources["languageSubtagRegistry"].SHA256)
	require.Equal(t, LocaleExtensionsRegistryFileDate, provenance.Sources["languageTagExtensionsRegistry"].FileDate)
	require.Equal(t, LocaleExtensionsRegistrySHA256, provenance.Sources["languageTagExtensionsRegistry"].SHA256)

	languageRaw, err := os.ReadFile(filepath.Join("testdata", "locale", "language-subtag-registry.txt"))
	require.NoError(t, err)
	extensionsRaw, err := os.ReadFile(filepath.Join("testdata", "locale", "language-tag-extensions-registry.txt"))
	require.NoError(t, err)
	require.Equal(t, LocaleRegistrySHA256, sha256Hex(languageRaw))
	require.Equal(t, LocaleExtensionsRegistrySHA256, sha256Hex(extensionsRaw))
	require.Equal(t, len(languageRaw), provenance.Sources["languageSubtagRegistry"].Bytes)
	require.Equal(t, len(extensionsRaw), provenance.Sources["languageTagExtensionsRegistry"].Bytes)
	require.Contains(t, string(languageRaw), "File-Date: "+LocaleRegistryFileDate)
	require.Contains(t, string(extensionsRaw), "File-Date: "+LocaleExtensionsRegistryFileDate)

	// Every runtime inventory must equal the committed table exactly.
	require.Equal(t, tables.Languages, sortedLocaleSet(localeLanguages))
	require.Equal(t, tables.Scripts, sortedLocaleSet(localeScripts))
	require.Equal(t, tables.Regions, sortedLocaleSet(localeRegions))
	require.Equal(t, tables.Variants, sortedLocaleSet(localeVariants))
	require.Equal(t, tables.Extlangs, localeExtlangPrefixes)
	require.Equal(t, tables.PreferredLanguages, localePreferredLanguages)
	require.Equal(t, tables.PreferredScripts, localePreferredScripts)
	require.Equal(t, tables.PreferredRegions, localePreferredRegions)
	require.Equal(t, tables.PreferredVariants, localePreferredVariants)
	require.Equal(t, tables.ExtensionSingletons, sortedLocaleSet(localeExtensionSingletons))
	require.Equal(t, tables.Grandfathered, generatedWholeTagTable(localeGrandfatheredRecords))
	require.Equal(t, tables.Redundant, generatedWholeTagTable(localeRedundantRecords))
	require.Equal(t, tables.PrivateUseRanges, generatedPrivateUseRanges())

	require.Equal(t, len(tables.Languages), provenance.Inventory["languages"])
	require.Equal(t, len(tables.Scripts), provenance.Inventory["scripts"])
	require.Equal(t, len(tables.Regions), provenance.Inventory["regions"])
	require.Equal(t, len(tables.Variants), provenance.Inventory["variants"])
	require.Equal(t, len(tables.Extlangs), provenance.Inventory["extlangs"])
	require.Equal(t, len(tables.PreferredLanguages), provenance.Inventory["preferredLanguages"])
	require.Equal(t, len(tables.PreferredScripts), provenance.Inventory["preferredScripts"])
	require.Equal(t, len(tables.PreferredRegions), provenance.Inventory["preferredRegions"])
	require.Equal(t, len(tables.PreferredVariants), provenance.Inventory["preferredVariants"])
	require.Equal(t, len(tables.Grandfathered), provenance.Inventory["grandfathered"])
	require.Equal(t, len(tables.Redundant), provenance.Inventory["redundant"])
	require.Equal(t, len(tables.ExtensionSingletons), provenance.Inventory["extensionSingletons"])

	// Spot checks keep the inventory meaningful, not merely internally equal.
	require.True(t, localeLanguages.has("zh"))
	require.True(t, localeScripts.has("Latn"))
	require.True(t, localeRegions.has("GB"))
	require.True(t, localeVariants.has("1901"))
	require.Equal(t, "zh", localeExtlangPrefixes["cmn"])
	require.Equal(t, "he", localePreferredLanguages["iw"])
	require.Equal(t, "MM", localePreferredRegions["BU"])
	require.Equal(t, "alalc97", localePreferredVariants["heploc"])
	require.True(t, localeExtensionSingletons.has("t"))
	require.True(t, localeExtensionSingletons.has("u"))
	require.True(t, localeRangeContains(localePrivateUseLanguageRanges, "qtz"))
	require.False(t, localeRangeContains(localePrivateUseLanguageRanges, "qzu"))
}

func sortedLocaleSet(set localeSet) []string {
	values := make([]string, 0, len(set.values))
	for value := range set.values {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func generatedWholeTagTable(records []localeWholeTagRecord) []localeWholeTagTable {
	table := make([]localeWholeTagTable, len(records))
	for index, record := range records {
		table[index] = localeWholeTagTable{Tag: record.tag, Preferred: record.preferred}
	}
	return table
}

func generatedPrivateUseRanges() map[string][][]string {
	convert := func(ranges []localeRange) [][]string {
		values := make([][]string, len(ranges))
		for index, item := range ranges {
			values[index] = []string{item.low, item.high}
		}
		return values
	}
	return map[string][][]string{
		"language": convert(localePrivateUseLanguageRanges),
		"script":   convert(localePrivateUseScriptRanges),
		"region":   convert(localePrivateUseRegionRanges),
	}
}

func TestLocaleCanonicalCollisions(t *testing.T) {
	pairs := [][2]string{
		{"iw-IL", "he-IL"},
		{"en-GB-oed", "en-GB-oxendict"},
		{"i-klingon", "tlh"},
		{"zh-cmn", "cmn"},
		{"sgn-US", "ase"},
	}
	for _, pair := range pairs {
		left := AdmitAuthoredLocale(pair[0])
		right := AdmitAuthoredLocale(pair[1])
		require.True(t, left.Accepted, pair[0])
		require.True(t, right.Accepted, pair[1])
		require.Equal(t, left.Tag, right.Tag, pair[0])
		require.Equal(t, right.Tag, AdmitAuthoredLocale(pair[0]).Tag)
	}
}

// TestLocaleSelectionInputs proves the data the later catalogue resolver needs
// without implementing that resolver here: exact canonical tag, declared
// canonical primary language, then fallback. No script/region/progressive
// intermediate lookup is available.
func TestLocaleSelectionInputs(t *testing.T) {
	request := NormalizeVisitorLocale("en-US")
	require.Equal(t, "en-US", request.Tag)
	require.Equal(t, "en", request.PrimaryLanguage)
	declared := []string{
		NormalizeVisitorLocale("en").Tag,
		NormalizeVisitorLocale("en-GB").Tag,
	}
	require.NotContains(t, declared, request.Tag)
	require.Contains(t, declared, request.PrimaryLanguage)

	request = NormalizeVisitorLocale("zh-Hant-TW")
	require.Equal(t, "zh-Hant-TW", request.Tag)
	require.Equal(t, "zh", request.PrimaryLanguage)
	declared = []string{
		NormalizeVisitorLocale("zh-Hant").Tag,
		NormalizeVisitorLocale("zh").Tag,
	}
	require.NotContains(t, declared, request.Tag)
	require.Contains(t, declared, request.PrimaryLanguage)

	privateOnly := NormalizeVisitorLocale("x-request")
	require.Equal(t, "x-request", privateOnly.Tag)
	require.Empty(t, privateOnly.PrimaryLanguage)

	invalid := NormalizeVisitorLocale("en_US")
	require.False(t, invalid.Accepted)
	require.Equal(t, LocaleCodeMalformed, LocaleErrorCode(invalid.Err()))
}

func TestLocaleRefusalShape(t *testing.T) {
	result := NormalizeVisitorLocale("en_US")
	first := result.Err()
	var firstRefusal *LocaleError
	require.ErrorAs(t, first, &firstRefusal)
	firstRefusal.Issues[0].Message = "changed by caller"
	require.NotEqual(t, "changed by caller", result.Issues[0].Message)
	second := result.Err()
	var secondRefusal *LocaleError
	require.ErrorAs(t, second, &secondRefusal)
	require.NotEqual(t, "changed by caller", secondRefusal.Issues[0].Message)

	require.Contains(t, LocaleIssue{Code: LocaleCodeMalformed, Message: "m"}.Error(), "malformed_locale")
	require.Equal(t, LocaleCodeMalformed, LocaleErrorCode(LocaleIssue{Code: LocaleCodeMalformed}))
	require.Equal(t, LocaleCode(""), LocaleErrorCode(errors.New("unrelated")))

	single := &LocaleError{Issues: []LocaleIssue{{Code: LocaleCodeMalformed, Subtag: "x"}}}
	require.Contains(t, single.Error(), "malformed_locale")
	multiple := &LocaleError{Issues: []LocaleIssue{
		{Code: LocaleCodeMalformed},
		{Code: LocaleCodeTooLong},
	}}
	require.Contains(t, multiple.Error(), "2 locale issues")
	require.Contains(t, (&LocaleError{}).Error(), "refused")
	require.True(t, multiple.Recoverable())
}

func TestLocaleUnsupportedSnapshotMessages(t *testing.T) {
	language := AdmitAuthoredLocale("zz")
	require.Equal(t, LocaleCodeUnsupportedRegistryVersion, LocaleErrorCode(language.Err()))
	require.Equal(t, LocaleRegistryFileDate, language.Issues[0].Snapshot)
	require.Contains(t, language.Issues[0].Message, LocaleRegistryFileDate)

	singleton := AdmitAuthoredLocale("en-a-bbb")
	require.Equal(t, LocaleCodeUnsupportedRegistryVersion, LocaleErrorCode(singleton.Err()))
	require.Equal(t, LocaleExtensionsRegistryFileDate, singleton.Issues[0].Snapshot)
	require.Contains(t, singleton.Issues[0].Message, LocaleExtensionsRegistryFileDate)
}

func TestLocaleCanonicalOutputBound(t *testing.T) {
	parts := localeParts{privateOnly: true}
	for index := 0; index < 8; index++ {
		parts.privateUse = append(parts.privateUse, "abcdefgh")
	}
	result := resolveLocaleParts(parts, false)
	require.False(t, result.Accepted)
	require.Equal(t, LocaleCodeTooLong, LocaleErrorCode(result.Err()))
	tag, ok := renderLocaleParts(parts)
	require.False(t, ok)
	require.Empty(t, tag)
}

func TestLocaleBoundaries(t *testing.T) {
	within := "x" + strings.Repeat("-abcdefgh", 7)
	require.Len(t, within, MaxLocaleCharacters)
	require.True(t, AdmitAuthoredLocale(within).Accepted)
	require.True(t, NormalizeVisitorLocale(within).Accepted)

	beyond := "x" + strings.Repeat("-abcdefg", 8)
	require.Len(t, beyond, MaxLocaleCharacters+1)
	require.Equal(t, LocaleCodeTooLong, LocaleErrorCode(AdmitAuthoredLocale(beyond).Err()))
	require.Equal(t, LocaleCodeTooLong, LocaleErrorCode(NormalizeVisitorLocale(beyond).Err()))

	alias := NormalizeVisitorLocale("en-GB-oed")
	require.True(t, alias.Accepted)
	require.Equal(t, "en-GB-oxendict", alias.Tag)
	require.LessOrEqual(t, len(alias.Tag), MaxLocaleCharacters)
}

func TestLocalePreferredScriptMapping(t *testing.T) {
	// The pinned snapshot currently registers no script Preferred-Value, so
	// exercise the generic mapping branch with an in-test table entry and
	// restore the generated table afterwards.
	localePreferredScripts["Test"] = "Latn"
	t.Cleanup(func() { delete(localePreferredScripts, "Test") })
	result := resolveLocaleParts(localeParts{language: "en", script: "Test"}, false)
	require.True(t, result.Accepted)
	require.Equal(t, "en-Latn", result.Tag)
}

func TestLocaleOperationsArePure(t *testing.T) {
	first := AdmitAuthoredLocale("ZH-cmn-Hans-CN")
	second := AdmitAuthoredLocale("ZH-cmn-Hans-CN")
	require.Equal(t, first, second)
	require.Equal(t, "cmn-Hans-CN", first.Tag)
	require.True(t, NormalizeVisitorLocale("en-xyz").Accepted)
	require.False(t, AdmitAuthoredLocale("en-xyz").Accepted)
	require.Equal(t, "en-xyz", NormalizeVisitorLocale("en-xyz").Tag)
}
