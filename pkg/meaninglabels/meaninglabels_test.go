package meaninglabels

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

var t0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
var owner = Actor{ID: "user:alex"}
var agent = Actor{ID: "agent:claude", Agent: true, AgentUser: "user:alex"}

func col(table, column string) Target { return Target{Source: "nw", Table: table, Column: column} }
func tbl(table string) Target         { return Target{Source: "nw", Table: table} }

func mustApply(t *testing.T, doc Document, c Change, a Actor) (Document, Event) {
	t.Helper()
	out, ev, err := Apply(doc, c, a, t0)
	if err != nil {
		t.Fatalf("apply %+v: %v", c, err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("result invalid: %v", err)
	}
	return out, ev
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	if ErrorCode(err) != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func byColumn(labels []ColumnLabel) map[string]ColumnLabel {
	m := map[string]ColumnLabel{}
	for _, l := range labels {
		m[l.Column] = l
	}
	return m
}

func cols(names ...string) []Column {
	out := make([]Column, len(names))
	for i, n := range names {
		out[i] = Column{Name: n, DbType: "text"}
	}
	return out
}

func TestClassesAndKinds(t *testing.T) {
	if Class("x").Rank() != -1 || ClassNone.Protected() || !ClassPII.Protected() {
		t.Fatal("rank")
	}
	wantCode(t, ValidateClassKind("confidential", ""), CodeClassUnknown)
	wantCode(t, ValidateClassKind(ClassPII, "card-number"), CodeKindClassMismatch)
	if ValidateClassKind(ClassFinancial, "card-number") != nil || ValidateClassKind(ClassNone, "") != nil {
		t.Fatal("valid kinds refused")
	}
	e := &Error{Code: "C", Message: "m"}
	if e.Error() != "C: m" || ErrorCode(errors.New("x")) != "" {
		t.Fatal("error")
	}
}

func TestTargets(t *testing.T) {
	if col("t", "c").Kind() != TargetColumn || tbl("t").Kind() != TargetTable || (Target{Table: "t"}).Kind() != "" {
		t.Fatal("kind")
	}
	wantCode(t, Target{}.Validate(), CodeInvalidTarget)
	wantCode(t, Target{Source: "nw", Table: " t"}.Validate(), CodeInvalidTarget)
	if col("t", "c").String() != "nw/t.c" || tbl("t").String() != "nw/t" {
		t.Fatal("string")
	}
	if !TableMatches("customers", "public.customers") || !TableMatches("Public.Customers", "public.customers") ||
		!TableMatches("dbo.Customers", "Customers") || !TableMatches("dbo.Customers", "Northwind.dbo.Customers") ||
		TableMatches("sales.customers", "public.customers") || TableMatches("customers", "orders") {
		t.Fatal("table matches")
	}
}

func TestDocumentValidation(t *testing.T) {
	d := NewDocument()
	if d.Validate() != nil || d.Revision != "" {
		t.Fatal("new document invalid")
	}
	bad := func(mut func(*Document), code string) {
		t.Helper()
		x := NewDocument()
		mut(&x)
		x.Revision = x.ComputeRevision()
		err := x.Validate()
		if code == "" {
			if err == nil {
				t.Fatal("want error")
			}
			return
		}
		wantCode(t, err, code)
	}
	l := Label{Target: col("t", "c"), Class: ClassPII, State: StateConfirmed, Origin: OriginOwner, At: t0}
	bad(func(x *Document) { x.Format = "x" }, CodeInvalidDocument)
	bad(func(x *Document) { x.Labels = []Label{{Target: Target{}}} }, CodeInvalidTarget)
	bad(func(x *Document) { x.Labels = []Label{{Target: col("t", "c"), Class: "x"}} }, CodeClassUnknown)
	bad(func(x *Document) {
		x.Labels = []Label{{Target: col("t", "c"), Class: ClassSecret, State: StateConfirmed, Origin: OriginOwner}}
	}, CodeSecretNonRevealable)
	bad(func(x *Document) {
		x.Labels = []Label{{Target: col("t", "c"), Class: ClassNone, State: StateConfirmed, Origin: OriginProposed}}
	}, CodeInvalidDocument)
	bad(func(x *Document) {
		x.Labels = []Label{{Target: col("t", "c"), Class: ClassPII, State: StatePending, Origin: OriginOwner}}
	}, CodeInvalidDocument)
	bad(func(x *Document) { x.Labels = []Label{l, l} }, CodeInvalidDocument)
	bad(func(x *Document) { x.Dismissed = []Dismissal{{Target: Target{}}} }, "")
	bad(func(x *Document) { x.Dismissed = []Dismissal{{Target: col("t", "c"), Class: "x"}} }, "")
	bad(func(x *Document) { x.ClassPolicies = []ClassPolicy{{Class: ClassPII}} }, CodeInvalidDocument)
	bad(func(x *Document) { x.ClassPolicies = []ClassPolicy{{Source: "nw", Class: ClassSecret}} }, CodeSecretNonRevealable)
	x := NewDocument()
	x.Revision = "stale"
	wantCode(t, x.Validate(), CodeInvalidDocument)
	p := []ClassPolicy{{Source: "nw", Class: ClassPII, NonRevealable: true}}
	if (Document{ClassPolicies: p}).ComputeRevision() != (Document{Labels: []Label{}, ClassPolicies: p}).ComputeRevision() || (Document{}).ComputeRevision() != "" {
		t.Fatal("nil and empty labels must hash equally")
	}
}

func TestOwnerSetConfirmDismissClear(t *testing.T) {
	d := NewDocument()
	_, _, err := Apply(d, Change{Action: ActionSet, Target: col("t", "c"), Class: ClassPII}, owner, t0)
	wantCode(t, err, CodeReasonRequired)
	_, _, err = Apply(d, Change{Action: ActionSet, Target: col("t", "c"), Class: "x", Reason: "r"}, owner, t0)
	wantCode(t, err, CodeClassUnknown)
	_, _, err = Apply(d, Change{Action: ActionSet, Target: Target{}, Class: ClassPII, Reason: "r"}, owner, t0)
	wantCode(t, err, CodeInvalidTarget)
	_, _, err = Apply(d, Change{Action: ActionSet, Target: col("t", "c"), Class: ClassNone, NonRevealable: true, Reason: "r"}, owner, t0)
	wantCode(t, err, CodeInvalidDocument)
	_, _, err = Apply(d, Change{Action: "nope", Target: col("t", "c"), Reason: "r"}, owner, t0)
	wantCode(t, err, CodeUnknownAction)

	d, ev := mustApply(t, d, Change{Action: ActionSet, Target: col("api_tokens", "secret"), Class: ClassSecret, Kind: "token", Reason: "keys"}, owner)
	if ev.Type != "meaning.label.set" || !ev.After.NonRevealable || ev.Loosens {
		t.Fatalf("secret set: %+v", ev)
	}
	_, _, err = Apply(d, Change{Action: ActionSet, Target: col("api_tokens", "secret"), Class: ClassSecret, Reason: "x"}, owner, t0)
	wantCode(t, err, CodeSecretNonRevealable)
	d, ev = mustApply(t, d, Change{Action: ActionSet, Target: col("api_tokens", "secret"), Class: ClassPII, Reason: "lower"}, owner)
	if !ev.Loosens || ev.Before.Class != ClassSecret {
		t.Fatalf("lowering must loosen: %+v", ev)
	}
	d, ev = mustApply(t, d, Change{Action: ActionSet, Target: tbl("users"), Class: ClassPII, Reason: "people"}, owner)
	if ev.Loosens {
		t.Fatal("table pii does not loosen")
	}
	// confirm a read-time suggestion
	_, _, err = Apply(d, Change{Action: ActionConfirm, Target: col("users", "email"), Reason: "r"}, owner, t0)
	wantCode(t, err, CodeSuggestionNotPending)
	_, _, err = Apply(d, Change{Action: ActionConfirm, Target: col("users", "email"), Class: ClassPII, Kind: "card-number", Reason: "r"}, owner, t0)
	wantCode(t, err, CodeKindClassMismatch)
	d, ev = mustApply(t, d, Change{Action: ActionConfirm, Target: col("users", "email"), Class: ClassPII, Kind: "email", Reason: "yes"}, owner)
	if ev.Type != "meaning.label.confirmed" || ev.After.Origin != OriginOwner || ev.After.Reason != "yes" {
		t.Fatalf("confirm: %+v", ev)
	}
	d, _ = mustApply(t, d, Change{Action: ActionConfirm, Target: col("users", "pwd"), Class: ClassSecret, Reason: "r"}, owner)
	// dismiss a read-time suggestion
	_, _, err = Apply(d, Change{Action: ActionDismiss, Target: col("w", "address"), Reason: "r"}, owner, t0)
	wantCode(t, err, CodeSuggestionNotPending)
	_, _, err = Apply(d, Change{Action: ActionDismiss, Target: col("w", "address"), Class: ClassPII, Kind: "iban", Reason: "r"}, owner, t0)
	wantCode(t, err, CodeKindClassMismatch)
	d, ev = mustApply(t, d, Change{Action: ActionDismiss, Target: col("w", "address"), Class: ClassPII, Kind: "postal-address", Reason: "business"}, owner)
	if !ev.Loosens || len(d.Dismissed) != 1 {
		t.Fatalf("dismiss: %+v", ev)
	}
	_, _, err = Apply(d, Change{Action: ActionDismiss, Target: col("w", "address"), Class: ClassPII, Reason: "again"}, owner, t0)
	wantCode(t, err, CodeSuggestionNotPending)
	// clear owner label, then the dismissal, then nothing
	d, ev = mustApply(t, d, Change{Action: ActionClear, Target: tbl("users"), Reason: "r"}, owner)
	if ev.Type != "meaning.label.cleared" || !ev.Loosens {
		t.Fatalf("clear: %+v", ev)
	}
	d, ev = mustApply(t, d, Change{Action: ActionClear, Target: col("w", "address"), Reason: "r"}, owner)
	if ev.Type != "meaning.label.dismissal_cleared" || len(d.Dismissed) != 0 {
		t.Fatalf("clear dismissal: %+v", ev)
	}
	_, _, err = Apply(d, Change{Action: ActionClear, Target: col("w", "address"), Reason: "r"}, owner, t0)
	wantCode(t, err, CodeLabelNotFound)
}

func TestAgentOnlyProposes(t *testing.T) {
	d := NewDocument()
	_, _, err := Apply(d, Change{Action: ActionClear, Target: col("customers", "email"), Reason: "r"}, agent, t0)
	wantCode(t, err, CodeAgentNotAllowed)
	_, _, err = Apply(d, Change{Action: ActionPropose, Target: col("s", "homepage"), Class: ClassNone}, agent, t0)
	wantCode(t, err, CodeClassUnknown)
	_, _, err = Apply(d, Change{Action: ActionPropose, Target: col("s", "homepage"), Class: ClassPII, Kind: "iban"}, agent, t0)
	wantCode(t, err, CodeKindClassMismatch)
	d, ev := mustApply(t, d, Change{Action: ActionPropose, Target: col("s", "homepage"), Class: ClassPII, Kind: "phone"}, agent)
	if ev.Type != "meaning.label.proposed" || ev.After.State != StatePending {
		t.Fatalf("propose: %+v", ev)
	}
	_, ev = mustApply(t, d, Change{Action: ActionPropose, Target: col("s", "homepage"), Class: ClassPII}, agent)
	if ev.Type != "meaning.label.proposal_ignored" {
		t.Fatalf("equal proposal must be ignored: %+v", ev)
	}
	d, ev = mustApply(t, d, Change{Action: ActionPropose, Target: col("s", "homepage"), Class: ClassSecret}, agent)
	if ev.Before == nil || !ev.After.NonRevealable {
		t.Fatalf("stricter proposal replaces: %+v", ev)
	}
	// owner dismisses the stored proposal
	d, ev = mustApply(t, d, Change{Action: ActionDismiss, Target: col("s", "homepage"), Reason: "not a secret"}, owner)
	if ev.Before == nil || len(d.Labels) != 0 {
		t.Fatalf("dismiss proposal: %+v", ev)
	}
	// proposal confirmed by the owner replaces the owner label
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: col("u", "code"), Class: ClassPII, Reason: "r"}, owner)
	d, _ = mustApply(t, d, Change{Action: ActionPropose, Target: col("u", "code"), Class: ClassPII}, agent)
	d, _ = mustApply(t, d, Change{Action: ActionPropose, Target: col("u", "code"), Class: ClassSecret, Kind: "token"}, agent)
	if len(d.Labels) != 2 {
		t.Fatalf("owner label and proposal coexist: %+v", d.Labels)
	}
	d, ev = mustApply(t, d, Change{Action: ActionConfirm, Target: col("u", "code"), Reason: "agreed"}, owner)
	if len(d.Labels) != 1 || d.Labels[0].Class != ClassSecret || ev.Before.Class != ClassPII {
		t.Fatalf("confirm proposal: %+v %+v", d.Labels, ev)
	}
	// owner setting again settles a pending proposal
	d, _ = mustApply(t, d, Change{Action: ActionPropose, Target: col("u", "x"), Class: ClassPII}, agent)
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: col("u", "x"), Class: ClassFinancial, Reason: "r"}, owner)
	if len(d.Labels) != 2 {
		t.Fatalf("set must settle the proposal: %+v", d.Labels)
	}
}

