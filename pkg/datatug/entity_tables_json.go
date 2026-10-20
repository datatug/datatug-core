package datatug

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// These methods belong to the named collection, never DBCollectionKey: adding
// methods to that anonymously embedded type would hide owner columns/FKs.
type entityTableWire struct {
	Name      string              `json:"name"`
	Type      CollectionType      `json:"type,omitempty"`
	Schema    string              `json:"schema,omitempty"`
	Catalog   string              `json:"catalog,omitempty"`
	Database  string              `json:"database,omitempty"`
	RefSchema string              `json:"refSchema,omitempty"`
	Parent    []entityTableParent `json:"parent,omitempty"`
	LegacyRef json.RawMessage     `json:"Ref,omitempty"`
}
type entityTableParent struct {
	Collection string `json:"collection"`
	ID         string `json:"id"`
	IDType     string `json:"idType,omitempty"`
}

var errEntityTableWire = errors.New("invalid entity table identity")

func (v TableKeys) MarshalJSON() ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	out := make([]entityTableWire, len(v))
	for i, key := range v {
		wire, err := entityTableToWire(key)
		if err != nil {
			return nil, err
		}
		out[i] = wire
	}
	body, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(body) > 256<<10 {
		return nil, errEntityTableWire
	}
	return body, nil
}
func (v *TableKeys) UnmarshalJSON(body []byte) error {
	if len(body) > 256<<10 || validateEntityTableJSON(body) != nil {
		return errEntityTableWire
	}
	var wire []entityTableWire
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&wire) != nil {
		return errEntityTableWire
	}
	if wire == nil {
		*v = nil
		return nil
	}
	out := make(TableKeys, len(wire))
	for i, item := range wire {
		key, err := entityTableFromWire(item)
		if err != nil {
			return err
		}
		out[i] = key
	}
	*v = out
	return nil
}
func entityTableToWire(key DBCollectionKey) (entityTableWire, error) {
	wire := entityTableWire{Name: key.Name(), Type: key.t, Schema: key.schema, Catalog: key.catalog, Database: key.Ref.Database(), RefSchema: key.Ref.Schema()}
	if wire.Name == "" || key.Validate() != nil || key.Ref.Alias() != "" || key.Ref.ScanLimit() != 0 || len(key.Ref.ScanOrders()) > 0 {
		return wire, errEntityTableWire
	}
	seen := map[*record.Key]bool{}
	for parent := key.Ref.Parent(); parent != nil; parent = parent.Parent() {
		if seen[parent] || len(wire.Parent) >= 16 || strings.TrimSpace(parent.Collection()) == "" {
			return wire, errEntityTableWire
		}
		seen[parent] = true
		segment := entityTableParent{Collection: parent.Collection()}
		switch id := parent.ID.(type) {
		case string:
			if id == "" {
				return wire, errEntityTableWire
			}
			segment.ID = id
		default:
			value := reflect.ValueOf(parent.ID)
			segment.IDType = value.Kind().String()
			switch value.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				segment.ID = strconv.FormatInt(value.Int(), 10)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				segment.ID = strconv.FormatUint(value.Uint(), 10)
			default:
				return wire, errEntityTableWire
			}
		}
		wire.Parent = append(wire.Parent, segment)
	}
	for i, j := 0, len(wire.Parent)-1; i < j; i, j = i+1, j-1 {
		wire.Parent[i], wire.Parent[j] = wire.Parent[j], wire.Parent[i]
	}
	if len(wire.Parent) > 0 && (wire.Database != "" || wire.RefSchema != "") {
		return wire, errEntityTableWire
	}
	return wire, nil
}
func entityTableFromWire(wire entityTableWire) (DBCollectionKey, error) {
	var key DBCollectionKey
	if wire.Name == "" || wire.Type != "" && !IsKnownCollectionType(wire.Type) || len(wire.Parent) > 16 {
		return key, errEntityTableWire
	}
	// Old Go-generated Ref:{} alone lost its name and cannot be reconstructed.
	// A redundant empty Ref beside an explicit logical name is harmless legacy.
	if len(wire.LegacyRef) > 0 && !bytes.Equal(bytes.TrimSpace(wire.LegacyRef), []byte("{}")) {
		return key, errEntityTableWire
	}
	var parent *record.Key
	for _, p := range wire.Parent {
		if strings.TrimSpace(p.Collection) == "" || p.ID == "" {
			return key, errEntityTableWire
		}
		id, err := entityTableParentID(p)
		if err != nil {
			return key, err
		}
		parent = record.NewKeyWithParentAndID(parent, p.Collection, id)
		if parent.Validate() != nil {
			return key, errEntityTableWire
		}
	}
	if parent != nil && (wire.Database != "" || wire.RefSchema != "") {
		return key, errEntityTableWire
	}
	ref := dal.NewCollectionRef(wire.Name, "", parent)
	if wire.Database != "" {
		ref = dal.NewDatabaseCollectionRef(wire.Database, wire.RefSchema, wire.Name, "")
	} else if wire.RefSchema != "" {
		ref = dal.NewQualifiedRootCollectionRef(wire.RefSchema, wire.Name, "")
	}
	return DBCollectionKey{t: wire.Type, schema: wire.Schema, catalog: wire.Catalog, Ref: ref}, nil
}
func entityTableParentID(p entityTableParent) (any, error) {
	if p.IDType == "" {
		return p.ID, nil
	}
	var typ reflect.Type
	switch p.IDType {
	case "int":
		typ = reflect.TypeOf(int(0))
	case "int8":
		typ = reflect.TypeOf(int8(0))
	case "int16":
		typ = reflect.TypeOf(int16(0))
	case "int32":
		typ = reflect.TypeOf(int32(0))
	case "int64":
		typ = reflect.TypeOf(int64(0))
	case "uint":
		typ = reflect.TypeOf(uint(0))
	case "uint8":
		typ = reflect.TypeOf(uint8(0))
	case "uint16":
		typ = reflect.TypeOf(uint16(0))
	case "uint32":
		typ = reflect.TypeOf(uint32(0))
	case "uint64":
		typ = reflect.TypeOf(uint64(0))
	default:
		return nil, errEntityTableWire
	}
	value := reflect.New(typ).Elem()
	if typ.Kind() >= reflect.Int && typ.Kind() <= reflect.Int64 {
		n, err := strconv.ParseInt(p.ID, 10, typ.Bits())
		if err != nil || strconv.FormatInt(n, 10) != p.ID {
			return nil, errEntityTableWire
		}
		value.SetInt(n)
	} else {
		n, err := strconv.ParseUint(p.ID, 10, typ.Bits())
		if err != nil || strconv.FormatUint(n, 10) != p.ID {
			return nil, errEntityTableWire
		}
		value.SetUint(n)
	}
	return value.Interface(), nil
}
func validateEntityTableJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var value func(int) error
	value = func(depth int) error {
		if depth > 40 {
			return errEntityTableWire
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, container := token.(json.Delim)
		if !container {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errEntityTableWire
				}
				seen[name] = true
				if err = value(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err = value(depth + 1); err != nil {
					return err
				}
			}
		default:
			return errEntityTableWire
		}
		_, err = decoder.Token()
		return err
	}
	if err := value(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errEntityTableWire
	}
	return nil
}
