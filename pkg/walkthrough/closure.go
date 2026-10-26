package walkthrough

import (
	"sort"
	"unicode/utf8"
)

// OwnerBindingKey ties one trusted lookup result to the exact admitted
// project, source pin, declaration digest and package. Returning only a
// package-local ID is never enough: the same spelling in another source or
// package is a different binding.
type OwnerBindingKey struct {
	Project           ProjectIdentity
	ContentPin        ContentPin
	DeclarationSHA256 string
	Package           PackageReference
}

// PackageAssetBinding is one owner-admitted package asset with its reviewed
// codec kind and actual supplied byte count. Kind, never an author mediaType,
// decides text/binary accounting.
type PackageAssetBinding struct {
	AssetID   string
	Kind      string
	Reference AssetReference
	Bytes     Integer
}

// NarrativeInventory is the coherent owner snapshot of one admitted package:
// reviewed codec assets, package-local inventories and their binding key. Its
// asset set must exactly cover the accepted a2 package inventory.
type NarrativeInventory struct {
	Binding             OwnerBindingKey
	Assets              []PackageAssetBinding
	Cases               []string
	Incidents           []string
	Checks              []string
	Queries             []string
	Proposals           []string
	PresentationTargets []string
}

// CheckBinding is an owner-valid Check identity and its check-definition
// asset. The owner has already run native Check/catalog validation; this
// package only closes the package-local reference.
type CheckBinding struct {
	Binding           OwnerBindingKey
	CheckID           string
	DefinitionAssetID string
}

// ProposalBinding is one unsaved package proposal with its typed definition
// and evidence references.
type ProposalBinding struct {
	Binding      OwnerBindingKey
	ProposalID   string
	Kind         string
	DefinitionID string
	EvidenceIDs  []string
	Rationale    string
}

// PresentationClosure is the trusted owner's typed closure for a presentation
// target's referenced object. Absence or a kind/identity mismatch refuses the
// target; a bare target ID is not closure.
type PresentationClosure struct {
	Kind     string
	ID       string
	Check    *CheckBinding
	Proposal *ProposalBinding
}

// PresentationTargetBinding is a manifest-to-view descriptor backed by an
// already-validated typed owner closure. It is never selector, component,
// router or focus authority.
type PresentationTargetBinding struct {
	Binding  OwnerBindingKey
	TargetID string
	Kind     string
	RefID    string
	Closure  *PresentationClosure
}

// NarrativeOwnerContext is the narrative-specific trusted lookup extension.
// It composes the accepted a2 port without widening ordinary declaration or
// source callers, and adds only the Check, proposal, presentation-target and
// accounting snapshots this narrative closure needs.
type NarrativeOwnerContext interface {
	TrainingOwnerContext
	// AdmitNarrativeInventory returns one coherent package snapshot for the
	// current admission. The caller must keep it stable for the whole call;
	// a changed reread refuses rather than mixing inventories.
	AdmitNarrativeInventory(ref PackageReference) (NarrativeInventory, error)
	// AdmitCheck resolves an owner-valid package Check within the exact
	// admitted package binding.
	AdmitCheck(ref PackageReference, checkID string) (CheckBinding, error)
	// AdmitProposal resolves one package proposal within the exact admitted
	// package binding.
	AdmitProposal(ref PackageReference, proposalID string) (ProposalBinding, error)
	// AdmitPresentationTarget resolves one registered presentation target and
	// its already-validated typed closure.
	AdmitPresentationTarget(ref PackageReference, targetID string) (PresentationTargetBinding, error)
}

// NarrativeAssetBytes is one raw supplied asset and the reference it claims to
// satisfy. Catalogue references must match the sidecar's pinned reference; the
// sidecar's own bytes are checked against the admitted declaration reference.
type NarrativeAssetBytes struct {
	Reference AssetReference
	Bytes     []byte
}

// NarrativeAdmissionInput is the caller-supplied raw bytes for one admission.
// Request is nil when the visitor made no explicit locale choice.
type NarrativeAdmissionInput struct {
	Sidecar    NarrativeAssetBytes
	Catalogues []NarrativeAssetBytes
	Request    *string
}

// NarrativeCatalogues is one resolved catalogue snapshot. Values is copied.
type NarrativeCatalogues struct {
	Locale string
	Values map[string]string
}

// ResolvedMessage is one chat event's resolved plain text and the catalogue
// that supplied it.
type ResolvedMessage struct {
	Key          string
	Text         string
	Locale       string
	FromFallback bool
}

// ResolvedNarrativeEvent is one admitted event with its resolved text and any
// owner-typed binding the closure established.
type ResolvedNarrativeEvent struct {
	ID                 string
	ChapterID          string
	Type               string
	Message            *ResolvedMessage
	Check              *CheckBinding
	Proposal           *ProposalBinding
	PresentationTarget *PresentationTargetBinding
}

// AdmittedNarrative is the immutable result of narrative admission. No timing,
// settled state or executable projection is constructed here.
type AdmittedNarrative struct {
	Sidecar    *NarrativeSidecar
	Package    PackageReference
	Locale     NarrativeLocaleChoice
	TextBytes  Integer
	Catalogues []NarrativeCatalogues
	Events     []ResolvedNarrativeEvent
}

// NarrativeResolution is the outcome of narrative admission. Narrative is
// non-nil only when Admitted is true.
type NarrativeResolution struct {
	Admitted  bool
	Narrative *AdmittedNarrative
	Issues    []Issue
}

