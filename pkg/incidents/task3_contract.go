package incidents

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/datatug/datatug-core/pkg/investigation"
)

func (a Actor) Validate() error {
	if a.Kind != ActorHuman && a.Kind != ActorAgent && a.Kind != ActorSystem {
		return fmt.Errorf("invalid actor kind %q", a.Kind)
	}
	if strings.TrimSpace(a.ID) == "" {
		return fmt.Errorf("actor id is required")
	}
	if a.Via != "" && a.Via != ActorViaWeb && a.Via != ActorViaCLI && a.Via != ActorViaAPI && a.Via != ActorViaSlack {
		return fmt.Errorf("invalid actor via %q", a.Via)
	}
	return nil
}

func (i Incident) Validate() error {
	if err := i.Ref.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(i.UID) == "" || strings.TrimSpace(i.Title) == "" || i.LastSeq == 0 || !validStatus(i.Status) {
		return fmt.Errorf("incident requires uid, title, positive lastSeq, and valid status")
	}
	if i.Outcome != "" && !validOutcome(i.Outcome) {
		return fmt.Errorf("invalid outcome %q", i.Outcome)
	}
	if i.MergedInto != nil {
		if err := i.MergedInto.Validate(); err != nil {
			return fmt.Errorf("mergedInto: %w", err)
		}
		if *i.MergedInto == i.Ref {
			return fmt.Errorf("mergedInto must identify a different incident")
		}
	}
	for index, project := range i.Projects {
		if err := project.Validate(); err != nil {
			return fmt.Errorf("project %d: %w", index, err)
		}
	}
	for index, ref := range i.AssetRefs {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("asset ref %d: %w", index, err)
		}
	}
	for _, participant := range i.Participants {
		if participant.Role != ParticipantReporter {
			return fmt.Errorf("invalid participant role %q", participant.Role)
		}
		if err := participant.Actor.Validate(); err != nil {
			return err
		}
	}
	seenNotes := make(map[string]bool, len(i.NoteEntries))
	for index, note := range i.NoteEntries {
		if !validSegment(note.EventID) || strings.TrimSpace(note.Body) == "" {
			return fmt.Errorf("note entry %d requires eventId and body", index)
		}
		if seenNotes[note.EventID] {
			return fmt.Errorf("duplicate note eventId %q", note.EventID)
		}
		seenNotes[note.EventID] = true
		if len(i.NoteEntries) != len(i.Notes) || note.Body != i.Notes[index] {
			return fmt.Errorf("note entries must align exactly with notes")
		}
	}
	return i.CanonicalContext.Validate()
}

type FactVisibility string

const (
	FactVisible       FactVisibility = "visible"
	FactValueRedacted FactVisibility = "value-redacted"
	FactHidden        FactVisibility = "hidden"
)

// ViewPolicy is a set of decisions produced by the serving adapter's current
// policy evaluator. It is deliberately not persisted and performs no policy
// evaluation itself.
type ViewPolicy struct {
	Facts          map[string]FactVisibility
	WithheldEvents map[string]bool
}

// IncidentView is a detached current-policy projection. The stored Incident
// always contains canonical facts; only this read model may contain redaction
// markers.
type IncidentView struct {
	Ref              IncidentRef               `json:"ref"`
	UID              string                    `json:"uid"`
	Title            string                    `json:"title"`
	Description      string                    `json:"description,omitempty"`
	Status           Status                    `json:"status"`
	Outcome          Outcome                   `json:"outcome,omitempty"`
	MergedInto       *IncidentRef              `json:"mergedInto,omitempty"`
	Projects         []ProjectRef              `json:"projects,omitempty"`
	Participants     []Participant             `json:"participants,omitempty"`
	CanonicalContext investigation.ContextView `json:"canonicalContext"`
	AssetRefs        []ArtifactRef             `json:"assetRefs,omitempty"`
	Notes            []string                  `json:"notes,omitempty"`
	LastSeq          uint64                    `json:"lastSeq"`
}

func (i IncidentView) Validate() error {
	storedShape := Incident{
		Ref: i.Ref, UID: i.UID, Title: i.Title, Description: i.Description, Status: i.Status,
		Outcome: i.Outcome, MergedInto: i.MergedInto, Projects: i.Projects,
		Participants: i.Participants, AssetRefs: i.AssetRefs, Notes: i.Notes, LastSeq: i.LastSeq,
	}
	if err := storedShape.validateWithoutContext(); err != nil {
		return err
	}
	return i.CanonicalContext.Validate()
}

func (i Incident) validateWithoutContext() error {
	i.CanonicalContext = investigation.Context{}
	if err := i.Validate(); err != nil {
		return err
	}
	return nil
}

