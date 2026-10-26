package walkthrough

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// LocaleProfile identifies the deterministic BCP-47 profile implemented by
// [AdmitAuthoredLocale] and [NormalizeVisitorLocale].
const LocaleProfile = "datatug-bcp47-profile/1"

// MaxLocaleCharacters is the inclusive bound on both an input tag and the
// canonical tag produced from it. Locale tags are US-ASCII, so this is also the
// byte bound.
const MaxLocaleCharacters = 64

// LocaleCode is a stable refusal code for locale admission or normalization.
// Callers may switch on it; human wording is not part of the contract.
type LocaleCode string

const (
	// LocaleCodeEmpty means the explicit locale string is empty. Caller
	// absence is not an empty string; default selection belongs to the later
	// catalogue resolver.
	LocaleCodeEmpty LocaleCode = "empty_locale"
	// LocaleCodeTooLong means the input or its canonical form exceeds
	// [MaxLocaleCharacters].
	LocaleCodeTooLong LocaleCode = "too_long"
	// LocaleCodeNonASCII means the input contains a non-ASCII byte.
	LocaleCodeNonASCII LocaleCode = "non_ascii"
	// LocaleCodeMalformed means the input does not match the profile's
	// hyphen-separated ASCII BCP-47 grammar.
	LocaleCodeMalformed LocaleCode = "malformed_locale"
	// LocaleCodeDuplicateVariant means one variant subtag appears twice.
	LocaleCodeDuplicateVariant LocaleCode = "duplicate_variant"
	// LocaleCodeDuplicateSingleton means one extension singleton appears twice.
	LocaleCodeDuplicateSingleton LocaleCode = "duplicate_singleton"
	// LocaleCodeReservedExtlangPosition means a second or third extlang
	// position was used; RFC 5646 permanently reserves those positions.
	LocaleCodeReservedExtlangPosition LocaleCode = "reserved_extlang_position"
	// LocaleCodeExtlangPrefixMismatch means a registered extlang does not
	// follow its registered Prefix primary language.
	LocaleCodeExtlangPrefixMismatch LocaleCode = "extlang_prefix_mismatch"
	// LocaleCodeUnsupportedRegistryVersion means an authored value is not
	// recognized in the pinned registry snapshot. It is recoverable and says
	// nothing about validity in a later registry.
	LocaleCodeUnsupportedRegistryVersion LocaleCode = "unsupported_registry_version"
)

// LocaleIssue is one structured locale refusal. Subtag is the offending
// subtag when there is one; Snapshot names the pinned registry File-Date for
// [LocaleCodeUnsupportedRegistryVersion].
type LocaleIssue struct {
	Code     LocaleCode `json:"code"`
	Subtag   string     `json:"subtag,omitempty"`
	Snapshot string     `json:"snapshot,omitempty"`
	Message  string     `json:"message"`
}

func (i LocaleIssue) Error() string {
	return fmt.Sprintf("walkthrough: %s at %q: %s", i.Code, i.Subtag, i.Message)
}

// LocaleError is the aggregate, recoverable refusal returned by
// [LocaleResult.Err].
type LocaleError struct {
	Issues []LocaleIssue `json:"issues"`
}

func (e *LocaleError) Error() string {
	if len(e.Issues) == 1 {
		return e.Issues[0].Error()
	}
	if len(e.Issues) == 0 {
		return "walkthrough: locale refused"
	}
	return fmt.Sprintf(
		"walkthrough: %d locale issues: %s",
		len(e.Issues),
		e.Issues[0].Error(),
	)
}

// Recoverable reports whether retrying with corrected input or a later
// registry snapshot can succeed. Every refusal produced by this profile is
// recoverable.
func (e *LocaleError) Recoverable() bool { return true }

// LocaleErrorCode returns the first stable locale issue code, or "" when err
// is not a locale refusal.
func LocaleErrorCode(err error) LocaleCode {
	var refusal *LocaleError
	if errors.As(err, &refusal) && len(refusal.Issues) > 0 {
		return refusal.Issues[0].Code
	}
	var issue LocaleIssue
	if errors.As(err, &issue) {
		return issue.Code
	}
	return ""
}

