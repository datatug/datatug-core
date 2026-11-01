package dataaccess

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sync"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/dalgo/tugql"
)

// StructuredResourceIdentity describes one source discovered by DALgo, never
// a permission. Table components are byte-exact; Path names a structural source.
// A database-qualified/federated source is not admitted by this single-span API.
type StructuredResourceIdentity struct {
	Table access.TableName
	Path  string
}

type StructuredIdentityRequest struct {
	Connection ConnectionRef
	Resources  []StructuredResourceIdentity
}

type StructuredIdentityFacts struct {
	Connection ConnectionRef
	Resources  []StructuredResourceIdentity
}

// StructuredReadSource is a trusted execution-only host port. Open follows Core
// candidacy. It must pin an independently identified native read-only span and
// a compiler profile that supports the actual operation; no permissions, grants,
// user policies or model-selected credentials enter it. Unsupported native
// profiles refuse. A TableReadSource never implies this broader profile.
type StructuredReadSource interface {
	OpenStructuredRead(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error)
}

// StructuredReadSpan has the same cancellation, rollback, allocation and
// cleanup-reporting duties as TableReadSpan. Verify must independently attest
// ALL requested resources and byte-exact compiler/source routing for this span.
// It is SQL-free while rows are active; request facts are not evidence to echo.
type StructuredReadSpan interface {
	Session() dal.ReadSession
	Verify(context.Context, StructuredIdentityRequest) (StructuredIdentityFacts, error)
	Close() error
}

func structuredIdentities(resources []access.Resource) ([]StructuredResourceIdentity, error) {
	if len(resources) == 0 {
		return nil, fmt.Errorf("dataaccess: structured resources required")
	}
	identities := make([]StructuredResourceIdentity, 0, len(resources))
	for _, resource := range resources {
		identity := StructuredResourceIdentity{}
		if table, ok := resource.Table(); ok {
			if table.Database != "" || !canonical(table.Schema) || !canonical(table.Name) {
				return nil, fmt.Errorf("dataaccess: structured source requires independent connection authority")
			}
			identity.Table = table
		} else if resource.Kind() == access.PathResource && resource.String() != "/" {
			identity.Path = resource.String()
		} else {
			return nil, fmt.Errorf("dataaccess: opaque structured source unsupported")
		}
		if !slices.Contains(identities, identity) {
			identities = append(identities, identity)
		}
	}
	return identities, nil
}

type structuredIdentityBinding struct {
	span    StructuredReadSpan
	request StructuredIdentityRequest
}

func (b *structuredIdentityBinding) verify(ctx context.Context) error {
	request := b.request
	request.Resources = slices.Clone(request.Resources)
	facts, err := b.span.Verify(ctx, request)
	if err != nil {
		return err
	}
	if !sameConnection(facts.Connection, b.request.Connection) || !slices.Equal(facts.Resources, b.request.Resources) {
		return fmt.Errorf("dataaccess: native structured identity mismatch")
	}
	return ctx.Err()
}

// ReadStructuredResult freezes serialized intent before resolving authority,
// opens only after every resource is a candidate, and retains the same original
// authority ceiling through native reads, serialization and guarded output.
// Additional DALgo context policies remain restrictive subordinate constraints.
func (a *Authority) ReadStructuredResult(ctx context.Context, request ResultRequest, operation StructuredRead, source StructuredReadSource) (*Result, error) {
	if a == nil || a.resolve == nil || ctx == nil || isNilValue(source) {
		return nil, fmt.Errorf("dataaccess: structured source, authority and context required")
	}
	expected, err := operation.RequestFor(request.Nonce)
	if err != nil || request != expected {
		return nil, fmt.Errorf("dataaccess: structured request binding mismatch")
	}
	query, readRequest, err := operation.query()
	if err != nil {
		return nil, err
	}
	// A human principal or role list carried by the caller's context cannot
	// become the authority's policy identity, especially for fixed Agent reads.
	subject := a.request.Subject
	ctx = access.WithPrincipal(ctx, access.Principal{Subject: &access.PrincipalRef{Realm: subject.Realm, Kind: access.PrincipalKind(subject.Kind), ID: subject.ID}})
	prepared, bound, cancel, err := a.prepareRequest(ctx, nil, readRequest)
	if err != nil {
		return nil, err
	}
	clampContext(ctx, bound)
	result := a.newStructuredResult(ctx, request, operation, bound, make([]byte, 0, int(operation.MaxBytes)))
	result.ownCleanup(cancel)
	identities, _ := structuredIdentities(readRequest.Resources)
	factsRequest := StructuredIdentityRequest{Connection: a.request.Connection, Resources: identities}
	opening := factsRequest
	opening.Resources = slices.Clone(identities)
	span, err := source.OpenStructuredRead(prepared, opening)
	if !isNilValue(span) {
		result.ownCleanup(func() { _ = span.Close() })
	}
	if err != nil || isNilValue(span) {
		result.Close()
		return nil, a.denial(DataRead, "STRUCTURED_SOURCE_UNAVAILABLE")
	}
	result.mu.Lock()
	bound.identity = &structuredIdentityBinding{span: span, request: factsRequest}
	result.mu.Unlock()
	if err := result.readStructured(prepared, span, query, operation); err != nil {
		result.Close()
		return nil, err
	}
	return result, nil
}

