// Package deliveries exposes privacy-safe, read-only durable delivery state.
package deliveries

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/google/uuid"
)

const (
	DefaultPageSize = 50
	MaxPageSize     = 100
)

type GetQuery struct {
	Scope     string    `json:"scope"`
	EventID   uuid.UUID `json:"event_id,omitempty"`
	MessageID uuid.UUID `json:"message_id,omitempty"`
}

type ListQuery struct {
	Scope          string    `json:"scope"`
	DefinitionCode string    `json:"definition_code,omitempty"`
	Channel        string    `json:"channel,omitempty"`
	Status         string    `json:"status,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	CreatedAfter   time.Time `json:"created_after"`
	CreatedBefore  time.Time `json:"created_before"`
	Cursor         string    `json:"cursor,omitempty"`
	Limit          int       `json:"limit,omitempty"`
}

// View is a fixed metadata-only projection. It has no recipient, content,
// template data, URL, arbitrary metadata, provider response, or raw error.
type View struct {
	EventID       uuid.UUID `json:"event_id"`
	MessageID     uuid.UUID `json:"message_id,omitempty"`
	Definition    string    `json:"definition"`
	Channel       string    `json:"channel,omitempty"`
	Provider      string    `json:"provider,omitempty"`
	Status        string    `json:"status"`
	AttemptCount  int       `json:"attempt_count,omitempty"`
	ErrorCode     string    `json:"error_code,omitempty"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Page struct {
	Items      []View `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type Service struct{ repository store.DeliveryQueryRepository }

func New(repository store.DeliveryQueryRepository) (*Service, error) {
	return &Service{repository: repository}, nil
}

func (s *Service) Get(ctx context.Context, query GetQuery) (View, error) {
	if s == nil || s.repository == nil {
		return View{}, safeError("configuration", "delivery_inspection_unavailable", "notification delivery inspection is unavailable")
	}
	if (query.EventID == uuid.Nil) == (query.MessageID == uuid.Nil) {
		return View{}, safeError("validation", "delivery_identity_invalid", "exactly one delivery identity is required")
	}
	tenantID, err := tenantIDForScope(query.Scope)
	if err != nil {
		return View{}, err
	}
	record, err := s.repository.GetDelivery(ctx, store.DeliveryQuery{
		TenantID: tenantID, EventID: query.EventID, MessageID: query.MessageID,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return View{}, safeError("not_found", "delivery_not_found", "notification delivery was not found")
		}
		return View{}, safeError("notification", "delivery_get_failed", "notification delivery lookup failed")
	}
	return viewFromRecord(record), nil
}

func (s *Service) List(ctx context.Context, query ListQuery) (Page, error) {
	if s == nil || s.repository == nil {
		return Page{}, safeError("configuration", "delivery_inspection_unavailable", "notification delivery inspection is unavailable")
	}
	limit := query.Limit
	if limit == 0 {
		limit = DefaultPageSize
	}
	if limit < 1 || limit > MaxPageSize {
		return Page{}, safeError("validation", "delivery_page_size_invalid", "delivery page size must be between 1 and 100")
	}
	if !query.CreatedAfter.IsZero() && !query.CreatedBefore.IsZero() && query.CreatedBefore.Before(query.CreatedAfter) {
		return Page{}, safeError("validation", "delivery_time_range_invalid", "delivery time range is invalid")
	}
	tenantID, err := tenantIDForScope(query.Scope)
	if err != nil {
		return Page{}, err
	}
	cursorTime, cursorID, err := decodeCursor(query.Cursor)
	if err != nil {
		return Page{}, safeError("validation", "delivery_cursor_invalid", "delivery cursor is invalid")
	}
	records, hasMore, err := s.repository.ListDeliveries(ctx, store.DeliveryQuery{
		TenantID: tenantID, DefinitionCode: strings.TrimSpace(query.DefinitionCode),
		Channel: strings.TrimSpace(query.Channel), Status: strings.ToLower(strings.TrimSpace(query.Status)),
		ErrorCode: strings.TrimSpace(query.ErrorCode), CreatedAfter: query.CreatedAfter,
		CreatedBefore: query.CreatedBefore, CursorTime: cursorTime, CursorID: cursorID, Limit: limit,
	})
	if err != nil {
		return Page{}, safeError("notification", "delivery_list_failed", "notification delivery list failed")
	}
	page := Page{Items: make([]View, len(records)), HasMore: hasMore}
	for i := range records {
		page.Items[i] = viewFromRecord(records[i])
	}
	if hasMore && len(records) > 0 {
		last := records[len(records)-1]
		page.NextCursor = encodeCursor(last.CreatedAt, rowID(last))
	}
	return page, nil
}

func tenantIDForScope(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "system" {
		return "", nil
	}
	if strings.HasPrefix(value, "tenant:") && strings.TrimSpace(strings.TrimPrefix(value, "tenant:")) != "" {
		return strings.TrimSpace(strings.TrimPrefix(value, "tenant:")), nil
	}
	return "", safeError("validation", "delivery_scope_invalid", "delivery scope must be system or tenant:<id>")
}

func encodeCursor(createdAt time.Time, id uuid.UUID) string {
	payload := createdAt.UTC().Format(time.RFC3339Nano) + "\n" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeCursor(value string) (time.Time, uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, uuid.Nil, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	parts := strings.SplitN(string(body), "\n", 2)
	if len(parts) != 2 {
		return time.Time{}, uuid.Nil, errors.New("invalid cursor")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, uuid.Nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil || id == uuid.Nil {
		return time.Time{}, uuid.Nil, errors.New("invalid cursor")
	}
	return createdAt.UTC(), id, nil
}

func rowID(record store.DeliveryRecord) uuid.UUID {
	if record.MessageID != uuid.Nil {
		return record.MessageID
	}
	return record.EventID
}

func viewFromRecord(record store.DeliveryRecord) View {
	return View{
		EventID: record.EventID, MessageID: record.MessageID, Definition: record.Definition,
		Channel: record.Channel, Provider: record.Provider, Status: record.Status,
		AttemptCount: record.AttemptCount, ErrorCode: record.ErrorCode,
		CorrelationID: record.CorrelationID, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func safeError(category, code, message string) error {
	return privacy.SafeError{Category: category, Code: code, Message: message}
}
