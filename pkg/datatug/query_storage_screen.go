package datatug

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/strongo/validation"
)

// Identifier shape rule, the second of the two screens a value persisted
// into a query pair may get (the first is EmbeddedCredentialReason,
// query_credentials.go).
//
// A field whose realistic values are machine identifiers - a parameter ID,
// a declared type, an entity/field reference, a column or key name, a
// capture's provenance - is screened by shape rather than by meaning: a
// rule about the value's shape says everything that needs saying about it
// and, unlike the credential screen, never has to guess what a word means.
//
// That matters because the credential screen is tuned for free text and
// has documented false positives a realistic identifier would hit: it
// reads "from:alice@example.com" as DSN userinfo, and it refuses any
// key/value pair whose key ends in a secret suffix whatever the value
// holds. A capture author is usually an e-mail address and a collection
// may well be named "passwords" or "password_resets", so screening these
// fields that way would refuse valid content.
//
// The shape rule is not the weaker of the two. Each of the five syntaxes
// EmbeddedCredentialReason recognizes needs punctuation this rule
// forbids: a URL userinfo needs "://", a DSN userinfo needs ":", a
// key/value pair needs "=", a JSON member needs a double quote and ":",
// and an HTTP credential header needs ":". A value holding none of ':',
// '=' and '"' therefore cannot express any of them and cannot trip the
// credential screen either - which is what
// TestIdentifierShape_IsNotWeakerThanTheCredentialScreen proves, in both
// directions, on the screen's own probes. Control characters and line
// breaks are refused as well, so no field can smuggle a second line (an
// "Authorization:" header line, say) into the file; so is a value that is
// not valid UTF-8, which would reach the JSON as replacement characters,
// and one padded with spaces, which names nothing that a lookup would
// find.
const maxIdentifierFieldLength = 200

// Refusal reasons returned by identifierShapeReason.
const (
	identifierShapeWhitespaceReason  = "must not start or end with whitespace"
	identifierShapeEncodingReason    = "must be valid UTF-8"
	identifierShapeControlReason     = "must not contain control characters or line breaks"
	identifierShapePunctuationReason = `must not contain ':', '=' or '"': this field names something, and cannot be a connection string, a key/value pair or a JSON object`
)

// identifierShapeReason reports why value cannot be stored as an
// identifier in a git-tracked project file (see the shape notes above), or
// ("", false) when it can. An empty value passes: which fields are
// required is decided by each type's own Validate, not here.
func identifierShapeReason(value string) (reason string, found bool) {
	if value == "" {
		return "", false
	}
	if strings.TrimSpace(value) != value {
		return identifierShapeWhitespaceReason, true
	}
	if !utf8.ValidString(value) {
		return identifierShapeEncodingReason, true
	}
	if len(value) > maxIdentifierFieldLength {
		return fmt.Sprintf("exceeds max length (%d): %d", maxIdentifierFieldLength, len(value)), true
	}
	for _, r := range value {
		switch {
		case r == ':' || r == '=' || r == '"':
			return identifierShapePunctuationReason, true
		case unicode.IsControl(r):
			return identifierShapeControlReason, true
		}
	}
	return "", false
}

// storedQueryHint is appended to a credential refusal, saying why the
// value cannot be stored at all rather than merely being discouraged.
const storedQueryHint = "a query is stored in git-tracked project files"

