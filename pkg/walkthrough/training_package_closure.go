package walkthrough

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/challenge"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
)

// trainingPathPin is one unique canonical source+path declaration. Package
// assets all resolve against the same declaring source pin, so a safe
// project-relative path is the canonical identity.
type trainingPathPin struct {
	path     string
	class    string
	sha256   string
	bytes    Integer
	assetIDs []string
}

// packagePaths rejects self-reference and conflicting pins, and returns the
// unique canonical source+path set. Aliases sharing one path may use different
// codecs as long as every alias independently validates; only a text/binary
// accounting-class conflict rejects, because the owner defines exactly one
// budget class per canonical raw buffer.
func (c *checker) packagePaths(pkg *AdmittedTrainingPackage, reference PackageReference) map[string]*trainingPathPin {
	paths := make(map[string]*trainingPathPin, len(pkg.Assets))
	for index := range pkg.Assets {
		asset := &pkg.Assets[index]
		path := pointerIndex("/assets", index)
		if asset.Reference.Path == reference.Path {
			c.report(CodeInvalidValue, pointerChild(path, "path"),
				"the package index is not one of its own assets")
			continue
		}
		class := packageCodecKinds[asset.Kind]
		existing, present := paths[asset.Reference.Path]
		if !present {
			paths[asset.Reference.Path] = &trainingPathPin{
				path:     asset.Reference.Path,
				class:    class,
				sha256:   asset.Reference.SHA256,
				bytes:    asset.Reference.Bytes,
				assetIDs: []string{asset.ID},
			}
			continue
		}
		if existing.sha256 != asset.Reference.SHA256 {
			c.report(CodeMismatch, pointerChild(path, "sha256"),
				"conflicting pins for the same canonical source+path")
			continue
		}
		if existing.bytes != asset.Reference.Bytes {
			c.report(CodeMismatch, pointerChild(path, "bytes"),
				"conflicting byte lengths for the same canonical source+path")
			continue
		}
		if existing.class != class {
			// The same canonical buffer cannot be both text and binary under the
			// owner's kind-owned budgets. Refuse the ambiguous accounting class
			// explicitly instead of picking the first alias or counting twice.
			c.report(CodeMismatch, pointerChild(path, "kind"),
				"one canonical source+path cannot mix text and binary codec classes")
			continue
		}
		existing.assetIDs = append(existing.assetIDs, asset.ID)
	}
	return paths
}

// packageBudgets preflights every declared text and binary pin before any
// child byte is consumed. Kind decides the class; mediaType never does.
func (c *checker) packageBudgets(pkg *AdmittedTrainingPackage, paths map[string]*trainingPathPin) []Issue {
	seen := make(map[string]bool, len(paths))
	textTotal := int64(0)
	binaryTotal := int64(0)
	for index := range pkg.Assets {
		asset := &pkg.Assets[index]
		if seen[asset.Reference.Path] {
			continue
		}
		seen[asset.Reference.Path] = true
		pin := paths[asset.Reference.Path]
		path := pointerIndex("/assets", index)
		switch pin.class {
		case "text":
			bytes, ok := integerAsInt(string(pin.bytes), MaxTextAssetBytes)
			if !ok {
				c.report(CodeTooLong, pointerChild(path, "bytes"),
					"a text asset may be at most 64 KiB")
				continue
			}
			textTotal += bytes
		default:
			bytes, ok := integerAsInt(string(pin.bytes), MaxChallengeFixtureBytes)
			if !ok {
				c.report(CodeTooLong, pointerChild(path, "bytes"),
					"a challenge fixture may be at most 10 MiB")
				continue
			}
			binaryTotal += bytes
		}
	}
	if textTotal > MaxPackageTextBytes {
		c.report(CodeTooLarge, "", "package-contributed text exceeds 6 MiB")
	}
	if binaryTotal > MaxPackageBinaryBytes {
		c.report(CodeTooLarge, "", "unique binary assets exceed 64 MiB")
	}
	if len(c.issues) > 0 {
		return c.issues
	}
	pkg.Contributions = make([]TrainingAssetContribution, 0, len(paths))
	seen = make(map[string]bool, len(paths))
	for index := range pkg.Assets {
		asset := &pkg.Assets[index]
		if seen[asset.Reference.Path] {
			continue
		}
		seen[asset.Reference.Path] = true
		pin := paths[asset.Reference.Path]
		pkg.Contributions = append(pkg.Contributions, TrainingAssetContribution{
			Path:     pin.path,
			SHA256:   pin.sha256,
			Class:    pin.class,
			Bytes:    pin.bytes,
			AssetIDs: append([]string{}, pin.assetIDs...),
		})
	}
	pkg.TextBytes = Integer(itoa(textTotal))
	pkg.BinaryBytes = Integer(itoa(binaryTotal))
	return nil
}

// packageAssetBytes resolves the caller-supplied child bytes against the
// unique declared paths and verifies every length and digest before any codec
// parse. Extra or duplicate supplies refuse.
func packageAssetBytes(
	pkg *AdmittedTrainingPackage,
	paths map[string]*trainingPathPin,
	supplied []TrainingPackageAssetBytes,
) (map[string][]byte, []Issue) {
	byPath := make(map[string][]byte, len(supplied))
	for _, asset := range supplied {
		if _, present := byPath[asset.Path]; present {
			return nil, []Issue{{
				Code:    CodeMismatch,
				Path:    "/assets",
				Message: "asset " + asset.Path + " was supplied more than once",
			}}
		}
		byPath[asset.Path] = asset.Bytes
	}
	for index := range pkg.Assets {
		asset := &pkg.Assets[index]
		pin := paths[asset.Reference.Path]
		// Each unique path is verified once, in declaration order.
		if pin.assetIDs[0] != asset.ID {
			continue
		}
		data, present := byPath[asset.Reference.Path]
		if !present {
			return nil, []Issue{{
				Code:    CodeMissingAsset,
				Path:    "/assets",
				Message: "declared asset " + asset.Reference.Path + " bytes were not supplied",
			}}
		}
		reference := AssetReference{Path: pin.path, SHA256: pin.sha256, Bytes: pin.bytes}
		if err := reference.VerifyBytes(data); err != nil {
			issues := issuesOfError(err)
			for index := range issues {
				if issues[index].Path == "" {
					issues[index].Path = "/assets"
				}
			}
			return nil, issues
		}
	}
	for index := range supplied {
		if _, declared := paths[supplied[index].Path]; !declared {
			return nil, []Issue{{
				Code:    CodeMismatch,
				Path:    "/assets",
				Message: "supplied asset " + supplied[index].Path + " is not declared by the package",
			}}
		}
	}
	return byPath, nil
}