func TestClassPolicy(t *testing.T) {
	d := NewDocument()
	_, _, err := Apply(d, Change{Action: ActionClassMark, Target: Target{Source: "nw"}, Class: ClassNone, Reason: "r"}, owner, t0)
	wantCode(t, err, CodeInvalidDocument)
	_, _, err = Apply(d, Change{Action: ActionClassMark, Target: Target{Source: "nw"}, Class: ClassSecret, Reason: "r"}, owner, t0)
	wantCode(t, err, CodeSecretNonRevealable)
	d, ev := mustApply(t, d, Change{Action: ActionClassMark, Target: Target{Source: "nw"}, Class: ClassPII, NonRevealable: true, Reason: "r"}, owner)
	if ev.Loosens || len(d.ClassPolicies) != 1 {
		t.Fatal("class policy set")
	}
	d, _ = mustApply(t, d, Change{Action: ActionClassMark, Target: Target{Source: "other"}, Class: ClassPII, NonRevealable: true, Reason: "r"}, owner)
	got := byColumn(Effective(d, "nw", "customers", cols("email")))["email"]
	if !got.NonRevealable || got.Revealable() {
		t.Fatalf("class policy makes non-revealable: %+v", got)
	}
	d, ev = mustApply(t, d, Change{Action: ActionClassMark, Target: Target{Source: "nw"}, Class: ClassPII, Reason: "r"}, owner)
	if !ev.Loosens || len(d.ClassPolicies) != 1 {
		t.Fatalf("removing the policy loosens: %+v", ev)
	}
}

