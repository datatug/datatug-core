package apicontract

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/datatug/datatug-core/pkg/incidents"
)

// RowEvidence is optional retained-result metadata. Structural validity does not
// establish current rights: the provider must re-admit every evidence open.
type RowEvidence struct {
	Version              int               `json:"version"`
	Execution            ExecutionRef      `json:"execution"`
	RecordsetFingerprint string            `json:"recordsetFingerprint"`
	Scope                RowEvidenceScope  `json:"scope"`
	Source               RowEvidenceSource `json:"source"`
	RowKeyColumn         string            `json:"rowKeyColumn"`
	Rows                 []RowEvidenceRow  `json:"rows"`
}
type RowEvidenceScope struct {
	StoreID     string `json:"storeId"`
	SpaceID     string `json:"spaceID,omitempty"`
	ProjectID   string `json:"projectId"`
	Environment string `json:"environment"`
}
type RowEvidenceSource struct {
	ID         string `json:"id"`
	Generation string `json:"generation"`
}
type RowEvidenceRow struct {
	RowKey TypedValue                 `json:"rowKey"`
	Slots  map[string]RowEvidenceSlot `json:"slots"`
}
type RowEvidenceSlot struct {
	State string                  `json:"state"`
	Refs  []incidents.ArtifactRef `json:"refs,omitempty"`
}

// RowEvidenceSelector retains the exact held row and slot, including scope and
// source generation. It never names a new query or authorizes a hidden rerun.
type RowEvidenceSelector struct {
	Execution            ExecutionRef      `json:"execution"`
	RecordsetFingerprint string            `json:"recordsetFingerprint"`
	Scope                RowEvidenceScope  `json:"scope"`
	Source               RowEvidenceSource `json:"source"`
	RowKeyColumn         string            `json:"rowKeyColumn"`
	RowKey               TypedValue        `json:"rowKey"`
	SlotColumn           string            `json:"slotColumn"`
}

func canonicalEvidenceText(s string) bool { return s != "" && strings.TrimSpace(s) == s }

// UnmarshalJSON rejects unknown/duplicate fields even when its Result owner is
// decoded with encoding/json rather than DecodeStrict.
func (e *RowEvidence) UnmarshalJSON(data []byte) error {
	type wire RowEvidence
	var value wire
	if err := DecodeStrict(data, &value); err != nil {
		return err
	}
	*e = RowEvidence(value)
	return nil
}

func evidenceKey(value TypedValue) (string, error) {
	if err := value.Validate(); err != nil {
		return "", err
	}
	if value.Type != ValueTypeString && value.Type != ValueTypeInteger {
		return "", fmt.Errorf("row key must be a typed string or integer")
	}
	return string(value.Type) + ":" + value.Str, nil
}