// trainingClosure is the package-local closure state for one admission.
type trainingClosure struct {
	c            *checker
	pkg          *AdmittedTrainingPackage
	bytesByPath  map[string][]byte
	assetsByID   map[string]*TrainingPackageAsset
	evidence     map[string]bool
	actors       map[string]bool
	sources      map[string]*TrainingSourceBinding
	cases        map[string]bool
	incidents    map[string]bool
	checks       map[string]bool
	queries      map[string]bool
	proposals    map[string]bool
	targets      map[string]bool
	nativeChecks map[string]*detective.Check
	manifestByID map[string][]byte
}

// closeTrainingPackage runs native validation, local reference closure and
// budget accounting. It never executes SQL, a Check, a Watch or a challenge.
func (c *checker) closeTrainingPackage(pkg *AdmittedTrainingPackage, bytesByPath map[string][]byte) {
	closure := &trainingClosure{
		c:            c,
		pkg:          pkg,
		bytesByPath:  bytesByPath,
		assetsByID:   make(map[string]*TrainingPackageAsset, len(pkg.Assets)),
		evidence:     map[string]bool{},
		actors:       map[string]bool{},
		sources:      make(map[string]*TrainingSourceBinding, len(pkg.Sources)),
		cases:        map[string]bool{},
		incidents:    map[string]bool{},
		checks:       map[string]bool{},
		queries:      map[string]bool{},
		proposals:    map[string]bool{},
		targets:      map[string]bool{},
		nativeChecks: map[string]*detective.Check{},
		manifestByID: map[string][]byte{},
	}
	for index := range pkg.Assets {
		asset := &pkg.Assets[index]
		closure.assetsByID[asset.ID] = asset
		if isEvidenceKind(asset.Kind) {
			closure.evidence[asset.ID] = true
		}
	}
	for _, actor := range pkg.FictionalActors {
		closure.actors[actor.ID] = true
	}
	for index := range pkg.Sources {
		closure.sources[pkg.Sources[index].ID] = &pkg.Sources[index]
	}
	for _, incident := range pkg.Incidents {
		closure.incidents[incident.ID] = true
	}
	for _, check := range pkg.Checks {
		closure.checks[check.ID] = true
	}
	for _, query := range pkg.Queries {
		closure.queries[query.ID] = true
	}
	for _, proposal := range pkg.Proposals {
		closure.proposals[proposal.ID] = true
	}
	for _, target := range pkg.PresentationTargets {
		closure.targets[target.ID] = true
	}
	for _, trainingCase := range pkg.Cases {
		closure.cases[trainingCase.ID] = true
	}
	closure.closeSources()
	closure.closeCheckActors()
	closure.closeChecks()
	if len(c.issues) > 0 {
		return
	}
	closure.closeQueries()
	closure.closeWatches()
	closure.closeChallenges()
	closure.closeProposals()
	closure.closeCases()
	closure.closeIncidents()
	closure.closePresentationTargets()
}

// closeSources validates every declared source binding independently of query
// traversal. An unused source still has to resolve a challenge fixture.
func (cl *trainingClosure) closeSources() {
	for index := range cl.pkg.Sources {
		source := &cl.pkg.Sources[index]
		asset, present := cl.assetsByID[source.FixtureAssetID]
		if !present || asset.Kind != "challenge-fixture" || !asset.Binary {
			cl.c.report(CodeInvalidReference,
				pointerChild(pointerIndex("/sources", index), "fixtureAssetId"),
				"fixtureAssetId must resolve a challenge-fixture asset")
		}
	}
}

func (cl *trainingClosure) closeCheckActors() {
	for index := range cl.pkg.Assets {
		asset := &cl.pkg.Assets[index]
		if asset.Kind != "check-definition" || asset.Check == nil {
			continue
		}
		if !cl.actors[asset.Check.UpdatedBy] {
			cl.c.report(CodeInvalidReference, pointerIndex("/assets", index),
				"updatedBy must be a declared fictional actor")
		}
	}
}

func (cl *trainingClosure) closeChecks() {
	for index := range cl.pkg.Checks {
		record := &cl.pkg.Checks[index]
		path := pointerIndex("/checks", index)
		asset, present := cl.assetsByID[record.DefinitionAssetID]
		if !present || asset.Kind != "check-definition" || asset.Check == nil {
			cl.c.report(CodeInvalidReference, pointerChild(path, "definitionAssetId"),
				"definitionAssetId must resolve a check-definition asset")
			continue
		}
		if asset.Check.ID != record.ID {
			cl.c.report(CodeMismatch, pointerChild(path, "id"),
				"the loaded native Check id must equal the inventory id")
			continue
		}
		cl.nativeChecks[record.ID] = asset.Check
	}
	if len(cl.c.issues) > 0 {
		return
	}
	for index := range cl.pkg.Checks {
		record := cl.pkg.Checks[index]
		for _, step := range cl.nativeChecks[record.ID].Steps {
			if step.Kind == "query" && !cl.queries[step.QueryID] {
				cl.c.report(CodeInvalidReference, pointerIndex("/checks", index),
					"a Check query step must resolve a declared package query")
			}
		}
	}
	if len(cl.c.issues) > 0 {
		return
	}
	catalog := detective.Catalog{
		Checks:          map[string]detective.Check{},
		Validators:      map[string]detective.PublishedValidator{},
		QueryValidators: map[string]detective.QueryValidators{},
	}
	for id := range cl.nativeChecks {
		catalog.Checks[id] = *cl.nativeChecks[id]
	}
	if err := detective.ValidateCatalog(catalog); err != nil {
		code := CodeInvalidValue
		if errors.Is(err, detective.ErrNotFound) {
			code = CodeInvalidReference
		}
		cl.c.report(code, "/checks", "native catalog graph validation refused the package Checks")
	}
}

