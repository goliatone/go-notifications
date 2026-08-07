package bunrepo

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/uptrace/bun"
)

// RetentionRepository executes each bounded purge in a single database
// transaction. The SQL is intentionally shared by SQLite and PostgreSQL.
type RetentionRepository struct{ db *bun.DB }

func NewRetentionRepository(db *bun.DB) *RetentionRepository {
	return &RetentionRepository{db: db}
}

type retentionDelete struct {
	name   string
	query  string
	args   []any
	assign func(*store.RetentionCounts, int)
}

func (r *RetentionRepository) PurgeTerminal(
	ctx context.Context,
	cutoffs store.RetentionCutoffs,
	batchSize int,
) (store.RetentionCounts, bool, error) {
	counts := store.RetentionCounts{}
	hasMore := false
	err := r.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		remaining := batchSize
		deletes := retentionDeletes(cutoffs)
		for _, deletion := range deletes {
			if remaining == 0 {
				break
			}
			result, err := tx.NewRaw(deletion.query, append(deletion.args, remaining)...).Exec(ctx)
			if err != nil {
				return fmt.Errorf("retention %s: %w", deletion.name, err)
			}
			affected, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("retention %s count: %w", deletion.name, err)
			}
			deletion.assign(&counts, int(affected))
			remaining -= int(affected)
		}
		for _, check := range retentionChecks(cutoffs) {
			var exists bool
			if err := tx.NewRaw(check.query, check.args...).Scan(ctx, &exists); err != nil {
				return fmt.Errorf("retention %s continuation: %w", check.name, err)
			}
			if exists {
				hasMore = true
				break
			}
		}
		return nil
	})
	return counts, hasMore, err
}

func retentionDeletes(c store.RetentionCutoffs) []retentionDelete {
	return []retentionDelete{
		{
			name: "attempts",
			query: `DELETE FROM notification_delivery_attempts WHERE id IN (
SELECT a.id FROM notification_delivery_attempts a
JOIN notification_messages m ON m.id = a.message_id
WHERE a.updated_at < ? AND m.status IN (?, ?)
ORDER BY a.updated_at ASC, a.id ASC LIMIT ?)`,
			args:   []any{c.AttemptsBefore, domain.MessageStatusDelivered, domain.MessageStatusSkipped},
			assign: func(v *store.RetentionCounts, n int) { v.Attempts = n },
		},
		{
			name: "inbox",
			query: `DELETE FROM notification_inbox_items WHERE id IN (
SELECT i.id FROM notification_inbox_items i
WHERE i.updated_at < ? AND i.dismissed_at IS NOT NULL
ORDER BY i.updated_at ASC, i.id ASC LIMIT ?)`,
			args:   []any{c.InboxBefore},
			assign: func(v *store.RetentionCounts, n int) { v.Inbox = n },
		},
		{
			name: "retry operations",
			query: `DELETE FROM notification_retry_operations WHERE id IN (
SELECT r.id FROM notification_retry_operations r
WHERE r.updated_at < ? AND r.status IN (?, ?)
ORDER BY r.updated_at ASC, r.id ASC LIMIT ?)`,
			args:   []any{c.RetryOperationsBefore, domain.RetryStatusCompleted, domain.RetryStatusFailed},
			assign: func(v *store.RetentionCounts, n int) { v.RetryOperations = n },
		},
		{
			name: "messages",
			query: `DELETE FROM notification_messages WHERE id IN (
SELECT m.id FROM notification_messages m
WHERE m.updated_at < ? AND m.status IN (?, ?)
AND NOT EXISTS (SELECT 1 FROM notification_delivery_attempts a WHERE a.message_id = m.id)
AND NOT EXISTS (SELECT 1 FROM notification_inbox_items i WHERE i.message_id = m.id)
ORDER BY m.updated_at ASC, m.id ASC LIMIT ?)`,
			args:   []any{c.MessagesBefore, domain.MessageStatusDelivered, domain.MessageStatusSkipped},
			assign: func(v *store.RetentionCounts, n int) { v.Messages = n },
		},
		{
			name: "events",
			query: `DELETE FROM notification_events WHERE id IN (
SELECT e.id FROM notification_events e
WHERE e.updated_at < ? AND e.status = ?
AND NOT EXISTS (SELECT 1 FROM notification_messages m WHERE m.event_id = e.id)
AND NOT EXISTS (SELECT 1 FROM notification_retry_operations r WHERE r.event_id = e.id)
ORDER BY e.updated_at ASC, e.id ASC LIMIT ?)`,
			args:   []any{c.EventsBefore, domain.EventStatusProcessed},
			assign: func(v *store.RetentionCounts, n int) { v.Events = n },
		},
		{
			name: "publications",
			query: `DELETE FROM notification_publications WHERE id IN (
SELECT p.id FROM notification_publications p
WHERE p.updated_at < ? AND p.status IN (?, ?)
AND NOT EXISTS (SELECT 1 FROM notification_events e WHERE e.publication_id = p.id)
ORDER BY p.updated_at ASC, p.id ASC LIMIT ?)`,
			args:   []any{c.PublicationsBefore, domain.PublicationStatusCompleted, domain.PublicationStatusFailed},
			assign: func(v *store.RetentionCounts, n int) { v.Publications = n },
		},
	}
}

type retentionCheck struct {
	name  string
	query string
	args  []any
}

func retentionChecks(c store.RetentionCutoffs) []retentionCheck {
	deletes := retentionDeletes(c)
	checks := make([]retentionCheck, 0, len(deletes))
	for _, deletion := range deletes {
		start := strings.Index(deletion.query, "SELECT ")
		limit := strings.LastIndex(deletion.query, " LIMIT ?)")
		if start < 0 || limit < start {
			continue
		}
		checks = append(checks, retentionCheck{
			name:  deletion.name,
			query: "SELECT EXISTS (" + deletion.query[start:limit] + " LIMIT 1)",
			args:  deletion.args,
		})
	}
	return checks
}