func TestPrecedence(t *testing.T) {
	d := NewDocument()
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: tbl("customers"), Class: ClassPII, Reason: "people"}, owner)
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: col("customers", "country"), Class: ClassNone, Reason: "public"}, owner)
	got := byColumn(Effective(d, "nw", "customers", cols("country", "card_number", "password_hash", "nickname", "ssn")))
	if got["country"].Class != ClassNone || got["country"].Origin != FromColumn {
		t.Fatalf("country: %+v", got["country"])
	}
	if got["card_number"].Class != ClassFinancial || got["card_number"].Origin != FromSuggested || got["card_number"].State != StatePending || got["card_number"].Rule != "financial-card-number" {
		t.Fatalf("card: %+v", got["card_number"])
	}
	if got["password_hash"].Class != ClassSecret || got["password_hash"].Revealable() {
		t.Fatalf("hash: %+v", got["password_hash"])
	}
	if got["nickname"].Class != ClassPII || got["nickname"].Origin != FromTable {
		t.Fatalf("future column: %+v", got["nickname"])
	}
	if got["ssn"].Kind != "national-id" || got["ssn"].Class != ClassPII || got["ssn"].Origin != FromSuggested {
		t.Fatalf("ssn: %+v", got["ssn"])
	}
	// table none suppresses nothing
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: tbl("users"), Class: ClassNone, Reason: "reviewed"}, owner)
	if l := byColumn(Effective(d, "nw", "users", cols("api_key", "plan")))["api_key"]; l.Class != ClassSecret || l.Origin != FromSuggested {
		t.Fatalf("api_key: %+v", l)
	}
	if l := byColumn(Effective(d, "nw", "users", cols("plan")))["plan"]; l.Protected() || l.Class != "" {
		t.Fatalf("plan: %+v", l)
	}
	// a stricter proposal applies over an owner column label
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: col("users", "recovery_code"), Class: ClassPII, NonRevealable: true, Reason: "r"}, owner)
	before := byColumn(Effective(d, "nw", "users", cols("recovery_code")))["recovery_code"]
	if before.Class != ClassPII || !before.NonRevealable {
		t.Fatalf("before: %+v", before)
	}
	d, _ = mustApply(t, d, Change{Action: ActionPropose, Target: col("users", "recovery_code"), Class: ClassSecret, Kind: "token"}, agent)
	after := byColumn(Effective(d, "nw", "users", cols("recovery_code")))["recovery_code"]
	if after.Class != ClassSecret || after.Origin != FromProposed || after.State != StatePending {
		t.Fatalf("after: %+v", after)
	}
	// stricter-wins among contributors keeps non-revealable marks
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: tbl("vault"), Class: ClassPII, NonRevealable: true, Reason: "r"}, owner)
	d, _ = mustApply(t, d, Change{Action: ActionPropose, Target: col("vault", "note"), Class: ClassFinancial}, agent)
	d, _ = mustApply(t, d, Change{Action: ActionPropose, Target: tbl("vault"), Class: ClassPII}, agent)
	v := byColumn(Effective(d, "nw", "vault", cols("note")))["note"]
	if v.Class != ClassFinancial || !v.NonRevealable || v.Origin != FromProposed {
		t.Fatalf("vault: %+v", v)
	}
}

