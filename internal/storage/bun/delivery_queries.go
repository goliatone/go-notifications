package bunrepo

import (
	"context"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/schema"
)

type DeliveryQueryRepository struct{ db *bun.DB }

func NewDeliveryQueryRepository(db *bun.DB) *DeliveryQueryRepository {
	return &DeliveryQueryRepository{db: db}
}

func (r *DeliveryQueryRepository) GetDelivery(ctx context.Context, query store.DeliveryQuery) (store.DeliveryRecord, error) {
	if query.EventID != uuid.Nil && query.MessageID == uuid.Nil {
		return r.getEventSummary(ctx, query.TenantID, query.EventID)
	}
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

type deliveryQueryRow struct {
	EventID             uuid.UUID
	MessageID           uuid.UUID
	Definition          string
	Channel             string
	Status              string
	CorrelationID       string
	CreatedAt           time.Time
	BaseUpdatedAt       time.Time
	AttemptCount        int
	ProviderCount       int
	LatestProvider      string
	LatestAttemptStatus string
	LatestErrorCode     string
	AttemptUpdatedAt    schema.NullTime
}

func (row deliveryQueryRow) record() store.DeliveryRecord {
	record := store.DeliveryRecord{
		EventID: row.EventID, MessageID: row.MessageID, Definition: row.Definition,
		Channel: row.Channel, Status: row.Status, AttemptCount: row.AttemptCount,
		CorrelationID: row.CorrelationID, CreatedAt: row.CreatedAt, UpdatedAt: row.BaseUpdatedAt,
	}
	if !row.AttemptUpdatedAt.IsZero() && record.UpdatedAt.Before(row.AttemptUpdatedAt.Time) {
		record.UpdatedAt = row.AttemptUpdatedAt.Time
	}
	if row.ProviderCount == 1 && row.LatestProvider != "" &&
		attemptAgreesWithMessageStatus(row.Status, row.LatestAttemptStatus) {
		record.Provider = row.LatestProvider
		record.ErrorCode = row.LatestErrorCode
	}
	return record
}

func (r *DeliveryQueryRepository) list(ctx context.Context, query store.DeliveryQuery) ([]store.DeliveryRecord, bool, error) {
	var querySQL strings.Builder
	querySQL.WriteString(`SELECT
e.id AS event_id,
m.id AS message_id,
e.definition_code AS definition,
COALESCE(m.channel, '') AS channel,
COALESCE(NULLIF(m.status, ''), e.status, '') AS status,
COALESCE(e.correlation_id, '') AS correlation_id,
COALESCE(m.created_at, e.created_at) AS created_at,
COALESCE(m.updated_at, e.updated_at) AS base_updated_at,
(SELECT COUNT(*) FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL) AS attempt_count,
(SELECT COUNT(DISTINCT a.adapter) FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL) AS provider_count,
COALESCE((SELECT a.adapter FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL
  ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') AS latest_provider,
COALESCE((SELECT a.status FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL
  ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') AS latest_attempt_status,
COALESCE((SELECT a.error_code FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL
  ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') AS latest_error_code,
(SELECT MAX(a.updated_at) FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL) AS attempt_updated_at
FROM notification_events e
LEFT JOIN notification_messages m ON m.event_id = e.id AND m.deleted_at IS NULL
WHERE e.deleted_at IS NULL AND COALESCE(e.tenant_id, '') = ?`)
	args := []any{query.TenantID}
	if query.EventID != uuid.Nil {
		querySQL.WriteString(" AND e.id = ?")
		args = append(args, query.EventID)
	}
	if query.MessageID != uuid.Nil {
		querySQL.WriteString(" AND m.id = ?")
		args = append(args, query.MessageID)
	}
	if query.DefinitionCode != "" {
		querySQL.WriteString(" AND e.definition_code = ?")
		args = append(args, query.DefinitionCode)
	}
	if query.Channel != "" {
		querySQL.WriteString(" AND m.channel = ?")
		args = append(args, query.Channel)
	}
	if query.Status != "" {
		querySQL.WriteString(" AND LOWER(COALESCE(NULLIF(m.status, ''), e.status, '')) = ?")
		args = append(args, query.Status)
	}
	if query.ErrorCode != "" {
		args = appendDeliveryErrorCodeFilter(&querySQL, args, query.ErrorCode)
	}
	if !query.CreatedAfter.IsZero() {
		querySQL.WriteString(" AND COALESCE(m.created_at, e.created_at) >= ?")
		args = append(args, query.CreatedAfter)
	}
	if !query.CreatedBefore.IsZero() {
		querySQL.WriteString(" AND COALESCE(m.created_at, e.created_at) <= ?")
		args = append(args, query.CreatedBefore)
	}
	if !query.CursorTime.IsZero() {
		querySQL.WriteString(` AND (COALESCE(m.created_at, e.created_at) < ? OR
  (COALESCE(m.created_at, e.created_at) = ? AND COALESCE(m.id, e.id) < ?))`)
		args = append(args, query.CursorTime, query.CursorTime, query.CursorID)
	}
	querySQL.WriteString(" ORDER BY COALESCE(m.created_at, e.created_at) DESC, COALESCE(m.id, e.id) DESC")
	limit := query.Limit
	if limit <= 0 {
		limit = 1
	}
	querySQL.WriteString(" LIMIT ?")
	args = append(args, limit+1)

	rows := make([]deliveryQueryRow, 0, limit+1)
	if err := r.db.NewRaw(querySQL.String(), args...).Scan(ctx, &rows); err != nil {
		return nil, false, mapError(err)
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	records := make([]store.DeliveryRecord, len(rows))
	for i := range rows {
		records[i] = rows[i].record()
	}
	return records, hasMore, nil
}

func appendDeliveryErrorCodeFilter(querySQL *strings.Builder, args []any, errorCode string) []any {
	querySQL.WriteString(` AND (SELECT COUNT(DISTINCT a.adapter) FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL) = 1
AND COALESCE((SELECT a.adapter FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL
  ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') <> ''
AND COALESCE((SELECT a.error_code FROM notification_delivery_attempts a
  WHERE a.message_id = m.id AND a.deleted_at IS NULL
  ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') = ?
AND ((m.status = ? AND COALESCE((SELECT a.status FROM notification_delivery_attempts a
       WHERE a.message_id = m.id AND a.deleted_at IS NULL
       ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') = ?)
  OR (m.status = ? AND COALESCE((SELECT a.status FROM notification_delivery_attempts a
       WHERE a.message_id = m.id AND a.deleted_at IS NULL
       ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') = ?)
  OR (m.status = ? AND COALESCE((SELECT a.status FROM notification_delivery_attempts a
       WHERE a.message_id = m.id AND a.deleted_at IS NULL
       ORDER BY a.created_at DESC, a.id DESC LIMIT 1), '') = ?))`)
	return append(args,
		errorCode,
		domain.MessageStatusDelivered, domain.AttemptStatusSucceeded,
		domain.MessageStatusFailed, domain.AttemptStatusFailed,
		domain.MessageStatusPending, domain.AttemptStatusPending,
	)
}

type eventSummaryRow struct {
	EventID          uuid.UUID
	Definition       string
	Status           string
	CorrelationID    string
	CreatedAt        time.Time
	EventUpdatedAt   time.Time
	MessageUpdatedAt schema.NullTime
	AttemptUpdatedAt schema.NullTime
	AttemptCount     int
}

func (r *DeliveryQueryRepository) getEventSummary(ctx context.Context, tenantID string, eventID uuid.UUID) (store.DeliveryRecord, error) {
	row := eventSummaryRow{}
	err := r.db.NewRaw(`SELECT
e.id AS event_id,
e.definition_code AS definition,
COALESCE(e.status, '') AS status,
COALESCE(e.correlation_id, '') AS correlation_id,
e.created_at AS created_at,
e.updated_at AS event_updated_at,
MAX(m.updated_at) AS message_updated_at,
MAX(a.updated_at) AS attempt_updated_at,
COUNT(a.id) AS attempt_count
FROM notification_events e
LEFT JOIN notification_messages m ON m.event_id = e.id AND m.deleted_at IS NULL
LEFT JOIN notification_delivery_attempts a ON a.message_id = m.id AND a.deleted_at IS NULL
WHERE e.deleted_at IS NULL AND COALESCE(e.tenant_id, '') = ? AND e.id = ?
GROUP BY e.id, e.definition_code, e.status, e.correlation_id, e.created_at, e.updated_at`, tenantID, eventID).Scan(ctx, &row)
	if err != nil {
		return store.DeliveryRecord{}, mapError(err)
	}
	record := store.DeliveryRecord{
		EventID: row.EventID, Definition: row.Definition, Status: row.Status,
		AttemptCount: row.AttemptCount, CorrelationID: row.CorrelationID,
		CreatedAt: row.CreatedAt, UpdatedAt: row.EventUpdatedAt,
	}
	for _, candidate := range []schema.NullTime{row.MessageUpdatedAt, row.AttemptUpdatedAt} {
		if !candidate.IsZero() && record.UpdatedAt.Before(candidate.Time) {
			record.UpdatedAt = candidate.Time
		}
	}
	return record, nil
}

func attemptAgreesWithMessageStatus(messageStatus, attemptStatus string) bool {
	switch messageStatus {
	case domain.MessageStatusDelivered:
		return attemptStatus == domain.AttemptStatusSucceeded
	case domain.MessageStatusFailed:
		return attemptStatus == domain.AttemptStatusFailed
	case domain.MessageStatusPending:
		return attemptStatus == domain.AttemptStatusPending
	default:
		return false
	}
}
