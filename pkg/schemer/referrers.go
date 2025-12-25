package schemer

import "context"

type ReferrersProvider interface {
	GetReferrers(ctx context.Context, schema, table string) ([]ForeignKey, error)
}