func TestLooseningMatchesExactly(t *testing.T) {
	d := NewDocument()
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: Target{Source: "nw", Table: "sales.customers", Column: "contact_name"}, Class: ClassNone, Reason: "company contact"}, owner)
	if l := byColumn(Effective(d, "nw", "sales.customers", cols("contact_name")))["contact_name"]; l.Protected() {
		t.Fatalf("exact none must decide: %+v", l)
	}
	if l := byColumn(Effective(d, "nw", "public.customers", cols("contact_name")))["contact_name"]; !l.Protected() {
		t.Fatalf("none must not widen to another schema: %+v", l)
	}
	// loosely matching owner column label contributes stricter-wins
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: col("orders", "ship_name"), Class: ClassFinancial, Reason: "r"}, owner)
	if l := byColumn(Effective(d, "nw", "public.orders", cols("ship_name")))["ship_name"]; l.Class != ClassFinancial || l.Origin != FromTable {
		t.Fatalf("loose owner label contributes: %+v", l)
	}
	// dismissal matches exactly
	d, _ = mustApply(t, d, Change{Action: ActionDismiss, Target: col("w", "address"), Class: ClassPII, Reason: "business"}, owner)
	if l := byColumn(Effective(d, "nw", "w", cols("address")))["address"]; l.Protected() {
		t.Fatalf("dismissed: %+v", l)
	}
	if l := byColumn(Effective(d, "nw", "W", cols("Address")))["Address"]; !l.Protected() {
		t.Fatalf("dismissal must not widen: %+v", l)
	}
	tables := map[string][]string{"sales.customers": {"contact_name"}, "public.customers": {"contact_name"}}
	wantCode(t, ExactTarget(col("customers", "contact_name"), tables), CodeTargetAmbiguous)
	wantCode(t, ExactTarget(Target{Source: "nw", Table: "sales.customers", Column: "Contact_Name"}, tables), CodeTargetAmbiguous)
	if ExactTarget(Target{Source: "nw", Table: "sales.customers", Column: "contact_name"}, tables) != nil ||
		ExactTarget(Target{Source: "nw", Table: "sales.customers"}, tables) != nil {
		t.Fatal("exact targets refused")
	}
	if CheckLooseningTarget(d, Target{Source: "nw", Table: "sales.customers", Column: "contact_name"}, nil) != nil {
		t.Fatal("a stored target may be loosened as written")
	}
	if CheckLooseningTarget(d, col("w", "address"), nil) != nil {
		t.Fatal("a stored dismissal may be cleared as written")
	}
	wantCode(t, CheckLooseningTarget(d, col("customers", "contact_name"), tables), CodeTargetAmbiguous)
}

