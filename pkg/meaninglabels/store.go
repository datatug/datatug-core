package meaninglabels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// MaxDocumentBytes bounds a stored label document.
const MaxDocumentBytes = 1 << 20

// Store loads and saves a project's label document. Save is compare-and-set:
// ifRevision must be the revision the change was based on ("" for a project
// with no document yet).
type Store interface {
	Load(ctx context.Context) (Document, error)
	Save(ctx context.Context, doc Document, ifRevision string) error
}

// Collection is the DALgo collection that holds label documents.
const Collection = "meaningLabels"

// DocumentID is the ID of a project's one label document.
const DocumentID = "labels"

// Key returns the label document's key under a project key.
func Key(project *record.Key) *record.Key {
	return record.NewKeyWithParentAndID(project, Collection, DocumentID)
}

// stored is the driver-neutral codec: the canonical JSON document as bytes.
type stored struct {
	Content []byte `json:"content" firestore:"content"`
}

// DALgoStore keeps the document in any dal.DB; the edge composes the driver.
type DALgoStore struct {
	DB  dal.DB
	Key *record.Key
}

var _ Store = DALgoStore{}

// Load returns the stored document, or an empty one when none exists.
func (s DALgoStore) Load(ctx context.Context) (doc Document, err error) {
	err = s.DB.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		doc, err = load(ctx, tx, s.Key)
		return err
	})
	return doc, err
}

// Save writes doc if the stored revision still equals ifRevision.
func (s DALgoStore) Save(ctx context.Context, doc Document, ifRevision string) error {
	body, err := Encode(doc)
	if err != nil {
		return err
	}
	return s.DB.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		current, err := load(ctx, tx, s.Key)
		if err != nil {
			return err
		}
		if err := CheckRevision(current, ifRevision); err != nil {
			return err
		}
		return tx.Set(ctx, record.NewRecordWithData(s.Key, &stored{Content: body}))
	})
}

func load(ctx context.Context, tx dal.ReadSession, key *record.Key) (Document, error) {
	var data stored
	if err := tx.Get(ctx, record.NewRecordWithData(key, &data)); err != nil {
		if record.IsNotFound(err) {
			return Document{Format: Format, Labels: []Label{}}, nil
		}
		return Document{}, err
	}
	return Decode(data.Content)
}

// CheckRevision refuses a save based on a stale revision. A missing document
// has revision "".
func CheckRevision(current Document, ifRevision string) error {
	if current.Revision != ifRevision {
		return errorf(CodeRevisionConflict, "the labels changed since they were read; read them again and retry")
	}
	return nil
}

// Encode validates and serializes a document.
func Encode(doc Document) ([]byte, error) {
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	body, _ := json.MarshalIndent(doc, "", "  ") // plain values; cannot fail
	if len(body) > MaxDocumentBytes {
		return nil, errorf(CodeInvalidDocument, "the label document exceeds %d bytes", MaxDocumentBytes)
	}
	return append(body, '\n'), nil
}

// Decode parses and validates a document.
func Decode(body []byte) (Document, error) {
	if len(body) > MaxDocumentBytes {
		return Document{}, errorf(CodeInvalidDocument, "the label document exceeds %d bytes", MaxDocumentBytes)
	}
	var doc Document
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		return Document{}, errors.Join(errorf(CodeInvalidDocument, "the label document is not valid JSON"), err)
	}
	if doc.Labels == nil {
		doc.Labels = []Label{}
	}
	if err := doc.Validate(); err != nil {
		return Document{}, err
	}
	return doc, nil
}