func (cl *trainingClosure) closeQueries() {
	for index := range cl.pkg.Queries {
		record := &cl.pkg.Queries[index]
		path := pointerIndex("/queries", index)
		metadataAsset, metadataOK := cl.assetsByID[record.MetadataAssetID]
		if !metadataOK || metadataAsset.Kind != "query-metadata" || metadataAsset.Metadata == nil {
			cl.c.report(CodeInvalidReference, pointerChild(path, "metadataAssetId"),
				"metadataAssetId must resolve a query-metadata asset")
			continue
		}
		bodyAsset, bodyOK := cl.assetsByID[record.BodyAssetID]
		if !bodyOK || bodyAsset.Kind != "query-body-sql" || bodyAsset.Text == "" {
			cl.c.report(CodeInvalidReference, pointerChild(path, "bodyAssetId"),
				"bodyAssetId must resolve a nonempty query-body-sql asset")
			continue
		}
		if _, sourceOK := cl.sources[record.SourceID]; !sourceOK {
			cl.c.report(CodeInvalidReference, pointerChild(path, "sourceId"),
				"sourceId must resolve a declared fixture source")
			continue
		}
		if metadataAsset.Metadata.ID != record.ID {
			cl.c.report(CodeMismatch, pointerChild(path, "metadataAssetId"),
				"the metadata id must equal the inventory query id")
			continue
		}
		if metadataAsset.Metadata.ConnectionID != record.SourceID {
			cl.c.report(CodeMismatch, pointerChild(path, "sourceId"),
				"the metadata connectionId must equal the query sourceId")
			continue
		}
		record.Hydrated = hydrateTrainingQuery(metadataAsset.Metadata, bodyAsset.Text)
		if err := record.Hydrated.Validate(); err != nil {
			cl.c.report(CodeInvalidValue, path, "native QueryDef validation refused the hydrated query")
		}
	}
}

func hydrateTrainingQuery(metadata *TrainingQueryMetadata, body string) datatug.QueryDef {
	query := datatug.QueryDef{
		Type:         datatug.QueryTypeSQL,
		Text:         body,
		ConnectionID: metadata.ConnectionID,
		Parameters:   make(datatug.Parameters, 0, len(metadata.Parameters)),
	}
	query.ID = metadata.ID
	query.Title = metadata.Title
	for _, parameter := range metadata.Parameters {
		definition := datatug.ParameterDef{ID: parameter.ID, Type: parameter.Type}
		if parameter.Title != nil {
			definition.Title = *parameter.Title
		}
		if parameter.IsRequired != nil {
			definition.IsRequired = *parameter.IsRequired
		}
		if parameter.Default != nil {
			switch parameter.Default.Kind {
			case "string":
				definition.DefaultValue = parameter.Default.Text
			case "boolean":
				definition.DefaultValue = parameter.Default.Bool
			case "integer":
				definition.DefaultValue = json.Number(parameter.Default.Number)
			case "number":
				definition.DefaultValue = parameter.Default.Float
			}
		}
		query.Parameters = append(query.Parameters, definition)
	}
	return query
}

func (cl *trainingClosure) closeWatches() {
	for index := range cl.pkg.Assets {
		asset := &cl.pkg.Assets[index]
		if asset.Kind != "watch-recommendation" || asset.Watch == nil {
			continue
		}
		path := pointerIndex("/assets", index)
		check, present := cl.nativeChecks[asset.Watch.CheckID]
		if !present {
			cl.c.report(CodeInvalidReference, pointerChild(path, "checkId"),
				"checkId must resolve a declared package Check")
			continue
		}
		if asset.Watch.CheckVersion != strconv.FormatInt(check.Version, 10) {
			cl.c.report(CodeMismatch, pointerChild(path, "checkVersion"),
				"checkVersion must equal the native Check version")
			continue
		}
		if err := detective.ValidateInputs(check.Inputs, asset.Watch.Inputs); err != nil {
			cl.c.report(CodeInvalidValue, pointerChild(path, "inputs"),
				"native input validation refused the Watch recommendation inputs")
		}
	}
}

func (cl *trainingClosure) closeChallenges() {
	for index := range cl.pkg.Challenges {
		record := &cl.pkg.Challenges[index]
		path := pointerIndex("/challenges", index)
		asset, present := cl.assetsByID[record.ManifestAssetID]
		if !present || asset.Kind != "challenge-manifest" || asset.Manifest == nil {
			cl.c.report(CodeInvalidReference, pointerChild(path, "manifestAssetId"),
				"manifestAssetId must resolve a challenge-manifest asset")
			continue
		}
		manifest := asset.Manifest
		manifestBytes := cl.bytesByPath[asset.Reference.Path]
		if manifest.ChallengeID != record.ID {
			cl.c.report(CodeMismatch, pointerChild(path, "id"),
				"the manifest challengeId must equal the inventory id")
			continue
		}
		if record.Revision != Integer(strconv.Itoa(manifest.Revision)) {
			cl.c.report(CodeMismatch, pointerChild(path, "revision"),
				"the manifest revision must equal the inventory revision")
			continue
		}
		// Native Open owns fixture-suite equality: it rejects missing and extra
		// fixtures and verifies every actual byte length and hash against the
		// manifest pins. This loop only resolves each declared fixture asset.
		fixtures := make(map[string][]byte, len(record.Fixtures))
		for fixtureIndex := range record.Fixtures {
			binding := record.Fixtures[fixtureIndex]
			fixturePath := pointerIndex(pointerChild(path, "fixtures"), fixtureIndex)
			fixtureAsset, ok := cl.assetsByID[binding.AssetID]
			if !ok || !fixtureAsset.Binary {
				cl.c.report(CodeInvalidReference, pointerChild(fixturePath, "assetId"),
					"a challenge fixture must resolve a challenge-fixture asset")
				continue
			}
			fixtures[binding.FixtureID] = cl.bytesByPath[fixtureAsset.Reference.Path]
		}
		if len(cl.c.issues) > 0 {
			continue
		}
		// Open verifies every actual fixture length and hash. Its retained
		// snapshots are discarded here; package admission never returns an
		// executable capability.
		if _, err := challenge.Open(manifestBytes, fixtures); err != nil {
			cl.c.reportChallengeError(path, err)
		}
	}
}