func TestViewsAndTighten(t *testing.T) {
	d := NewDocument()
	if Reviewed(d, "nw", "v_contacts") {
		t.Fatal("unreviewed")
	}
	masked := byColumn(EffectiveObject(d, "nw", "v_contacts", true, cols("contact", "email")))
	if masked["contact"].Origin != FromUnreviewedObject || masked["email"].Origin != FromSuggested || masked["contact"].NonRevealable {
		t.Fatalf("view: %+v", masked)
	}
	if l := byColumn(EffectiveObject(d, "nw", "t", false, cols("contact")))["contact"]; l.Protected() {
		t.Fatalf("a table is not masked whole: %+v", l)
	}
	// a column label on a view does not mark it reviewed
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: col("v_contacts", "email"), Class: ClassSecret, Reason: "r"}, owner)
	if Reviewed(d, "nw", "v_contacts") {
		t.Fatal("column label must not review the view")
	}
	d, _ = mustApply(t, d, Change{Action: ActionClassMark, Target: Target{Source: "nw"}, Class: ClassPII, NonRevealable: true, Reason: "r"}, owner)
	if l := byColumn(EffectiveObject(d, "nw", "v_contacts", true, cols("contact")))["contact"]; !l.NonRevealable {
		t.Fatalf("class policy applies to an unreviewed view: %+v", l)
	}
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: tbl("v_contacts"), Class: ClassNone, Reason: "reviewed"}, owner)
	if !Reviewed(d, "nw", "v_contacts") || Reviewed(d, "nw", "V_Contacts") {
		t.Fatal("reviewed exact")
	}
	authoritative := []ColumnLabel{{Column: "email", Class: ClassPII}, {Column: "phone"}, {Column: "card", Class: ClassFinancial}}
	extra := []ColumnLabel{{Column: "EMAIL", Class: ClassNone}, {Column: "phone", Class: ClassPII, NonRevealable: true},
		{Column: "card", Class: ClassPII, NonRevealable: true}, {Column: "email", Class: ClassSecret, NonRevealable: true}}
	got := byColumn(Tighten(authoritative, extra))
	if got["email"].Class != ClassSecret || !got["phone"].NonRevealable || got["card"].Class != ClassFinancial || !got["card"].NonRevealable {
		t.Fatalf("tighten: %+v", got)
	}
	got = byColumn(Tighten([]ColumnLabel{{Column: "x", Class: ClassPII, NonRevealable: true}}, []ColumnLabel{{Column: "x", Class: ClassSecret}}))
	if !got["x"].NonRevealable {
		t.Fatal("tighten keeps non-revealable")
	}
}

