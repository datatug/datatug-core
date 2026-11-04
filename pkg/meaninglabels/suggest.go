package meaninglabels

import (
	"regexp"
	"strings"
	"unicode"
)

// Rule is one published suggestion rule. Pattern runs against a normalized
// column name: lower case words joined by "_" ("CardNumber" -> "card_number").
type Rule struct {
	ID      string
	Class   Class
	Kind    Kind
	Pattern *regexp.Regexp
}

func rule(id string, class Class, kind Kind, pattern string) Rule {
	return Rule{ID: id, Class: class, Kind: kind, Pattern: regexp.MustCompile(pattern)}
}

// Rules is the deterministic suggestion rule table, first match wins. It reads
// names and declared types only, never values.
var Rules = []Rule{
	rule("secret-password-hash", ClassSecret, "password-hash", `(^|_)(password|passwd|pwd|pass)_?(hash|hashed|digest|salt)($|_)|(^|_)(hashed_?password|salt)($|_)`),
	rule("secret-password", ClassSecret, "password", `(^|_)(password|passwd|pwd|pin|pin_?code|credentials?)($|_)`),
	rule("secret-api-key", ClassSecret, "api-key", `(^|_)(api_?key|apikey|access_?key|secret_?key|client_?secret|signing_?key|encryption_?key)($|_)`),
	rule("secret-private-key", ClassSecret, "private-key", `(^|_)private_?key($|_)`),
	rule("secret-otp-seed", ClassSecret, "otp-seed", `(^|_)(otp|totp|mfa)_?(seed|secret)($|_)`),
	rule("secret-session-id", ClassSecret, "session-id", `(^|_)session_?(id|key|token)($|_)`),
	rule("secret-token", ClassSecret, "token", `(^|_)(token|access_?token|refresh_?token|auth_?token|bearer|secret)($|_)`),
	rule("financial-card-number", ClassFinancial, "card-number", `(^|_)(card|cc|credit_?card)_?(number|num|no)($|_)|(^|_)pan($|_)`),
	rule("financial-card-cvv", ClassFinancial, "card-cvv", `(^|_)(cvv|cvv2|cvc|csc)($|_)`),
	rule("financial-iban", ClassFinancial, "iban", `(^|_)iban($|_)`),
	rule("financial-routing-number", ClassFinancial, "routing-number", `(^|_)(routing_?(number|no)|sort_?code|swift|bic)($|_)`),
	rule("financial-bank-account", ClassFinancial, "bank-account", `(^|_)(bank_?)?(account|acct)_?(number|num|no)($|_)|(^|_)bank_?account($|_)`),
	rule("pii-email", ClassPII, "email", `(^|_)e_?mail($|_)|(^|_)email_?address($|_)`),
	rule("pii-phone", ClassPII, "phone", `(^|_)(phone|phonenumber|mobile|fax|telephone|tel|cell)($|_)`),
	rule("pii-national-id", ClassPII, "national-id", `(^|_)(ssn|social_?security(_?number)?|national_?id|tax_?id|nin|sin|drivers?_?licen[cs]e(_?(number|no))?)($|_)`),
	rule("pii-passport-number", ClassPII, "passport-number", `(^|_)passport(_?(number|no))?($|_)`),
	rule("pii-birth-date", ClassPII, "birth-date", `(^|_)(birth_?date|date_?of_?birth|dob|birthday)($|_)`),
	rule("pii-person-name", ClassPII, "person-name", `(^|_)((first|last|full|middle|given|family|contact|sur|maiden|card_?holder|holder)_?name|surname|cardholder)($|_)`),
	rule("pii-ip-address", ClassPII, "ip-address", `(^|_)(ip|ip_?address|ipv4|ipv6)($|_)`),
	rule("pii-postal-address", ClassPII, "postal-address", `(^|_)(address\d?|street|postal_?code|zip|zip_?code|post_?code)($|_)`),
	rule("pii-geo-location", ClassPII, "geo-location", `(^|_)(latitude|longitude|lat|lng|lon|geo_?location)($|_)`),
}

// skip matches names that describe a sensitive value without holding it: a
// describing last token (email_verified, password_changed_at) or an is_/has_
// prefix (has_ssn). A trailing id is skipped too (address_id is a key, not an
// address) unless the rule itself names an identifier (session_id).
var skip = regexp.MustCompile(`(_(count|enabled|verified|flag|required|changed|updated|created|expires?|expiry|at|on|length|type|format|policy|hint)$)|(^(is|has)_)`)

var underscores = regexp.MustCompile(`_+`)

// idHoldsValue lists kinds whose "*_id" columns usually hold the value itself
// (session_id, national_id, passport_id, email_id), not a key to another row.
var idHoldsValue = map[Kind]bool{"session-id": true, "national-id": true, "passport-number": true, "email": true}

// notPersonal matches names that look like a rule's but are not about a person.
var notPersonal = regexp.MustCompile(`(^|_)mac_?address($|_)`)

// Column is a scanned column offered to Suggest.
type Column struct {
	Name   string
	DbType string
}

// Suggestion is a rule's suggestion for one column.
type Suggestion struct {
	Target Target `json:"target"`
	Class  Class  `json:"class"`
	Kind   Kind   `json:"kind"`
	Rule   string `json:"rule"`
}

// NormalizeName turns an identifier into lower-case words joined by "_".
func NormalizeName(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case unicode.IsUpper(r):
			if i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1]) ||
				(i+1 < len(runes) && unicode.IsLower(runes[i+1]) && unicode.IsUpper(runes[i-1]))) {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return strings.Trim(underscores.ReplaceAllString(b.String(), "_"), "_")
}

// SuggestColumn returns the first matching rule for one column, if any.
func SuggestColumn(name, dbType string) (Rule, bool) {
	t := strings.ToLower(dbType)
	if strings.Contains(t, "bool") || t == "bit" {
		return Rule{}, false
	}
	n := NormalizeName(name)
	if skip.MatchString(n) || notPersonal.MatchString(n) {
		return Rule{}, false
	}
	for _, r := range Rules {
		if r.Pattern.MatchString(n) {
			if strings.HasSuffix(n, "_id") && !idHoldsValue[r.Kind] {
				return Rule{}, false
			}
			return r, true
		}
	}
	return Rule{}, false
}

// Suggest runs the rule table over a table's columns.
func Suggest(source, table string, columns []Column) []Suggestion {
	var out []Suggestion
	for _, c := range columns {
		if r, ok := SuggestColumn(c.Name, c.DbType); ok {
			out = append(out, Suggestion{Target: Target{Source: source, Table: table, Column: c.Name}, Class: r.Class, Kind: r.Kind, Rule: r.ID})
		}
	}
	return out
}
