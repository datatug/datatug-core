package datatug

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/strongo/validation"
)

// storageProbe is the secret every refusal case below carries; a save-path
// test in pkg/storage/filestore uses the same idea to prove no file on
// disk ends up holding it.
const storageProbe = "s3cr3t-value"

// storageCredentialValue embeds storageProbe in a syntax both screens
// refuse: EmbeddedCredentialReason reads a URL password, and the
// identifier shape rule refuses the ':' it needs.
const storageCredentialValue = "postgres://u:" + storageProbe + "@h/db"

// queryDefWithEveryPersistedField is a realistic query that populates
// every string the query pair persists, using values that must all be
// accepted: a "user:<id>" folder, whose ':' belongs to the documented
// root-folder syntax and is stripped before that segment is screened;
// "key:value" tags and a provider-qualified user id, which hold a ':' the
// shape rule would refuse, which is why those two fields get the
// credential screen instead; a parameter named after a password reset; a
// title that talks about one; prose that mentions one; and a lookup SQL
// that binds a token as a parameter.
func queryDefWithEveryPersistedField() QueryDef {
	return QueryDef{
		ProjectItem: ProjectItem{
			Access:  "private",
			UserIDs: []string{"google:12345"},
			ProjItemBrief: ProjItemBrief{
				ID:         "customer-invoices",
				Title:      "Customer invoices",
				Folder:     "user:alex/reports",
				ListOfTags: ListOfTags{Tags: []string{"team:finance", "revenue"}},
			},
		},
		Type:    QueryTypeDTQL,
		Text:    "from: Invoice\n",
		Purpose: "Which customers reset their password last week, and did any invoice go unpaid?",
		Parameters: Parameters{
			{
				ID: "CustomerId", Type: "integer", IsRequired: true,
				Meta: &EntityFieldRef{Entity: "Customer", Field: "ID"},
			},
			{
				ID: "PasswordResetId", Type: "string", Title: "Password reset date",
				DefaultValue: "unpaid",
				Meta:         &EntityFieldRef{Entity: "PasswordReset", Field: "ID"},
				Lookup: &ParameterLookup{
					DB:        "chinook",
					SQL:       "SELECT MediaTypeId, Name FROM MediaType WHERE token = @token",
					KeyFields: []string{"MediaTypeId"},
				},
			},
		},
		Targets: []QueryDefTarget{{
			Driver: "sqlite3", Host: "db.example.com",
			Credentials: Credentials{Username: "reader@corp"},
		}},
		Recordsets: []RecordsetDefinition{{
			ProjectItem: ProjectItem{
				Access:  "protected",
				UserIDs: []string{"google:67890"},
				ProjItemBrief: ProjItemBrief{
					ID:         "invoices",
					Title:      "Invoices, with the password-reset audit row",
					Folder:     "~",
					ListOfTags: ListOfTags{Tags: []string{"env:prod"}},
				},
			},
			RecordsetBaseDef: RecordsetBaseDef{
				PrimaryKey: &UniqueKey{Name: "PK_Invoice", Columns: []string{"InvoiceId"}},
				ForeignKeys: ForeignKeys{{
					Name: "FK_Invoice_Customer", Columns: []string{"CustomerId"},
					MatchOption: "SIMPLE", UpdateRule: "NO_ACTION", DeleteRule: "CASCADE",
				}},
				AlternateKeys: []UniqueKey{{Name: "AK_Invoice_Number", Columns: []string{"Number"}}},
				ActiveIssues:  &Issues{Schema: []string{"the password column was dropped upstream"}},
			},
			Columns: RecordsetColumnDefs{{
				Name: "InvoiceId", Type: "integer",
				Meta:   &EntityFieldRef{Entity: "Invoice", Field: "ID"},
				HideIf: HideRecordsetColIf{Parameters: []string{"CustomerId"}},
			}},
			Type:       "json",
			JSONSchema: `{"type":"object","properties":{"password":{"type":"string"}}}`,
			Files:      []string{"recordsets/invoices.json"},
			Errors:     []string{"column Total is missing a not-null constraint"},
		}},
		Capture: validQueryCapture(),
	}
}