func ApplyIncidentView(stored Incident, policy ViewPolicy) IncidentView {
	return IncidentView{
		Ref: stored.Ref, UID: stored.UID, Title: stored.Title, Description: stored.Description,
		Status: stored.Status, Outcome: stored.Outcome, MergedInto: stored.MergedInto,
		Projects:         append([]ProjectRef(nil), stored.Projects...),
		Participants:     append([]Participant(nil), stored.Participants...),
		CanonicalContext: filterFacts(stored.CanonicalContext.Facts, policy),
		AssetRefs:        append([]ArtifactRef(nil), stored.AssetRefs...), Notes: filterNotes(stored, policy),
		LastSeq: stored.LastSeq,
	}
}

func filterNotes(stored Incident, policy ViewPolicy) []string {
	if len(stored.NoteEntries) == 0 {
		if len(policy.WithheldEvents) > 0 {
			return nil
		}
		return append([]string(nil), stored.Notes...)
	}
	notes := make([]string, 0, len(stored.NoteEntries))
	for _, note := range stored.NoteEntries {
		if !policy.WithheldEvents[note.EventID] {
			notes = append(notes, note.Body)
		}
	}
	return notes
}

func ApplyEventView(stored Event, policy ViewPolicy) (Event, bool, error) {
	if policy.WithheldEvents[stored.ID] {
		return Event{}, false, nil
	}
	view := stored
	view.Refs = append([]ArtifactRef(nil), stored.Refs...)
	if stored.Type == EventIncidentCreated {
		var payload CreatedPayload
		if err := decodePayload(stored.Payload, &payload); err != nil {
			return Event{}, false, err
		}
		viewPayload := CreatedViewPayload{
			UID: payload.UID, Title: payload.Title, Description: payload.Description,
			Projects: payload.Projects, Reporter: payload.Reporter,
			CanonicalContext: filterFacts(payload.CanonicalContext.Facts, policy),
		}
		// Every potentially failing JSON value above was decoded through the
		// canonical strict types first; this fixed view has no other fallible
		// marshaler, so an error is structurally unreachable.
		data, _ := json.Marshal(viewPayload)
		view.Payload = data
	}
	return view, true, nil
}

type CreatedViewPayload struct {
	UID              string                    `json:"uid"`
	Title            string                    `json:"title"`
	Description      string                    `json:"description"`
	Projects         []ProjectRef              `json:"projects,omitempty"`
	Reporter         Actor                     `json:"reporter"`
	CanonicalContext investigation.ContextView `json:"canonicalContext"`
}

func filterFacts(stored []investigation.Fact, policy ViewPolicy) investigation.ContextView {
	view := make([]investigation.FactView, 0, len(stored))
	for _, fact := range stored {
		switch policy.Facts[fact.ID] {
		case FactHidden:
			continue
		case FactValueRedacted:
			view = append(view, investigation.RedactedFact(fact, true))
		default:
			view = append(view, investigation.VisibleFact(fact))
		}
	}
	return investigation.ContextView{Facts: view}
}

type ListQuery struct {
	Statuses  []Status `json:"statuses,omitempty"`
	ProjectID string   `json:"project,omitempty"`
	QueryID   string   `json:"query,omitempty"`
	CheckID   string   `json:"check,omitempty"`
	BoardID   string   `json:"board,omitempty"`
}

func (q ListQuery) Validate() error {
	for _, status := range q.Statuses {
		if !validStatus(status) {
			return fmt.Errorf("invalid status %q", status)
		}
	}
	for name, value := range map[string]string{"project": q.ProjectID, "query": q.QueryID, "check": q.CheckID, "board": q.BoardID} {
		if value != "" && !validFilterValue(value) {
			return fmt.Errorf("invalid %s filter", name)
		}
	}
	return nil
}

func MatchesListQuery(incident Incident, query ListQuery) bool {
	if len(query.Statuses) > 0 && !containsStatus(query.Statuses, incident.Status) {
		return false
	}
	if query.ProjectID != "" && !hasProject(incident, query.ProjectID) {
		return false
	}
	return hasAssetFilter(incident, RefQuery, query.QueryID) && hasAssetFilter(incident, RefCheck, query.CheckID) && hasAssetFilter(incident, RefBoard, query.BoardID)
}

func containsStatus(values []Status, wanted Status) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func hasProject(incident Incident, projectID string) bool {
	for _, project := range incident.Projects {
		if project.ProjectID == projectID {
			return true
		}
	}
	return false
}

func hasAssetFilter(incident Incident, kind RefKind, id string) bool {
	if id == "" {
		return true
	}
	for _, ref := range incident.AssetRefs {
		if ref.Kind == kind && ref.Artifact != nil && ref.Artifact.ID == id {
			return true
		}
	}
	return false
}