func (cl *trainingClosure) closeProposals() {
	for index := range cl.pkg.Proposals {
		proposal := &cl.pkg.Proposals[index]
		path := pointerIndex("/proposals", index)
		switch proposal.Kind {
		case "query":
			if !cl.queries[proposal.DefinitionID] {
				cl.c.report(CodeInvalidReference, pointerChild(path, "definitionId"),
					"a query proposal must resolve a declared package query")
			}
		case "check":
			if !cl.checks[proposal.DefinitionID] {
				cl.c.report(CodeInvalidReference, pointerChild(path, "definitionId"),
					"a check proposal must resolve a declared package Check")
			}
		case "watch":
			asset, present := cl.assetsByID[proposal.DefinitionID]
			if !present || asset.Kind != "watch-recommendation" || asset.Watch == nil {
				cl.c.report(CodeInvalidReference, pointerChild(path, "definitionId"),
					"a watch proposal must resolve a watch-recommendation asset")
			}
		}
		for _, evidenceID := range proposal.EvidenceIDs {
			if !cl.evidence[evidenceID] {
				cl.c.report(CodeInvalidReference, pointerChild(path, "evidenceIds"),
					"every proposal evidence id must resolve a package evidence asset")
			}
		}
	}
}

func (cl *trainingClosure) closeCases() {
	for index := range cl.pkg.Cases {
		trainingCase := &cl.pkg.Cases[index]
		path := pointerIndex("/cases", index)
		for _, incidentID := range trainingCase.IncidentIDs {
			if !cl.incidents[incidentID] {
				cl.c.report(CodeInvalidReference, pointerChild(path, "incidentIds"),
					"every incident id must resolve a package incident")
			}
		}
		for _, evidenceID := range trainingCase.EvidenceIDs {
			if !cl.evidence[evidenceID] {
				cl.c.report(CodeInvalidReference, pointerChild(path, "evidenceIds"),
					"every evidence id must resolve a package evidence asset")
			}
		}
		localFindings := make(map[string]bool, len(trainingCase.Findings))
		for findingIndex := range trainingCase.Findings {
			finding := &trainingCase.Findings[findingIndex]
			findingPath := pointerIndex(pointerChild(path, "findings"), findingIndex)
			localFindings[finding.ID] = true
			native := detective.Finding{
				ID:           finding.ID,
				Text:         finding.Text,
				Status:       finding.Status,
				EvidenceRefs: finding.EvidenceRefs,
			}
			if err := detective.ValidateFindingFields(native); err != nil {
				cl.c.report(CodeInvalidValue, findingPath,
					"native Finding field validation refused the record")
				continue
			}
			for _, evidenceID := range finding.EvidenceRefs {
				if !cl.evidence[evidenceID] {
					cl.c.report(CodeInvalidReference, pointerChild(findingPath, "evidenceRefs"),
						"every Finding evidence reference must resolve a package evidence asset")
				}
			}
		}
		for timelineIndex := range trainingCase.Timeline {
			entry := &trainingCase.Timeline[timelineIndex]
			entryPath := pointerIndex(pointerChild(path, "timeline"), timelineIndex)
			if !cl.actors[entry.ActorID] {
				cl.c.report(CodeInvalidReference, pointerChild(entryPath, "actorId"),
					"actorId must resolve a declared fictional actor")
			}
			switch entry.Ref.Kind {
			case "finding":
				if !localFindings[entry.Ref.ID] {
					cl.c.report(CodeInvalidReference, pointerChild(pointerChild(entryPath, "ref"), "id"),
						"a finding timeline reference must resolve a Case-local finding")
				}
			case "evidence":
				if !cl.evidence[entry.Ref.ID] {
					cl.c.report(CodeInvalidReference, pointerChild(pointerChild(entryPath, "ref"), "id"),
						"an evidence timeline reference must resolve a package evidence asset")
				}
			case "check":
				if !cl.checks[entry.Ref.ID] {
					cl.c.report(CodeInvalidReference, pointerChild(pointerChild(entryPath, "ref"), "id"),
						"a check timeline reference must resolve a package Check")
				}
			}
		}
	}
}

func (cl *trainingClosure) closeIncidents() {
	for index := range cl.pkg.Incidents {
		incident := &cl.pkg.Incidents[index]
		path := pointerIndex("/incidents", index)
		for _, evidenceID := range incident.EvidenceIDs {
			if !cl.evidence[evidenceID] {
				cl.c.report(CodeInvalidReference, pointerChild(path, "evidenceIds"),
					"every incident evidence id must resolve a package evidence asset")
			}
		}
	}
}

func (cl *trainingClosure) closePresentationTargets() {
	for index := range cl.pkg.PresentationTargets {
		target := &cl.pkg.PresentationTargets[index]
		path := pointerIndex("/presentationTargets", index)
		var resolved bool
		switch target.Kind {
		case "case":
			resolved = cl.cases[target.RefID]
		case "incident":
			resolved = cl.incidents[target.RefID]
		case "evidence":
			resolved = cl.evidence[target.RefID]
		case "check":
			resolved = cl.checks[target.RefID]
		case "proposal":
			resolved = cl.proposals[target.RefID]
		}
		if !resolved {
			cl.c.report(CodeInvalidReference, pointerChild(path, "refId"),
				"refId must resolve the inventory of the declared kind")
		}
	}
}

// TrustedPackageInventory returns this snapshot in the accepted a2
// [PackageInventory] port shape. It is an intentionally reduced identity
// projection: only the verified package, asset and Case identity are exposed.
// It is not a live authority witness and not the complete conformance view.
func (p *AdmittedTrainingPackage) TrustedPackageInventory() PackageInventory {
	assets := make([]AssetReference, len(p.Assets))
	for index := range p.Assets {
		assets[index] = p.Assets[index].Reference
	}
	return PackageInventory{
		Ref:    p.Reference,
		Assets: assets,
		Cases:  caseIDs(p.Cases),
	}
}

// TrustedNarrativeInventory returns this snapshot in the accepted
// [NarrativeInventory] owner shape so a later orchestration adapter can consume
// it directly. It is an intentionally reduced identity projection, not the
// complete conformance view. It carries no fabricated owner verdict: the live
// [NarrativeOwnerContext] must still admit every Check, proposal and
// presentation target.
func (p *AdmittedTrainingPackage) TrustedNarrativeInventory() NarrativeInventory {
	assets := make([]PackageAssetBinding, len(p.Assets))
	for index := range p.Assets {
		asset := &p.Assets[index]
		assets[index] = PackageAssetBinding{
			AssetID:   asset.ID,
			Kind:      asset.Kind,
			Reference: asset.Reference,
			Bytes:     asset.Reference.Bytes,
		}
	}
	return NarrativeInventory{
		Binding: OwnerBindingKey{
			Project:           p.Binding.Project,
			ContentPin:        p.Binding.ContentPin,
			DeclarationSHA256: p.Binding.DeclarationSHA256,
			Package:           p.Reference,
		},
		Assets:              assets,
		Cases:               caseIDs(p.Cases),
		Incidents:           incidentIDs(p.Incidents),
		Checks:              checkIDs(p.Checks),
		Queries:             queryIDs(p.Queries),
		Proposals:           proposalIDs(p.Proposals),
		PresentationTargets: targetIDs(p.PresentationTargets),
	}
}

