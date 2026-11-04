package integrity

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strconv"
	"time"
)

// EncodingVersion is the version of the field set and value encoding below.
// Any change to either needs a new version; leaves commit to it.
const EncodingVersion uint16 = 1

// Value encoding, version 1. An absent value (empty string, nil pointer,
// zero time, empty list) encodes as zero bytes. Otherwise:
//
//	string:  0x01 || utf-8 bytes
//	int:     0x02 || int64 big-endian
//	time:    0x03 || int64 Unix seconds || uint32 nanoseconds (big-endian)
//	strings: 0x04 || uint32 count || count × (uint32 length || bytes)
//
// Composite fields are fixed-arity string lists with canonical decimal
// integers. Decoding is strict: a value must re-encode to the same bytes.
const (
	tagString  byte = 1
	tagInt     byte = 2
	tagTime    byte = 3
	tagStrings byte = 4
)

var errValue = errors.New("non-canonical value")

func encString(s string) []byte {
	if s == "" {
		return nil
	}
	return append([]byte{tagString}, s...)
}

func decString(v []byte) (string, error) {
	if len(v) == 0 {
		return "", nil
	}
	if v[0] != tagString {
		return "", errValue
	}
	return string(v[1:]), nil
}

func encInt(p *int64) []byte {
	if p == nil {
		return nil
	}
	return binary.BigEndian.AppendUint64([]byte{tagInt}, uint64(*p))
}

func decInt(v []byte) (*int64, error) {
	if len(v) == 0 {
		return nil, nil
	}
	if len(v) != 9 || v[0] != tagInt {
		return nil, errValue
	}
	n := int64(binary.BigEndian.Uint64(v[1:]))
	return &n, nil
}

func encTime(t time.Time) []byte {
	if t.IsZero() {
		return nil
	}
	w := builder{b: []byte{tagTime}}
	w.time(t)
	return w.b
}

func decTime(v []byte) (time.Time, error) {
	if len(v) == 0 {
		return time.Time{}, nil
	}
	if len(v) != 13 || v[0] != tagTime {
		return time.Time{}, errValue
	}
	sec := int64(binary.BigEndian.Uint64(v[1:9]))
	nsec := int64(binary.BigEndian.Uint32(v[9:]))
	return time.Unix(sec, nsec).UTC(), nil
}

func encStrings(ss []string) []byte {
	if len(ss) == 0 {
		return nil
	}
	w := builder{b: []byte{tagStrings}}
	w.u32(uint32(len(ss)))
	for _, s := range ss {
		w.str(s)
	}
	return w.b
}

func decStrings(v []byte) ([]string, error) {
	if len(v) == 0 {
		return nil, nil
	}
	if len(v) < 5 || v[0] != tagStrings {
		return nil, errValue
	}
	n := binary.BigEndian.Uint32(v[1:5])
	rest := v[5:]
	var out []string
	for range n {
		if len(rest) < 4 {
			return nil, errValue
		}
		l := binary.BigEndian.Uint32(rest)
		rest = rest[4:]
		if uint64(len(rest)) < uint64(l) {
			return nil, errValue
		}
		out = append(out, string(rest[:l]))
		rest = rest[l:]
	}
	if len(rest) != 0 {
		return nil, errValue
	}
	return out, nil
}

