package perimeter

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/datatug/datatug-core/pkg/dataaccess"
	"github.com/stretchr/testify/require"
)

// recordingSource is the fake adapter: it records whether a read passed every
// Core and DALgo check and reached native dispatch. It never returns a row.
type recordingSource struct{ dispatched bool }

func (s *recordingSource) OpenTableRead(_ context.Context, request dataaccess.TableIdentityRequest) (dataaccess.TableReadSpan, error) {
	return recordingSpan{source: s, request: request}, nil
}

type recordingSpan struct {
	source  *recordingSource
	request dataaccess.TableIdentityRequest
}

func (s recordingSpan) Session() dal.ReadSession { return recordingSession{source: s.source} }
func (s recordingSpan) Verify(context.Context, dataaccess.TableIdentityRequest) (dataaccess.TableIdentityFacts, error) {
	return dataaccess.TableIdentityFacts{Connection: s.request.Connection, Table: s.request.Table}, nil
}
func (recordingSpan) Close() error { return nil }

type recordingSession struct {
	dal.ReadSession
	source *recordingSource
}

func (s recordingSession) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	s.source.dispatched = true
	return nil, errors.New("recording adapter returns no rows")
}

// admitted runs the real enforcement entry point, Authority.ReadTableResult,
// behind a resolver written independently of the evaluator, mirroring the
// backend's ResolveAccessWithExactSnapshot: membership, approved grants
// unexpired at server time, then ProjectExactRead. Exact connections bind the
// sealed mandatory policy into the execution context, as hosts do.
func admitted(t *testing.T, s Snapshot, subject dataaccess.Subject, object dataaccess.ObjectRef) bool {
	t.Helper()
	conn := s.Connections[0]
	resolve := func(ctx context.Context, request dataaccess.ResolveRequest) (dataaccess.Resolution, error) {
		if !slices.Contains(s.Members, request.Subject) {
			return dataaccess.Resolution{}, errors.New("not a project member")
		}
		r := dataaccess.Resolution{Subject: request.Subject, Connection: conn.Ref, Protection: *conn.Protection,
			Grants: []dataaccess.Grant{}, ServerTime: s.AsOf, ValidUntil: s.AsOf.Add(time.Minute)}
		for _, g := range conn.Grants {
			if g.Subject == request.Subject && g.State == dataaccess.Approved && g.ExpiresAt.After(s.AsOf) {
				r.Grants = append(r.Grants, g)
			}
		}
		if conn.Protection.ReadPosture == dataaccess.ReadExact {
			return dataaccess.ProjectExactRead(ctx, r, conn.Exact)
		}
		return r, nil
	}
	authority, err := dataaccess.NewAuthority(subject, conn.Ref, resolve)
	require.NoError(t, err)
	ctx := context.Background()
	if conn.Protection.ReadPosture == dataaccess.ReadExact {
		ctx, err = conn.Exact.BindContext(ctx, subject)
		require.NoError(t, err)
	}
	source := &recordingSource{}
	request := dataaccess.ResultRequest{Nonce: "perimeter-parity-nonce", RequestSHA256: strings.Repeat("a", 64)}
	result, err := authority.ReadTableResult(ctx, request, dataaccess.TableRead{Object: object, MaxRows: 1, MaxBytes: 4096}, source)
	require.Error(t, err, "the recording adapter never completes a read")
	require.Nil(t, result)
	return source.dispatched
}

// TestViewsMatchEnforcement is contract test T-C1: for a matrix of postures,
// grant states and mandatory denies, the evaluator allows exactly what the real
// Authority admits, for every subject and table.
func TestViewsMatchEnforcement(t *testing.T) {
	active := asOf.Add(4 * time.Hour)
	invalid := grant("g-invalid", blue, active, invoices)
	invalid.Scope.Records = &dataaccess.RecordScope{Language: "tugqtree"}
	self := grant("g-self", blue, active, invoices)
	self.Decision = &dataaccess.Decision{By: blue, At: asOf}
	variants := map[string][]dataaccess.Grant{
		"no grants":               nil,
		"object grant":            {grant("g1", blue, active, invoices)},
		"connection grant":        {grant("g-conn", blue, active)},
		"two grants":              {grant("g1", blue, active, invoices), grant("g2", blue, asOf.Add(time.Hour), customers, invoices)},
		"expires exactly at AsOf": {grant("g-edge", blue, asOf, invoices)},
		"expired":                 {grant("g-old", blue, asOf.Add(-time.Second), invoices)},
		"revoked":                 {withState(grant("g-rev", blue, active, invoices), dataaccess.Revoked)},
		"refused":                 {withState(grant("g-ref", blue, active, invoices), dataaccess.Refused)},
		"requested":               {withState(grant("g-req", blue, active, invoices), dataaccess.Requested)},
		"withdrawn":               {withState(grant("g-wd", blue, active, invoices), dataaccess.Withdrawn)},
		"grant under mandatory":   {grant("g-sec", blue, active, secrets)},
		"non-member grant":        {grant("g-mal", mallory, active, invoices)},
		"unsupported grant scope": {invalid},
		// Core admission does not see who decided a grant; the backend refuses
		// self-decisions. The evaluator is stricter here, never looser (R-1).
		"self-decided grant": {self},
	}
	postures := []dataaccess.ReadPosture{dataaccess.ReadAll, dataaccess.ReadNone, dataaccess.ReadExact}
	cases, allows := 0, 0
	for name, grants := range variants {
		for _, posture := range postures {
			for _, fineGrained := range []bool{false, true} {
				s := snapshot(t, posture, grants...)
				s.Connections[0].Protection.MandatoryFineGrained = fineGrained
				for _, subject := range []dataaccess.Subject{red, green, blue, mallory} {
					for _, object := range objects {
						entry, err := Explain(context.Background(), s, subject, dataaccess.DataRead, resource(object))
						require.NoError(t, err)
						real := admitted(t, s, subject, object)
						if name == "self-decided grant" && subject == blue && object == invoices && posture != dataaccess.ReadAll && !fineGrained {
							require.True(t, real)
							require.Equal(t, Deny, entry.Decision)
							cases++
							continue
						}
						require.Equalf(t, real, entry.Decision == Allow, "%s/%s/fineGrained=%v/%s/%s: evaluator %s %v, enforcement admitted=%v",
							name, posture, fineGrained, subject.ID, object.Name, entry.Decision, entry.Blockers, real)
						cases++
						if real {
							allows++
						}
					}
				}
			}
		}
	}
	require.Equal(t, len(variants)*len(postures)*2*4*len(objects), cases)
	require.Positive(t, allows, "the matrix must exercise admitted reads")
	require.Less(t, allows, cases, "the matrix must exercise refused reads")
}