func TestQueryDefStorageScreen_AcceptsARealisticQuery(t *testing.T) {
	if err := queryDefWithEveryPersistedField().Validate(); err != nil {
		t.Fatalf("expected a realistic query populating every persisted field to be accepted, got: %v", err)
	}
}

// storageScreenCase is one persisted field, the JSON path it must be
// refused under, and how to write the probe into it.
type storageScreenCase struct {
	path   string
	mutate func(q *QueryDef)
}

// storageScreenCases covers every field screenQueryDefForStorage itself
// screens - the rows its table marks "identifier" or "credential" with no
// other owner. TestQueryDefStorageScreen_CoversEveryPersistedStringField
// checks this set against the table, so a field cannot be screened without
// a refusal test or tabled without a screen.
func storageScreenCases() []storageScreenCase {
	set := func(path string, mutate func(q *QueryDef)) storageScreenCase {
		return storageScreenCase{path: path, mutate: mutate}
	}
	rs := func(q *QueryDef) *RecordsetDefinition { return &q.Recordsets[0] }
	return []storageScreenCase{
		set("id", func(q *QueryDef) { q.ID = storageCredentialValue }),
		// A folder must still parse as a folder path, or ValidateFolderPath
		// refuses it first and this would prove nothing about screening.
		set("folder", func(q *QueryDef) { q.Folder = "user:alex/Password=" + storageProbe }),
		set("tags[0]", func(q *QueryDef) { q.Tags[0] = "Password=" + storageProbe }),
		set("userIds[0]", func(q *QueryDef) { q.UserIDs[0] = storageCredentialValue }),
		set("parameters[1].id", func(q *QueryDef) { q.Parameters[1].ID = storageCredentialValue }),
		set("parameters[1].type", func(q *QueryDef) { q.Parameters[1].Type = storageCredentialValue }),
		set("parameters[1].title", func(q *QueryDef) { q.Parameters[1].Title = storageCredentialValue }),
		set("parameters[1].meta.entity", func(q *QueryDef) { q.Parameters[1].Meta.Entity = storageCredentialValue }),
		set("parameters[1].meta.field", func(q *QueryDef) { q.Parameters[1].Meta.Field = storageCredentialValue }),
		set("parameters[1].lookup.db", func(q *QueryDef) { q.Parameters[1].Lookup.DB = storageCredentialValue }),
		set("parameters[1].lookup.sql", func(q *QueryDef) { q.Parameters[1].Lookup.SQL = storageCredentialValue }),
		set("parameters[1].lookup.keyFields[0]", func(q *QueryDef) { q.Parameters[1].Lookup.KeyFields[0] = storageCredentialValue }),
		set("recordsets[0].id", func(q *QueryDef) { rs(q).ID = storageCredentialValue }),
		set("recordsets[0].title", func(q *QueryDef) { rs(q).Title = storageCredentialValue }),
		set("recordsets[0].folder", func(q *QueryDef) { rs(q).Folder = storageCredentialValue }),
		set("recordsets[0].tags[0]", func(q *QueryDef) { rs(q).Tags[0] = storageCredentialValue }),
		set("recordsets[0].userIds[0]", func(q *QueryDef) { rs(q).UserIDs[0] = storageCredentialValue }),
		set("recordsets[0].access", func(q *QueryDef) { rs(q).Access = storageCredentialValue }),
		set("recordsets[0].type", func(q *QueryDef) { rs(q).Type = storageCredentialValue }),
		set("recordsets[0].jsonSchema", func(q *QueryDef) { rs(q).JSONSchema = storageCredentialValue }),
		set("recordsets[0].files[0]", func(q *QueryDef) { rs(q).Files[0] = storageCredentialValue }),
		set("recordsets[0].errors[0]", func(q *QueryDef) { rs(q).Errors[0] = storageCredentialValue }),
		set("recordsets[0].primaryKey.name", func(q *QueryDef) { rs(q).PrimaryKey.Name = storageCredentialValue }),
		set("recordsets[0].primaryKey.columns[0]", func(q *QueryDef) { rs(q).PrimaryKey.Columns[0] = storageCredentialValue }),
		set("recordsets[0].foreignKeys[0].name", func(q *QueryDef) { rs(q).ForeignKeys[0].Name = storageCredentialValue }),
		set("recordsets[0].foreignKeys[0].columns[0]", func(q *QueryDef) { rs(q).ForeignKeys[0].Columns[0] = storageCredentialValue }),
		set("recordsets[0].foreignKeys[0].matchOption", func(q *QueryDef) { rs(q).ForeignKeys[0].MatchOption = storageCredentialValue }),
		set("recordsets[0].foreignKeys[0].updateRule", func(q *QueryDef) { rs(q).ForeignKeys[0].UpdateRule = storageCredentialValue }),
		set("recordsets[0].foreignKeys[0].deleteRule", func(q *QueryDef) { rs(q).ForeignKeys[0].DeleteRule = storageCredentialValue }),
		set("recordsets[0].alternateKey[0].name", func(q *QueryDef) { rs(q).AlternateKeys[0].Name = storageCredentialValue }),
		set("recordsets[0].alternateKey[0].columns[0]", func(q *QueryDef) { rs(q).AlternateKeys[0].Columns[0] = storageCredentialValue }),
		set("recordsets[0].issues.schema[0]", func(q *QueryDef) { rs(q).ActiveIssues.Schema[0] = storageCredentialValue }),
		set("recordsets[0].columns[0].name", func(q *QueryDef) { rs(q).Columns[0].Name = storageCredentialValue }),
		set("recordsets[0].columns[0].type", func(q *QueryDef) { rs(q).Columns[0].Type = storageCredentialValue }),
		set("recordsets[0].columns[0].meta.entity", func(q *QueryDef) { rs(q).Columns[0].Meta.Entity = storageCredentialValue }),
		set("recordsets[0].columns[0].meta.field", func(q *QueryDef) { rs(q).Columns[0].Meta.Field = storageCredentialValue }),
		set("recordsets[0].columns[0].hideIf.parameters[0]", func(q *QueryDef) { rs(q).Columns[0].HideIf.Parameters[0] = storageCredentialValue }),
	}
}