// LocaleResult is the outcome of one locale operation. Accepted is true only
// when Tag is canonical. PrimaryLanguage is the genuine regular primary
// language of the canonical tag, or "" for a private-use-only tag and for an
// irregular grandfathered tag without a replacement. Tag is also the
// canonical comparison key: equivalent spellings collide on it, so catalogue
// admission can reject duplicate tags before selection.
type LocaleResult struct {
	Accepted        bool          `json:"accepted"`
	Tag             string        `json:"tag,omitempty"`
	PrimaryLanguage string        `json:"primaryLanguage,omitempty"`
	Issues          []LocaleIssue `json:"issues,omitempty"`
}

// Err returns nil for an admitted tag or an aggregate recoverable
// [LocaleError].
func (r LocaleResult) Err() error {
	if r.Accepted {
		return nil
	}
	issues := make([]LocaleIssue, len(r.Issues))
	copy(issues, r.Issues)
	return &LocaleError{Issues: issues}
}

// AdmitAuthoredLocale admits and canonicalizes one authored `/2` locale field
// (a sidecar default, a catalogue reference or a catalogue document locale).
// Every language, script, region, variant, registered extlang and extension
// singleton must exist in the pinned registry snapshots; recognized private-use
// ranges are admitted too. An unknown but well-formed value is a recoverable
// [LocaleCodeUnsupportedRegistryVersion] naming the snapshot, never a silent
// rewrite and never a claim about a future registry.
func AdmitAuthoredLocale(input string) LocaleResult { return resolveLocale(input, true) }

// NormalizeVisitorLocale normalizes one visitor request. Syntactically
// well-formed unknown values are preserved for the later exact / primary /
// fallback selection instead of being rejected merely because the pinned
// snapshot lacks them. Malformed syntax is a structured refusal, never an
// ambient browser language. A registered extlang still has to match its
// registered Prefix, and reserved extlang positions stay invalid.
func NormalizeVisitorLocale(input string) LocaleResult { return resolveLocale(input, false) }

func resolveLocale(input string, authored bool) LocaleResult {
	if input == "" {
		return localeRefusal(LocaleIssue{
			Code:    LocaleCodeEmpty,
			Message: "explicit locale is empty; absence is a caller decision",
		})
	}
	if len(input) > MaxLocaleCharacters {
		return localeRefusal(LocaleIssue{
			Code:    LocaleCodeTooLong,
			Message: fmt.Sprintf("locale exceeds %d characters", MaxLocaleCharacters),
		})
	}
	for index := 0; index < len(input); index++ {
		if input[index] >= 0x80 {
			return localeRefusal(LocaleIssue{
				Code:    LocaleCodeNonASCII,
				Message: "locale must contain only US-ASCII characters",
			})
		}
	}

	if record, ok := localeWholeTags[strings.ToLower(input)]; ok {
		if record.preferred != "" {
			return resolveLocale(record.preferred, authored)
		}
		primary := localeSyntaxPrimaryLanguage(record.tag)
		return localeSuccess(record.tag, primary)
	}

	parts, issue := parseLocaleSyntax(input)
	if issue != nil {
		return localeRefusal(*issue)
	}
	return resolveLocaleParts(parts, authored)
}

// resolveLocaleParts is the shared tail of both entry points. It is separate
// so the defensive canonical-length guard above the input bound stays
// exercisable without exposing a test-only input path.
func resolveLocaleParts(parts localeParts, authored bool) LocaleResult {
	if issue := validateLocaleRegistry(parts, authored); issue != nil {
		return localeRefusal(*issue)
	}
	canonical := canonicalizeLocaleParts(parts)
	tag, ok := renderLocaleParts(canonical)
	if !ok {
		return localeRefusal(LocaleIssue{
			Code:    LocaleCodeTooLong,
			Message: fmt.Sprintf("canonical locale exceeds %d characters", MaxLocaleCharacters),
		})
	}
	return localeSuccess(tag, canonicalPrimaryLanguage(canonical))
}

func localeSuccess(tag, primary string) LocaleResult {
	return LocaleResult{Accepted: true, Tag: tag, PrimaryLanguage: primary}
}

func localeRefusal(issue LocaleIssue) LocaleResult {
	return LocaleResult{Issues: []LocaleIssue{issue}}
}

// localeSet is the immutable membership set generated from one registry type.
type localeSet struct{ values map[string]struct{} }

func newLocaleSet(packed string) localeSet {
	values := make(map[string]struct{})
	for _, value := range strings.Fields(packed) {
		values[value] = struct{}{}
	}
	return localeSet{values: values}
}

func (s localeSet) has(value string) bool {
	_, ok := s.values[value]
	return ok
}