// ValidateForResult verifies metadata against the exact raw held recordset.
// Missing metadata is permitted; supplied malformed metadata is rejected.
func (e RowEvidence) ValidateForResult(r Result) error {
	if e.Version != 1 {
		return fmt.Errorf("unsupported rowEvidence version")
	}
	if r.Execution == nil || e.Execution != *r.Execution {
		return fmt.Errorf("rowEvidence execution mismatch")
	}
	if err := e.Execution.Validate(); err != nil {
		return err
	}
	scope := incidents.ProjectRef{StoreID: e.Scope.StoreID, ProjectID: e.Scope.ProjectID, Environment: e.Scope.Environment}
	if err := scope.Validate(); err != nil {
		return err
	}
	if !canonicalEvidenceText(e.Scope.Environment) || (e.Scope.SpaceID != "" && !canonicalEvidenceText(e.Scope.SpaceID)) || (e.Scope.StoreID == "firestore" && e.Scope.SpaceID == "") {
		return fmt.Errorf("rowEvidence requires environment and hosted Space scope")
	}
	if e.Execution.StoreID != e.Scope.StoreID || e.Execution.ProjectID != e.Scope.ProjectID {
		return fmt.Errorf("rowEvidence scope mismatch")
	}
	if !canonicalEvidenceText(e.Source.ID) || !canonicalEvidenceText(e.Source.Generation) {
		return fmt.Errorf("rowEvidence requires source and generation")
	}
	fingerprint, err := FingerprintRecordset(r.Recordset)
	if err != nil {
		return err
	}
	if fingerprint != e.RecordsetFingerprint {
		return fmt.Errorf("rowEvidence recordset fingerprint mismatch")
	}
	columns := make(map[string]int, len(r.Recordset.Columns))
	for i, column := range r.Recordset.Columns {
		if !canonicalEvidenceText(column.Name) {
			return fmt.Errorf("rowEvidence column names must be canonical")
		}
		if _, exists := columns[column.Name]; exists {
			return fmt.Errorf("rowEvidence duplicate column")
		}
		columns[column.Name] = i
	}
	keyIndex, exists := columns[e.RowKeyColumn]
	if !exists {
		return fmt.Errorf("rowEvidence row key column missing")
	}
	held := make(map[string]bool, len(r.Recordset.Rows))
	for _, row := range r.Recordset.Rows {
		key, err := evidenceKey(row[keyIndex])
		if err != nil {
			return err
		}
		if held[key] {
			return fmt.Errorf("rowEvidence duplicate held row key")
		}
		held[key] = true
	}
	seen := make(map[string]bool, len(e.Rows))
	for _, row := range e.Rows {
		key, err := evidenceKey(row.RowKey)
		if err != nil {
			return err
		}
		if !held[key] || seen[key] {
			return fmt.Errorf("rowEvidence unknown or duplicate row key")
		}
		seen[key] = true
		for column, slot := range row.Slots {
			if _, exists := columns[column]; !exists {
				return fmt.Errorf("rowEvidence slot column missing")
			}
			switch slot.State {
			case "unavailable":
				if len(slot.Refs) != 0 {
					return fmt.Errorf("unavailable rowEvidence must not carry refs")
				}
			case "available":
				matching := false
				for _, ref := range slot.Refs {
					if err := ref.Validate(); err != nil {
						return err
					}
					if ref.Kind == incidents.RefExecution && ref.Execution != nil && *ref.Execution == e.Execution {
						matching = true
					}
				}
				if !matching {
					return fmt.Errorf("available rowEvidence requires the exact retained execution ref")
				}
			default:
				return fmt.Errorf("invalid rowEvidence slot state")
			}
		}
	}
	return nil
}

// ResolveRowEvidence returns a detached structurally available slot. false means
// absent/unavailable metadata, never a passing result or a grant of access.
func (r Result) ResolveRowEvidence(selector RowEvidenceSelector) (RowEvidenceSlot, bool, error) {
	if err := r.Validate(); err != nil {
		return RowEvidenceSlot{}, false, err
	}
	if _, err := evidenceKey(selector.RowKey); err != nil {
		return RowEvidenceSlot{}, false, err
	}
	if !canonicalEvidenceText(selector.SlotColumn) {
		return RowEvidenceSlot{}, false, fmt.Errorf("rowEvidence selector slot is required")
	}
	e := r.RowEvidence
	if e == nil {
		return RowEvidenceSlot{}, false, nil
	}
	if selector.Execution != e.Execution || selector.RecordsetFingerprint != e.RecordsetFingerprint || selector.Scope != e.Scope || selector.Source != e.Source || selector.RowKeyColumn != e.RowKeyColumn {
		return RowEvidenceSlot{}, false, fmt.Errorf("rowEvidence selector mismatch")
	}
	for _, row := range e.Rows {
		if row.RowKey != selector.RowKey {
			continue
		}
		slot, exists := row.Slots[selector.SlotColumn]
		if !exists || slot.State != "available" {
			return RowEvidenceSlot{}, false, nil
		}
		// ArtifactRef contains pointers. Detach the returned graph from held data.
		data, _ := json.Marshal(slot)
		var detached RowEvidenceSlot
		_ = json.Unmarshal(data, &detached)
		return detached, true, nil
	}
	return RowEvidenceSlot{}, false, nil
}
