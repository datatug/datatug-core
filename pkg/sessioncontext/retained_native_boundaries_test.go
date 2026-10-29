package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

func TestRetainedNativeExecutionRejectsMismatchedAuthorityAndState(t *testing.T) {
	inv, _, p, owner, id, observation, read := retainedDetectiveFixture(t, true)
	if _, _, err := owner.ClaimDispatch(context.Background(), p, id); err != nil {
		t.Fatal(err)
	}
	accepted := read()
	completeObservation(observation)
	if _, err := owner.Reconcile(context.Background(), p, id); err != nil {
		t.Fatal(err)
	}
	settled := read()
	for _, mode := range []string{"run envelope", "input reference", "action", "request digest", "session state", "accepted completion", "unknown outcome", "owner envelope", "premature receipt", "handoff state", "produced identity"} {
		t.Run(mode, func(t *testing.T) {
			base := inv
			collection, docID := Runs, id.RunID
			switch mode {
			case "owner envelope", "produced identity":
				base = accepted
				collection = DetectiveOwners
			case "premature receipt", "handoff state":
				base = settled
				collection = DetectiveOwners
				if mode == "handoff state" {
					collection = Outbox
				}
			case "session state":
				collection = Sessions
				docID = id.SessionID
			case "input reference", "action", "request digest":
				collection = Inputs
				docID = ""
			}
			bad := mutateRetainedContext(t, base, collection, func() string {
				if docID != "" {
					return docID
				}
				for _, d := range base.Documents {
					if d.Collection == collection {
						return d.ID
					}
				}
				t.Fatal("missing input")
				return ""
			}(), func(d *RetainedDocument) {
				if mode == "run envelope" {
					d.SessionID = "different"
					return
				}
				if mode == "owner envelope" {
					d.Owner = "bob"
					return
				}
				var b map[string]any
				_ = json.Unmarshal(d.Payload, &b)
				switch mode {
				case "input reference":
					b["detective"].(map[string]any)["reference"].(map[string]any)["environmentId"] = "foreign"
				case "action":
					b["detective"].(map[string]any)["dispatch"].(map[string]any)["action"] = "future-action"
				case "request digest":
					b["detective"].(map[string]any)["dispatch"].(map[string]any)["nativeRequestDigest"] = strings.Repeat("a", 64)
				case "session state":
					b["state"] = "future"
				case "accepted completion":
					b["completedAt"] = time.Now().UTC()
				case "unknown outcome":
					b["state"] = "future"
				case "premature receipt":
					b["handoffReceiptId"] = "receipt"
				case "handoff state":
					b["state"] = "future"
				case "produced identity":
					b["observation"] = map[string]any{"produced": true, "identity": map[string]any{}, "nativeRunId": "run"}
				}
				d.Payload, _ = json.Marshal(b)
			})
			// Keep the work dispatch consistent so the independent digest check is
			// reached before the separately sealed owner protocol rejects the corruption.
			if mode == "request digest" {
				bad = mutateRetainedBody(t, bad, DetectiveWork, id.RunID, func(b map[string]any) {
					b["dispatch"].(map[string]any)["nativeRequestDigest"] = strings.Repeat("a", 64)
				})
			}
			out, err := ProjectRetainedDetectiveExecutions(context.Background(), bad)
			if !errors.Is(err, ErrUnavailable) || out != nil {
				t.Fatal("corrupt native source exposed output", mode, err)
			}
		})
	}
	original := canonicalRetainedInputReference
	t.Cleanup(func() { canonicalRetainedInputReference = original })
	canonicalRetainedInputReference = func(apicontract.SessionInputReference) (apicontract.SessionInputReference, error) {
		return apicontract.SessionInputReference{}, errors.New("canonical codec failed")
	}
	if out, err := ProjectRetainedDetectiveExecutions(context.Background(), inv); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatal("canonical reference failure exposed output")
	}
}

func TestRetainedNativeInventoryRejectsOrphanInputsAndDuplicateConsumption(t *testing.T) {
	inv, _, _, _, _, _, _ := retainedDetectiveFixture(t, false)
	var input RetainedDocument
	for _, d := range inv.Documents {
		if d.Collection == Inputs {
			input = d
		}
	}
	orphan := inv
	orphan.Documents = append(append([]RetainedDocument(nil), inv.Documents...), input)
	orphan.Documents[len(orphan.Documents)-1].ID = "orphan"
	if out, err := ProjectRetainedDetectiveExecutions(context.Background(), orphan); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatal("orphan input ignored")
	}
	bad := inv
	bad.Documents = append([]RetainedDocument(nil), inv.Documents...)
	for i := range bad.Documents {
		if bad.Documents[i].Collection == Inputs {
			bad.Documents[i].Owner = ""
			break
		}
	}
	if _, err := retainedReader(bad); !errors.Is(err, ErrUnavailable) {
		t.Fatal("ownerless retained envelope admitted")
	}
	// Duplicate input consumption cannot arise from current writers, but every
	// retained native row must independently claim one retained input.
	old := decodeRetainedSource
	t.Cleanup(func() { decodeRetainedSource = old })
	seen := 0
	decodeRetainedSource = func(raw []byte, v any, forbidden ...string) error {
		if err := old(raw, v, forbidden...); err != nil {
			return err
		}
		if input, ok := v.(*InputSnapshot); ok {
			seen++
			if seen == 1 {
				input.ID = "not-in-manifest"
			}
		}
		return nil
	}
	if out, err := ProjectRetainedDetectiveExecutions(context.Background(), inv); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatal("input identity mismatch admitted")
	}
}