func TestSuggest(t *testing.T) {
	cases := map[string]string{
		"password_hash": "secret-password-hash", "PwdSalt": "secret-password-hash", "hashed_password": "secret-password-hash",
		"Password": "secret-password", "api_key": "secret-api-key", "ClientSecret": "secret-api-key", "private_key": "secret-private-key",
		"totp_seed": "secret-otp-seed", "session_id": "secret-session-id", "refresh_token": "secret-token", "secret": "secret-token",
		"CardNumber": "financial-card-number", "pan": "financial-card-number", "cvv": "financial-card-cvv", "IBAN": "financial-iban",
		"sort_code": "financial-routing-number", "account_number": "financial-bank-account",
		"Email": "pii-email", "email_address": "pii-email", "HomePhone": "pii-phone", "Fax": "pii-phone", "SSN": "pii-national-id",
		"passport_no": "pii-passport-number", "BirthDate": "pii-birth-date", "ContactName": "pii-person-name", "LastName": "pii-person-name",
		"IPAddress": "pii-ip-address", "Address": "pii-postal-address", "PostalCode": "pii-postal-address", "latitude": "pii-geo-location",
	}
	for name, rule := range cases {
		r, ok := SuggestColumn(name, "varchar")
		if !ok || r.ID != rule {
			t.Errorf("%s: want %s, got %q (%v)", name, rule, r.ID, ok)
		}
	}
	for name, rule := range map[string]string{
		"created_by_email": "pii-email", "updated_by_ip": "pii-ip-address", "email_on_file": "pii-email", "phone_at_signup": "pii-phone",
		"name_on_card": "", "phonenumber": "pii-phone", "bank_account": "financial-bank-account", "signing_key": "secret-api-key",
		"encryption_key": "secret-api-key", "credentials": "secret-password", "salt": "secret-password-hash", "pin": "secret-password",
		"drivers_license": "pii-national-id", "passport_id": "pii-passport-number", "email_id": "pii-email", "card_holder_name": "pii-person-name", "session_id": "secret-session-id", "national_id": "pii-national-id",
	} {
		r, ok := SuggestColumn(name, "varchar")
		if rule == "" {
			continue
		}
		if !ok || r.ID != rule {
			t.Errorf("%s: want %s, got %q (%v)", name, rule, r.ID, ok)
		}
	}
	for _, name := range []string{"company", "token_count", "email_verified", "password_changed_at", "CompanyName", "id", "pandemic",
		"has_ssn", "is_email", "address_id", "billing_address_id", "mac_address", "phone_type"} {
		if r, ok := SuggestColumn(name, "varchar"); ok {
			t.Errorf("%s: unexpected %s", name, r.ID)
		}
	}
	if _, ok := SuggestColumn("email", "BOOLEAN"); ok {
		t.Error("boolean suggested")
	}
	if _, ok := SuggestColumn("pan", "bit"); ok {
		t.Error("bit suggested")
	}
	got := Suggest("nw", "customers", []Column{{Name: "Email", DbType: "text"}, {Name: "Country", DbType: "text"}})
	if len(got) != 1 || got[0].Target != col("customers", "Email") || got[0].Kind != "email" {
		t.Fatalf("suggest: %+v", got)
	}
	if NormalizeName("XMLHttpRequest-ID") != "xml_http_request_id" || NormalizeName("__a__b") != "a_b" {
		t.Fatalf("normalize: %q %q", NormalizeName("XMLHttpRequest-ID"), NormalizeName("__a__b"))
	}
}

