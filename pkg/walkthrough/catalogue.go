package walkthrough

import "sort"

// NarrativeLocaleChoice is the deterministic result of /2 narrative locale
// selection: an absent visitor request uses the sidecar default, then exactly
// the declared requested tag, the declared primary-language tag and finally the
// declared fallback. Tags are canonical BCP-47 comparison keys.
type NarrativeLocaleChoice struct {
	RequestedLocale string
	SelectedLocale  string
	Matched         string // exact, primary or fallback
	FallbackLocale  string
}

// NarrativeLocaleResult is the outcome of one pure selection.
type NarrativeLocaleResult struct {
	Accepted bool
	Choice   NarrativeLocaleChoice
	Issues   []Issue
}

// Err returns nil for an accepted selection or the aggregate refusal.
func (r NarrativeLocaleResult) Err() error {
	if r.Accepted {
		return nil
	}
	return &Error{Issues: r.Issues}
}

// SelectNarrativeLocale applies the owner selection algorithm to canonical
// declared catalogue tags. request is nil when the visitor made no explicit
// choice; an empty string is an explicit malformed request, not absence.
func SelectNarrativeLocale(
	request *string,
	declaredLocales []string,
	defaultLocale string,
	fallbackLocale string,
) NarrativeLocaleResult {
	c := newChecker(true)
	declared := make([]string, 0, len(declaredLocales))
	seen := make(map[string]bool, len(declaredLocales))
	for index, raw := range declaredLocales {
		result := AdmitAuthoredLocale(raw)
		if !result.Accepted {
			c.localeIssues(pointerIndex("/declaredLocales", index), result)
			continue
		}
		if seen[result.Tag] {
			c.report(CodeDuplicateID, pointerIndex("/declaredLocales", index),
				"catalogue locale %q is repeated after canonicalization", result.Tag)
			continue
		}
		seen[result.Tag] = true
		declared = append(declared, result.Tag)
	}

	defaultResult := AdmitAuthoredLocale(defaultLocale)
	if !defaultResult.Accepted {
		c.localeIssues("/defaultLocale", defaultResult)
	}
	fallbackResult := AdmitAuthoredLocale(fallbackLocale)
	if !fallbackResult.Accepted {
		c.localeIssues("/fallbackLocale", fallbackResult)
	}
	if defaultResult.Accepted && !seen[defaultResult.Tag] {
		c.report(CodeInvalidReference, "/defaultLocale",
			"the default locale has no pinned catalogue")
	}
	if fallbackResult.Accepted && !seen[fallbackResult.Tag] {
		c.report(CodeInvalidReference, "/fallbackLocale",
			"the fallback locale has no pinned catalogue")
	}
	if len(c.issues) > 0 {
		return NarrativeLocaleResult{Issues: c.issues}
	}

	requested := defaultResult.Tag
	if request != nil {
		normalized := NormalizeVisitorLocale(*request)
		if !normalized.Accepted {
			c.localeIssues("/request", normalized)
			return NarrativeLocaleResult{Issues: c.issues}
		}
		requested = normalized.Tag
		choice := NarrativeLocaleChoice{
			RequestedLocale: requested,
			FallbackLocale:  fallbackResult.Tag,
			Matched:         "fallback",
			SelectedLocale:  fallbackResult.Tag,
		}
		if seen[requested] {
			choice.Matched = "exact"
			choice.SelectedLocale = requested
			return NarrativeLocaleResult{Accepted: true, Choice: choice}
		}
		if normalized.PrimaryLanguage != "" && seen[normalized.PrimaryLanguage] {
			choice.Matched = "primary"
			choice.SelectedLocale = normalized.PrimaryLanguage
			return NarrativeLocaleResult{Accepted: true, Choice: choice}
		}
		return NarrativeLocaleResult{Accepted: true, Choice: choice}
	}

	choice := NarrativeLocaleChoice{
		RequestedLocale: requested,
		SelectedLocale:  requested,
		Matched:         "exact",
		FallbackLocale:  fallbackResult.Tag,
	}
	return NarrativeLocaleResult{Accepted: true, Choice: choice}
}

func (c *checker) localeIssues(path string, result LocaleResult) {
	for _, issue := range result.Issues {
		c.report(Code(issue.Code), path, "%s", issue.Message)
	}
}

// Projection renders the stable selection view.
func (c NarrativeLocaleChoice) Projection() map[string]any {
	return map[string]any{
		"requestedLocale": c.RequestedLocale,
		"selectedLocale":  c.SelectedLocale,
		"matched":         c.Matched,
		"fallbackLocale":  c.FallbackLocale,
	}
}

// narrativeTextItem is one canonical text contribution. identical source+path
// aliases count once; conflicting pins at one path reject.
type narrativeTextItem struct {
	path          string
	sha256        string
	pinnedBytes   Integer
	actualBytes   int
	perAssetLimit bool
}

// accountNarrativeText returns the aggregate actual UTF-8 bytes of unique
// canonical source+path contributions. The declaring source pin is the same
// for every item, so identity is its safe project-relative path.
func accountNarrativeText(items []narrativeTextItem) (Integer, []Issue) {
	issues := make([]Issue, 0)
	seen := make(map[string]narrativeTextItem, len(items))
	total := int64(0)
	for _, item := range items {
		pinned, ok := integerAsInt(string(item.pinnedBytes), int64(MaxNarrativeTextBytes)+1)
		if !ok {
			issues = append(issues, Issue{
				Code:    CodeInvalidValue,
				Path:    "",
				Message: "a text asset byte count is not representable",
			})
			continue
		}
		if int64(item.actualBytes) != pinned {
			issues = append(issues, Issue{
				Code:    CodeByteCountMismatch,
				Path:    "",
				Message: "text asset " + item.path + " actual bytes disagree with its pin",
			})
			continue
		}
		if existing, exists := seen[item.path]; exists {
			if existing.sha256 != item.sha256 || existing.pinnedBytes != item.pinnedBytes {
				issues = append(issues, Issue{
					Code:    CodeMismatch,
					Path:    "",
					Message: "conflicting pins for canonical source+path " + item.path,
				})
			}
			continue
		}
		seen[item.path] = item
		if item.perAssetLimit && pinned > MaxTextAssetBytes {
			issues = append(issues, Issue{
				Code:    CodeTooLong,
				Path:    "",
				Message: "text asset " + item.path + " exceeds 64 KiB",
			})
			continue
		}
		total += pinned
	}
	if total > MaxNarrativeTextBytes {
		issues = append(issues, Issue{
			Code:    CodeTooLarge,
			Path:    "",
			Message: "narrative sidecar, catalogues and package text exceed 6 MiB",
		})
	}
	return Integer(itoa(total)), issues
}

func integerAsInt(lexeme string, max int64) (int64, bool) {
	value := int64(0)
	for index := 0; index < len(lexeme); index++ {
		digit := lexeme[index]
		if digit < '0' || digit > '9' {
			return 0, false
		}
		value = value*10 + int64(digit-'0')
		if value > max {
			return 0, false
		}
	}
	return value, true
}

func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	position := len(digits)
	for value > 0 {
		position--
		digits[position] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[position:])
}

// sortedCopy returns a sorted copy of a string slice for order-insensitive
// snapshot comparison.
func sortedCopy(values []string) []string {
	copied := append([]string{}, values...)
	sort.Strings(copied)
	return copied
}