// localeRange is one inclusive registered private-use range.
type localeRange struct{ low, high string }

func localeRangeContains(ranges []localeRange, value string) bool {
	for _, item := range ranges {
		if value >= item.low && value <= item.high {
			return true
		}
	}
	return false
}

// localeWholeTagRecord is one registered grandfathered or redundant tag.
type localeWholeTagRecord struct {
	tag       string
	preferred string
}

var localeWholeTags = buildLocaleWholeTags()

func buildLocaleWholeTags() map[string]localeWholeTagRecord {
	index := make(
		map[string]localeWholeTagRecord,
		len(localeGrandfatheredRecords)+len(localeRedundantRecords),
	)
	for _, records := range [][]localeWholeTagRecord{
		localeGrandfatheredRecords,
		localeRedundantRecords,
	} {
		for _, record := range records {
			index[strings.ToLower(record.tag)] = record
		}
	}
	return index
}

type localeExtension struct {
	singleton string
	payload   []string
}

type localeParts struct {
	language    string
	extlang     string
	script      string
	region      string
	variants    []string
	extensions  []localeExtension
	privateUse  []string
	privateOnly bool
}

// parseLocaleSyntax checks the pinned ASCII BCP-47 grammar and structure. It
// deliberately knows nothing about registry membership, so it can also parse a
// regular grandfathered tag for its genuine primary language.
func parseLocaleSyntax(input string) (localeParts, *LocaleIssue) {
	var parts localeParts
	segments := strings.Split(input, "-")
	for _, segment := range segments {
		if segment == "" {
			return parts, localeMalformed("locale contains an empty subtag")
		}
		if !localeAlphanumeric(segment) {
			return parts, localeMalformed(
				"subtag %q must contain only ASCII letters and digits",
				segment,
			)
		}
	}

	if strings.EqualFold(segments[0], "x") {
		if len(segments) == 1 {
			return parts, localeMalformed(
				"private-use sequence needs at least one subtag",
			)
		}
		parts.privateOnly = true
		for _, segment := range segments[1:] {
			if len(segment) > 8 {
				return parts, localeMalformed(
					"private-use subtag %q exceeds 8 characters",
					segment,
				)
			}
			parts.privateUse = append(parts.privateUse, strings.ToLower(segment))
		}
		return parts, nil
	}

	language := strings.ToLower(segments[0])
	if len(language) < 2 || len(language) > 8 || !localeAlphabetic(language) {
		return parts, localeMalformed(
			"primary language subtag %q must be 2 to 8 ASCII letters",
			segments[0],
		)
	}
	parts.language = language
	index := 1

	if len(language) <= 3 && index < len(segments) && localeExtlangShape(segments[index]) {
		parts.extlang = strings.ToLower(segments[index])
		index++
		if index < len(segments) && localeExtlangShape(segments[index]) {
			return parts, &LocaleIssue{
				Code:   LocaleCodeReservedExtlangPosition,
				Subtag: strings.ToLower(segments[index]),
				Message: fmt.Sprintf(
					"extended language subtag %q is in a permanently reserved position",
					segments[index],
				),
			}
		}
	}

	if index < len(segments) && localeScriptShape(segments[index]) {
		parts.script = titleCaseASCII(segments[index])
		index++
	}
	if index < len(segments) && localeRegionShape(segments[index]) {
		parts.region = strings.ToUpper(segments[index])
		index++
	}

	seenVariants := make(map[string]bool)
	for index < len(segments) && localeVariantShape(segments[index]) {
		variant := strings.ToLower(segments[index])
		if seenVariants[variant] {
			return parts, &LocaleIssue{
				Code:   LocaleCodeDuplicateVariant,
				Subtag: variant,
				Message: fmt.Sprintf(
					"variant subtag %q appears more than once",
					variant,
				),
			}
		}
		seenVariants[variant] = true
		parts.variants = append(parts.variants, variant)
		index++
	}

	seenSingletons := make(map[string]bool)
	for index < len(segments) {
		segment := segments[index]
		if len(segment) != 1 {
			return parts, localeMalformed(
				"unexpected subtag %q after variants",
				segment,
			)
		}
		singleton := strings.ToLower(segment)
		if singleton == "x" {
			index++
			if index >= len(segments) {
				return parts, localeMalformed(
					"private-use sequence needs at least one subtag",
				)
			}
			for ; index < len(segments); index++ {
				if len(segments[index]) > 8 {
					return parts, localeMalformed(
						"private-use subtag %q exceeds 8 characters",
						segments[index],
					)
				}
				parts.privateUse = append(parts.privateUse, strings.ToLower(segments[index]))
			}
			break
		}
		if seenSingletons[singleton] {
			return parts, &LocaleIssue{
				Code:   LocaleCodeDuplicateSingleton,
				Subtag: singleton,
				Message: fmt.Sprintf(
					"extension singleton %q appears more than once",
					singleton,
				),
			}
		}
		seenSingletons[singleton] = true
		index++
		if index >= len(segments) || len(segments[index]) == 1 {
			return parts, localeMalformed(
				"extension singleton %q needs at least one 2-8 character subtag",
				singleton,
			)
		}
		extension := localeExtension{singleton: singleton}
		for index < len(segments) && len(segments[index]) != 1 {
			payload := segments[index]
			if len(payload) < 2 || len(payload) > 8 {
				return parts, localeMalformed(
					"extension subtag %q must be 2 to 8 characters",
					payload,
				)
			}
			extension.payload = append(extension.payload, strings.ToLower(payload))
			index++
		}
		parts.extensions = append(parts.extensions, extension)
	}

	return parts, nil
}

