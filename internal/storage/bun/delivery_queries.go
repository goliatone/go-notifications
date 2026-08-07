package bunrepo

import (
	"context"
	"strings"

	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type DeliveryQueryRepository struct{ db *bun.DB }

func NewDeliveryQueryRepository(db *bun.DB) *DeliveryQueryRepository {
	return &DeliveryQueryRepository{db: db}
}

func (r *DeliveryQueryRepository) GetDelivery(ctx context.Context, query store.DeliveryQuery) (store.DeliveryRecord, error) {
	query.Limit = 1
	records, _, err := r.list(ctx, query)
	if err != nil {
		return store.DeliveryRecord{}, err
	}
	if len(records) == 0 {
		return store.DeliveryRecord{}, store.ErrNotFound
	}
	return records[0], nil
}

func (r *DeliveryQueryRepository) ListDeliveries(ctx context.Context, query store.DeliveryQuery) ([]store.DeliveryRecord, bool, error) {
	return r.list(ctx, query)
}

func (r *DeliveryQueryRepository) list(ctx context.Context, query store.DeliveryQuery) ([]store.DeliveryRecord, bool, error) {
	var sql strings.Builder
	sql.WriteString(`SELECT
e.id AS event_id,
m.id AS message_id,
e.definition_code AS definition,
COALESCE(m.channel, '') AS channel,
COALESCE((SELECT a.adapter FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL
  ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') AS provider,
COALESCE(NULLIF(m.status, ''), e.status, '') AS status,
(SELECT COUNT(*) FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL) AS attempt_count,
COALESCE((SELECT a.error_code FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL
  ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') AS error_code,
COALESCE(e.correlation_id, '') AS correlation_id,
COALESCE(m.created_at, e.created_at) AS created_at,
COALESCE((SELECT a.updated_at FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL
  ORDER BY a.created_at DESC, a.id DESC LIMIT 1), m.updated_at, e.updated_at) AS updated_at
FROM notification_events e
LEFT JOIN notification_messages m ON m.event_id = e.id AND m.deleted_at IS NULL
WHERE e.deleted_at IS NULL AND COALESCE(e.tenant_id, '') = ?`)
	args := []any{query.TenantID}
	if query.EventID != uuid.Nil {
		sql.WriteString(" AND e.id = ?")
		args = append(args, query.EventID)
	}
	if query.MessageID != uuid.Nil {
		sql.WriteString(" AND m.id = ?")
		args = append(args, query.MessageID)
	}
	if query.DefinitionCode != "" {
		sql.WriteString(" AND e.definition_code = ?")
		args = append(args, query.DefinitionCode)
	}
	if query.Channel != "" {
		sql.WriteString(" AND m.channel = ?")
		args = append(args, query.Channel)
	}
	if query.Status != "" {
		sql.WriteString(" AND LOWER(COALESCE(NULLIF(m.status, ''), e.status, '')) = ?")
		args = append(args, query.Status)
	}
	if query.ErrorCode != "" {
		sql.WriteString(` AND COALESCE((SELECT a.error_code FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL
  ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') = ?`)
		args = append(args, query.ErrorCode)
	}
	if !query.CreatedAfter.IsZero() {
		sql.WriteString(" AND COALESCE(m.created_at, e.created_at) >= ?")
		args = append(args, query.CreatedAfter)
	}
	if !query.CreatedBefore.IsZero() {
		sql.WriteString(" AND COALESCE(m.created_at, e.created_at) <= ?")
		args = append(args, query.CreatedBefore)
	}
	if !query.CursorTime.IsZero() {
		sql.WriteString(` AND (COALESCE(m.created_at, e.created_at) < ? OR
  (COALESCE(m.created_at, e.created_at) = ? AND COALESCE(m.id, e.id) < ?))`)
		args = append(args, query.CursorTime, query.CursorTime, query.CursorID)
	}
	sql.WriteString(" ORDER BY COALESCE(m.created_at, e.created_at) DESC, COALESCE(m.id, e.id) DESC")
	limit := query.Limit
	if limit <= 0 {
		limit = 1
	}
	sql.WriteString(" LIMIT ?")
	args = append(args, limit+1)

	records := make([]store.DeliveryRecord, 0, limit+1)
	if err := r.db.NewRaw(sql.String(), args...).Scan(ctx, &records); err != nil {
		return nil, false, mapError(err)
	}
	hasMore := len(records) > limit
	if hasMore {
		records = records[:limit]
	}
	return records, hasMore, nil
}