func (r *Result) readStructured(ctx context.Context, span StructuredReadSpan, query dal.StructuredQuery, op StructuredRead) error {
	if err := r.guard(ctx); err != nil {
		return err
	}
	session := span.Session()
	if isNilValue(session) {
		return r.authority.denial(DataRead, "ACL_ENFORCEMENT_UNSUPPORTED")
	}
	secured := access.SecureReadSession(frozenStructuredSession{ReadSession: session, result: r, span: span}, r.authorization.policy)
	reader, err := secured.ExecuteQueryToRecordsetReader(ctx, query)
	var once sync.Once
	var closeErr error
	closeReader := func() { once.Do(func() { closeErr = reader.Close() }) }
	if !isNilValue(reader) {
		r.ownCleanup(closeReader)
	}
	if err != nil || isNilValue(reader) {
		return r.authority.denial(DataRead, "STRUCTURED_READ_FAILED")
	}
	if err := r.guard(ctx); err != nil {
		return err
	}
	if err := r.readTableRows(ctx, reader, TableRead{MaxRows: op.MaxRows, MaxBytes: op.MaxBytes}); err != nil {
		return err
	}
	closeReader()
	reader = nil
	if closeErr != nil {
		return r.authority.denial(DataRead, "STRUCTURED_READER_CLOSE_FAILED")
	}
	if err := r.guard(ctx); err != nil {
		return err
	}
	return r.sealTable()
}

type frozenStructuredSession struct {
	dal.ReadSession
	result *Result
	span   StructuredReadSpan
}

func (s frozenStructuredSession) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	structured, ok := query.(dal.StructuredQuery)
	if !ok || len(options) != 0 {
		return nil, fmt.Errorf("dataaccess: unsupported structured rewrite")
	}
	// DALgo's row/field constraints may rewrite a query. Freeze their complete
	// effective operation and authorize its actual sources before native data
	// dispatch. Never substitute only the original operation's resource subset.
	serialized, err := tugql.Serialize(structured)
	if err != nil {
		return nil, err
	}
	defer clear(serialized)
	if len(serialized) > 131072 {
		return nil, fmt.Errorf("dataaccess: effective structured query exceeds byte limit")
	}
	frozen, actual, err := parseStructuredOperation(serialized)
	if err != nil {
		return nil, err
	}
	s.result.mu.Lock()
	bound := *s.result.authorization
	s.result.mu.Unlock()
	resources := unionStructuredResources(bound.request.Resources, actual.Resources)
	// Both sets were independently admitted by structuredIdentities before
	// this union; taking their union cannot introduce an unsupported identity.
	identities, _ := structuredIdentities(resources)
	bound.identity = &structuredIdentityBinding{span: s.span, request: StructuredIdentityRequest{Connection: s.result.authority.request.Connection, Resources: identities}}
	request := &access.Request{Operation: access.Query, Resources: resources}
	prepared, current, cancel, err := s.result.authority.prepareWithCeiling(ctx, &bound, request, s.result.tightenResultCeiling)
	if err != nil {
		return nil, err
	}
	// Keep both the original and transformed sources bound for retained release.
	s.result.mu.Lock()
	s.result.authorization.request = copyReadRequest(request)
	s.result.authorization.identity = bound.identity
	s.result.executedTugQL = string(serialized)
	s.result.mu.Unlock()
	s.result.ownCleanup(cancel)
	return s.result.authority.executeStructuredReader(prepared, current, s.ReadSession, frozen)
}

func (a *Authority) executeStructuredReader(ctx context.Context, current *lease, session dal.ReadSession, query dal.StructuredQuery) (dal.RecordsetReader, error) {
	if err := a.check(ctx, current); err != nil {
		return nil, err
	}
	return session.ExecuteQueryToRecordsetReader(ctx, query)
}

func unionStructuredResources(original, actual []access.Resource) []access.Resource {
	resources := slices.Clone(original)
	for _, resource := range actual {
		if !slices.ContainsFunc(resources, func(existing access.Resource) bool { return reflect.DeepEqual(existing, resource) }) {
			resources = append(resources, resource)
		}
	}
	return resources
}