// screenQueryDefForStorage screens every string a QueryDef persists that
// no other check already owns. A query is written as a pair of git-tracked
// files - "<id>.query.json" (the whole QueryDef with Text cleared) and
// "<id>.query.<type>" (the Text) - so a secret in any of them reaches git,
// not just one in the obvious fields. Every save path (PutQuery, the
// legacy SaveQuery/CreateQuery/UpdateQuery, and a project save through
// Project.Validate) runs QueryDef.Validate, which calls this, so each of
// them refuses such a write before anything reaches disk.
//
// # Every persisted string and how it is screened
//
// The table below is the whole of it: one row per string-valued leaf
// reachable from QueryDef in the JSON it is persisted as.
// TestQueryDefStorageScreen_CoversEveryPersistedStringField walks the type
// by reflection and fails if a field is added without a row here, so this
// is a sweep, not a list. "identifier" is identifierShapeReason above;
// "credential" is EmbeddedCredentialReason (query_credentials.go).
//
//	JSON path                                     screen
//	--------------------------------------------- ------------------------------------
//	id                                            identifier
//	title                                         credential (QueryDef.Validate)
//	folder                                        identifier, one "/"-separated segment at
//	                                              a time, with the documented "user:<id>"
//	                                              root prefix stripped first - it holds a
//	                                              ':' by design. Screening the whole path
//	                                              as one value instead would let a secret
//	                                              hide behind the separator: the credential
//	                                              screen reads "Password=" as a key/value
//	                                              pair only after a separator it knows, and
//	                                              '/' is not one
//	tags[]                                        credential - a "team:finance" tag
//	                                              convention is realistic
//	userIds[]                                     credential - a principal ID may be
//	                                              provider-qualified ("google:12345")
//	access                                        closed set (ProjectItem.ValidateWithOptions)
//	type                                          closed set (QueryDef.Validate's switch)
//	text                                          credential (QueryDef.Validate)
//	purpose                                       credential (QueryDef.Validate)
//	parameters[].id                               identifier
//	parameters[].type                             identifier
//	parameters[].title                            credential - free prose, like a query title
//	parameters[].defaultValue                     credential (QueryDef.Validate, recursive)
//	parameters[].meta.entity                      identifier
//	parameters[].meta.field                       identifier
//	parameters[].lookup.db                        identifier
//	parameters[].lookup.sql                       credential - free SQL, like a query body
//	parameters[].lookup.keyFields[]               identifier
//	targets[].driver                              credential (QueryDefTarget.Validate)
//	targets[].catalog                             credential (QueryDefTarget.Validate)
//	targets[].protocol                            credential (QueryDefTarget.Validate)
//	targets[].host                                credential (QueryDefTarget.Validate)
//	targets[].username                            credential (QueryDefTarget.Validate)
//	targets[].password                            refused outright (QueryDefTarget.Validate)
//	recordsets[].id                               identifier
//	recordsets[].title                            credential
//	recordsets[].folder                           identifier, per segment (as folder above)
//	recordsets[].tags[]                           credential (as tags[] above)
//	recordsets[].userIds[]                        credential (as userIds[] above)
//	recordsets[].access                           identifier - RecordsetDefinition.Validate,
//	                                              which confines it to a closed set, is
//	                                              never reached on the query path
//	recordsets[].type                             identifier - likewise unvalidated here
//	recordsets[].jsonSchema                       credential - free JSON text
//	recordsets[].files[]                          credential - a path may hold a ':'
//	recordsets[].errors[]                         credential - free prose, and a driver
//	                                              error can echo a connection string
//	recordsets[].primaryKey.name                  identifier
//	recordsets[].primaryKey.columns[]             identifier
//	recordsets[].foreignKeys[].name               identifier
//	recordsets[].foreignKeys[].columns[]          identifier
//	recordsets[].foreignKeys[].matchOption        identifier
//	recordsets[].foreignKeys[].updateRule         identifier
//	recordsets[].foreignKeys[].deleteRule         identifier
//	recordsets[].alternateKey[].name              identifier
//	recordsets[].alternateKey[].columns[]         identifier
//	recordsets[].issues.schema[]                  credential - free prose diagnostics
//	recordsets[].columns[].name                   identifier
//	recordsets[].columns[].type                   identifier
//	recordsets[].columns[].meta.entity            identifier
//	recordsets[].columns[].meta.field             identifier
//	recordsets[].columns[].hideIf.parameters[]    identifier
//	capture.author                                identifier (QueryCapture.Validate)
//	capture.environment                           identifier (QueryCapture.Validate)
//	capture.source                                identifier (QueryCapture.Validate)
//	capture.collection                            identifier (QueryCapture.Validate)
//	capture.bindings[].parameterId                identifier (QueryCapture.Validate)
//	capture.bindings[].origin                     closed set (QueryCapture.Validate)
//
// recordsets[].foreignKeys[].refTable persists no string of its own:
// DBCollectionKey keeps its schema, catalog and type unexported and its
// one exported field, dal.CollectionRef, has none either, so the JSON
// holds an empty object.
// TestQueryDefStorageScreen_ForeignKeyRefTablePersistsNoString pins that,
// and the coverage guard fails if a future dalgo release exports one.
//
// The screen is deliberately scoped to a query rather than pushed down
// into ParameterDef.Validate: Parameters is shared with boards, which are
// stored elsewhere and are not this AC's subject.
func screenQueryDefForStorage(v QueryDef) error {
	s := &queryStorageScreen{}
	s.projectItem("", v.ProjectItem)
	s.parameters(v.Parameters)
	s.recordsets(v.Recordsets)
	return s.err
}

// queryStorageScreen collects the first refusal so a walk of every field
// reads as a list of checks rather than a ladder of error returns.
type queryStorageScreen struct{ err error }

// identifier screens a value whose realistic content names something.
func (s *queryStorageScreen) identifier(field, value string) {
	if s.err != nil {
		return
	}
	if reason, found := identifierShapeReason(value); found {
		s.err = validation.NewErrBadRecordFieldValue(field, reason)
	}
}

// prose screens a value whose realistic content is free text.
func (s *queryStorageScreen) prose(field, value string) {
	if s.err != nil {
		return
	}
	if reason, found := EmbeddedCredentialReason(value); found {
		s.err = validation.NewErrBadRecordFieldValue(field, reason+"; "+storedQueryHint)
	}
}