// Projection renders the complete deterministic, language-neutral conformance
// view used by the independent corpus. It exposes every admitted codec/native
// field, pin and closure result but never opaque project identity or executable
// state. Optional native fields follow the owner DTO's omitempty normalization.
// Callers receive a fresh mutable tree; this is not a tamper-proof capability.
func (p *AdmittedTrainingPackage) Projection() map[string]any {
	assetsByID := make(map[string]*TrainingPackageAsset, len(p.Assets))
	assets := make([]any, len(p.Assets))
	codecs := make([]any, 0, len(p.Assets))
	for index := range p.Assets {
		asset := &p.Assets[index]
		assetsByID[asset.ID] = asset
		assets[index] = map[string]any{
			"id":        asset.ID,
			"kind":      asset.Kind,
			"mediaType": asset.MediaType,
			"path":      asset.Reference.Path,
			"sha256":    asset.Reference.SHA256,
			"bytes":     string(asset.Reference.Bytes),
		}
		codecs = append(codecs, projectAssetCodec(asset))
	}
	contributions := make([]any, len(p.Contributions))
	for index, contribution := range p.Contributions {
		contributions[index] = map[string]any{
			"path":     contribution.Path,
			"sha256":   contribution.SHA256,
			"class":    contribution.Class,
			"bytes":    string(contribution.Bytes),
			"assetIds": append([]string{}, contribution.AssetIDs...),
		}
	}
	return map[string]any{
		"schema":   p.Schema,
		"id":       p.ID,
		"revision": string(p.Revision),
		"package": map[string]any{
			"id":       p.Reference.ID,
			"revision": string(p.Reference.Revision),
			"path":     p.Reference.Path,
			"sha256":   p.Reference.SHA256,
			"bytes":    string(p.Reference.Bytes),
		},
		"binding": map[string]any{
			"contentPin":        projectTrainingContentPin(p.Binding.ContentPin),
			"declarationSha256": p.Binding.DeclarationSHA256,
			"narrativeSha256":   p.Binding.NarrativeSHA256,
			"packageSha256":     p.Binding.PackageSHA256,
			"scenarioId":        p.Binding.ScenarioID,
			"scenarioRevision":  string(p.Binding.ScenarioRevision),
		},
		"assets":              assets,
		"codecs":              codecs,
		"fictionalActors":     projectActors(p.FictionalActors),
		"sources":             projectSources(p.Sources),
		"cases":               projectCases(p.Cases),
		"incidents":           projectIncidents(p.Incidents),
		"checks":              projectCheckRecords(p.Checks),
		"queries":             projectQueryRecords(p.Queries, assetsByID),
		"challenges":          projectChallengeRecords(p.Challenges, assetsByID),
		"proposals":           projectProposals(p.Proposals),
		"presentationTargets": projectPresentationTargets(p.PresentationTargets),
		"textBytes":           string(p.TextBytes),
		"binaryBytes":         string(p.BinaryBytes),
		"contributions":       contributions,
	}
}

func projectTrainingContentPin(pin ContentPin) map[string]any {
	projected := map[string]any{"kind": pin.Kind}
	switch pin.Kind {
	case PinGit:
		projected["commit"] = pin.Commit
	case PinDigest:
		projected["sha256"] = pin.SHA256
	}
	return projected
}

func projectAssetCodec(asset *TrainingPackageAsset) map[string]any {
	projected := map[string]any{"assetId": asset.ID, "kind": asset.Kind}
	switch asset.Kind {
	case "evidence-text", "query-body-sql":
		projected["text"] = asset.Text
	case "evidence-records":
		projected["records"] = projectRecords(asset.Records)
	case "evidence-log":
		projected["log"] = projectLog(asset.Log)
	case "query-metadata":
		projected["metadata"] = projectMetadata(asset.Metadata)
	case "check-definition":
		projected["check"] = projectNativeCheck(asset.Check)
	case "challenge-manifest":
		projected["manifest"] = projectManifest(asset.Manifest)
	case "watch-recommendation":
		projected["watch"] = projectWatch(asset.Watch)
	case "challenge-fixture":
		projected["binary"] = true
	}
	return projected
}

func projectRecords(records *TrainingRecords) map[string]any {
	if records == nil {
		return nil
	}
	columns := make([]any, len(records.Columns))
	for index, column := range records.Columns {
		columns[index] = map[string]any{"name": column.Name, "type": string(column.Type)}
	}
	rows := make([]any, len(records.Rows))
	for rowIndex, row := range records.Rows {
		cells := make([]any, len(row))
		for cellIndex := range row {
			cells[cellIndex] = projectTypedValue(row[cellIndex])
		}
		rows[rowIndex] = cells
	}
	return map[string]any{"columns": columns, "rows": rows}
}

func projectLog(entries []TrainingLogEntry) []any {
	projected := make([]any, len(entries))
	for index, entry := range entries {
		projected[index] = map[string]any{
			"id": entry.ID, "at": entry.At, "actorId": entry.ActorID,
			"kind": entry.Kind, "message": entry.Message,
		}
	}
	return projected
}

func projectMetadata(metadata *TrainingQueryMetadata) map[string]any {
	if metadata == nil {
		return nil
	}
	parameters := make([]any, len(metadata.Parameters))
	for index := range metadata.Parameters {
		parameters[index] = projectParameter(&metadata.Parameters[index])
	}
	return map[string]any{
		"id": metadata.ID, "title": metadata.Title, "type": metadata.Type,
		"connectionId": metadata.ConnectionID, "parameters": parameters,
	}
}

func projectParameter(parameter *TrainingParameter) map[string]any {
	projected := map[string]any{"id": parameter.ID, "type": parameter.Type}
	if parameter.Title != nil {
		projected["title"] = *parameter.Title
	}
	if parameter.IsRequired != nil {
		projected["isRequired"] = *parameter.IsRequired
	}
	if parameter.Default != nil {
		defaultValue := map[string]any{"type": parameter.Default.Kind}
		switch parameter.Default.Kind {
		case "string":
			defaultValue["value"] = parameter.Default.Text
		case "boolean":
			defaultValue["value"] = parameter.Default.Bool
		case "integer":
			defaultValue["value"] = parameter.Default.Number
		case "number":
			defaultValue["value"] = parameter.Default.Float
		}
		projected["defaultValue"] = defaultValue
	}
	return projected
}