func TestMaskPlan(t *testing.T) {
	labels := []ColumnLabel{
		{Column: "Email", Class: ClassPII, Kind: "email", Origin: FromSuggested},
		{Column: "secret", Class: ClassSecret, NonRevealable: true},
		{Column: "card", Class: ClassFinancial},
		{Column: "name"},
	}
	plan := NewPlan(labels, []string{"card", "SECRET"})
	for _, ref := range []string{`c.email`, `"Email"`, "[email]", "`EMAIL`", `c."Email"`} {
		if _, ok := plan.IsMasked(ref); !ok {
			t.Errorf("%s must be masked", ref)
		}
	}
	for ref, want := range map[string]string{`"e.mail"`: "e.mail", "[e.mail]": "e.mail", "`a.b`": "a.b", "t.c": "c", " c ": "c", "'q'": "q", `x"`: `x"`} {
		if got := BareName(ref); got != want {
			t.Errorf("BareName(%s) = %q, want %q", ref, got, want)
		}
	}
	if _, ok := NewPlan([]ColumnLabel{{Column: "e.mail", Class: ClassPII}}, nil).IsMasked(`t."e.mail"`); !ok {
		t.Error("quoted dotted name must be masked")
	}
	if p := NewPlan(labels, nil).Protection("secret"); p == nil || p.Ask != NeverText {
		t.Errorf("non-revealable ask: %+v", p)
	}
	twins := NewPlan([]ColumnLabel{{Column: "Email", Class: ClassPII}, {Column: "email", Class: ClassPII}}, nil)
	if len(twins.Columns()) != 2 {
		t.Fatalf("case twins must both be masked: %v", twins.Columns())
	}
	if plan.Empty() || len(plan.Columns()) != 2 || plan.Columns()[0] != "Email" {
		t.Fatalf("plan: %+v", plan.Columns())
	}
	if _, ok := plan.IsMasked("email"); !ok {
		t.Fatal("case-insensitive mask")
	}
	p := plan.Protection("EMAIL")
	if p == nil || p.Class != ClassPII || !p.Revealable || p.State != "masked" || p.Ask == "" || plan.Protection("name") != nil {
		t.Fatalf("protection: %+v", p)
	}
	if !NewPlan(nil, nil).Empty() {
		t.Fatal("empty")
	}
	wantCode(t, plan.CheckReferenced("customers", []string{"name", "email"}), CodeFieldProtected)
	if plan.CheckReferenced("customers", []string{"name"}) != nil {
		t.Fatal("unmasked reference refused")
	}
	if CheckReveal(labels, nil, true) != nil || CheckReveal(labels, []string{"card"}, false) != nil {
		t.Fatal("reveal refused")
	}
	wantCode(t, CheckReveal(labels, []string{"card"}, true), CodeRevealNotForAgent)
	wantCode(t, CheckReveal(labels, []string{"secret"}, false), CodeFieldNonRevealable)
	d := NewDocument()
	wantCode(t, RawRefusal(d, "nw", nil, true), CodeRawRefused)
	tables := map[string][]Column{"orders": cols("id", "total")}
	if AnyProtected(d, "nw", tables) || RawRefusal(d, "nw", tables, true) != nil {
		t.Fatal("unlabelled source")
	}
	tables["customers"] = cols("email")
	wantCode(t, RawRefusal(d, "nw", tables, true), CodeRawRefused)
	wantCode(t, RawRefusal(d, "nw", map[string][]Column{"orders": cols("id"), "hidden": {}}, true), CodeRawRefused)
	if RawRefusal(d, "nw", tables, false) != nil {
		t.Fatal("people keep raw locally")
	}
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: tbl("x"), Class: ClassPII, Reason: "r"}, owner)
	if !AnyProtected(d, "NW", nil) {
		t.Fatal("stored label")
	}
}

func TestStore(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	project := record.NewKeyWithID("projects", "demo")
	s := DALgoStore{DB: db, Key: Key(project)}
	empty, err := s.Load(ctx)
	if err != nil || empty.Revision != "" || len(empty.Labels) != 0 {
		t.Fatalf("empty load: %+v %v", empty, err)
	}
	doc, _ := mustApply(t, empty, Change{Action: ActionSet, Target: col("customers", "card_number"), Class: ClassFinancial, Kind: "card-number", Reason: "PAN"}, owner)
	if err := s.Save(ctx, doc, ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(ctx)
	if err != nil || got.Revision != doc.Revision || len(got.Labels) != 1 {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	wantCode(t, s.Save(ctx, doc, ""), CodeRevisionConflict)
	_, err = Encode(Document{Format: "bad"})
	wantCode(t, err, CodeInvalidDocument)
	wantCode(t, s.Save(ctx, Document{Format: "bad"}, ""), CodeInvalidDocument)
	big := NewDocument()
	for i := 0; i < 5000; i++ {
		big.Labels = append(big.Labels, Label{Target: Target{Source: "nw", Table: "t", Column: strings.Repeat("c", 200) + strconv.Itoa(i)},
			Class: ClassPII, State: StateConfirmed, Origin: OriginOwner, At: t0})
	}
	big.Revision = big.ComputeRevision()
	_, err = Encode(big)
	wantCode(t, err, CodeInvalidDocument)
	_, err = Decode(make([]byte, MaxDocumentBytes+1))
	wantCode(t, err, CodeInvalidDocument)
	_, err = Decode([]byte("{"))
	wantCode(t, err, CodeInvalidDocument)
	_, err = Decode([]byte(`{"format":"datatug-labels/v1","revision":"x"}`))
	wantCode(t, err, CodeInvalidDocument)
	_, err = Decode([]byte(`{"format":"datatug-labels/v1","labels":[],"future":1}`))
	wantCode(t, err, CodeInvalidDocument)
	// a corrupt stored document surfaces on load and on save
	if err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(Key(project), &stored{Content: []byte("{")}))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ctx); ErrorCode(err) != CodeInvalidDocument {
		t.Fatalf("corrupt load: %v", err)
	}
	wantCode(t, s.Save(ctx, doc, doc.Revision), CodeInvalidDocument)
}

