package detective

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"time"
)

// OperationBinding ties an idempotency key to the authenticated principal,
// project scope, action, resource and the exact semantic request. The adapter
// must compute it server-side after authorization, never accept a client hash.
type OperationBinding struct {
	Project       string
	Actor         string
	Action        string
	ResourceID    string
	PayloadDigest string
}

// BindOperation computes a deterministic SHA-256 over Go's canonical JSON
// encoding (sorted map keys, stable struct fields). The payload is never put
// into OperationReceipt or the audit stream.
func BindOperation(project, actor, action, resourceID string, payload any) (OperationBinding, error) {
	if project == "" || actor == "" || action == "" || payload == nil {
		return OperationBinding{}, ErrInvalid
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return OperationBinding{}, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var normalized any
	// json.Marshal above guarantees syntactically valid JSON; decoding that
	// value into generic JSON numbers cannot fail. The second marshal sorts
	// keys even when payload supplied its own Marshaler.
	_ = decoder.Decode(&normalized)
	encoded, _ := json.Marshal(struct {
		Project    string `json:"project"`
		Actor      string `json:"actor"`
		Action     string `json:"action"`
		ResourceID string `json:"resourceId"`
		Payload    any    `json:"payload"`
	}{project, actor, action, resourceID, normalized})
	digest := sha256.Sum256(encoded)
	return OperationBinding{Project: project, Actor: actor, Action: action, ResourceID: resourceID, PayloadDigest: hex.EncodeToString(digest[:])}, nil
}

// NewOperationReceipt binds one successful atomic mutation to its accepted
// request. The adapter persists this with resource changes and audit event.
func NewOperationReceipt(id string, binding OperationBinding, resourceVersion int64, at time.Time) (OperationReceipt, error) {
	receipt := OperationReceipt{ID: id, Project: binding.Project, Actor: binding.Actor, Action: binding.Action, ResourceID: binding.ResourceID, ResourceVersion: resourceVersion, PayloadDigest: binding.PayloadDigest, At: at}
	return receipt, receipt.Validate()
}

func (r OperationReceipt) Validate() error {
	if r.ID == "" || r.Project == "" || r.Actor == "" || r.Action == "" || r.ResourceVersion < 0 || r.At.IsZero() || len(r.PayloadDigest) != 64 {
		return ErrInvalid
	}
	if _, err := hex.DecodeString(r.PayloadDigest); err != nil {
		return ErrInvalid
	}
	return nil
}

// CheckOperationReplay is required before returning a stored response for an
// existing operation ID. Any binding mismatch is an idempotency conflict.
func CheckOperationReplay(saved OperationReceipt, current OperationBinding) error {
	if saved.Validate() != nil || saved.Project != current.Project || saved.Actor != current.Actor || saved.Action != current.Action || saved.ResourceID != current.ResourceID || len(current.PayloadDigest) != 64 || subtle.ConstantTimeCompare([]byte(saved.PayloadDigest), []byte(current.PayloadDigest)) != 1 {
		return ErrConflict
	}
	return nil
}