// tuple decodes a fixed-arity string list.
func tuple(v []byte, arity int) ([]string, error) {
	ss, err := decStrings(v)
	if err != nil || ss == nil {
		return nil, err
	}
	if len(ss) != arity {
		return nil, errValue
	}
	return ss, nil
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func atoi(s string) (int64, error) { return strconv.ParseInt(s, 10, 64) }

// fieldSpec is one committed field of [Event].
type fieldSpec struct {
	name string
	get  func(*Event) []byte
	set  func(*Event, []byte) error
}

func strField(name string, p func(*Event) *string) fieldSpec {
	return fieldSpec{
		name: name,
		get:  func(e *Event) []byte { return encString(*p(e)) },
		set: func(e *Event, v []byte) (err error) {
			*p(e), err = decString(v)
			return err
		},
	}
}

func intField(name string, p func(*Event) **int64) fieldSpec {
	return fieldSpec{
		name: name,
		get:  func(e *Event) []byte { return encInt(*p(e)) },
		set: func(e *Event, v []byte) (err error) {
			*p(e), err = decInt(v)
			return err
		},
	}
}

func timeField(name string, p func(*Event) *time.Time) fieldSpec {
	return fieldSpec{
		name: name,
		get:  func(e *Event) []byte { return encTime(*p(e)) },
		set: func(e *Event, v []byte) (err error) {
			*p(e), err = decTime(v)
			return err
		},
	}
}

func listField(name string, p func(*Event) *[]string) fieldSpec {
	return fieldSpec{
		name: name,
		get:  func(e *Event) []byte { return encStrings(*p(e)) },
		set: func(e *Event, v []byte) (err error) {
			*p(e), err = decStrings(v)
			return err
		},
	}
}

func objectsField() fieldSpec {
	return fieldSpec{
		name: "resource.objects",
		get: func(e *Event) []byte {
			ss := make([]string, 0, 2*len(e.Resource.Objects))
			for _, o := range e.Resource.Objects {
				ss = append(ss, o.Namespace, o.Name)
			}
			return encStrings(ss)
		},
		set: func(e *Event, v []byte) error {
			ss, err := decStrings(v)
			if err != nil || len(ss)%2 != 0 {
				return errValue
			}
			e.Resource.Objects = nil
			for i := 0; i < len(ss); i += 2 {
				e.Resource.Objects = append(e.Resource.Objects, ObjectRef{Namespace: ss[i], Name: ss[i+1]})
			}
			return nil
		},
	}
}

func bagField() fieldSpec {
	return fieldSpec{
		name: "resource.bag",
		get: func(e *Event) []byte {
			if b := e.Resource.Bag; b != nil {
				return encStrings([]string{b.Session, b.Bag, itoa(b.Revision)})
			}
			return nil
		},
		set: func(e *Event, v []byte) error {
			ss, err := tuple(v, 3)
			if err != nil || ss == nil {
				return err
			}
			rev, err := atoi(ss[2])
			e.Resource.Bag = &BagRef{Session: ss[0], Bag: ss[1], Revision: rev}
			return err
		},
	}
}

func caseField() fieldSpec {
	return fieldSpec{
		name: "caseRef",
		get: func(e *Event) []byte {
			if c := e.CaseRef; c != nil {
				return encStrings([]string{c.Kind, c.ID})
			}
			return nil
		},
		set: func(e *Event, v []byte) error {
			ss, err := tuple(v, 2)
			if err != nil || ss == nil {
				return err
			}
			e.CaseRef = &CaseRef{Kind: ss[0], ID: ss[1]}
			return nil
		},
	}
}

func execField() fieldSpec {
	return fieldSpec{
		name: "executionRef",
		get: func(e *Event) []byte {
			if x := e.Execution; x != nil {
				return encStrings([]string{x.StoreID, x.Project, x.ID})
			}
			return nil
		},
		set: func(e *Event, v []byte) error {
			ss, err := tuple(v, 3)
			if err != nil || ss == nil {
				return err
			}
			e.Execution = &ExecRef{StoreID: ss[0], Project: ss[1], ID: ss[2]}
			return nil
		},
	}
}

func digestField() fieldSpec {
	return fieldSpec{
		name: "statementDigest",
		get: func(e *Event) []byte {
			if d := e.Statement; d != nil {
				return encStrings([]string{itoa(d.KeyVersion), d.HMAC})
			}
			return nil
		},
		set: func(e *Event, v []byte) error {
			ss, err := tuple(v, 2)
			if err != nil || ss == nil {
				return err
			}
			kv, err := atoi(ss[0])
			e.Statement = &StmtHMAC{KeyVersion: kv, HMAC: ss[1]}
			return err
		},
	}
}

func refField(name string, p func(*Event) *ActorRef) fieldSpec {
	return strField(name, func(e *Event) *string { return (*string)(p(e)) })
}

// schemaV1 is the committed field set of [EncodingVersion] 1, in order.
var schemaV1 = []fieldSpec{
	strField("id", func(e *Event) *string { return &e.ID }),
	timeField("at", func(e *Event) *time.Time { return &e.At }),
	strField("kind", func(e *Event) *string { return &e.Kind }),
	strField("action", func(e *Event) *string { return &e.Action }),
	strField("tier", func(e *Event) *string { return &e.Tier }),
	strField("outcome", func(e *Event) *string { return &e.Outcome }),
	strField("reason", func(e *Event) *string { return &e.Reason }),
	refField("actor.ref", func(e *Event) *ActorRef { return &e.Actor.Ref }),
	strField("actor.kind", func(e *Event) *string { return &e.Actor.Kind }),
	strField("actor.credentialFamily", func(e *Event) *string { return &e.Actor.CredentialFamily }),
	strField("actor.clientId", func(e *Event) *string { return &e.Actor.ClientID }),
	refField("actor.onBehalfOf", func(e *Event) *ActorRef { return &e.Actor.OnBehalfOf }),
	refField("access.requester", func(e *Event) *ActorRef { return &e.Access.Requester }),
	refField("access.approver", func(e *Event) *ActorRef { return &e.Access.Approver }),
	refField("access.beneficiary", func(e *Event) *ActorRef { return &e.Access.Beneficiary }),
	strField("resource.space", func(e *Event) *string { return &e.Resource.Space }),
	strField("resource.project", func(e *Event) *string { return &e.Resource.Project }),
	strField("resource.environment", func(e *Event) *string { return &e.Resource.Environment }),
	strField("resource.connection", func(e *Event) *string { return &e.Resource.Connection }),
	strField("resource.fingerprint", func(e *Event) *string { return &e.Resource.Fingerprint }),
	objectsField(),
	listField("resource.fields", func(e *Event) *[]string { return &e.Resource.Fields }),
	bagField(),
	strField("operation.op", func(e *Event) *string { return &e.Operation.Op }),
	strField("operation.correlationId", func(e *Event) *string { return &e.Operation.CorrelationID }),
	intField("operation.recordCount", func(e *Event) **int64 { return &e.Operation.RecordCount }),
	intField("operation.pages", func(e *Event) **int64 { return &e.Operation.Pages }),
	intField("operation.denials", func(e *Event) **int64 { return &e.Operation.Denials }),
	listField("authority.grants", func(e *Event) *[]string { return &e.Authority.Grants }),
	listField("authority.decisions", func(e *Event) *[]string { return &e.Authority.Decisions }),
	listField("authority.standing", func(e *Event) *[]string { return &e.Authority.Standing }),
	listField("authority.roles", func(e *Event) *[]string { return &e.Authority.Roles }),
	listField("authority.teamBasis", func(e *Event) *[]string { return &e.Authority.TeamBasis }),
	strField("authority.policyFingerprint", func(e *Event) *string { return &e.Authority.PolicyFingerprint }),
	intField("authority.protectionRevision", func(e *Event) **int64 { return &e.Authority.ProtectionRevision }),
	caseField(),
	execField(),
	digestField(),
	strField("window", func(e *Event) *string { return &e.Window }),
	timeField("occurredAt", func(e *Event) *time.Time { return &e.Occurred }),
}

// FieldNames returns the committed field names of [EncodingVersion], in
// commitment order.
func FieldNames() []string {
	names := make([]string, len(schemaV1))
	for i, f := range schemaV1 {
		names[i] = f.name
	}
	return names
}

// checkCanonical decodes v as field f and requires it to re-encode to v.
func (f fieldSpec) checkCanonical(e *Event, v []byte) error {
	if err := f.set(e, v); err != nil {
		return errValue
	}
	if !bytes.Equal(f.get(e), v) {
		return errValue
	}
	return nil
}