// TestQueryDefStorageScreen_RefusesACredentialInEveryPersistedField is the
// refusal half of hub AC query-pair-storage-guards-writes for the fields
// no earlier check owned: a secret written into any of them is refused
// with a typed bad-record error naming that field, so it never reaches the
// git-tracked pair.
func TestQueryDefStorageScreen_RefusesACredentialInEveryPersistedField(t *testing.T) {
	for _, c := range storageScreenCases() {
		t.Run(c.path, func(t *testing.T) {
			q := queryDefWithEveryPersistedField()
			c.mutate(&q)
			err := q.Validate()
			if err == nil {
				t.Fatalf("expected a credential in %s to be refused", c.path)
			}
			if !validation.IsBadRecordError(err) {
				t.Errorf("expected a bad-record error, got %T: %v", err, err)
			}
			if !strings.Contains(err.Error(), c.path) {
				t.Errorf("refusal %q does not name %q", err, c.path)
			}
		})
	}
}

// TestQueryDefStorageScreen_IdentifiersGetTheShapeRule proves the
// identifier fields really are screened by shape and not merely by the
// credential screen: a control character carries no credential syntax at
// all, so only the shape rule refuses it.
func TestQueryDefStorageScreen_IdentifiersGetTheShapeRule(t *testing.T) {
	const unusable = "Customer\x07Id"
	if _, found := EmbeddedCredentialReason(unusable); found {
		t.Fatalf("%q is meant to be invisible to the credential screen; pick another probe", unusable)
	}
	for _, tt := range []struct {
		path   string
		mutate func(q *QueryDef)
	}{
		{"parameters[1].id", func(q *QueryDef) { q.Parameters[1].ID = unusable }},
		{"recordsets[0].columns[0].name", func(q *QueryDef) { q.Recordsets[0].Columns[0].Name = unusable }},
		{"recordsets[0].primaryKey.columns[0]", func(q *QueryDef) { q.Recordsets[0].PrimaryKey.Columns[0] = unusable }},
	} {
		t.Run(tt.path, func(t *testing.T) {
			q := queryDefWithEveryPersistedField()
			tt.mutate(&q)
			err := q.Validate()
			if err == nil {
				t.Fatalf("expected %q in %s to be refused", unusable, tt.path)
			}
			if !strings.Contains(err.Error(), identifierShapeControlReason) {
				t.Errorf("expected the shape rule's refusal, got: %v", err)
			}
			if !strings.Contains(err.Error(), tt.path) {
				t.Errorf("refusal %q does not name %q", err, tt.path)
			}
		})
	}
}