func projectNativeCheck(check *detective.Check) map[string]any {
	if check == nil {
		return nil
	}
	inputs := make([]any, len(check.Inputs))
	for index, input := range check.Inputs {
		entry := map[string]any{
			"name": input.Name, "type": string(input.Type),
			"cardinality": input.Cardinality,
		}
		// required is a native omitempty boolean: false is the owner's
		// normalized absence, so the projection must not invent a present false.
		if input.Required {
			entry["required"] = true
		}
		inputs[index] = entry
	}
	steps := make([]any, len(check.Steps))
	for index := range check.Steps {
		steps[index] = projectNativeStep(&check.Steps[index])
	}
	projected := map[string]any{
		"id": check.ID, "title": check.Title,
		"version": strconv.FormatInt(check.Version, 10),
		"inputs":  inputs, "steps": steps,
		"updatedAt": check.UpdatedAt.Format(time.RFC3339),
		"updatedBy": check.UpdatedBy,
	}
	if check.Description != "" {
		projected["description"] = check.Description
	}
	if check.CreatedAt != "" {
		projected["createdAt"] = check.CreatedAt
	}
	return projected
}

func projectNativeStep(step *detective.Step) map[string]any {
	projected := map[string]any{"id": step.ID, "kind": step.Kind}
	if step.QueryID != "" {
		projected["queryId"] = step.QueryID
	}
	if step.CheckID != "" {
		projected["checkId"] = step.CheckID
	}
	if step.OnUnavailable != "" {
		projected["onUnavailable"] = step.OnUnavailable
	}
	bindings := make([]any, len(step.Bindings))
	for index, binding := range step.Bindings {
		bindings[index] = projectBinding(binding)
	}
	if len(bindings) > 0 {
		projected["bindings"] = bindings
	}
	validators := make([]any, len(step.Validators))
	for index := range step.Validators {
		validators[index] = projectValidator(&step.Validators[index])
	}
	if len(validators) > 0 {
		projected["validators"] = validators
	}
	return projected
}

func projectBinding(binding detective.Binding) map[string]any {
	projected := map[string]any{"input": binding.Input}
	if binding.FromInput != "" {
		projected["fromInput"] = binding.FromInput
	}
	if binding.Part != "" {
		projected["part"] = binding.Part
	}
	if binding.FromColumn != "" {
		projected["fromColumn"] = binding.FromColumn
	}
	return projected
}

func projectValidator(validator *detective.Validator) map[string]any {
	projected := map[string]any{"id": validator.ID}
	for _, field := range []struct {
		name  string
		value string
	}{
		{"validatorId", validator.ValidatorID},
		{"scope", validator.Scope},
		{"kind", validator.Kind},
		{"severity", validator.Severity},
		{"field", validator.Field},
		{"otherField", validator.OtherField},
		{"checkId", validator.CheckID},
		{"onUnavailable", validator.OnUnavailable},
	} {
		if field.value != "" {
			projected[field.name] = field.value
		}
	}
	if validator.Expected != nil {
		projected["expected"] = projectTypedValue(*validator.Expected)
	}
	if len(validator.Bindings) > 0 {
		bindings := make([]any, len(validator.Bindings))
		for index, binding := range validator.Bindings {
			bindings[index] = projectBinding(binding)
		}
		projected["bindings"] = bindings
	}
	return projected
}

func projectManifest(manifest *challenge.Manifest) map[string]any {
	if manifest == nil {
		return nil
	}
	fixtures := make([]any, len(manifest.FixtureSuite.Fixtures))
	for index, fixture := range manifest.FixtureSuite.Fixtures {
		fixtures[index] = map[string]any{
			"id": fixture.ID, "label": fixture.Label,
			"sha256": fixture.SHA256, "bytes": fixture.Bytes, "runs": fixture.Runs,
		}
	}
	tables := make([]any, len(manifest.VisibleSchema.Tables))
	for tableIndex, table := range manifest.VisibleSchema.Tables {
		columns := make([]any, len(table.Columns))
		for columnIndex, column := range table.Columns {
			projectedColumn := map[string]any{"name": column.Name, "type": column.Type}
			if column.PrimaryKey {
				projectedColumn["primaryKey"] = true
			}
			if column.NotNull {
				projectedColumn["notNull"] = true
			}
			if column.Collation != "" {
				projectedColumn["collation"] = column.Collation
			}
			columns[columnIndex] = projectedColumn
		}
		tables[tableIndex] = map[string]any{"name": table.Name, "columns": columns}
	}
	parts := make([]any, len(manifest.Parts))
	for index := range manifest.Parts {
		parts[index] = projectManifestPart(&manifest.Parts[index])
	}
	hints := make([]any, len(manifest.Hints))
	for index, hint := range manifest.Hints {
		hints[index] = map[string]any{"id": hint.ID, "text": hint.Text}
	}
	references := make([]any, len(manifest.ReferenceSolutions))
	for index := range manifest.ReferenceSolutions {
		references[index] = projectReferenceSolution(&manifest.ReferenceSolutions[index])
	}
	return map[string]any{
		"schemaVersion":    manifest.SchemaVersion,
		"gradingVersion":   manifest.GradingVersion,
		"challengeId":      manifest.ChallengeID,
		"revision":         manifest.Revision,
		"title":            manifest.Title,
		"objective":        manifest.Objective,
		"difficulty":       manifest.Difficulty,
		"dialect":          manifest.Dialect,
		"allowedOperation": manifest.AllowedOperation,
		"timing":           string(manifest.Timing),
		"engine": map[string]any{
			"package":       manifest.Engine.Package,
			"version":       manifest.Engine.Version,
			"sqliteVersion": manifest.Engine.SQLiteVersion,
		},
		"fixtureSuite": map[string]any{
			"id": manifest.FixtureSuite.ID, "fixtures": fixtures,
		},
		"visibleSchema":      map[string]any{"tables": tables},
		"parts":              parts,
		"hints":              hints,
		"referenceSolutions": references,
	}
}