func (s *queryStorageScreen) identifiers(field string, values []string) {
	for i, value := range values {
		s.identifier(fmt.Sprintf("%s[%d]", field, i), value)
	}
}

func (s *queryStorageScreen) proseList(field string, values []string) {
	for i, value := range values {
		s.prose(fmt.Sprintf("%s[%d]", field, i), value)
	}
}

// projectItem screens the fields every stored project item carries. Title
// and Access are left out: at the top level QueryDef.Validate screens the
// title itself and ProjectItem.ValidateWithOptions confines access to a
// closed set. A recordset reaches neither on the query path, so recordsets
// adds both for its own copy.
func (s *queryStorageScreen) projectItem(at string, item ProjectItem) {
	s.identifier(at+"id", item.ID)
	s.folderPath(at+"folder", item.Folder)
	s.proseList(at+"tags", item.Tags)
	s.proseList(at+"userIds", item.UserIDs)
}

// folderPath screens a "/"-separated folder path one segment at a time.
// A path is an identifier - the root is "~" or "user:<id>" and every other
// segment names a folder - so it gets the shape rule, with the "user:"
// prefix stripped from the root because that ':' is part of the
// documented syntax (see ValidateFolderPath). Screening the whole path as
// one value would be weaker than screening the segments: the credential
// screen recognizes a "Password=" key/value pair only after one of the
// separators it knows, and '/' is not among them, so "~/Password=secret"
// would pass it.
func (s *queryStorageScreen) folderPath(field, value string) {
	if value == "" {
		return
	}
	for i, segment := range strings.Split(value, FoldersPathSeparator) {
		if i == 0 {
			segment = strings.TrimPrefix(segment, RootUserFolderPrefix)
		}
		s.identifier(field, segment)
	}
}

func (s *queryStorageScreen) entityFieldRef(at string, ref *EntityFieldRef) {
	if ref == nil {
		return
	}
	s.identifier(at+".entity", ref.Entity)
	s.identifier(at+".field", ref.Field)
}

func (s *queryStorageScreen) uniqueKey(at string, key *UniqueKey) {
	if key == nil {
		return
	}
	s.identifier(at+".name", key.Name)
	s.identifiers(at+".columns", key.Columns)
}

func (s *queryStorageScreen) parameters(parameters Parameters) {
	for i, p := range parameters {
		at := fmt.Sprintf("parameters[%d].", i)
		s.identifier(at+"id", p.ID)
		s.identifier(at+"type", p.Type)
		s.prose(at+"title", p.Title)
		s.entityFieldRef(at+"meta", p.Meta)
		if p.Lookup != nil {
			s.identifier(at+"lookup.db", p.Lookup.DB)
			s.prose(at+"lookup.sql", p.Lookup.SQL)
			s.identifiers(at+"lookup.keyFields", p.Lookup.KeyFields)
		}
	}
}

// recordsets screens a query's recordset definitions. They are persisted
// inside "<id>.query.json" like everything else, yet nothing on the query
// path validates them - QueryDef.Validate never calls
// RecordsetDefinition.Validate, which is why the demo project's
// id-less, type-less recordsets still load - so this is the only screen
// they get.
func (s *queryStorageScreen) recordsets(recordsets []RecordsetDefinition) {
	for i, r := range recordsets {
		at := fmt.Sprintf("recordsets[%d].", i)
		s.projectItem(at, r.ProjectItem)
		s.prose(at+"title", r.Title)
		s.identifier(at+"access", r.Access)
		s.identifier(at+"type", r.Type)
		s.prose(at+"jsonSchema", r.JSONSchema)
		s.proseList(at+"files", r.Files)
		s.proseList(at+"errors", r.Errors)
		s.uniqueKey(at+"primaryKey", r.PrimaryKey)
		for k, fk := range r.ForeignKeys {
			if fk == nil {
				continue
			}
			fkAt := fmt.Sprintf("%sforeignKeys[%d].", at, k)
			s.identifier(fkAt+"name", fk.Name)
			s.identifiers(fkAt+"columns", fk.Columns)
			s.identifier(fkAt+"matchOption", fk.MatchOption)
			s.identifier(fkAt+"updateRule", fk.UpdateRule)
			s.identifier(fkAt+"deleteRule", fk.DeleteRule)
		}
		for k := range r.AlternateKeys {
			s.uniqueKey(fmt.Sprintf("%salternateKey[%d]", at, k), &r.AlternateKeys[k])
		}
		if r.ActiveIssues != nil {
			s.proseList(at+"issues.schema", r.ActiveIssues.Schema)
		}
		for j, c := range r.Columns {
			colAt := fmt.Sprintf("%scolumns[%d].", at, j)
			s.identifier(colAt+"name", c.Name)
			s.identifier(colAt+"type", c.Type)
			s.entityFieldRef(colAt+"meta", c.Meta)
			s.identifiers(colAt+"hideIf.parameters", c.HideIf.Parameters)
		}
	}
}