// Err returns nil for an admitted narrative or the aggregate refusal.
func (r NarrativeResolution) Err() error {
	if r.Admitted {
		return nil
	}
	return &Error{Issues: r.Issues}
}

// AdmitNarrative binds a structurally valid sidecar to its admitted /2
// declaration, catalogues, package inventory and typed owner lookups. It
// verifies every supplied byte pin before parsing, performs complete local
// reference closure and text accounting, and returns no partial narrative on
// any refusal.
func AdmitNarrative(
	bundle *AdmittedTraining,
	authority ProjectAuthority,
	owners NarrativeOwnerContext,
	input NarrativeAdmissionInput,
) NarrativeResolution {
	if bundle == nil || bundle.Declaration == nil {
		return narrativeRefusal(CodeMissingContext, "", "an admitted training bundle is required")
	}
	if authority == nil {
		return narrativeRefusal(CodeMissingContext, "", "a trusted project authority is required")
	}
	if owners == nil {
		return narrativeRefusal(CodeMissingContext, "", "a trusted narrative owner context is required")
	}
	if err := bundle.Narrative.VerifyBytes(input.Sidecar.Bytes); err != nil {
		return NarrativeResolution{Issues: issuesOfError(err)}
	}
	parsed := ParseNarrativeSidecar(input.Sidecar.Bytes)
	if !parsed.Accepted {
		return NarrativeResolution{Issues: parsed.Issues}
	}
	sidecar := parsed.Sidecar
	issues := make([]Issue, 0)
	if sidecar.ScenarioID != bundle.Package.ID {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    "/scenarioId",
			Message: "scenarioId must equal the admitted package id",
		})
	}
	if sidecar.Revision != bundle.Package.Revision {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    "/revision",
			Message: "revision must equal the admitted package revision",
		})
	}
	declaredChapters := make([]string, len(bundle.Declaration.Chapters))
	for index := range bundle.Declaration.Chapters {
		declaredChapters[index] = bundle.Declaration.Chapters[index].ID
	}
	if !stringSlicesEqual(sidecar.ChapterRefs, declaredChapters) {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    "/chapterRefs",
			Message: "chapterRefs must equal the declared chapter ids in order",
		})
	}

	a2Inventory, err := owners.AdmitPackage(bundle.Package)
	if err != nil {
		issues = append(issues, Issue{
			Code:    CodeNotAdmitted,
			Path:    "/trainingPackage",
			Message: err.Error(),
		})
		return NarrativeResolution{Issues: issues}
	}
	if !a2Inventory.Ref.Equal(bundle.Package) {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    "/trainingPackage",
			Message: "the admitted package does not equal the declaration package",
		})
		return NarrativeResolution{Issues: issues}
	}
	rawInventory, err := owners.AdmitNarrativeInventory(bundle.Package)
	if err != nil {
		issues = append(issues, Issue{
			Code:    CodeNotAdmitted,
			Path:    "/trainingPackage",
			Message: err.Error(),
		})
		return NarrativeResolution{Issues: issues}
	}
	inventory := snapshotNarrativeInventory(rawInventory)
	issues = append(issues, validateNarrativeInventory(
		inventory, a2Inventory, bundle, authority)...)
	if len(issues) > 0 {
		return NarrativeResolution{Issues: issues}
	}

	for index := range sidecar.Assets {
		if err := owners.AdmitAsset(sidecar.Assets[index].Asset()); err != nil {
			issues = append(issues, Issue{
				Code:    CodeNotAdmitted,
				Path:    pointerIndex("/assets", index),
				Message: err.Error(),
			})
		}
	}

	catalogues, catalogueIssues := admitCatalogues(sidecar, input.Catalogues, owners, bundle)
	issues = append(issues, catalogueIssues...)
	if len(issues) > 0 {
		return NarrativeResolution{Issues: issues}
	}
	if _, ok := catalogues[sidecar.Locale]; !ok {
		issues = append(issues, Issue{
			Code:    CodeInvalidReference,
			Path:    "/locale",
			Message: "the sidecar default locale has no pinned catalogue",
		})
	}
	if _, ok := catalogues[sidecar.FallbackLocale]; !ok {
		issues = append(issues, Issue{
			Code:    CodeInvalidReference,
			Path:    "/fallbackLocale",
			Message: "the fallback locale has no pinned catalogue",
		})
	}

	selection := SelectNarrativeLocale(
		input.Request, catalogueLocales(sidecar), sidecar.Locale, sidecar.FallbackLocale)
	if !selection.Accepted {
		issues = append(issues, selection.Issues...)
	}

	admission := &narrativeAdmission{
		bundle:     bundle,
		authority:  authority,
		owners:     owners,
		sidecar:    sidecar,
		inventory:  inventory,
		catalogues: catalogues,
		checks:     make(map[string]*CheckBinding),
		proposals:  make(map[string]*ProposalBinding),
		targets:    make(map[string]*PresentationTargetBinding),
	}
	issues = append(issues, admission.checkSegmentBindings()...)
	events := admission.resolveEvents(selection.Choice)
	issues = append(issues, admission.issues...)
	if len(issues) > 0 {
		return NarrativeResolution{Issues: issues}
	}

	texts := make([]narrativeTextItem, 0)
	texts = append(texts, narrativeTextItem{
		path:        bundle.Narrative.Path,
		sha256:      bundle.Narrative.SHA256,
		pinnedBytes: bundle.Narrative.Bytes,
		actualBytes: len(input.Sidecar.Bytes),
	})
	for _, catalogue := range input.Catalogues {
		texts = append(texts, narrativeTextItem{
			path:          catalogue.Reference.Path,
			sha256:        catalogue.Reference.SHA256,
			pinnedBytes:   catalogue.Reference.Bytes,
			actualBytes:   len(catalogue.Bytes),
			perAssetLimit: true,
		})
	}
	for _, asset := range inventory.Assets {
		if !isPackageTextKind(asset.Kind) {
			continue
		}
		actual, ok := integerAsInt(string(asset.Bytes), int64(MaxNarrativeTextBytes)+1)
		if !ok {
			issues = append(issues, Issue{
				Code:    CodeInvalidValue,
				Path:    "",
				Message: "package text asset " + asset.AssetID + " has an unrepresentable byte count",
			})
			continue
		}
		texts = append(texts, narrativeTextItem{
			path:          asset.Reference.Path,
			sha256:        asset.Reference.SHA256,
			pinnedBytes:   asset.Reference.Bytes,
			actualBytes:   int(actual),
			perAssetLimit: true,
		})
	}
	textBytes, textIssues := accountNarrativeText(texts)
	issues = append(issues, textIssues...)

	// A caller must not mix inventories: reread the coherent snapshot and
	// refuse if it changed while this admission was running.
	again, err := owners.AdmitNarrativeInventory(bundle.Package)
	if err != nil {
		issues = append(issues, Issue{
			Code:    CodeNotAdmitted,
			Path:    "/trainingPackage",
			Message: err.Error(),
		})
	} else if !equalNarrativeInventory(inventory, snapshotNarrativeInventory(again), authority) {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    "",
			Message: "the narrative owner inventory changed during admission",
		})
	}
	if len(issues) > 0 {
		return NarrativeResolution{Issues: issues}
	}

	return NarrativeResolution{
		Admitted: true,
		Narrative: &AdmittedNarrative{
			Sidecar:    sidecar,
			Package:    bundle.Package,
			Locale:     selection.Choice,
			TextBytes:  textBytes,
			Catalogues: catalogueSnapshots(catalogues),
			Events:     events,
		},
	}
}