func projectManifestPart(part *challenge.Part) map[string]any {
	projected := map[string]any{
		"id": part.ID, "title": part.Title,
		"required": part.Required, "answerKind": part.AnswerKind,
	}
	if part.SQL != nil {
		cases := make([]any, len(part.SQL.Cases))
		for index, item := range part.SQL.Cases {
			cases[index] = map[string]any{
				"fixture": item.Fixture, "oracle": projectOracle(&item.Oracle),
			}
		}
		projected["sql"] = map[string]any{"cases": cases}
	}
	if part.Structured != nil {
		projected["structured"] = projectStructuredPart(part.Structured)
	}
	if part.CheckDefinition != nil {
		projected["checkDefinition"] = projectCheckDefinitionPart(part.CheckDefinition)
	}
	return projected
}

func projectOracle(oracle *challenge.Oracle) map[string]any {
	if oracle == nil {
		return nil
	}
	switch oracle.Kind {
	case challenge.OracleResult:
		result := oracle.Result
		if result == nil {
			return map[string]any{"kind": challenge.OracleResult}
		}
		projected := map[string]any{
			"kind":     challenge.OracleResult,
			"columns":  projectResultColumns(result.Columns),
			"rows":     projectResultRows(result.Rows),
			"ordering": result.Ordering,
		}
		if result.RequireColumnNames {
			projected["requireColumnNames"] = true
		}
		if result.NumericTolerance != "" {
			projected["numericTolerance"] = result.NumericTolerance
		}
		return projected
	case challenge.OracleFinalState:
		finalState := oracle.FinalState
		if finalState == nil {
			return map[string]any{"kind": challenge.OracleFinalState}
		}
		tables := make([]any, len(finalState.Tables))
		for index, table := range finalState.Tables {
			tables[index] = map[string]any{
				"name":    table.Name,
				"columns": projectResultColumns(table.Columns),
				"rows":    projectResultRows(table.Rows),
			}
		}
		projected := map[string]any{"kind": challenge.OracleFinalState, "tables": tables}
		if len(finalState.Invariants) > 0 {
			invariants := make([]any, len(finalState.Invariants))
			for index, invariant := range finalState.Invariants {
				invariants[index] = map[string]any{
					"id": invariant.ID, "kind": invariant.Kind, "table": invariant.Table,
					"groupBy": append([]string{}, invariant.GroupBy...),
					"column":  invariant.Column, "description": invariant.Description,
				}
			}
			projected["invariants"] = invariants
		}
		return projected
	default:
		return map[string]any{"kind": oracle.Kind}
	}
}

func projectResultColumns(columns []apicontract.Column) []any {
	projected := make([]any, len(columns))
	for index, column := range columns {
		projected[index] = map[string]any{"name": column.Name, "type": column.Type}
	}
	return projected
}

func projectResultRows(rows [][]apicontract.TypedValue) []any {
	projected := make([]any, len(rows))
	for rowIndex, row := range rows {
		cells := make([]any, len(row))
		for cellIndex := range row {
			cells[cellIndex] = projectTypedValue(row[cellIndex])
		}
		projected[rowIndex] = cells
	}
	return projected
}

func projectStructuredPart(part *challenge.StructuredPart) map[string]any {
	fields := make([]any, len(part.Fields))
	for index, field := range part.Fields {
		options := make([]any, len(field.Options))
		for optionIndex, option := range field.Options {
			options[optionIndex] = map[string]any{"id": option.ID, "label": option.Label}
		}
		projectedField := map[string]any{
			"id": field.ID, "label": field.Label,
			"cardinality": field.Cardinality, "required": field.Required,
			"options": options,
		}
		if field.MinSelections != 0 {
			projectedField["minSelections"] = field.MinSelections
		}
		if field.MaxSelections != 0 {
			projectedField["maxSelections"] = field.MaxSelections
		}
		fields[index] = projectedField
	}
	return map[string]any{
		"fields": fields,
		"expected": map[string]any{
			"selections": projectSelections(part.Expected.Selections),
		},
	}
}

func projectSelections(selections map[string][]string) map[string]any {
	projected := make(map[string]any, len(selections))
	for name, values := range selections {
		projected[name] = append([]string{}, values...)
	}
	return projected
}

func projectCheckDefinitionPart(part *challenge.CheckDefinitionPart) map[string]any {
	cases := make([]any, len(part.Cases))
	for index, item := range part.Cases {
		cases[index] = map[string]any{
			"fixture": item.Fixture, "expectedOutcome": item.ExpectedOutcome,
		}
	}
	return map[string]any{
		"queryIds":    append([]string{}, part.QueryIDs...),
		"expectation": projectValidator(&part.Expectation),
		"cases":       cases,
	}
}

func projectReferenceSolution(solution *challenge.ReferenceSolution) map[string]any {
	answer := map[string]any{}
	if solution.Answer.SQL != "" {
		answer["sql"] = solution.Answer.SQL
	}
	if solution.Answer.Structured != nil {
		answer["structured"] = map[string]any{
			"selections": projectSelections(solution.Answer.Structured.Selections),
		}
	}
	if solution.Answer.CheckDefinition != nil {
		answer["checkDefinition"] = map[string]any{
			"queryId":     solution.Answer.CheckDefinition.QueryID,
			"expectation": projectValidator(&solution.Answer.CheckDefinition.Expectation),
		}
	}
	return map[string]any{"partId": solution.PartID, "answer": answer}
}

func projectWatch(watch *TrainingWatchRecommendation) map[string]any {
	if watch == nil {
		return nil
	}
	inputs := make(map[string]any, len(watch.Inputs))
	for name, value := range watch.Inputs {
		inputs[name] = projectInputValue(value)
	}
	return map[string]any{
		"title": watch.Title, "checkId": watch.CheckID, "checkVersion": watch.CheckVersion,
		"inputs": inputs, "intervalSeconds": string(watch.IntervalSeconds),
		"trigger": watch.Trigger, "initialObservation": watch.InitialObservation,
		"mode": watch.Mode, "cooldownSeconds": string(watch.CooldownSeconds),
		"recovery": watch.Recovery,
	}
}

func projectTypedValue(value investigation.TypedValue) map[string]any {
	projected := map[string]any{"type": string(value.Type)}
	switch value.Type {
	case investigation.ValueTypeNumber:
		projected["value"] = value.Num
	case investigation.ValueTypeBoolean:
		projected["value"] = value.Bool
	case investigation.ValueTypeNull:
		projected["value"] = nil
	default:
		projected["value"] = value.Str
	}
	return projected
}

