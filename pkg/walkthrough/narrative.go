package walkthrough

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"unicode/utf8"
)

// NarrativeSchemaVersion is the only narrative sidecar schema version this
// package admits. It is distinct from the declaration schema major.
const NarrativeSchemaVersion = 1

// Sidecar, catalogue and presentation bounds from the owner declaration and
// training-package formats. Every text asset, including the narrative sidecar,
// is bound by the independent 64 KiB per-text-asset cap. The parser applies it
// to the sidecar directly and admission accounting applies it to every other
// contributed text asset; the combined 6 MiB aggregate is likewise enforced
// during admission.
const (
	MaxTextAssetBytes        = 64 << 10
	MaxNarrativeSidecarBytes = MaxTextAssetBytes
	MaxNarrativeTextBytes    = 6 << 20
	MaxCatalogueBytes        = 64 << 10
	MaxCatalogueMessages     = 1000
	MaxNarrativeCatalogues   = 20
	MaxNarrativeChapterRefs  = 20
	MaxNarrativeActors       = 50
	MaxNarrativeAssets       = 100
	MaxNarrativeSegments     = 200
	MaxNarrativeEvents       = 1000
	MaxNarrativeNameChars    = 120
	MaxNarrativeMediaType    = 120
	MaxPresentationDelayMs   = 10000
	MaxTypingCharsPerSecond  = 200
	MinTypingCharsPerSecond  = 5
)