func narrativeRefusal(code Code, path, message string) NarrativeResolution {
	return NarrativeResolution{Issues: []Issue{{Code: code, Path: path, Message: message}}}
}

func issuesOfError(err error) []Issue {
	if refusal, ok := err.(*Error); ok {
		return append([]Issue{}, refusal.Issues...)
	}
	return []Issue{{Code: CodeMismatch, Path: "", Message: err.Error()}}
}

func validateNarrativeInventory(
	inventory NarrativeInventory,
	a2 PackageInventory,
	bundle *AdmittedTraining,
	authority ProjectAuthority,
) []Issue {
	issues := make([]Issue, 0)
	if !equalOwnerBindingKey(inventory.Binding, expectedBinding(bundle), authority) {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    "",
			Message: "the narrative inventory is not bound to the current project, source and package",
		})
	}
	if !sortedEqual(inventory.Cases, a2.Cases) {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    "",
			Message: "the narrative inventory Case set disagrees with the admitted package",
		})
	}
	assetIDs := make(map[string]bool, len(inventory.Assets))
	referenceCounts := make(map[string]int, len(a2.Assets))
	for _, ref := range a2.Assets {
		referenceCounts[assetReferenceKey(ref)]++
	}
	for _, asset := range inventory.Assets {
		if asset.AssetID == "" || !idPattern.MatchString(asset.AssetID) {
			issues = append(issues, Issue{
				Code:    CodeInvalidValue,
				Path:    "",
				Message: "package asset id must match the walkthrough ID grammar",
			})
			continue
		}
		if assetIDs[asset.AssetID] {
			issues = append(issues, Issue{
				Code:    CodeDuplicateID,
				Path:    "",
				Message: "package asset id " + asset.AssetID + " is repeated",
			})
			continue
		}
		assetIDs[asset.AssetID] = true
		if !knownPackageKind(asset.Kind) {
			issues = append(issues, Issue{
				Code:    CodeInvalidValue,
				Path:    "",
				Message: "package asset " + asset.AssetID + " has an unknown codec kind",
			})
		}
		if !validAssetPath(asset.Reference.Path) || !isLowerHex(asset.Reference.SHA256, 64) ||
			!positiveIntegerLexeme(string(asset.Reference.Bytes)) {
			issues = append(issues, Issue{
				Code:    CodeInvalidValue,
				Path:    "",
				Message: "package asset " + asset.AssetID + " has an invalid reference pin",
			})
		}
		if asset.Bytes != asset.Reference.Bytes {
			issues = append(issues, Issue{
				Code:    CodeByteCountMismatch,
				Path:    "",
				Message: "package asset " + asset.AssetID + " actual bytes disagree with its pin",
			})
		}
		referenceCounts[assetReferenceKey(asset.Reference)]--
	}
	keys := make([]string, 0, len(referenceCounts))
	for key := range referenceCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if referenceCounts[key] != 0 {
			issues = append(issues, Issue{
				Code:    CodeMismatch,
				Path:    "",
				Message: "package asset inventory does not cover reference " + key,
			})
		}
	}
	inventories := []struct {
		name   string
		values []string
	}{
		{"cases", inventory.Cases},
		{"incidents", inventory.Incidents},
		{"checks", inventory.Checks},
		{"queries", inventory.Queries},
		{"proposals", inventory.Proposals},
		{"presentationTargets", inventory.PresentationTargets},
	}
	for _, bounded := range inventories {
		if !uniqueStrings(bounded.values) {
			issues = append(issues, Issue{
				Code:    CodeDuplicateID,
				Path:    "",
				Message: "the package " + bounded.name + " inventory repeats an id",
			})
		}
	}
	return issues
}