func projectInputValue(value detective.InputValue) map[string]any {
	switch {
	case value.Scalar != nil:
		return map[string]any{"scalar": projectTypedValue(*value.Scalar)}
	case value.Set != nil:
		values := make([]any, len(value.Set.Values))
		for index, item := range value.Set.Values {
			values[index] = projectTypedValue(item)
		}
		return map[string]any{"set": values}
	default:
		start := investigation.TypedValue{}
		end := investigation.TypedValue{}
		if value.Range != nil {
			start = value.Range.Start
			end = value.Range.End
		}
		return map[string]any{
			"range": map[string]any{
				"start": projectTypedValue(start),
				"end":   projectTypedValue(end),
			},
		}
	}
}

func projectActors(actors []TrainingFictionalActor) []any {
	projected := make([]any, len(actors))
	for index, actor := range actors {
		projected[index] = map[string]any{"id": actor.ID, "name": actor.Name}
	}
	return projected
}

func projectSources(sources []TrainingSourceBinding) []any {
	projected := make([]any, len(sources))
	for index, source := range sources {
		projected[index] = map[string]any{
			"id": source.ID, "fixtureAssetId": source.FixtureAssetID,
		}
	}
	return projected
}

func projectCases(cases []TrainingCase) []any {
	projected := make([]any, len(cases))
	for index := range cases {
		trainingCase := &cases[index]
		context := make(map[string]any, len(trainingCase.Context))
		for name, value := range trainingCase.Context {
			context[name] = projectInputValue(value)
		}
		findings := make([]any, len(trainingCase.Findings))
		for findingIndex, finding := range trainingCase.Findings {
			entry := map[string]any{
				"id": finding.ID, "text": finding.Text, "status": finding.Status,
			}
			if finding.HasEvidenceRefs {
				entry["evidenceRefs"] = append([]string{}, finding.EvidenceRefs...)
			}
			findings[findingIndex] = entry
		}
		timeline := make([]any, len(trainingCase.Timeline))
		for entryIndex, entry := range trainingCase.Timeline {
			timeline[entryIndex] = map[string]any{
				"id": entry.ID, "at": entry.At, "actorId": entry.ActorID, "type": entry.Type,
				"ref": map[string]any{"kind": entry.Ref.Kind, "id": entry.Ref.ID},
			}
		}
		projected[index] = map[string]any{
			"id": trainingCase.ID, "title": trainingCase.Title, "status": trainingCase.Status,
			"context": context, "incidentIds": append([]string{}, trainingCase.IncidentIDs...),
			"evidenceIds": append([]string{}, trainingCase.EvidenceIDs...),
			"findings":    findings, "timeline": timeline,
		}
	}
	return projected
}

func projectIncidents(incidents []TrainingIncident) []any {
	projected := make([]any, len(incidents))
	for index, incident := range incidents {
		projected[index] = map[string]any{
			"id": incident.ID, "title": incident.Title, "description": incident.Description,
			"status": incident.Status, "evidenceIds": append([]string{}, incident.EvidenceIDs...),
		}
	}
	return projected
}

func projectCheckRecords(records []TrainingCheckRecord) []any {
	projected := make([]any, len(records))
	for index, record := range records {
		projected[index] = map[string]any{
			"id": record.ID, "definitionAssetId": record.DefinitionAssetID,
		}
	}
	return projected
}

func projectQueryRecords(records []TrainingQueryRecord, assets map[string]*TrainingPackageAsset) []any {
	projected := make([]any, len(records))
	for index, record := range records {
		parameters := make([]any, 0, len(record.Hydrated.Parameters))
		for parameterIndex := range record.Hydrated.Parameters {
			parameter := &record.Hydrated.Parameters[parameterIndex]
			entry := map[string]any{"id": parameter.ID, "type": parameter.Type}
			if parameter.Title != "" {
				entry["title"] = parameter.Title
			}
			if parameter.IsRequired {
				entry["isRequired"] = true
			}
			if parameter.DefaultValue != nil {
				entry["defaultValue"] = jsonPlainValue(parameter.DefaultValue)
			}
			parameters = append(parameters, entry)
		}
		projected[index] = map[string]any{
			"id": record.ID, "metadataAssetId": record.MetadataAssetID,
			"bodyAssetId": record.BodyAssetID, "sourceId": record.SourceID,
			"hydrated": map[string]any{
				"id": record.Hydrated.ID, "type": string(record.Hydrated.Type),
				"text": record.Hydrated.Text, "connectionId": record.Hydrated.ConnectionID,
				"parameters": parameters,
			},
		}
	}
	return projected
}

// jsonPlainValue renders one native parameter default losslessly.
func jsonPlainValue(value any) any {
	switch typed := value.(type) {
	case json.Number:
		return string(typed)
	default:
		return typed
	}
}

func projectChallengeRecords(records []TrainingChallengeRecord, assets map[string]*TrainingPackageAsset) []any {
	projected := make([]any, len(records))
	for index, record := range records {
		fixtures := make([]any, len(record.Fixtures))
		for fixtureIndex, fixture := range record.Fixtures {
			entry := map[string]any{
				"fixtureId": fixture.FixtureID, "assetId": fixture.AssetID,
			}
			if asset, present := assets[fixture.AssetID]; present {
				entry["path"] = asset.Reference.Path
				entry["sha256"] = asset.Reference.SHA256
				entry["bytes"] = string(asset.Reference.Bytes)
			}
			fixtures[fixtureIndex] = entry
		}
		projected[index] = map[string]any{
			"id": record.ID, "revision": string(record.Revision),
			"manifestAssetId": record.ManifestAssetID, "fixtures": fixtures,
		}
	}
	return projected
}

func projectProposals(proposals []TrainingProposal) []any {
	projected := make([]any, len(proposals))
	for index, proposal := range proposals {
		projected[index] = map[string]any{
			"id": proposal.ID, "kind": proposal.Kind,
			"definitionId": proposal.DefinitionID,
			"evidenceIds":  append([]string{}, proposal.EvidenceIDs...),
			"rationale":    proposal.Rationale,
		}
	}
	return projected
}

func projectPresentationTargets(targets []TrainingPresentationTarget) []any {
	projected := make([]any, len(targets))
	for index, target := range targets {
		projected[index] = map[string]any{
			"id": target.ID, "kind": target.Kind, "refId": target.RefID,
		}
	}
	return projected
}
