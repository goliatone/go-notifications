package bunrepo

import (
	"time"

	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	repository "github.com/goliatone/go-repository-bun"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

func withID(id uuid.UUID) repository.SelectCriteria {
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("id = ?", id)
	}
}

func withoutDeleted() repository.SelectCriteria {
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		return q.Where("deleted_at IS NULL")
	}
}

func withTimeRange(field string, since, until time.Time) repository.SelectCriteria {
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		if !since.IsZero() {
			q = q.Where("? >= ?", bun.Ident(field), since)
		}
		if !until.IsZero() {
			q = q.Where("? <= ?", bun.Ident(field), until)
		}
		return q
	}
}

func withListOptions(opts store.ListOptions) repository.SelectCriteria {
	return func(q *bun.SelectQuery) *bun.SelectQuery {
		if opts.Limit > 0 {
			q = q.Limit(opts.Limit)
		}
		if opts.Offset > 0 {
			q = q.Offset(opts.Offset)
		}
		if !opts.IncludeSoftDeleted {
			q = q.Where("deleted_at IS NULL")
		}
		if !opts.Since.IsZero() {
			q = q.Where("created_at >= ?", opts.Since)
		}
		if !opts.Until.IsZero() {
			q = q.Where("created_at <= ?", opts.Until)
		}
		return q.Order("created_at ASC")
	}
}