func expectedBinding(bundle *AdmittedTraining) OwnerBindingKey {
	return OwnerBindingKey{
		Project:           bundle.Project,
		ContentPin:        bundle.ContentPin,
		DeclarationSHA256: bundle.DeclarationSHA256,
		Package:           bundle.Package,
	}
}

func equalOwnerBindingKey(left, right OwnerBindingKey, authority ProjectAuthority) bool {
	if !left.ContentPin.Equal(right.ContentPin) ||
		left.DeclarationSHA256 != right.DeclarationSHA256 ||
		!left.Package.Equal(right.Package) {
		return false
	}
	if left.Project.Value == nil || right.Project.Value == nil {
		return false
	}
	return authority.EqualProject(left.Project, right.Project)
}

func positiveIntegerLexeme(lexeme string) bool {
	return canonicalIntegerPattern.MatchString(lexeme) && lexeme != "0"
}

func knownPackageKind(kind string) bool {
	_, ok := packageCodecKinds[kind]
	return ok
}

func isPackageTextKind(kind string) bool {
	class, ok := packageCodecKinds[kind]
	return ok && class == "text"
}

func isEvidenceKind(kind string) bool {
	return kind == "evidence-text" || kind == "evidence-records" || kind == "evidence-log"
}

var packageCodecKinds = map[string]string{
	"evidence-text":        "text",
	"evidence-records":     "text",
	"evidence-log":         "text",
	"query-metadata":       "text",
	"query-body-sql":       "text",
	"check-definition":     "text",
	"challenge-manifest":   "text",
	"challenge-fixture":    "binary",
	"watch-recommendation": "text",
}

type narrativeAdmission struct {
	bundle     *AdmittedTraining
	authority  ProjectAuthority
	owners     NarrativeOwnerContext
	sidecar    *NarrativeSidecar
	inventory  NarrativeInventory
	catalogues map[string]*MessageCatalogue
	checks     map[string]*CheckBinding
	proposals  map[string]*ProposalBinding
	targets    map[string]*PresentationTargetBinding
	issues     []Issue
}

func (a *narrativeAdmission) checkSegmentBindings() []Issue {
	issues := make([]Issue, 0)
	expected := make(map[string]struct {
		chapterID string
		stepID    string
	})
	for chapterIndex := range a.bundle.Declaration.Chapters {
		chapter := a.bundle.Declaration.Chapters[chapterIndex]
		for stepIndex := range chapter.Steps {
			step := chapter.Steps[stepIndex]
			if step.Kind != "scripted-dialogue" {
				continue
			}
			expected[step.SegmentID] = struct {
				chapterID string
				stepID    string
			}{chapterID: chapter.ID, stepID: step.ID}
		}
	}
	seen := make(map[string]bool, len(a.sidecar.Segments))
	for index := range a.sidecar.Segments {
		segment := a.sidecar.Segments[index]
		path := pointerIndex("/segments", index)
		want, ok := expected[segment.ID]
		if !ok {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    pointerChild(path, "id"),
				Message: "segment does not bind a declared scripted-dialogue step",
			})
			continue
		}
		seen[segment.ID] = true
		if segment.ChapterID != want.chapterID || segment.StepID != want.stepID {
			issues = append(issues, Issue{
				Code:    CodeMismatch,
				Path:    path,
				Message: "segment chapter/step does not equal its declared dialogue step",
			})
		}
	}
	for segmentID := range expected {
		if !seen[segmentID] {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    "/segments",
				Message: "declared dialogue segment " + segmentID + " has no sidecar segment",
			})
		}
	}
	sort.Slice(issues, func(left, right int) bool {
		if issues[left].Path != issues[right].Path {
			return issues[left].Path < issues[right].Path
		}
		return issues[left].Message < issues[right].Message
	})
	return issues
}