func localeMalformed(format string, args ...any) *LocaleIssue {
	return &LocaleIssue{
		Code:    LocaleCodeMalformed,
		Message: fmt.Sprintf(format, args...),
	}
}

// validateLocaleRegistry applies the snapshot membership rules. Visitor
// normalization only enforces registered relationships: an unknown but
// well-formed value stays available for selection.
func validateLocaleRegistry(parts localeParts, authored bool) *LocaleIssue {
	if authored && !parts.privateOnly &&
		!localeLanguages.has(parts.language) &&
		!localeRangeContains(localePrivateUseLanguageRanges, parts.language) {
		return unsupportedLocaleValue("language", parts.language)
	}

	if parts.extlang != "" {
		prefix, registered := localeExtlangPrefixes[parts.extlang]
		switch {
		case registered && prefix != parts.language:
			return &LocaleIssue{
				Code:   LocaleCodeExtlangPrefixMismatch,
				Subtag: parts.extlang,
				Message: fmt.Sprintf(
					"extended language subtag %q requires primary language %q",
					parts.extlang,
					prefix,
				),
			}
		case !registered && authored:
			return unsupportedLocaleValue("extended language", parts.extlang)
		}
	}

	if authored && parts.script != "" &&
		!localeScripts.has(parts.script) &&
		!localeRangeContains(localePrivateUseScriptRanges, parts.script) {
		return unsupportedLocaleValue("script", parts.script)
	}
	if authored && parts.region != "" &&
		!localeRegions.has(parts.region) &&
		!localeRangeContains(localePrivateUseRegionRanges, parts.region) {
		return unsupportedLocaleValue("region", parts.region)
	}
	if authored {
		for _, variant := range parts.variants {
			if !localeVariants.has(variant) {
				return unsupportedLocaleValue("variant", variant)
			}
		}
		for _, extension := range parts.extensions {
			if !localeExtensionSingletons.has(extension.singleton) {
				return &LocaleIssue{
					Code:     LocaleCodeUnsupportedRegistryVersion,
					Subtag:   extension.singleton,
					Snapshot: LocaleExtensionsRegistryFileDate,
					Message: fmt.Sprintf(
						"extension singleton %q is not registered in the IANA Language Tag Extensions Registry %s",
						extension.singleton,
						LocaleExtensionsRegistryFileDate,
					),
				}
			}
		}
	}

	return nil
}

func unsupportedLocaleValue(kind, subtag string) *LocaleIssue {
	return &LocaleIssue{
		Code:     LocaleCodeUnsupportedRegistryVersion,
		Subtag:   subtag,
		Snapshot: LocaleRegistryFileDate,
		Message: fmt.Sprintf(
			"%s subtag %q is not recognized in the IANA Language Subtag Registry %s",
			kind,
			subtag,
			LocaleRegistryFileDate,
		),
	}
}