// TestQueryDefStorageScreen_AcceptsRealisticContent is the other half of
// the decision: the values a real project holds must survive, including
// the ones a credential screen would misread and the ones a shape rule
// would.
func TestQueryDefStorageScreen_AcceptsRealisticContent(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(q *QueryDef)
	}{
		{"a parameter id naming a password reset", func(q *QueryDef) { q.Parameters[1].ID = "PasswordResetId" }},
		{"a token-named parameter id", func(q *QueryDef) { q.Parameters[1].ID = "api_token_id" }},
		{"a parameter title about a password", func(q *QueryDef) { q.Parameters[1].Title = "Password reset date" }},
		{"a parameter title mentioning a token", func(q *QueryDef) { q.Parameters[1].Title = "Token issued at (UTC)" }},
		{"a description mentioning a password", func(q *QueryDef) {
			q.Purpose = "Find accounts whose password_hash is null so support can invite them to set one."
		}},
		{"a password-named entity reference", func(q *QueryDef) {
			q.Parameters[1].Meta = &EntityFieldRef{Entity: "PasswordReset", Field: "TokenId"}
		}},
		{"a lookup binding a token as a parameter", func(q *QueryDef) {
			q.Parameters[1].Lookup.SQL = "SELECT id, Name FROM ApiKey WHERE token = @token"
		}},
		{"a lookup db named after secrets", func(q *QueryDef) { q.Parameters[1].Lookup.DB = "secrets_readonly" }},
		{"a key:value tag", func(q *QueryDef) { q.Tags[0] = "team:finance" }},
		{"a user folder", func(q *QueryDef) { q.Folder = "user:alex/password resets" }},
		{"a provider-qualified user id", func(q *QueryDef) { q.UserIDs[0] = "google:12345" }},
		{"an e-mail user id", func(q *QueryDef) { q.UserIDs[0] = "alice@example.com" }},
		{"a column named password_hash", func(q *QueryDef) { q.Recordsets[0].Columns[0].Name = "password_hash" }},
		{"a key over a password column", func(q *QueryDef) {
			q.Recordsets[0].PrimaryKey = &UniqueKey{Name: "PK_PasswordReset", Columns: []string{"PasswordResetId"}}
		}},
		{"a recordset titled after passwords", func(q *QueryDef) { q.Recordsets[0].Title = "Password resets, last 7 days" }},
		{"a schema issue mentioning a password", func(q *QueryDef) {
			q.Recordsets[0].ActiveIssues.Schema[0] = "the password column has no not-null constraint"
		}},
		{"a JSON schema declaring a password property", func(q *QueryDef) {
			q.Recordsets[0].JSONSchema = `{"type":"object","properties":{"password":{"type":"string"}}}`
		}},
		{"a windows-style recordset file path", func(q *QueryDef) {
			q.Recordsets[0].Files[0] = `C:\data\recordsets\invoices.json`
		}},
		{"no recordsets, parameters or capture at all", func(q *QueryDef) {
			q.Recordsets, q.Parameters, q.Capture = nil, nil, nil
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			q := queryDefWithEveryPersistedField()
			tt.mutate(&q)
			if err := q.Validate(); err != nil {
				t.Fatalf("expected the query to be accepted, got: %v", err)
			}
		})
	}
}