func (a *narrativeAdmission) resolveEvents(choice NarrativeLocaleChoice) []ResolvedNarrativeEvent {
	resolved := make([]ResolvedNarrativeEvent, 0, len(a.sidecar.Events))
	actors := make(map[string]bool, len(a.sidecar.Actors))
	for _, actor := range a.sidecar.Actors {
		actors[actor.ID] = true
	}
	assets := make(map[string]bool, len(a.sidecar.Assets))
	for _, asset := range a.sidecar.Assets {
		assets[asset.ID] = true
	}
	selected := a.catalogues[choice.SelectedLocale]
	fallback := a.catalogues[choice.FallbackLocale]
	for index := range a.sidecar.Events {
		event := a.sidecar.Events[index]
		path := pointerIndex("/events", index)
		payloadPath := pointerChild(path, "payload")
		entry := ResolvedNarrativeEvent{
			ID:        event.ID,
			ChapterID: event.ChapterID,
			Type:      event.Type,
		}
		switch {
		case event.ChatMessage != nil:
			payload := event.ChatMessage
			if !actors[payload.ActorID] {
				a.issue(CodeInvalidReference, pointerChild(payloadPath, "actorId"),
					"actor does not resolve")
			}
			for attachmentIndex, attachmentID := range payload.AttachmentIDs {
				if !assets[attachmentID] {
					a.issue(CodeInvalidReference,
						pointerIndex(pointerChild(payloadPath, "attachmentIds"), attachmentIndex),
						"attachment asset does not resolve")
				}
			}
			if fallback != nil {
				if _, ok := fallback.Values[payload.MessageKey]; !ok {
					a.issue(CodeInvalidReference, pointerChild(payloadPath, "messageKey"),
						"messageKey has no fallback catalogue message")
				}
			}
			if selected != nil && fallback != nil {
				if text, ok := selected.Values[payload.MessageKey]; ok {
					entry.Message = &ResolvedMessage{
						Key:    payload.MessageKey,
						Text:   text,
						Locale: choice.SelectedLocale,
					}
				} else if text, ok := fallback.Values[payload.MessageKey]; ok {
					entry.Message = &ResolvedMessage{
						Key:          payload.MessageKey,
						Text:         text,
						Locale:       choice.FallbackLocale,
						FromFallback: true,
					}
				}
			}
		case event.Detective != nil:
			if !containsString(a.inventory.Cases, event.Detective.ManifestCaseID) {
				a.issue(CodeNotAdmitted, pointerChild(payloadPath, "manifestCaseId"),
					"package Case does not resolve")
			}
		case event.EvidenceReveal != nil:
			if !assets[event.EvidenceReveal.AssetID] {
				a.issue(CodeInvalidReference, pointerChild(payloadPath, "assetId"),
					"declared sidecar asset does not resolve")
			}
		case event.ChallengePrompt != nil:
			a.requireChallengeStep(event.ChallengePrompt.StepID, "", pointerChild(payloadPath, "stepId"))
		case event.ChallengeAwaitValidation != nil:
			a.requireChallengeStep(event.ChallengeAwaitValidation.StepID,
				event.ChallengeAwaitValidation.CheckpointID, payloadPath)
		case event.CheckRun != nil:
			binding := a.checkBinding(event.CheckRun.CheckID, pointerChild(payloadPath, "checkId"))
			if binding != nil {
				copied := snapshotCheckBinding(*binding)
				entry.Check = &copied
			}
		case event.PostmortemRecommend != nil:
			binding := a.proposalBinding(event.PostmortemRecommend.ProposalID, pointerChild(payloadPath, "proposalId"))
			if binding != nil {
				copied := snapshotProposalBinding(*binding)
				entry.Proposal = &copied
			}
		case event.CameraFocus != nil:
			binding := a.targetBinding(event.CameraFocus.TargetID, pointerChild(payloadPath, "targetId"))
			if binding != nil {
				copied := snapshotPresentationTargetBinding(*binding)
				entry.PresentationTarget = &copied
			}
		case event.Branch != nil:
			a.requireChallengeStep(event.Branch.StepID, event.Branch.CheckpointID, payloadPath)
		case event.Checkpoint != nil:
			// A pure reconstruction marker has no owner or completion identity.
		}
		if event.Presentation != nil && event.Presentation.CameraTargetID != "" {
			a.targetBinding(event.Presentation.CameraTargetID,
				pointerChild(pointerChild(path, "presentation"), "cameraTargetId"))
		}
		resolved = append(resolved, entry)
	}
	return resolved
}

func (a *narrativeAdmission) issue(code Code, path, message string) {
	a.issues = append(a.issues, Issue{Code: code, Path: path, Message: message})
}

func (a *narrativeAdmission) requireChallengeStep(stepID, checkpointID, path string) {
	for _, step := range a.bundle.ChallengeSteps {
		if step.StepID != stepID {
			continue
		}
		if checkpointID != "" && step.CheckpointID != checkpointID {
			a.issue(CodeMismatch, path,
				"checkpointId must equal the step's single whole-challenge mapping")
		}
		return
	}
	a.issue(CodeInvalidReference, path, "challenge step does not resolve")
}

func (a *narrativeAdmission) checkBinding(checkID, path string) *CheckBinding {
	binding, issues := a.resolveCheck(checkID, path)
	a.issues = append(a.issues, issues...)
	return binding
}

func (a *narrativeAdmission) resolveCheck(checkID, path string) (*CheckBinding, []Issue) {
	if binding, ok := a.checks[checkID]; ok {
		return binding, nil
	}
	raw, err := a.owners.AdmitCheck(a.bundle.Package, checkID)
	if err != nil {
		return nil, []Issue{{Code: CodeNotAdmitted, Path: path, Message: err.Error()}}
	}
	binding := snapshotCheckBinding(raw)
	issues := a.validateCheckBinding(binding, checkID, path)
	if len(issues) > 0 {
		return nil, issues
	}
	a.checks[checkID] = &binding
	return &binding, nil
}

func (a *narrativeAdmission) validateCheckBinding(binding CheckBinding, checkID, path string) []Issue {
	issues := make([]Issue, 0)
	if !equalOwnerBindingKey(binding.Binding, expectedBinding(a.bundle), a.projectAuthority()) {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    path,
			Message: "the Check binding is not tied to the current project, source and package",
		})
	}
	if binding.CheckID != checkID {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    path,
			Message: "the admitted Check id does not equal the requested id",
		})
	}
	asset, ok := a.packageAsset(binding.DefinitionAssetID)
	if !ok || asset.Kind != "check-definition" {
		issues = append(issues, Issue{
			Code:    CodeNotAdmitted,
			Path:    path,
			Message: "the Check definition asset does not resolve as a check-definition",
		})
	}
	return issues
}

func (a *narrativeAdmission) proposalBinding(proposalID, path string) *ProposalBinding {
	binding, issues := a.resolveProposal(proposalID, path)
	a.issues = append(a.issues, issues...)
	return binding
}

