package incidents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
	"unicode"
)

type StoreLocationKind string

const (
	StoreLocationProjectRepository     StoreLocationKind = "project-repository"
	StoreLocationDedicatedRepository   StoreLocationKind = "dedicated-repository"
	StoreLocationApplicationRepository StoreLocationKind = "application-repository"
)

// StoreLocation selects where one incident store is hosted. All kinds use
// SharedLayout; adapters differ only in how their DALgo database is opened.
type StoreLocation struct {
	StoreID string            `json:"storeId"`
	Kind    StoreLocationKind `json:"kind"`
	Project *ProjectRef       `json:"project,omitempty"`
}

func (l StoreLocation) Validate() error {
	if !validSegment(l.StoreID) {
		return fmt.Errorf("incidents: invalid storeId %q", l.StoreID)
	}
	switch l.Kind {
	case StoreLocationProjectRepository:
		if l.Project == nil {
			return fmt.Errorf("incidents: project-repository location requires project")
		}
		if err := l.Project.Validate(); err != nil {
			return err
		}
		if l.Project.StoreID != l.StoreID {
			return fmt.Errorf("incidents: location and project store ids differ")
		}
	case StoreLocationDedicatedRepository, StoreLocationApplicationRepository:
		if l.Project != nil {
			return fmt.Errorf("incidents: %s location must not carry project", l.Kind)
		}
	default:
		return fmt.Errorf("incidents: invalid store location kind %q", l.Kind)
	}
	return nil
}

type SharedLayout struct {
	IncidentDir         string
	Events              string
	Projection          string
	MutationReceiptsDir string
	StoreLock           string
	MergeIntentsDir     string
}

func LayoutFor(ref IncidentRef) (SharedLayout, error) {
	if err := ref.Validate(); err != nil {
		return SharedLayout{}, err
	}
	incidentDir := path.Join("incidents", ref.IncidentID)
	storeDir := path.Join("incidents", ".store")
	return SharedLayout{
		IncidentDir:         incidentDir,
		Events:              path.Join(incidentDir, "events.jsonl"),
		Projection:          path.Join(incidentDir, "incident.json"),
		MutationReceiptsDir: path.Join(storeDir, "mutations"),
		StoreLock:           path.Join(storeDir, "lock"),
		MergeIntentsDir:     path.Join(storeDir, "merge-intents"),
	}, nil
}

type EventDraft struct {
	At        time.Time       `json:"at"`
	Actor     Actor           `json:"actor"`
	Type      EventType       `json:"type"`
	Assertion Assertion       `json:"assertion"`
	Refs      []ArtifactRef   `json:"refs,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

func (d EventDraft) Validate(incident IncidentRef) error {
	return (Event{
		ID: "draft", Seq: 1, At: d.At, VisibleAt: d.At, Incident: incident,
		Actor: d.Actor, Type: d.Type, Assertion: d.Assertion, Refs: d.Refs, Payload: d.Payload,
	}).Validate()
}

type Mutation struct {
	MutationID  string      `json:"mutationId"`
	Incident    IncidentRef `json:"incident"`
	ExpectedSeq *uint64     `json:"expectedSeq,omitempty"`
	Event       EventDraft  `json:"event"`
}

func (m Mutation) Validate() error {
	if !validSegment(m.MutationID) {
		return fmt.Errorf("incidents: invalid mutationId %q", m.MutationID)
	}
	if err := m.Incident.Validate(); err != nil {
		return err
	}
	return m.Event.Validate(m.Incident)
}

type AppendResult struct {
	Event      Event    `json:"event"`
	Projection Incident `json:"projection"`
	Replayed   bool     `json:"replayed"`
}

type MergeMutation struct {
	MutationID string      `json:"mutationId"`
	Source     IncidentRef `json:"source"`
	Into       IncidentRef `json:"into"`
}

func (m MergeMutation) Validate() error {
	if !validSegment(m.MutationID) {
		return fmt.Errorf("incidents: invalid mutationId %q", m.MutationID)
	}
	if err := m.Source.Validate(); err != nil {
		return err
	}
	if err := m.Into.Validate(); err != nil {
		return err
	}
	if m.Source.StoreID != m.Into.StoreID {
		return fmt.Errorf("incidents: cross-store merge is not supported")
	}
	if m.Source == m.Into {
		return fmt.Errorf("incidents: source and destination must differ")
	}
	return nil
}

type MergeResult struct {
	Source   Incident `json:"source"`
	Into     Incident `json:"into"`
	Replayed bool     `json:"replayed"`
}

var (
	ErrMutationConflict = errors.New("incident mutation id was reused with different content")
	ErrSequenceConflict = errors.New("incident event sequence changed")
)

// Store is implemented once per configured StoreLocation. Append and Merge
// own cross-process exclusion, durable mutation receipts, and crash recovery.
type Store interface {
	Append(ctx context.Context, mutation Mutation) (AppendResult, error)
	Events(ctx context.Context, ref IncidentRef, afterSeq uint64) ([]Event, error)
	Projection(ctx context.Context, ref IncidentRef, at *time.Time) (Incident, error)
	Merge(ctx context.Context, mutation MergeMutation) (MergeResult, error)
}

func validSegment(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || value == "." || value == ".." || strings.ContainsAny(value, `/\`) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