// queryStorageFieldDecisions is the table screenQueryDefForStorage's doc
// comment states, in a form a test can check: one row per string-valued
// leaf the query pair persists. A value of "identifier" or "credential"
// means screenQueryDefForStorage screens it itself; any other value names
// the check that already owned the field.
var queryStorageFieldDecisions = map[string]string{
	"id":        "identifier",
	"title":     "credential (QueryDef.Validate)",
	"folder":    "identifier",
	"tags[]":    "credential",
	"userIds[]": "credential",
	"access":    "closed set (ProjectItem.ValidateWithOptions)",
	"type":      "closed set (QueryDef.Validate)",
	"text":      "credential (QueryDef.Validate)",
	"purpose":   "credential (QueryDef.Validate)",

	"parameters[].id":                 "identifier",
	"parameters[].type":               "identifier",
	"parameters[].title":              "credential",
	"parameters[].defaultValue":       "credential (QueryDef.Validate)",
	"parameters[].meta.entity":        "identifier",
	"parameters[].meta.field":         "identifier",
	"parameters[].lookup.db":          "identifier",
	"parameters[].lookup.sql":         "credential",
	"parameters[].lookup.keyFields[]": "identifier",

	"targets[].driver":   "credential (QueryDefTarget.Validate)",
	"targets[].catalog":  "credential (QueryDefTarget.Validate)",
	"targets[].protocol": "credential (QueryDefTarget.Validate)",
	"targets[].host":     "credential (QueryDefTarget.Validate)",
	"targets[].username": "credential (QueryDefTarget.Validate)",
	"targets[].password": "refused outright (QueryDefTarget.Validate)",

	"recordsets[].id":                            "identifier",
	"recordsets[].title":                         "credential",
	"recordsets[].folder":                        "identifier",
	"recordsets[].tags[]":                        "credential",
	"recordsets[].userIds[]":                     "credential",
	"recordsets[].access":                        "identifier",
	"recordsets[].type":                          "identifier",
	"recordsets[].jsonSchema":                    "credential",
	"recordsets[].files[]":                       "credential",
	"recordsets[].errors[]":                      "credential",
	"recordsets[].primaryKey.name":               "identifier",
	"recordsets[].primaryKey.columns[]":          "identifier",
	"recordsets[].foreignKeys[].name":            "identifier",
	"recordsets[].foreignKeys[].columns[]":       "identifier",
	"recordsets[].foreignKeys[].matchOption":     "identifier",
	"recordsets[].foreignKeys[].updateRule":      "identifier",
	"recordsets[].foreignKeys[].deleteRule":      "identifier",
	"recordsets[].alternateKey[].name":           "identifier",
	"recordsets[].alternateKey[].columns[]":      "identifier",
	"recordsets[].issues.schema[]":               "credential",
	"recordsets[].columns[].name":                "identifier",
	"recordsets[].columns[].type":                "identifier",
	"recordsets[].columns[].meta.entity":         "identifier",
	"recordsets[].columns[].meta.field":          "identifier",
	"recordsets[].columns[].hideIf.parameters[]": "identifier",

	"capture.author":                 "identifier (QueryCapture.Validate)",
	"capture.environment":            "identifier (QueryCapture.Validate)",
	"capture.source":                 "identifier (QueryCapture.Validate)",
	"capture.collection":             "identifier (QueryCapture.Validate)",
	"capture.bindings[].parameterId": "identifier (QueryCapture.Validate)",
	"capture.bindings[].origin":      "closed set (QueryCapture.Validate)",
}