func (a *narrativeAdmission) resolveProposal(proposalID, path string) (*ProposalBinding, []Issue) {
	if binding, ok := a.proposals[proposalID]; ok {
		return binding, nil
	}
	raw, err := a.owners.AdmitProposal(a.bundle.Package, proposalID)
	if err != nil {
		return nil, []Issue{{Code: CodeNotAdmitted, Path: path, Message: err.Error()}}
	}
	binding := snapshotProposalBinding(raw)
	issues := a.validateProposalBinding(binding, proposalID, path)
	if len(issues) > 0 {
		return nil, issues
	}
	a.proposals[proposalID] = &binding
	return &binding, nil
}

func (a *narrativeAdmission) validateProposalBinding(binding ProposalBinding, proposalID, path string) []Issue {
	issues := make([]Issue, 0)
	if !equalOwnerBindingKey(binding.Binding, expectedBinding(a.bundle), a.projectAuthority()) {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    path,
			Message: "the proposal binding is not tied to the current project, source and package",
		})
	}
	if binding.ProposalID != proposalID {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    path,
			Message: "the admitted proposal id does not equal the requested id",
		})
	}
	if !containsString(a.inventory.Proposals, binding.ProposalID) {
		issues = append(issues, Issue{
			Code:    CodeNotAdmitted,
			Path:    path,
			Message: "the proposal is not in the admitted package inventory",
		})
	}
	switch binding.Kind {
	case "query":
		if !containsString(a.inventory.Queries, binding.DefinitionID) {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    path,
				Message: "the proposal query definition does not resolve",
			})
		}
	case "check":
		if !containsString(a.inventory.Checks, binding.DefinitionID) {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    path,
				Message: "the proposal Check definition does not resolve",
			})
		} else if _, checkIssues := a.resolveCheck(binding.DefinitionID, path); len(checkIssues) > 0 {
			issues = append(issues, checkIssues...)
		}
	case "watch":
		asset, ok := a.packageAsset(binding.DefinitionID)
		if !ok || asset.Kind != "watch-recommendation" {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    path,
				Message: "the watch recommendation asset does not resolve",
			})
		}
	default:
		issues = append(issues, Issue{
			Code:    CodeInvalidValue,
			Path:    path,
			Message: "the proposal kind must be query, check or watch",
		})
	}
	if !uniqueStrings(binding.EvidenceIDs) {
		issues = append(issues, Issue{
			Code:    CodeDuplicateID,
			Path:    path,
			Message: "the proposal evidence ids repeat",
		})
	}
	for _, evidenceID := range binding.EvidenceIDs {
		asset, ok := a.packageAsset(evidenceID)
		if !ok || !isEvidenceKind(asset.Kind) {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    path,
				Message: "the proposal evidence asset does not resolve",
			})
		}
	}
	if utf8.RuneCountInString(binding.Rationale) > MaxTextChars {
		issues = append(issues, Issue{
			Code:    CodeTooLong,
			Path:    path,
			Message: "the proposal rationale is too long",
		})
	}
	return issues
}

func (a *narrativeAdmission) targetBinding(targetID, path string) *PresentationTargetBinding {
	binding, issues := a.resolveTarget(targetID, path)
	a.issues = append(a.issues, issues...)
	return binding
}

func (a *narrativeAdmission) resolveTarget(targetID, path string) (*PresentationTargetBinding, []Issue) {
	if binding, ok := a.targets[targetID]; ok {
		return binding, nil
	}
	raw, err := a.owners.AdmitPresentationTarget(a.bundle.Package, targetID)
	if err != nil {
		return nil, []Issue{{Code: CodeNotAdmitted, Path: path, Message: err.Error()}}
	}
	binding := snapshotPresentationTargetBinding(raw)
	issues := a.validateTargetBinding(binding, targetID, path)
	if len(issues) > 0 {
		return nil, issues
	}
	a.targets[targetID] = &binding
	return &binding, nil
}

func (a *narrativeAdmission) validateTargetBinding(binding PresentationTargetBinding, targetID, path string) []Issue {
	issues := make([]Issue, 0)
	if !equalOwnerBindingKey(binding.Binding, expectedBinding(a.bundle), a.projectAuthority()) {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    path,
			Message: "the presentation target is not tied to the current project, source and package",
		})
	}
	if binding.TargetID != targetID {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    path,
			Message: "the admitted presentation target id does not equal the requested id",
		})
	}
	if !containsString(a.inventory.PresentationTargets, binding.TargetID) {
		issues = append(issues, Issue{
			Code:    CodeNotAdmitted,
			Path:    path,
			Message: "the presentation target is not in the admitted package inventory",
		})
	}
	if binding.Closure == nil || binding.Closure.Kind != binding.Kind ||
		binding.Closure.ID != binding.RefID {
		issues = append(issues, Issue{
			Code:    CodeMismatch,
			Path:    path,
			Message: "the presentation target has no matching typed closure",
		})
		return issues
	}
	switch binding.Kind {
	case "case":
		if !containsString(a.inventory.Cases, binding.RefID) {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    path,
				Message: "the presentation Case does not resolve",
			})
		}
	case "incident":
		if !containsString(a.inventory.Incidents, binding.RefID) {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    path,
				Message: "the presentation incident does not resolve",
			})
		}
	case "evidence":
		asset, ok := a.packageAsset(binding.RefID)
		if !ok || !isEvidenceKind(asset.Kind) {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    path,
				Message: "the presentation evidence does not resolve",
			})
		}
	case "check":
		if !containsString(a.inventory.Checks, binding.RefID) {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    path,
				Message: "the presentation Check does not resolve",
			})
		} else if _, checkIssues := a.resolveCheck(binding.RefID, path); len(checkIssues) > 0 {
			issues = append(issues, checkIssues...)
		}
	case "proposal":
		if !containsString(a.inventory.Proposals, binding.RefID) {
			issues = append(issues, Issue{
				Code:    CodeInvalidReference,
				Path:    path,
				Message: "the presentation proposal does not resolve",
			})
		} else if _, proposalIssues := a.resolveProposal(binding.RefID, path); len(proposalIssues) > 0 {
			issues = append(issues, proposalIssues...)
		}
	default:
		issues = append(issues, Issue{
			Code:    CodeInvalidValue,
			Path:    path,
			Message: "the presentation target kind must be case, incident, evidence, check or proposal",
		})
	}
	return issues
}