func validFilterValue(value string) bool {
	if strings.TrimSpace(value) != value || value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

type FactSignal struct {
	Entity string                   `json:"entity"`
	Field  string                   `json:"field"`
	Value  investigation.TypedValue `json:"value"`
}

func (s FactSignal) Validate() error {
	if strings.TrimSpace(s.Entity) == "" || strings.TrimSpace(s.Field) == "" {
		return fmt.Errorf("fact signal requires entity and field")
	}
	return s.Value.Validate()
}

type SearchQuery struct {
	Text  string       `json:"text"`
	Facts []FactSignal `json:"facts,omitempty"`
}

func (q SearchQuery) Validate() error {
	if strings.TrimSpace(q.Text) == "" && len(q.Facts) == 0 {
		return fmt.Errorf("search text or at least one fact is required")
	}
	for i, fact := range q.Facts {
		if err := fact.Validate(); err != nil {
			return fmt.Errorf("fact %d: %w", i, err)
		}
	}
	return nil
}

type SignalKind string

const (
	SignalText    SignalKind = "text"
	SignalFact    SignalKind = "fact"
	SignalQuery   SignalKind = "query"
	SignalCheck   SignalKind = "check"
	SignalBoard   SignalKind = "board"
	SignalProject SignalKind = "project"
)

type MatchedSignal struct {
	Kind  SignalKind `json:"kind"`
	Value string     `json:"value"`
}

func (s MatchedSignal) Validate() error {
	switch s.Kind {
	case SignalText, SignalFact, SignalQuery, SignalCheck, SignalBoard, SignalProject:
	default:
		return fmt.Errorf("invalid matched signal kind %q", s.Kind)
	}
	if strings.TrimSpace(s.Value) == "" {
		return fmt.Errorf("matched signal value is required")
	}
	return nil
}

type SearchMatch struct {
	Incident       IncidentView    `json:"incident"`
	MatchedSignals []MatchedSignal `json:"matchedSignals"`
}

func (m SearchMatch) Validate() error {
	if err := m.Incident.Validate(); err != nil {
		return err
	}
	return validateSignals(m.MatchedSignals)
}

type SimilarMatch struct {
	Incident       IncidentView    `json:"incident"`
	Score          int             `json:"score"`
	MatchedSignals []MatchedSignal `json:"matchedSignals"`
}

func (m SimilarMatch) Validate() error {
	if err := m.Incident.Validate(); err != nil {
		return err
	}
	if m.Score <= 0 {
		return fmt.Errorf("similarity score must be positive")
	}
	return validateSignals(m.MatchedSignals)
}

func validateSignals(signals []MatchedSignal) error {
	if len(signals) == 0 {
		return fmt.Errorf("at least one matched signal is required")
	}
	for i, signal := range signals {
		if err := signal.Validate(); err != nil {
			return fmt.Errorf("signal %d: %w", i, err)
		}
	}
	return nil
}

// Search operates only on current-policy views. Hidden and redacted fact
// values therefore cannot influence a match or leak through matched signals.
func Search(candidates []IncidentView, query SearchQuery) []SearchMatch {
	if query.Validate() != nil {
		return nil
	}
	text := strings.ToLower(strings.TrimSpace(query.Text))
	var matches []SearchMatch
	for _, candidate := range candidates {
		signals := make([]MatchedSignal, 0, len(query.Facts)+1)
		if text == "" {
			// Fact-only search adds no synthetic empty text signal.
		} else if strings.Contains(strings.ToLower(candidate.Title+"\n"+candidate.Description+"\n"+strings.Join(candidate.Notes, "\n")), text) {
			signals = append(signals, MatchedSignal{Kind: SignalText, Value: text})
		} else {
			continue
		}
		matchedFacts := true
		for _, wanted := range query.Facts {
			if hasFact(candidate, wanted) {
				signals = append(signals, MatchedSignal{Kind: SignalFact, Value: factSignalKey(wanted)})
			} else {
				matchedFacts = false
				break
			}
		}
		if matchedFacts {
			matches = append(matches, SearchMatch{Incident: candidate, MatchedSignals: signals})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Incident.Ref.String() < matches[j].Incident.Ref.String() })
	return matches
}

// Similar operates only on current-policy views for the same reason as Search.
func Similar(subject IncidentView, candidates []IncidentView) []SimilarMatch {
	var matches []SimilarMatch
	for _, candidate := range candidates {
		if candidate.Ref == subject.Ref {
			continue
		}
		signals, score := similaritySignals(subject, candidate)
		if score > 0 {
			matches = append(matches, SimilarMatch{Incident: candidate, Score: score, MatchedSignals: signals})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Incident.Ref.String() < matches[j].Incident.Ref.String()
	})
	return matches
}

func similaritySignals(left, right IncidentView) ([]MatchedSignal, int) {
	var signals []MatchedSignal
	for _, fact := range left.CanonicalContext.Facts {
		if fact.Value.Value == nil {
			continue
		}
		signal := FactSignal{Entity: fact.Entity, Field: fact.Field, Value: *fact.Value.Value}
		if hasFact(right, signal) {
			signals = append(signals, MatchedSignal{Kind: SignalFact, Value: factSignalKey(signal)})
		}
	}
	for _, ref := range left.AssetRefs {
		if ref.Artifact == nil || !hasMatchingAsset(right.AssetRefs, ref) {
			continue
		}
		kind := map[RefKind]SignalKind{RefQuery: SignalQuery, RefCheck: SignalCheck, RefBoard: SignalBoard}[ref.Kind]
		if kind != "" {
			signals = append(signals, MatchedSignal{Kind: kind, Value: ref.Artifact.ID})
		}
	}
	for _, project := range left.Projects {
		if hasExactProject(right.Projects, project) {
			signals = append(signals, MatchedSignal{Kind: SignalProject, Value: project.StoreID + "/" + project.ProjectID + "/" + project.Environment})
		}
	}
	for _, token := range sharedTextTokens(left.Title+" "+left.Description, right.Title+" "+right.Description) {
		signals = append(signals, MatchedSignal{Kind: SignalText, Value: token})
	}
	sort.Slice(signals, func(i, j int) bool {
		if signals[i].Kind != signals[j].Kind {
			return signals[i].Kind < signals[j].Kind
		}
		return signals[i].Value < signals[j].Value
	})
	score := 0
	for _, signal := range signals {
		switch signal.Kind {
		case SignalFact:
			score += 4
		case SignalCheck:
			score += 3
		case SignalQuery, SignalBoard:
			score += 2
		default:
			score++
		}
	}
	return signals, score
}

func hasFact(incident IncidentView, wanted FactSignal) bool {
	for _, fact := range incident.CanonicalContext.Facts {
		if fact.Value.Value != nil && fact.Entity == wanted.Entity && fact.Field == wanted.Field && *fact.Value.Value == wanted.Value {
			return true
		}
	}
	return false
}

func factSignalKey(signal FactSignal) string {
	value, _ := json.Marshal(signal.Value)
	return signal.Entity + "." + signal.Field + "=" + string(value)
}

func hasMatchingAsset(refs []ArtifactRef, wanted ArtifactRef) bool {
	for _, ref := range refs {
		if artifactRefsEqual(ref, wanted) {
			return true
		}
	}
	return false
}

func hasExactProject(projects []ProjectRef, wanted ProjectRef) bool {
	for _, project := range projects {
		if project == wanted {
			return true
		}
	}
	return false
}

func sharedTextTokens(left, right string) []string {
	rightTokens := textTokens(right)
	seen := map[string]bool{}
	var shared []string
	for token := range textTokens(left) {
		if rightTokens[token] && !seen[token] {
			seen[token] = true
			shared = append(shared, token)
		}
	}
	sort.Strings(shared)
	return shared
}

func textTokens(value string) map[string]bool {
	result := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if len(token) >= 4 {
			result[token] = true
		}
	}
	return result
}