var (
	catalogueKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]{0,127}$`)
)

// NarrativeOutcomes is the closed outcome registry shared by the checkpoint
// envelope, check.run references and branch results. Assistance is always
// orthogonal.
var NarrativeOutcomes = []string{"passed", "failed", "cancelled", "timed-out", "error"}

// NarrativeEventTypes is the closed sidecar event registry.
var NarrativeEventTypes = []string{
	"chat.message",
	"detective.create",
	"detective.open",
	"evidence.reveal",
	"challenge.prompt",
	"challenge.await_validation",
	"check.run",
	"postmortem.recommend",
	"camera.focus",
	"branch",
	"checkpoint",
}

// CatalogueReference pins one immutable message catalogue. Locale is stored in
// canonical BCP-47 form after owner-profile admission.
type CatalogueReference struct {
	Locale string
	Path   string
	SHA256 string
	Bytes  Integer
}

// Asset returns the reference's asset view.
func (r CatalogueReference) Asset() AssetReference {
	return AssetReference{Path: r.Path, SHA256: r.SHA256, Bytes: r.Bytes}
}

// NarrativeActor is one fictional presentation identity.
type NarrativeActor struct {
	ID   string
	Name string
}

// NarrativeAsset is one pinned sidecar asset. MediaType is descriptive and
// never decides text/binary accounting.
type NarrativeAsset struct {
	ID        string
	MediaType string
	Path      string
	SHA256    string
	Bytes     Integer
}

// Asset returns the asset's reference view.
func (a NarrativeAsset) Asset() AssetReference {
	return AssetReference{Path: a.Path, SHA256: a.SHA256, Bytes: a.Bytes}
}

// NarrativeSegment binds one declared scripted-dialogue step to its ordered
// normal events and visibly labelled prerequisite context events.
type NarrativeSegment struct {
	ID                          string
	ChapterID                   string
	StepID                      string
	EventIDs                    []string
	PrerequisiteContextEventIDs []string
}

// Presentation is the optional per-event presentation hint. Defaults are
// materialized so the projection is stable across languages.
type Presentation struct {
	DelayBeforeMs             int
	PauseAfterMs              int
	TypingSpeedPreset         string
	TypingSpeedCharsPerSecond int
	TypingIndicator           bool
	Transition                string
	CameraTargetID            string
}

// ChatMessagePayload carries a catalogue key and declared attachments. A
// literal message field is not representable.
type ChatMessagePayload struct {
	ActorID       string
	MessageKey    string
	AttachmentIDs []string
}

// DetectivePayload creates or opens a package-local training Case projection.
type DetectivePayload struct {
	ManifestCaseID string
}

// EvidenceRevealPayload reveals one declared sidecar asset.
type EvidenceRevealPayload struct {
	AssetID string
}

// ChallengePromptPayload presents a declared challenge step's pinned challenge.
type ChallengePromptPayload struct {
	StepID string
}

// ChallengeAwaitValidationPayload waits for the step's single whole-challenge
// checkpoint.
type ChallengeAwaitValidationPayload struct {
	StepID       string
	CheckpointID string
}

// CheckRunPayload carries a labelled reference outcome. It never runs a Check
// during reconstruction.
type CheckRunPayload struct {
	CheckID           string
	ReferenceOutcome  string
	ReferenceAssisted bool
}

// PostmortemRecommendPayload presents an unsaved package proposal.
type PostmortemRecommendPayload struct {
	ProposalID string
}

// CameraFocusPayload points at a registered presentation target.
type CameraFocusPayload struct {
	TargetID string
}

// BranchPayload is a finite declarative branch within one segment.
type BranchPayload struct {
	StepID        string
	CheckpointID  string
	Outcome       string
	Assisted      bool
	TargetEventID string
}

// CheckpointPayload is a pure reconstruction marker.
type CheckpointPayload struct {
	StateKey string
}

// NarrativeEvent is one closed-registry sidecar event. Exactly the payload
// pointer matching Type is populated.
type NarrativeEvent struct {
	ID                       string
	ChapterID                string
	Type                     string
	ChatMessage              *ChatMessagePayload
	Detective                *DetectivePayload
	EvidenceReveal           *EvidenceRevealPayload
	ChallengePrompt          *ChallengePromptPayload
	ChallengeAwaitValidation *ChallengeAwaitValidationPayload
	CheckRun                 *CheckRunPayload
	PostmortemRecommend      *PostmortemRecommendPayload
	CameraFocus              *CameraFocusPayload
	Branch                   *BranchPayload
	Checkpoint               *CheckpointPayload
	Presentation             *Presentation
}

// NarrativeSidecar is a structurally valid narrative sidecar schema 1. It is
// not an admitted narrative: chapter/step binding, owner references, catalogue
// resolution and text budgets require [AdmitNarrative].
type NarrativeSidecar struct {
	SchemaVersion     Integer
	ScenarioID        string
	Revision          Integer
	Locale            string
	FallbackLocale    string
	MessageCatalogues []CatalogueReference
	ChapterRefs       []string
	Actors            []NarrativeActor
	Assets            []NarrativeAsset
	Segments          []NarrativeSegment
	Events            []NarrativeEvent
}

// NarrativeResult is the outcome of one closed sidecar parse.
type NarrativeResult struct {
	Accepted bool
	Digest   string
	Sidecar  *NarrativeSidecar
	Issues   []Issue
}

// Err returns nil for an accepted sidecar or the aggregate recoverable refusal.
func (r NarrativeResult) Err() error {
	if r.Accepted {
		return nil
	}
	return &Error{Issues: r.Issues}
}

// CatalogueResult is the outcome of one closed catalogue parse.
type CatalogueResult struct {
	Accepted  bool
	Digest    string
	Catalogue *MessageCatalogue
	Issues    []Issue
}

// Err returns nil for an accepted catalogue or the aggregate refusal.
func (r CatalogueResult) Err() error {
	if r.Accepted {
		return nil
	}
	return &Error{Issues: r.Issues}
}

// MessageCatalogue is one pinned catalogue. Keys returns the stable sorted key
// order; Values keeps the exact plain text.
type MessageCatalogue struct {
	SchemaVersion Integer
	Locale        string
	Values        map[string]string
}

// Keys returns the catalogue keys in code-point order.
func (c *MessageCatalogue) Keys() []string {
	keys := make([]string, 0, len(c.Values))
	for key := range c.Values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ParseNarrativeSidecar validates one narrative sidecar schema 1 from exact
// bytes. It checks closed shape, bounds, uniqueness and sidecar-local
// references; declaration binding, catalogues, owner lookups and budgets are
// [AdmitNarrative]'s job. No partially admitted sidecar is exposed.
func ParseNarrativeSidecar(data []byte) NarrativeResult {
	digest, root, issue := decodeNarrativeAsset(data, MaxNarrativeSidecarBytes, "narrative sidecar")
	result := NarrativeResult{Digest: digest}
	if issue != nil {
		result.Issues = []Issue{*issue}
		return result
	}
	c := newChecker(true)
	c.schemaVersionOne(root, "")
	sidecar := c.narrativeSidecar(root)
	result.Issues = c.issues
	if len(c.issues) == 0 {
		result.Accepted = true
		result.Sidecar = sidecar
	}
	return result
}

// ParseMessageCatalogue validates one closed message catalogue from exact
// bytes. The file bound applies before parsing.
func ParseMessageCatalogue(data []byte) CatalogueResult {
	digest, root, issue := decodeNarrativeAsset(data, MaxCatalogueBytes, "message catalogue")
	result := CatalogueResult{Digest: digest}
	if issue != nil {
		result.Issues = []Issue{*issue}
		return result
	}
	c := newChecker(true)
	c.schemaVersionOne(root, "")
	locale := c.authoredLocale(root, "", "locale", true)
	messagesValue, present := root.member("messages")
	messages := make(map[string]string)
	if !present {
		c.report(CodeMissingField, "/messages", "required field is missing")
	} else if messagesValue.kind != jsonObject {
		c.report(CodeInvalidType, "/messages", "must be a JSON object")
	} else {
		if len(messagesValue.object) < 1 {
			c.report(CodeTooMany, "/messages", "must contain at least 1 message")
		}
		if len(messagesValue.object) > MaxCatalogueMessages {
			c.report(CodeTooMany, "/messages", "must contain at most %d messages", MaxCatalogueMessages)
		}
		for _, member := range messagesValue.object {
			path := pointerChild("/messages", member.key)
			if !catalogueKeyPattern.MatchString(member.key) {
				c.report(CodeInvalidValue, path,
					"message key must match ^[a-z][a-z0-9.-]{0,127}$")
			}
			if member.value.kind != jsonString {
				c.report(CodeInvalidType, path, "message must be a string")
				continue
			}
			if member.value.text == "" {
				c.report(CodeInvalidValue, path, "message must not be empty")
				continue
			}
			messages[member.key] = member.value.text
		}
	}
	c.rejectUnknown(root, "", catalogueFields)
	result.Issues = c.issues
	if len(c.issues) == 0 {
		result.Accepted = true
		result.Catalogue = &MessageCatalogue{
			SchemaVersion: Integer("1"),
			Locale:        locale,
			Values:        messages,
		}
	}
	return result
}

var narrativeSidecarFields = map[string]bool{
	"schemaVersion": true, "scenarioId": true, "revision": true,
	"locale": true, "fallbackLocale": true, "messageCatalogues": true,
	"chapterRefs": true, "actors": true, "assets": true, "segments": true,
	"events": true,
}

var catalogueFields = map[string]bool{
	"schemaVersion": true, "locale": true, "messages": true,
}

var catalogueReferenceFields = map[string]bool{
	"locale": true, "path": true, "sha256": true, "bytes": true,
}

var narrativeActorFields = map[string]bool{"id": true, "name": true}

var narrativeAssetFields = map[string]bool{
	"id": true, "mediaType": true, "path": true, "sha256": true, "bytes": true,
}

var narrativeSegmentFields = map[string]bool{
	"id": true, "chapterId": true, "stepId": true, "eventIds": true,
	"prerequisiteContextEventIds": true,
}

var narrativeEventFields = map[string]bool{
	"id": true, "chapterId": true, "type": true, "payload": true,
	"presentation": true,
}

var presentationFields = map[string]bool{
	"delayBeforeMs": true, "pauseAfterMs": true, "typingSpeed": true,
	"typingIndicator": true, "transition": true, "cameraTargetId": true,
}

var chatMessagePayloadFields = map[string]bool{
	"actorId": true, "messageKey": true, "attachmentIds": true,
}
var detectivePayloadFields = map[string]bool{"manifestCaseId": true}
var evidenceRevealPayloadFields = map[string]bool{"assetId": true}
var challengePromptPayloadFields = map[string]bool{"stepId": true}
var challengeAwaitPayloadFields = map[string]bool{"stepId": true, "checkpointId": true}
var checkRunPayloadFields = map[string]bool{
	"checkId": true, "referenceOutcome": true, "referenceAssisted": true,
}
var postmortemPayloadFields = map[string]bool{"proposalId": true}
var cameraFocusPayloadFields = map[string]bool{"targetId": true}
var branchPayloadFields = map[string]bool{
	"stepId": true, "checkpointId": true, "outcome": true, "assisted": true,
	"targetEventId": true,
}
var checkpointPayloadFields = map[string]bool{"stateKey": true}

func (c *checker) narrativeSidecar(root *jsonValue) *NarrativeSidecar {
	c.rejectUnknown(root, "", narrativeSidecarFields)
	sidecar := &NarrativeSidecar{SchemaVersion: "1"}

	sidecar.ScenarioID = c.requiredID(root, "", "scenarioId")
	sidecar.Revision = c.positiveInteger(root, "", "revision", true)
	sidecar.Locale = c.authoredLocale(root, "", "locale", true)
	sidecar.FallbackLocale = c.authoredLocale(root, "", "fallbackLocale", true)

	if values := c.arrayField(root, "", "messageCatalogues", true, 1, MaxNarrativeCatalogues); values != nil {
		seen := make(map[string]string)
		for index, value := range values {
			path := pointerIndex("/messageCatalogues", index)
			ref := c.catalogueReference(value, path)
			if ref.Locale != "" {
				if existing, ok := seen[ref.Locale]; ok {
					c.report(CodeDuplicateID, pointerChild(path, "locale"),
						"catalogue locale %q is already declared at %s", ref.Locale, existing)
				} else {
					seen[ref.Locale] = pointerChild(path, "locale")
				}
			}
			sidecar.MessageCatalogues = append(sidecar.MessageCatalogues, ref)
		}
	}

	chapterRefs := c.idArray(root, "", "chapterRefs", true, 1, MaxNarrativeChapterRefs)
	sidecar.ChapterRefs = chapterRefs
	chapterSet := make(map[string]bool, len(chapterRefs))
	for index, chapterID := range chapterRefs {
		if chapterSet[chapterID] {
			c.report(CodeDuplicateID, pointerIndex("/chapterRefs", index),
				"chapter reference %q is repeated", chapterID)
		}
		chapterSet[chapterID] = true
	}

	if values := c.arrayField(root, "", "actors", true, 1, MaxNarrativeActors); values != nil {
		seen := make(map[string]string)
		for index, value := range values {
			path := pointerIndex("/actors", index)
			actor := NarrativeActor{}
			if !c.requireObject(value, path) {
				continue
			}
			c.rejectUnknown(value, path, narrativeActorFields)
			actor.ID = c.requiredID(value, path, "id")
			actor.Name = c.textLimited(value, path, "name", MaxNarrativeNameChars, true)
			if actor.ID != "" {
				if existing, ok := seen[actor.ID]; ok {
					c.report(CodeDuplicateID, pointerChild(path, "id"),
						"actor id %q is already declared at %s", actor.ID, existing)
				} else {
					seen[actor.ID] = pointerChild(path, "id")
				}
			}
			sidecar.Actors = append(sidecar.Actors, actor)
		}
	}

	if values := c.arrayField(root, "", "assets", true, 0, MaxNarrativeAssets); values != nil {
		seen := make(map[string]string)
		for index, value := range values {
			path := pointerIndex("/assets", index)
			asset := NarrativeAsset{}
			if !c.requireObject(value, path) {
				continue
			}
			c.rejectUnknown(value, path, narrativeAssetFields)
			asset.ID = c.requiredID(value, path, "id")
			asset.MediaType = c.textLimited(value, path, "mediaType", MaxNarrativeMediaType, true)
			asset.Path = c.assetPath(value, path, "path", true)
			asset.SHA256 = c.lowerDigest(value, path, "sha256", true)
			asset.Bytes = c.positiveInteger(value, path, "bytes", true)
			if asset.ID != "" {
				if existing, ok := seen[asset.ID]; ok {
					c.report(CodeDuplicateID, pointerChild(path, "id"),
						"asset id %q is already declared at %s", asset.ID, existing)
				} else {
					seen[asset.ID] = pointerChild(path, "id")
				}
			}
			sidecar.Assets = append(sidecar.Assets, asset)
		}
	}

	// Events are parsed before segment cross-references so their declaration
	// index and chapter are available.
	if values := c.arrayField(root, "", "events", true, 1, MaxNarrativeEvents); values != nil {
		seen := make(map[string]string)
		for index, value := range values {
			path := pointerIndex("/events", index)
			event := c.narrativeEvent(value, path)
			if event.ID != "" {
				if existing, ok := seen[event.ID]; ok {
					c.report(CodeDuplicateID, pointerChild(path, "id"),
						"event id %q is already declared at %s", event.ID, existing)
				} else {
					seen[event.ID] = pointerChild(path, "id")
				}
			}
			if event.ChapterID != "" && !chapterSet[event.ChapterID] {
				c.report(CodeInvalidReference, pointerChild(path, "chapterId"),
					"event chapter %q is not a declared chapter reference", event.ChapterID)
			}
			sidecar.Events = append(sidecar.Events, event)
		}
	}

	if values := c.arrayField(root, "", "segments", true, 1, MaxNarrativeSegments); values != nil {
		seen := make(map[string]string)
		for index, value := range values {
			path := pointerIndex("/segments", index)
			segment := c.narrativeSegment(value, path)
			if segment.ID != "" {
				if existing, ok := seen[segment.ID]; ok {
					c.report(CodeDuplicateID, pointerChild(path, "id"),
						"segment id %q is already declared at %s", segment.ID, existing)
				} else {
					seen[segment.ID] = pointerChild(path, "id")
				}
			}
			sidecar.Segments = append(sidecar.Segments, segment)
		}
	}

	c.checkNarrativeLocalReferences(sidecar)
	return sidecar
}

func (c *checker) catalogueReference(value *jsonValue, path string) CatalogueReference {
	if !c.requireObject(value, path) {
		return CatalogueReference{}
	}
	c.rejectUnknown(value, path, catalogueReferenceFields)
	return CatalogueReference{
		Locale: c.authoredLocale(value, path, "locale", true),
		Path:   c.assetPath(value, path, "path", true),
		SHA256: c.lowerDigest(value, path, "sha256", true),
		Bytes:  c.positiveInteger(value, path, "bytes", true),
	}
}

func (c *checker) narrativeSegment(value *jsonValue, path string) NarrativeSegment {
	segment := NarrativeSegment{}
	if !c.requireObject(value, path) {
		return segment
	}
	c.rejectUnknown(value, path, narrativeSegmentFields)
	segment.ID = c.requiredID(value, path, "id")
	segment.ChapterID = c.requiredID(value, path, "chapterId")
	segment.StepID = c.requiredID(value, path, "stepId")
	segment.EventIDs = c.idArray(value, path, "eventIds", true, 0, MaxNarrativeEvents)
	segment.PrerequisiteContextEventIDs = c.idArray(
		value, path, "prerequisiteContextEventIds", true, 0, MaxNarrativeEvents)
	return segment
}

func (c *checker) narrativeEvent(value *jsonValue, path string) NarrativeEvent {
	event := NarrativeEvent{}
	if !c.requireObject(value, path) {
		return event
	}
	c.rejectUnknown(value, path, narrativeEventFields)
	event.ID = c.requiredID(value, path, "id")
	event.ChapterID = c.requiredID(value, path, "chapterId")
	typeValue, present := value.member("type")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "type"), "required field is missing")
		return event
	}
	if typeValue.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, "type"), "must be a string")
		return event
	}
	event.Type = typeValue.text
	if !knownNarrativeEventType(event.Type) {
		c.report(CodeInvalidValue, pointerChild(path, "type"),
			"unknown narrative event type %q", event.Type)
		return event
	}
	payloadValue, present := value.member("payload")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "payload"), "required field is missing")
		return event
	}
	payloadPath := pointerChild(path, "payload")
	if !c.requireObject(payloadValue, payloadPath) {
		return event
	}
	c.narrativePayload(payloadValue, payloadPath, &event)

	if presentationValue, present := value.member("presentation"); present {
		presentation := c.presentation(presentationValue, pointerChild(path, "presentation"))
		event.Presentation = &presentation
	}
	return event
}

func (c *checker) narrativePayload(value *jsonValue, path string, event *NarrativeEvent) {
	switch event.Type {
	case "chat.message":
		c.rejectUnknown(value, path, chatMessagePayloadFields)
		payload := &ChatMessagePayload{
			ActorID:    c.requiredID(value, path, "actorId"),
			MessageKey: c.catalogueKey(value, path, "messageKey"),
		}
		payload.AttachmentIDs = c.idArray(value, path, "attachmentIds", true, 0, MaxNarrativeAssets)
		event.ChatMessage = payload
	case "detective.create", "detective.open":
		c.rejectUnknown(value, path, detectivePayloadFields)
		event.Detective = &DetectivePayload{
			ManifestCaseID: c.requiredID(value, path, "manifestCaseId"),
		}
	case "evidence.reveal":
		c.rejectUnknown(value, path, evidenceRevealPayloadFields)
		event.EvidenceReveal = &EvidenceRevealPayload{
			AssetID: c.requiredID(value, path, "assetId"),
		}
	case "challenge.prompt":
		c.rejectUnknown(value, path, challengePromptPayloadFields)
		event.ChallengePrompt = &ChallengePromptPayload{
			StepID: c.requiredID(value, path, "stepId"),
		}
	case "challenge.await_validation":
		c.rejectUnknown(value, path, challengeAwaitPayloadFields)
		event.ChallengeAwaitValidation = &ChallengeAwaitValidationPayload{
			StepID:       c.requiredID(value, path, "stepId"),
			CheckpointID: c.requiredID(value, path, "checkpointId"),
		}
	case "check.run":
		c.rejectUnknown(value, path, checkRunPayloadFields)
		payload := &CheckRunPayload{
			CheckID:          c.requiredText(value, path, "checkId"),
			ReferenceOutcome: c.outcome(value, path, "referenceOutcome"),
		}
		if assisted, ok := c.requiredBool(value, path, "referenceAssisted"); ok {
			payload.ReferenceAssisted = assisted
		}
		event.CheckRun = payload
	case "postmortem.recommend":
		c.rejectUnknown(value, path, postmortemPayloadFields)
		event.PostmortemRecommend = &PostmortemRecommendPayload{
			ProposalID: c.requiredID(value, path, "proposalId"),
		}
	case "camera.focus":
		c.rejectUnknown(value, path, cameraFocusPayloadFields)
		event.CameraFocus = &CameraFocusPayload{
			TargetID: c.requiredID(value, path, "targetId"),
		}
	case "branch":
		c.rejectUnknown(value, path, branchPayloadFields)
		payload := &BranchPayload{
			StepID:        c.requiredID(value, path, "stepId"),
			CheckpointID:  c.requiredID(value, path, "checkpointId"),
			Outcome:       c.outcome(value, path, "outcome"),
			TargetEventID: c.requiredID(value, path, "targetEventId"),
		}
		if assisted, ok := c.requiredBool(value, path, "assisted"); ok {
			payload.Assisted = assisted
		}
		event.Branch = payload
	case "checkpoint":
		c.rejectUnknown(value, path, checkpointPayloadFields)
		event.Checkpoint = &CheckpointPayload{
			StateKey: c.requiredID(value, path, "stateKey"),
		}
	}
}

func (c *checker) presentation(value *jsonValue, path string) Presentation {
	presentation := Presentation{TypingSpeedPreset: "natural", Transition: "none"}
	if !c.requireObject(value, path) {
		return presentation
	}
	c.rejectUnknown(value, path, presentationFields)
	presentation.DelayBeforeMs = c.boundedInteger(value, path, "delayBeforeMs", 0, MaxPresentationDelayMs)
	presentation.PauseAfterMs = c.boundedInteger(value, path, "pauseAfterMs", 0, MaxPresentationDelayMs)
	if speedValue, present := value.member("typingSpeed"); present {
		speedPath := pointerChild(path, "typingSpeed")
		switch speedValue.kind {
		case jsonString:
			switch speedValue.text {
			case "natural", "fast", "deliberate":
				presentation.TypingSpeedPreset = speedValue.text
			default:
				c.report(CodeInvalidValue, speedPath,
					"typingSpeed must be natural, fast, deliberate or 5-200 characters per second")
			}
		case jsonNumber:
			if !canonicalIntegerPattern.MatchString(speedValue.number) {
				c.report(CodeInvalidValue, speedPath,
					"typingSpeed must be an integer 5-200")
				break
			}
			chars, ok := integerWithin(speedValue.number, MinTypingCharsPerSecond, MaxTypingCharsPerSecond)
			if !ok {
				c.report(CodeInvalidValue, speedPath,
					"typingSpeed must be an integer 5-200")
				break
			}
			presentation.TypingSpeedPreset = ""
			presentation.TypingSpeedCharsPerSecond = chars
		default:
			c.report(CodeInvalidType, speedPath,
				"typingSpeed must be a string or an integer")
		}
	}
	if indicator, ok := c.boolField(value, path, "typingIndicator"); ok {
		presentation.TypingIndicator = indicator
	}
	if transitionValue, present := value.member("transition"); present {
		transitionPath := pointerChild(path, "transition")
		if transitionValue.kind != jsonString {
			c.report(CodeInvalidType, transitionPath, "must be a string")
		} else {
			switch transitionValue.text {
			case "none", "fade":
				presentation.Transition = transitionValue.text
			default:
				c.report(CodeInvalidValue, transitionPath, "transition must be none or fade")
			}
		}
	}
	presentation.CameraTargetID = c.optionalID(value, path, "cameraTargetId")
	return presentation
}

// checkNarrativeLocalReferences validates the references that need no
// declaration or owner context: chapter refs, event existence, segment event
// chapter agreement, prerequisite ordering and branch target order.
func (c *checker) checkNarrativeLocalReferences(sidecar *NarrativeSidecar) {
	eventIndex := make(map[string]int, len(sidecar.Events))
	eventChapter := make(map[string]string, len(sidecar.Events))
	for index, event := range sidecar.Events {
		if event.ID == "" {
			continue
		}
		if _, exists := eventIndex[event.ID]; !exists {
			eventIndex[event.ID] = index
			eventChapter[event.ID] = event.ChapterID
		}
	}
	chapterRefs := make(map[string]bool, len(sidecar.ChapterRefs))
	for _, chapterID := range sidecar.ChapterRefs {
		chapterRefs[chapterID] = true
	}
	for segmentIndex, segment := range sidecar.Segments {
		segmentPath := pointerIndex("/segments", segmentIndex)
		if segment.ChapterID != "" && !chapterRefs[segment.ChapterID] {
			c.report(CodeInvalidReference, pointerChild(segmentPath, "chapterId"),
				"segment chapter %q is not a declared chapter reference", segment.ChapterID)
		}
		seen := make(map[string]bool, len(segment.EventIDs))
		for eventIndexInSegment, eventID := range segment.EventIDs {
			eventPath := pointerIndex(pointerChild(segmentPath, "eventIds"), eventIndexInSegment)
			if seen[eventID] {
				c.report(CodeDuplicateID, eventPath,
					"event reference %q is repeated in this segment", eventID)
			}
			seen[eventID] = true
			index, exists := eventIndex[eventID]
			if !exists {
				c.report(CodeInvalidReference, eventPath,
					"event reference %q does not resolve", eventID)
				continue
			}
			if eventChapter[eventID] != segment.ChapterID {
				c.report(CodeInvalidReference, eventPath,
					"event %q belongs to chapter %q, not the segment chapter %q",
					eventID, eventChapter[eventID], segment.ChapterID)
			}
			if branch := sidecar.Events[index].Branch; branch != nil {
				targetIndex, targetExists := indexOf(segment.EventIDs, branch.TargetEventID)
				if !targetExists || targetIndex <= eventIndexInSegment {
					c.report(CodeInvalidReference,
						pointerChild(pointerChild(pointerIndex("/events", index), "payload"), "targetEventId"),
						"branch target %q must be a later event in the same segment", branch.TargetEventID)
				}
			}
		}
		prerequisites := make(map[string]bool, len(segment.PrerequisiteContextEventIDs))
		firstNormal := -1
		for _, eventID := range segment.EventIDs {
			if index, exists := eventIndex[eventID]; exists {
				if firstNormal == -1 || index < firstNormal {
					firstNormal = index
				}
			}
		}
		for prerequisiteIndex, eventID := range segment.PrerequisiteContextEventIDs {
			prerequisitePath := pointerIndex(
				pointerChild(segmentPath, "prerequisiteContextEventIds"), prerequisiteIndex)
			if prerequisites[eventID] {
				c.report(CodeDuplicateID, prerequisitePath,
					"prerequisite context event %q is repeated in this segment", eventID)
			}
			prerequisites[eventID] = true
			index, exists := eventIndex[eventID]
			if !exists {
				c.report(CodeInvalidReference, prerequisitePath,
					"prerequisite context event %q does not resolve", eventID)
				continue
			}
			if firstNormal >= 0 && index >= firstNormal {
				c.report(CodeInvalidReference, prerequisitePath,
					"prerequisite context event %q must be declared earlier than the segment's events", eventID)
			}
		}
	}
}

// authoredLocale canonicalizes one authored locale and reports every profile
// issue at the field path.
func (c *checker) authoredLocale(object *jsonValue, path, name string, required bool) string {
	text, ok := c.text(object, path, name, required)
	if !ok {
		return ""
	}
	result := AdmitAuthoredLocale(text)
	if !result.Accepted {
		for _, issue := range result.Issues {
			c.report(Code(issue.Code), pointerChild(path, name), "%s", issue.Message)
		}
		return ""
	}
	return result.Tag
}

func (c *checker) catalogueKey(object *jsonValue, path, name string) string {
	text, ok := c.text(object, path, name, true)
	if !ok {
		return ""
	}
	if !catalogueKeyPattern.MatchString(text) {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"messageKey must match ^[a-z][a-z0-9.-]{0,127}$")
		return ""
	}
	return text
}

func (c *checker) requiredText(object *jsonValue, path, name string) string {
	text, ok := c.text(object, path, name, true)
	if !ok {
		return ""
	}
	if text == "" {
		c.report(CodeInvalidValue, pointerChild(path, name), "must not be empty")
	}
	return text
}

func (c *checker) outcome(object *jsonValue, path, name string) string {
	text, ok := c.text(object, path, name, true)
	if !ok {
		return ""
	}
	for _, outcome := range NarrativeOutcomes {
		if text == outcome {
			return text
		}
	}
	c.report(CodeInvalidValue, pointerChild(path, name),
		"outcome must be passed, failed, cancelled, timed-out or error")
	return ""
}

func (c *checker) idArray(object *jsonValue, path, name string, required bool, min, max int) []string {
	values := c.arrayField(object, path, name, required, min, max)
	if values == nil {
		return nil
	}
	ids := make([]string, 0, len(values))
	for index, value := range values {
		id := c.idValue(value, pointerIndex(pointerChild(path, name), index))
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func (c *checker) schemaVersionOne(object *jsonValue, path string) {
	value, present := object.member("schemaVersion")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "schemaVersion"), "required field is missing")
		return
	}
	if value.kind != jsonNumber {
		c.report(CodeInvalidType, pointerChild(path, "schemaVersion"), "must be the number 1")
		return
	}
	if value.number != "1" {
		c.report(CodeInvalidValue, pointerChild(path, "schemaVersion"),
			"schemaVersion must be 1")
	}
}

func (c *checker) boundedInteger(object *jsonValue, path, name string, min, max int) int {
	value, present := object.member(name)
	if !present {
		return min
	}
	if value.kind != jsonNumber {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a number")
		return min
	}
	if !canonicalIntegerPattern.MatchString(value.number) {
		c.report(CodeInvalidValue, pointerChild(path, name), "must be a canonical integer")
		return min
	}
	number, ok := integerWithin(value.number, min, max)
	if !ok {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"must be between %d and %d", min, max)
		return min
	}
	return number
}

func integerWithin(lexeme string, min, max int) (int, bool) {
	value := 0
	for index := 0; index < len(lexeme); index++ {
		value = value*10 + int(lexeme[index]-'0')
		if value > max {
			return 0, false
		}
	}
	if value < min {
		return 0, false
	}
	return value, true
}

func knownNarrativeEventType(eventType string) bool {
	for _, candidate := range NarrativeEventTypes {
		if eventType == candidate {
			return true
		}
	}
	return false
}

func indexOf(values []string, wanted string) (int, bool) {
	for index, value := range values {
		if value == wanted {
			return index, true
		}
	}
	return 0, false
}

func decodeNarrativeAsset(data []byte, maxBytes int, label string) (string, *jsonValue, *Issue) {
	digest := sha256.Sum256(data)
	hexDigest := hex.EncodeToString(digest[:])
	if len(data) > maxBytes {
		return hexDigest, nil, &Issue{
			Code:    CodeTooLarge,
			Path:    "",
			Message: label + " exceeds its byte budget",
		}
	}
	if !utf8.Valid(data) {
		return hexDigest, nil, &Issue{
			Code:    CodeInvalidUTF8,
			Path:    "",
			Message: label + " must be valid UTF-8",
		}
	}
	root, issue := parseJSON(data)
	if issue != nil {
		return hexDigest, nil, issue
	}
	if root.kind != jsonObject {
		return hexDigest, nil, &Issue{
			Code:    CodeNotObject,
			Path:    "",
			Message: label + " root must be a JSON object",
		}
	}
	return hexDigest, root, nil
}

// Projection renders the stable, language-neutral normalized view used by the
// shared conformance corpus.
func (s *NarrativeSidecar) Projection() map[string]any {
	catalogues := make([]any, len(s.MessageCatalogues))
	for index, ref := range s.MessageCatalogues {
		catalogues[index] = map[string]any{
			"locale": ref.Locale,
			"path":   ref.Path,
			"sha256": ref.SHA256,
			"bytes":  string(ref.Bytes),
		}
	}
	actors := make([]any, len(s.Actors))
	for index, actor := range s.Actors {
		actors[index] = map[string]any{"id": actor.ID, "name": actor.Name}
	}
	assets := make([]any, len(s.Assets))
	for index, asset := range s.Assets {
		assets[index] = map[string]any{
			"id":        asset.ID,
			"mediaType": asset.MediaType,
			"path":      asset.Path,
			"sha256":    asset.SHA256,
			"bytes":     string(asset.Bytes),
		}
	}
	segments := make([]any, len(s.Segments))
	for index, segment := range s.Segments {
		segments[index] = map[string]any{
			"id":                          segment.ID,
			"chapterId":                   segment.ChapterID,
			"stepId":                      segment.StepID,
			"eventIds":                    append([]string{}, segment.EventIDs...),
			"prerequisiteContextEventIds": append([]string{}, segment.PrerequisiteContextEventIDs...),
		}
	}
	events := make([]any, len(s.Events))
	for index := range s.Events {
		events[index] = projectNarrativeEvent(s.Events[index])
	}
	return map[string]any{
		"schemaVersion":     string(s.SchemaVersion),
		"scenarioId":        s.ScenarioID,
		"revision":          string(s.Revision),
		"locale":            s.Locale,
		"fallbackLocale":    s.FallbackLocale,
		"messageCatalogues": catalogues,
		"chapterRefs":       append([]string{}, s.ChapterRefs...),
		"actors":            actors,
		"assets":            assets,
		"segments":          segments,
		"events":            events,
	}
}

func projectNarrativeEvent(event NarrativeEvent) map[string]any {
	projected := map[string]any{
		"id":        event.ID,
		"chapterId": event.ChapterID,
		"type":      event.Type,
	}
	switch {
	case event.ChatMessage != nil:
		projected["payload"] = map[string]any{
			"actorId":       event.ChatMessage.ActorID,
			"messageKey":    event.ChatMessage.MessageKey,
			"attachmentIds": append([]string{}, event.ChatMessage.AttachmentIDs...),
		}
	case event.Detective != nil:
		projected["payload"] = map[string]any{
			"manifestCaseId": event.Detective.ManifestCaseID,
		}
	case event.EvidenceReveal != nil:
		projected["payload"] = map[string]any{"assetId": event.EvidenceReveal.AssetID}
	case event.ChallengePrompt != nil:
		projected["payload"] = map[string]any{"stepId": event.ChallengePrompt.StepID}
	case event.ChallengeAwaitValidation != nil:
		projected["payload"] = map[string]any{
			"stepId":       event.ChallengeAwaitValidation.StepID,
			"checkpointId": event.ChallengeAwaitValidation.CheckpointID,
		}
	case event.CheckRun != nil:
		projected["payload"] = map[string]any{
			"checkId":           event.CheckRun.CheckID,
			"referenceOutcome":  event.CheckRun.ReferenceOutcome,
			"referenceAssisted": event.CheckRun.ReferenceAssisted,
		}
	case event.PostmortemRecommend != nil:
		projected["payload"] = map[string]any{
			"proposalId": event.PostmortemRecommend.ProposalID,
		}
	case event.CameraFocus != nil:
		projected["payload"] = map[string]any{"targetId": event.CameraFocus.TargetID}
	case event.Branch != nil:
		projected["payload"] = map[string]any{
			"stepId":        event.Branch.StepID,
			"checkpointId":  event.Branch.CheckpointID,
			"outcome":       event.Branch.Outcome,
			"assisted":      event.Branch.Assisted,
			"targetEventId": event.Branch.TargetEventID,
		}
	case event.Checkpoint != nil:
		projected["payload"] = map[string]any{"stateKey": event.Checkpoint.StateKey}
	}
	if event.Presentation != nil {
		projected["presentation"] = projectPresentation(*event.Presentation)
	} else {
		projected["presentation"] = nil
	}
	return projected
}

func projectPresentation(presentation Presentation) map[string]any {
	var typingSpeed any
	if presentation.TypingSpeedPreset != "" {
		typingSpeed = presentation.TypingSpeedPreset
	} else {
		typingSpeed = presentation.TypingSpeedCharsPerSecond
	}
	return map[string]any{
		"delayBeforeMs":   presentation.DelayBeforeMs,
		"pauseAfterMs":    presentation.PauseAfterMs,
		"typingSpeed":     typingSpeed,
		"typingIndicator": presentation.TypingIndicator,
		"transition":      presentation.Transition,
		"cameraTargetId":  presentation.CameraTargetID,
	}
}

// Projection renders the stable catalogue view with sorted message keys.
func (c *MessageCatalogue) Projection() map[string]any {
	messages := make(map[string]string, len(c.Values))
	for key, value := range c.Values {
		messages[key] = value
	}
	return map[string]any{
		"schemaVersion": string(c.SchemaVersion),
		"locale":        c.Locale,
		"messages":      messages,
	}
}