func (a *narrativeAdmission) projectAuthority() ProjectAuthority {
	return a.authority
}

func (a *narrativeAdmission) packageAsset(assetID string) (PackageAssetBinding, bool) {
	for _, asset := range a.inventory.Assets {
		if asset.AssetID == assetID {
			return asset, true
		}
	}
	return PackageAssetBinding{}, false
}

func admitCatalogues(
	sidecar *NarrativeSidecar,
	supplied []NarrativeAssetBytes,
	owners NarrativeOwnerContext,
	bundle *AdmittedTraining,
) (map[string]*MessageCatalogue, []Issue) {
	issues := make([]Issue, 0)
	catalogues := make(map[string]*MessageCatalogue, len(sidecar.MessageCatalogues))
	matched := make([]bool, len(sidecar.MessageCatalogues))
	for supplyIndex, item := range supplied {
		refIndex := -1
		for index := range sidecar.MessageCatalogues {
			if matched[index] {
				continue
			}
			if sidecar.MessageCatalogues[index].Asset().Equal(item.Reference) {
				refIndex = index
				break
			}
		}
		if refIndex < 0 {
			issues = append(issues, Issue{
				Code:    CodeMismatch,
				Path:    pointerIndex("/catalogues", supplyIndex),
				Message: "supplied catalogue bytes do not match a pinned catalogue reference",
			})
			continue
		}
		matched[refIndex] = true
		ref := sidecar.MessageCatalogues[refIndex]
		if err := ref.Asset().VerifyBytes(item.Bytes); err != nil {
			iss := issuesOfError(err)
			for index := range iss {
				iss[index].Path = pointerIndex("/catalogues", supplyIndex)
			}
			issues = append(issues, iss...)
			continue
		}
		if err := owners.AdmitAsset(ref.Asset()); err != nil {
			issues = append(issues, Issue{
				Code:    CodeNotAdmitted,
				Path:    pointerIndex("/catalogues", supplyIndex),
				Message: err.Error(),
			})
			continue
		}
		parsed := ParseMessageCatalogue(item.Bytes)
		if !parsed.Accepted {
			issues = append(issues, parsed.Issues...)
			continue
		}
		catalogue := parsed.Catalogue
		if catalogue.Locale != ref.Locale {
			issues = append(issues, Issue{
				Code:    CodeMismatch,
				Path:    pointerIndex("/catalogues", supplyIndex),
				Message: "the catalogue document locale does not equal its pinned reference",
			})
			continue
		}
		catalogues[catalogue.Locale] = catalogue
	}
	for index := range matched {
		if !matched[index] {
			issues = append(issues, Issue{
				Code:    CodeMissingContext,
				Path:    pointerIndex("/catalogues", index),
				Message: "pinned catalogue bytes were not supplied",
			})
		}
	}
	return catalogues, issues
}

func catalogueLocales(sidecar *NarrativeSidecar) []string {
	locales := make([]string, len(sidecar.MessageCatalogues))
	for index := range sidecar.MessageCatalogues {
		locales[index] = sidecar.MessageCatalogues[index].Locale
	}
	return locales
}