// TestQueryDefStorageScreen_CoversEveryPersistedStringField is what makes
// the screening a sweep rather than a list of fields someone remembered:
// it walks QueryDef the way encoding/json does and fails if any persisted
// string has no screening decision, if the table names a field that no
// longer exists, or if a field the table says this package screens has no
// refusal case in storageScreenCases.
func TestQueryDefStorageScreen_CoversEveryPersistedStringField(t *testing.T) {
	persisted := map[string]bool{}
	persistedStringFields(reflect.TypeOf(QueryDef{}), "", map[reflect.Type]bool{}, persisted)

	for path := range persisted {
		if _, decided := queryStorageFieldDecisions[path]; !decided {
			t.Errorf("%q is persisted into the query pair but has no screening decision: add it to screenQueryDefForStorage's table, to queryStorageFieldDecisions and, unless another check owns it, to storageScreenCases", path)
		}
	}
	for path := range queryStorageFieldDecisions {
		if !persisted[path] {
			t.Errorf("%q is listed as a persisted field but no longer exists; drop it from the table", path)
		}
	}

	// Every field this package screens itself must have a refusal case,
	// and every refusal case must name a field it screens. Case paths are
	// indexed ("parameters[1].id"), the table's are not.
	indexed := strings.NewReplacer("[0]", "[]", "[1]", "[]")
	tested := map[string]bool{}
	for _, c := range storageScreenCases() {
		path := indexed.Replace(c.path)
		tested[path] = true
		if got := queryStorageFieldDecisions[path]; got != "identifier" && got != "credential" {
			t.Errorf("storageScreenCases covers %q, which the table says is %q rather than screened here", c.path, got)
		}
	}
	for path, decision := range queryStorageFieldDecisions {
		if (decision == "identifier" || decision == "credential") && !tested[path] {
			t.Errorf("%q is screened by screenQueryDefForStorage but has no case in storageScreenCases", path)
		}
	}
}

// TestQueryDefStorageScreen_ForeignKeyRefTablePersistsNoString pins the
// one exception the table records: a foreign key's refTable keeps its
// name, schema and catalog unexported, so none of them reaches the
// git-tracked JSON and none needs screening. If a dalgo release ever
// exports them, this fails and the coverage guard above reports the new
// fields.
func TestQueryDefStorageScreen_ForeignKeyRefTablePersistsNoString(t *testing.T) {
	const name, schema, catalog = "ZqxRefTableName", "ZqxRefTableSchema", "ZqxRefTableCatalog"
	q := queryDefWithEveryPersistedField()
	q.Recordsets[0].ForeignKeys[0].RefTable = NewTableKey(name, schema, catalog, nil)
	encoded, err := json.Marshal(q)
	if err != nil {
		t.Fatalf("failed to encode the query: %v", err)
	}
	for _, value := range []string{name, schema, catalog} {
		if strings.Contains(string(encoded), value) {
			t.Errorf("refTable now persists %q; it needs a screening decision in screenQueryDefForStorage", value)
		}
	}
}

// persistedStringFields records the JSON path of every string-valued leaf
// of t, following exactly what encoding/json persists: unexported fields
// and `json:"-"` are skipped, an embedded struct with no tag is flattened
// into its parent, a slice, array or map contributes "[]", and a pointer
// contributes nothing to the path. An interface field (a parameter's
// defaultValue) counts as a leaf, since it can hold a string. visited is
// the current recursion stack, not a seen-set, so a type reached twice
// under different paths reports both.
func persistedStringFields(t reflect.Type, prefix string, visited map[reflect.Type]bool, out map[string]bool) {
	switch t.Kind() {
	case reflect.Pointer:
		persistedStringFields(t.Elem(), prefix, visited, out)
	case reflect.Slice, reflect.Array, reflect.Map:
		persistedStringFields(t.Elem(), prefix+"[]", visited, out)
	case reflect.Struct:
		if visited[t] {
			return
		}
		visited[t] = true
		defer delete(visited, t)
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" { // unexported: encoding/json never writes it
				continue
			}
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "-" {
				continue
			}
			if f.Anonymous && name == "" {
				persistedStringFields(f.Type, prefix, visited, out)
				continue
			}
			if name == "" {
				name = f.Name
			}
			if prefix != "" {
				name = prefix + "." + name
			}
			persistedStringFields(f.Type, name, visited, out)
		}
	case reflect.String, reflect.Interface:
		out[prefix] = true
	default:
	}
}
