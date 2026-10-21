package datatug

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

func normalizedEnvironmentID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		if r == ' ' || r == '.' || r == '_' || r == '-' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
func environmentStage(id string) int {
	switch id {
	case "local", "localhost", "sandbox":
		return 0
	case "dev", "development", "demo":
		return 1
	case "qa", "test", "testing", "qualityassurance":
		return 2
	case "uat", "useracceptance", "stage", "staging", "preprod", "preproduction":
		return 3
	case "prod", "production", "live":
		return 5
	default:
		return 4
	}
}

// CompareEnvironmentIDs uses ASCII classification and Unicode scalar ordering.
func CompareEnvironmentIDs(a, b string) (int, error) {
	if !utf8.ValidString(a) || !utf8.ValidString(b) {
		return 0, fmt.Errorf("invalid Unicode environment ID")
	}
	x, y := normalizedEnvironmentID(a), normalizedEnvironmentID(b)
	if d := environmentStage(x) - environmentStage(y); d != 0 {
		return d, nil
	}
	if d := strings.Compare(x, y); d != 0 {
		return d, nil
	}
	return strings.Compare(a, b), nil
}

// ResolveEnvironmentOrder retains each live ID once, appending defaults for missing IDs.
func ResolveEnvironmentOrder(ids, saved []string) (order []string, needsRepair bool, err error) {
	live := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !utf8.ValidString(id) || live[id] {
			return nil, false, fmt.Errorf("invalid or duplicate live environment ID")
		}
		live[id] = true
	}
	order = make([]string, 0, len(ids))
	for _, id := range saved {
		if !utf8.ValidString(id) {
			return nil, false, fmt.Errorf("invalid Unicode environment ID")
		}
		if live[id] {
			order = append(order, id)
			delete(live, id)
		}
	}
	missing := make([]string, 0, len(live))
	for id := range live {
		missing = append(missing, id)
	}
	sort.Slice(missing, func(i, j int) bool { d, _ := CompareEnvironmentIDs(missing[i], missing[j]); return d < 0 })
	order = append(order, missing...)
	if saved != nil {
		needsRepair = len(saved) != len(order)
		if !needsRepair {
			for i, id := range saved {
				if id != order[i] {
					needsRepair = true
					break
				}
			}
		}
	}
	return
}

// ValidateEnvironmentPermutation rejects partial, duplicate, and unknown IDs.
func ValidateEnvironmentPermutation(ids, order []string) error {
	resolved, repair, err := ResolveEnvironmentOrder(ids, order)
	if err != nil {
		return err
	}
	if order == nil || repair || len(resolved) != len(order) {
		return fmt.Errorf("environment order must be a complete permutation")
	}
	return nil
}