func catalogueSnapshots(catalogues map[string]*MessageCatalogue) []NarrativeCatalogues {
	locales := make([]string, 0, len(catalogues))
	for locale := range catalogues {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	snapshots := make([]NarrativeCatalogues, 0, len(locales))
	for _, locale := range locales {
		snapshot := NarrativeCatalogues{Locale: locale, Values: make(map[string]string)}
		for key, value := range catalogues[locale].Values {
			snapshot.Values[key] = value
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func snapshotNarrativeInventory(source NarrativeInventory) NarrativeInventory {
	snapshot := NarrativeInventory{
		Binding:             snapshotOwnerBindingKey(source.Binding),
		Assets:              make([]PackageAssetBinding, len(source.Assets)),
		Cases:               append([]string{}, source.Cases...),
		Incidents:           append([]string{}, source.Incidents...),
		Checks:              append([]string{}, source.Checks...),
		Queries:             append([]string{}, source.Queries...),
		Proposals:           append([]string{}, source.Proposals...),
		PresentationTargets: append([]string{}, source.PresentationTargets...),
	}
	for index := range source.Assets {
		snapshot.Assets[index] = PackageAssetBinding{
			AssetID:   source.Assets[index].AssetID,
			Kind:      source.Assets[index].Kind,
			Reference: source.Assets[index].Reference,
			Bytes:     source.Assets[index].Bytes,
		}
	}
	return snapshot
}

func equalNarrativeInventory(left, right NarrativeInventory, authority ProjectAuthority) bool {
	if !stringSlicesEqual(left.Cases, right.Cases) ||
		!stringSlicesEqual(left.Incidents, right.Incidents) ||
		!stringSlicesEqual(left.Checks, right.Checks) ||
		!stringSlicesEqual(left.Queries, right.Queries) ||
		!stringSlicesEqual(left.Proposals, right.Proposals) ||
		!stringSlicesEqual(left.PresentationTargets, right.PresentationTargets) ||
		!equalOwnerBindingKey(left.Binding, right.Binding, authority) ||
		len(left.Assets) != len(right.Assets) {
		return false
	}
	for index := range left.Assets {
		if left.Assets[index] != right.Assets[index] {
			return false
		}
	}
	return true
}

func snapshotOwnerBindingKey(source OwnerBindingKey) OwnerBindingKey {
	return OwnerBindingKey{
		Project:           ProjectIdentity{Value: source.Project.Value},
		ContentPin:        source.ContentPin,
		DeclarationSHA256: source.DeclarationSHA256,
		Package:           source.Package,
	}
}

func snapshotCheckBinding(source CheckBinding) CheckBinding {
	return CheckBinding{
		Binding:           snapshotOwnerBindingKey(source.Binding),
		CheckID:           source.CheckID,
		DefinitionAssetID: source.DefinitionAssetID,
	}
}

func snapshotProposalBinding(source ProposalBinding) ProposalBinding {
	return ProposalBinding{
		Binding:      snapshotOwnerBindingKey(source.Binding),
		ProposalID:   source.ProposalID,
		Kind:         source.Kind,
		DefinitionID: source.DefinitionID,
		EvidenceIDs:  append([]string{}, source.EvidenceIDs...),
		Rationale:    source.Rationale,
	}
}

func snapshotPresentationTargetBinding(source PresentationTargetBinding) PresentationTargetBinding {
	snapshot := PresentationTargetBinding{
		Binding:  snapshotOwnerBindingKey(source.Binding),
		TargetID: source.TargetID,
		Kind:     source.Kind,
		RefID:    source.RefID,
	}
	if source.Closure != nil {
		closure := &PresentationClosure{
			Kind: source.Closure.Kind,
			ID:   source.Closure.ID,
		}
		if source.Closure.Check != nil {
			check := snapshotCheckBinding(*source.Closure.Check)
			closure.Check = &check
		}
		if source.Closure.Proposal != nil {
			proposal := snapshotProposalBinding(*source.Closure.Proposal)
			closure.Proposal = &proposal
		}
		snapshot.Closure = closure
	}
	return snapshot
}

func assetReferenceKey(ref AssetReference) string {
	return ref.Path + "\x00" + ref.SHA256 + "\x00" + string(ref.Bytes)
}

func uniqueStrings(values []string) bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func sortedEqual(left, right []string) bool {
	return stringSlicesEqual(sortedCopy(left), sortedCopy(right))
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// Projection renders the stable, language-neutral admission view used by the
// shared conformance corpus.
func (n *AdmittedNarrative) Projection() map[string]any {
	messages := make([]any, 0)
	checks := make([]any, 0)
	proposals := make([]any, 0)
	targets := make([]any, 0)
	checkSeen := make(map[string]bool)
	proposalSeen := make(map[string]bool)
	targetSeen := make(map[string]bool)
	for index := range n.Events {
		event := n.Events[index]
		if event.Message != nil {
			messages = append(messages, map[string]any{
				"eventId":      event.ID,
				"key":          event.Message.Key,
				"text":         event.Message.Text,
				"locale":       event.Message.Locale,
				"fromFallback": event.Message.FromFallback,
			})
		}
		if event.Check != nil && !checkSeen[event.Check.CheckID] {
			checkSeen[event.Check.CheckID] = true
			checks = append(checks, map[string]any{
				"checkId":           event.Check.CheckID,
				"definitionAssetId": event.Check.DefinitionAssetID,
			})
		}
		if event.Proposal != nil && !proposalSeen[event.Proposal.ProposalID] {
			proposalSeen[event.Proposal.ProposalID] = true
			proposals = append(proposals, projectProposalBinding(*event.Proposal))
		}
		if event.PresentationTarget != nil && !targetSeen[event.PresentationTarget.TargetID] {
			targetSeen[event.PresentationTarget.TargetID] = true
			targets = append(targets, map[string]any{
				"targetId": event.PresentationTarget.TargetID,
				"kind":     event.PresentationTarget.Kind,
				"refId":    event.PresentationTarget.RefID,
			})
		}
	}
	catalogueLocales := make([]string, len(n.Catalogues))
	for index := range n.Catalogues {
		catalogueLocales[index] = n.Catalogues[index].Locale
	}
	return map[string]any{
		"scenarioId":          n.Sidecar.ScenarioID,
		"revision":            string(n.Sidecar.Revision),
		"package":             projectPackageReference(n.Package),
		"locale":              n.Locale.Projection(),
		"textBytes":           string(n.TextBytes),
		"catalogueLocales":    catalogueLocales,
		"messages":            messages,
		"checks":              checks,
		"proposals":           proposals,
		"presentationTargets": targets,
	}
}

func projectProposalBinding(binding ProposalBinding) map[string]any {
	return map[string]any{
		"proposalId":   binding.ProposalID,
		"kind":         binding.Kind,
		"definitionId": binding.DefinitionID,
		"evidenceIds":  append([]string{}, binding.EvidenceIDs...),
		"rationale":    binding.Rationale,
	}
}