type EventCursor string

func (c EventCursor) Validate() error {
	value := string(c)
	if value == "" || len(value) > 2048 || strings.TrimSpace(value) != value {
		return fmt.Errorf("invalid event cursor")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return fmt.Errorf("invalid event cursor")
		}
	}
	return nil
}

type WatchQuery struct {
	Incident *IncidentRef `json:"incident,omitempty"`
	Since    EventCursor  `json:"since,omitempty"`
}

func (q WatchQuery) Validate() error {
	if q.Incident != nil {
		if err := q.Incident.Validate(); err != nil {
			return err
		}
	}
	if q.Since != "" {
		return q.Since.Validate()
	}
	return nil
}

type StreamItem struct {
	Cursor EventCursor `json:"cursor"`
	Event  Event       `json:"event"`
}

// ApplyStreamItemView applies the same current-policy event decision to watch
// items while preserving the provider's opaque resumable cursor.
func ApplyStreamItemView(stored StreamItem, policy ViewPolicy) (StreamItem, bool, error) {
	event, visible, err := ApplyEventView(stored.Event, policy)
	if err != nil || !visible {
		return StreamItem{}, visible, err
	}
	return StreamItem{Cursor: stored.Cursor, Event: event}, true, nil
}

func (i StreamItem) Validate() error {
	if err := i.Cursor.Validate(); err != nil {
		return err
	}
	return i.Event.ValidateView()
}

type EventStream interface {
	Next(ctx context.Context) (StreamItem, error)
	Close() error
}