type failingGetTx struct{ dal.ReadTransaction }

func (failingGetTx) Get(context.Context, record.Record) error { return errors.New("disk on fire") }

type failingGetDB struct{ dal.DB }

func (f failingGetDB) RunReadonlyTransaction(ctx context.Context, worker dal.ROTxWorker, options ...dal.TransactionOption) error {
	return f.DB.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return worker(ctx, failingGetTx{tx})
	}, options...)
}

func TestStoreGetError(t *testing.T) {
	db := failingGetDB{dalgo2memory.New(dalgo2memory.FirestoreProfile())}
	if _, err := (DALgoStore{DB: db, Key: Key(record.NewKeyWithID("projects", "p"))}).Load(context.Background()); err == nil || ErrorCode(err) != "" {
		t.Fatalf("want a driver error, got %v", err)
	}
}

func TestClearKeepsOtherDismissals(t *testing.T) {
	d := NewDocument()
	d, _ = mustApply(t, d, Change{Action: ActionDismiss, Target: col("w", "address"), Class: ClassPII, Reason: "r"}, owner)
	d, _ = mustApply(t, d, Change{Action: ActionDismiss, Target: col("w", "phone"), Class: ClassPII, Reason: "r"}, owner)
	d, _ = mustApply(t, d, Change{Action: ActionClear, Target: col("w", "address"), Reason: "r"}, owner)
	if len(d.Dismissed) != 1 || d.Dismissed[0].Target.Column != "phone" {
		t.Fatalf("dismissals: %+v", d.Dismissed)
	}
}

func TestDowngradesAreLoosening(t *testing.T) {
	d := NewDocument()
	d, _ = mustApply(t, d, Change{Action: ActionSet, Target: col("users", "api_key"), Class: ClassFinancial, NonRevealable: true, Reason: "r"}, owner)
	_, ev := mustApply(t, d, Change{Action: ActionConfirm, Target: col("users", "api_key"), Class: ClassPII, Reason: "r"}, owner)
	if !ev.Loosens {
		t.Fatal("confirm to a weaker class loosens")
	}
	d, ev = mustApply(t, d, Change{Action: ActionConfirm, Target: col("users", "api_key"), Class: ClassFinancial, Reason: "r"}, owner)
	if !ev.After.NonRevealable {
		t.Fatal("confirm keeps the owner's non-revealable mark")
	}
	_, ev = mustApply(t, d, Change{Action: ActionConfirm, Target: col("users", "token"), Class: ClassSecret, Reason: "r"}, owner)
	if ev.Loosens {
		t.Fatal("confirming secret on a column never loosens")
	}
	_, _, err := Apply(d, Change{Action: ActionConfirm, Target: col("users", "x"), Class: ClassPII}, owner, t0)
	wantCode(t, err, CodeReasonRequired)
	d, ev = mustApply(t, d, Change{Action: ActionSet, Target: tbl("vault"), Class: ClassSecret, Reason: "r"}, owner)
	if ev.Loosens {
		t.Fatal("a secret table label tightens")
	}
	_, ev = mustApply(t, d, Change{Action: ActionSet, Target: tbl("vault"), Class: ClassPII, Reason: "r"}, owner)
	if !ev.Loosens {
		t.Fatal("secret table to pii loosens")
	}
	_, ev = mustApply(t, d, Change{Action: ActionSet, Target: tbl("open"), Class: ClassNone, Reason: "r"}, owner)
	if !ev.Loosens {
		t.Fatal("a none table label is a review, treated as loosening")
	}
	_, ev = mustApply(t, d, Change{Action: ActionSet, Target: col("users", "notes"), Class: ClassPII, Reason: "r"}, owner)
	if !ev.Loosens {
		t.Fatal("an owner column label below secret may hide a suggestion")
	}
	_, _, err = Apply(d, Change{Action: ActionClassMark, Target: Target{Source: "nw", Table: "x"}, Class: ClassPII, Reason: "r"}, owner, t0)
	wantCode(t, err, CodeInvalidDocument)
	if (ColumnLabel{Class: ClassSecret}).Revealable() {
		t.Fatal("secret is never revealable")
	}
	empty, _ := mustApply(t, NewDocument(), Change{Action: ActionSet, Target: tbl("a"), Class: ClassPII, Reason: "r"}, owner)
	empty, _ = mustApply(t, empty, Change{Action: ActionClear, Target: tbl("a"), Reason: "r"}, owner)
	if empty.Revision != "" {
		t.Fatal("an emptied document has revision \"\"")
	}
}
