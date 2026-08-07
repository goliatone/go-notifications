package memory

import (
	"context"
	"sort"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

type DeliveryQueryRepository struct {
	events   *EventRepository
	messages *MessageRepository
	attempts *DeliveryRepository
}

func NewDeliveryQueryRepository(events *EventRepository, messages *MessageRepository, attempts *DeliveryRepository) *DeliveryQueryRepository {
	return &DeliveryQueryRepository{events: events, messages: messages, attempts: attempts}
}

func (r *DeliveryQueryRepository) GetDelivery(_ context.Context, query store.DeliveryQuery) (store.DeliveryRecord, error) {
	records := r.records(query)
	if len(records) == 0 {
		return store.DeliveryRecord{}, store.ErrNotFound
	}
	return records[0], nil
}

func (r *DeliveryQueryRepository) ListDeliveries(_ context.Context, query store.DeliveryQuery) ([]store.DeliveryRecord, bool, error) {
	records := r.records(query)
	hasMore := query.Limit > 0 && len(records) > query.Limit
	if hasMore {
		records = records[:query.Limit]
	}
	return records, hasMore, nil
}

func (r *DeliveryQueryRepository) records(query store.DeliveryQuery) []store.DeliveryRecord {
	r.events.base.mu.RLock()
	defer r.events.base.mu.RUnlock()
	r.messages.base.mu.RLock()
	defer r.messages.base.mu.RUnlock()
	r.attempts.base.mu.RLock()
	defer r.attempts.base.mu.RUnlock()

	messagesByEvent := make(map[uuid.UUID][]domain.NotificationMessage)
	for _, message := range r.messages.base.records {
		if message.DeletedAt.IsZero() {
			messagesByEvent[message.EventID] = append(messagesByEvent[message.EventID], message)
		}
	}
	records := make([]store.DeliveryRecord, 0)
	for _, event := range r.events.base.records {
		if !event.DeletedAt.IsZero() || event.TenantID != query.TenantID {
			continue
		}
		messages := messagesByEvent[event.ID]
		if query.EventID != uuid.Nil && query.MessageID == uuid.Nil {
			record := r.recordForEvent(event, messages)
			if deliveryMatches(record, query) {
				records = append(records, record)
			}
			continue
		}
		eventRecords := r.recordsForEvent(event, messages)
		for _, record := range eventRecords {
			if deliveryMatches(record, query) {
				records = append(records, record)
			}
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return deliveryRowID(records[i]).String() > deliveryRowID(records[j]).String()
		}
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	return records
}

func (r *DeliveryQueryRepository) recordForEvent(event domain.NotificationEvent, messages []domain.NotificationMessage) store.DeliveryRecord {
	record := store.DeliveryRecord{
		EventID: event.ID, Definition: event.DefinitionCode, Status: event.Status,
		CorrelationID: event.CorrelationID, CreatedAt: event.CreatedAt, UpdatedAt: event.UpdatedAt,
	}
	messageIDs := make(map[uuid.UUID]struct{}, len(messages))
	for _, message := range messages {
		messageIDs[message.ID] = struct{}{}
		if record.UpdatedAt.Before(message.UpdatedAt) {
			record.UpdatedAt = message.UpdatedAt
		}
	}
	for _, attempt := range r.attempts.base.records {
		if !attempt.DeletedAt.IsZero() {
			continue
		}
		if _, ok := messageIDs[attempt.MessageID]; !ok {
			continue
		}
		record.AttemptCount++
		if record.UpdatedAt.Before(attempt.UpdatedAt) {
			record.UpdatedAt = attempt.UpdatedAt
		}
	}
	return record
}

func (r *DeliveryQueryRepository) recordsForEvent(event domain.NotificationEvent, messages []domain.NotificationMessage) []store.DeliveryRecord {
	if len(messages) == 0 {
		return []store.DeliveryRecord{{
			EventID: event.ID, Definition: event.DefinitionCode, Status: event.Status,
			CorrelationID: event.CorrelationID, CreatedAt: event.CreatedAt, UpdatedAt: event.UpdatedAt,
		}}
	}
	records := make([]store.DeliveryRecord, 0, len(messages))
	for _, message := range messages {
		records = append(records, r.recordForMessage(event, message))
	}
	return records
}

func (r *DeliveryQueryRepository) recordForMessage(event domain.NotificationEvent, message domain.NotificationMessage) store.DeliveryRecord {
	record := store.DeliveryRecord{
		EventID: event.ID, MessageID: message.ID, Definition: event.DefinitionCode,
		Channel: message.Channel, Status: message.Status, CorrelationID: event.CorrelationID,
		CreatedAt: message.CreatedAt, UpdatedAt: message.UpdatedAt,
	}
	latest := domain.DeliveryAttempt{}
	providers := make(map[string]struct{})
	for _, attempt := range r.attempts.base.records {
		if !attempt.DeletedAt.IsZero() || attempt.MessageID != message.ID {
			continue
		}
		record.AttemptCount++
		providers[attempt.Adapter] = struct{}{}
		if record.UpdatedAt.Before(attempt.UpdatedAt) {
			record.UpdatedAt = attempt.UpdatedAt
		}
		if latest.ID == uuid.Nil || latest.CreatedAt.Before(attempt.CreatedAt) ||
			(latest.CreatedAt.Equal(attempt.CreatedAt) && latest.ID.String() < attempt.ID.String()) {
			latest = attempt
		}
	}
	if latest.ID != uuid.Nil && len(providers) == 1 && latest.Adapter != "" && attemptAgreesWithMessageStatus(message.Status, latest.Status) {
		record.Provider = latest.Adapter
		record.ErrorCode = latest.ErrorCode
	}
	return record
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

func deliveryMatches(record store.DeliveryRecord, query store.DeliveryQuery) bool {
	return deliveryFieldsMatch(record, query) && deliveryWindowMatches(record, query)
}

func deliveryFieldsMatch(record store.DeliveryRecord, query store.DeliveryQuery) bool {
	if query.EventID != uuid.Nil && record.EventID != query.EventID {
		return false
	}
	if query.MessageID != uuid.Nil && record.MessageID != query.MessageID {
		return false
	}
	if query.DefinitionCode != "" && record.Definition != query.DefinitionCode {
		return false
	}
	if query.Channel != "" && record.Channel != query.Channel {
		return false
	}
	if query.Status != "" && strings.ToLower(record.Status) != query.Status {
		return false
	}
	if query.ErrorCode != "" && record.ErrorCode != query.ErrorCode {
		return false
	}
	return true
}

func deliveryWindowMatches(record store.DeliveryRecord, query store.DeliveryQuery) bool {
	if !query.CreatedAfter.IsZero() && record.CreatedAt.Before(query.CreatedAfter) {
		return false
	}
	if !query.CreatedBefore.IsZero() && record.CreatedAt.After(query.CreatedBefore) {
		return false
	}
	if !query.CursorTime.IsZero() {
		if record.CreatedAt.After(query.CursorTime) {
			return false
		}
		if record.CreatedAt.Equal(query.CursorTime) && deliveryRowID(record).String() >= query.CursorID.String() {
			return false
		}
	}
	return true
}

func deliveryRowID(record store.DeliveryRecord) uuid.UUID {
	if record.MessageID != uuid.Nil {
		return record.MessageID
	}
	return record.EventID
}