// canonicalizeLocaleParts applies whole-subtag Preferred-Value mappings,
// contextual extlang replacement, extension-singleton ordering and ASCII case
// conventions. Explicit scripts, variant order and the private-use tail are
// preserved.
func canonicalizeLocaleParts(parts localeParts) localeParts {
	canonical := localeParts{
		language:    parts.language,
		extlang:     parts.extlang,
		script:      parts.script,
		region:      parts.region,
		variants:    append([]string(nil), parts.variants...),
		extensions:  make([]localeExtension, len(parts.extensions)),
		privateUse:  append([]string(nil), parts.privateUse...),
		privateOnly: parts.privateOnly,
	}
	if preferred, ok := localePreferredLanguages[canonical.language]; ok {
		canonical.language = preferred
	}
	if canonical.extlang != "" {
		if _, registered := localeExtlangPrefixes[canonical.extlang]; registered {
			canonical.language = canonical.extlang
			if preferred, ok := localePreferredLanguages[canonical.language]; ok {
				canonical.language = preferred
			}
			canonical.extlang = ""
		}
	}
	if preferred, ok := localePreferredScripts[canonical.script]; ok {
		canonical.script = preferred
	}
	if preferred, ok := localePreferredRegions[canonical.region]; ok {
		canonical.region = preferred
	}
	for index, variant := range canonical.variants {
		if preferred, ok := localePreferredVariants[variant]; ok {
			canonical.variants[index] = preferred
		}
	}
	for index, extension := range parts.extensions {
		copied := localeExtension{
			singleton: extension.singleton,
			payload:   append([]string(nil), extension.payload...),
		}
		canonical.extensions[index] = copied
	}
	sort.Slice(canonical.extensions, func(left, right int) bool {
		return canonical.extensions[left].singleton < canonical.extensions[right].singleton
	})
	return canonical
}

func renderLocaleParts(parts localeParts) (string, bool) {
	segments := make([]string, 0, 8+len(parts.variants)+len(parts.privateUse))
	if parts.privateOnly {
		segments = append(segments, "x")
		segments = append(segments, parts.privateUse...)
	} else {
		segments = append(segments, parts.language)
		if parts.extlang != "" {
			segments = append(segments, parts.extlang)
		}
		if parts.script != "" {
			segments = append(segments, parts.script)
		}
		if parts.region != "" {
			segments = append(segments, parts.region)
		}
		segments = append(segments, parts.variants...)
		for _, extension := range parts.extensions {
			segments = append(segments, extension.singleton)
			segments = append(segments, extension.payload...)
		}
		if len(parts.privateUse) > 0 {
			segments = append(segments, "x")
			segments = append(segments, parts.privateUse...)
		}
	}
	tag := strings.Join(segments, "-")
	if len(tag) > MaxLocaleCharacters {
		return "", false
	}
	return tag, true
}

func canonicalPrimaryLanguage(parts localeParts) string {
	if parts.privateOnly || parts.language == "" {
		return ""
	}
	return parts.language
}

// localeSyntaxPrimaryLanguage returns the regular primary language of a
// registered whole tag that has no Preferred-Value. Irregular tags and
// private-use-only tags have no primary language.
func localeSyntaxPrimaryLanguage(tag string) string {
	parts, issue := parseLocaleSyntax(tag)
	if issue != nil {
		return ""
	}
	return canonicalPrimaryLanguage(parts)
}

func localeAlphanumeric(value string) bool {
	for index := 0; index < len(value); index++ {
		character := value[index]
		if !localeAlphaByte(character) && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func localeAlphabetic(value string) bool {
	for index := 0; index < len(value); index++ {
		if !localeAlphaByte(value[index]) {
			return false
		}
	}
	return true
}

func localeNumeric(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}
	return true
}

func localeAlphaByte(character byte) bool {
	return (character >= 'a' && character <= 'z') ||
		(character >= 'A' && character <= 'Z')
}

func localeExtlangShape(value string) bool {
	return len(value) == 3 && localeAlphabetic(value)
}

func localeScriptShape(value string) bool {
	return len(value) == 4 && localeAlphabetic(value)
}

func localeRegionShape(value string) bool {
	return (len(value) == 2 && localeAlphabetic(value)) ||
		(len(value) == 3 && localeNumeric(value))
}

func localeVariantShape(value string) bool {
	if len(value) < 4 || len(value) > 8 || !localeAlphanumeric(value) {
		return false
	}
	if value[0] >= '0' && value[0] <= '9' {
		return len(value) >= 4
	}
	return len(value) >= 5
}

func titleCaseASCII(value string) string {
	lower := strings.ToLower(value)
	return strings.ToUpper(lower[:1]) + lower[1:]
}
