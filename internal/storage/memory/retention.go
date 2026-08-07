package memory

import (
	"context"
	"sort"
	"time"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/google/uuid"
)

// RetentionRepository coordinates one purge across all memory repositories.
// It acquires every participating repository lock so the pass is atomic with
// respect to normal in-memory CRUD operations.
type RetentionRepository struct {
	events       *EventRepository
	messages     *MessageRepository
	attempts     *DeliveryRepository
	inbox        *InboxRepository
	publications *PublicationRepository
	retries      *RetryOperationRepository
}

func NewRetentionRepository(
	events *EventRepository,
	messages *MessageRepository,
	attempts *DeliveryRepository,
	inbox *InboxRepository,
	publications *PublicationRepository,
	retries *RetryOperationRepository,
) *RetentionRepository {
	return &RetentionRepository{
		events: events, messages: messages, attempts: attempts,
		inbox: inbox, publications: publications, retries: retries,
	}
}

func (r *RetentionRepository) PurgeTerminal(
	_ context.Context,
	cutoffs store.RetentionCutoffs,
	batchSize int,
) (store.RetentionCounts, bool, error) {
	// Publication operations are the only existing cross-repository memory
	// transaction and acquire publication before event, so retain that order.
	r.publications.base.mu.Lock()
	defer r.publications.base.mu.Unlock()
	r.events.base.mu.Lock()
	defer r.events.base.mu.Unlock()
	r.messages.base.mu.Lock()
	defer r.messages.base.mu.Unlock()
	r.attempts.base.mu.Lock()
	defer r.attempts.base.mu.Unlock()
	r.inbox.base.mu.Lock()
	defer r.inbox.base.mu.Unlock()
	r.retries.base.mu.Lock()
	defer r.retries.base.mu.Unlock()

	remaining := batchSize
	counts := store.RetentionCounts{}

	for _, id := range r.attemptIDs(cutoffs.AttemptsBefore, remaining) {
		delete(r.attempts.base.records, id)
		counts.Attempts++
		remaining--
	}
	for _, id := range r.inboxIDs(cutoffs.InboxBefore, remaining) {
		delete(r.inbox.base.records, id)
		counts.Inbox++
		remaining--
	}
	for _, id := range r.retryIDs(cutoffs.RetryOperationsBefore, remaining) {
		operation := r.retries.base.records[id]
		delete(r.retries.identity, retryIdentity(operation.EventID, operation.RetryScope, operation.IdempotencyKey))
		delete(r.retries.base.records, id)
		counts.RetryOperations++
		remaining--
	}
	for _, id := range r.messageIDs(cutoffs.MessagesBefore, remaining) {
		delete(r.messages.base.records, id)
		counts.Messages++
		remaining--
	}
	for _, id := range r.eventIDs(cutoffs.EventsBefore, remaining) {
		event := r.events.base.records[id]
		delete(r.events.identity, eventIdentity(event.IdempotencyScope, event.DefinitionCode, event.IdempotencyKey))
		delete(r.events.base.records, id)
		counts.Events++
		remaining--
	}
	for _, id := range r.publicationIDs(cutoffs.PublicationsBefore, remaining) {
		delete(r.publications.base.records, id)
		counts.Publications++
		remaining--
	}

	hasMore := len(r.attemptIDs(cutoffs.AttemptsBefore, 1)) > 0 ||
		len(r.inboxIDs(cutoffs.InboxBefore, 1)) > 0 ||
		len(r.retryIDs(cutoffs.RetryOperationsBefore, 1)) > 0 ||
		len(r.messageIDs(cutoffs.MessagesBefore, 1)) > 0 ||
		len(r.eventIDs(cutoffs.EventsBefore, 1)) > 0 ||
		len(r.publicationIDs(cutoffs.PublicationsBefore, 1)) > 0
	return counts, hasMore, nil
}

type retentionCandidate struct {
	id uuid.UUID
	at time.Time
}

func candidateIDs(candidates []retentionCandidate, limit int) []uuid.UUID {
	if limit <= 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].at.Equal(candidates[j].at) {
			return candidates[i].id.String() < candidates[j].id.String()
		}
		return candidates[i].at.Before(candidates[j].at)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	ids := make([]uuid.UUID, len(candidates))
	for i := range candidates {
		ids[i] = candidates[i].id
	}
	return ids
}

func beforeCutoff(meta domain.RecordMeta, cutoff time.Time) bool {
	return !meta.UpdatedAt.IsZero() && meta.UpdatedAt.Before(cutoff)
}

func terminalMessage(status string) bool {
	return status == domain.MessageStatusDelivered || status == domain.MessageStatusSkipped
}

func terminalAttempt(status string) bool {
	return status == domain.AttemptStatusSucceeded || status == domain.AttemptStatusFailed
}

func (r *RetentionRepository) attemptIDs(cutoff time.Time, limit int) []uuid.UUID {
	items := make([]retentionCandidate, 0)
	for id, attempt := range r.attempts.base.records {
		message, ok := r.messages.base.records[attempt.MessageID]
		if ok && terminalAttempt(attempt.Status) && terminalMessage(message.Status) && beforeCutoff(attempt.RecordMeta, cutoff) {
			items = append(items, retentionCandidate{id: id, at: attempt.UpdatedAt})
		}
	}
	return candidateIDs(items, limit)
}

func (r *RetentionRepository) inboxIDs(cutoff time.Time, limit int) []uuid.UUID {
	items := make([]retentionCandidate, 0)
	for id, item := range r.inbox.base.records {
		if !item.DismissedAt.IsZero() && beforeCutoff(item.RecordMeta, cutoff) {
			items = append(items, retentionCandidate{id: id, at: item.UpdatedAt})
		}
	}
	return candidateIDs(items, limit)
}

func (r *RetentionRepository) retryIDs(cutoff time.Time, limit int) []uuid.UUID {
	items := make([]retentionCandidate, 0)
	for id, operation := range r.retries.base.records {
		if (operation.Status == domain.RetryStatusCompleted || operation.Status == domain.RetryStatusFailed) &&
			beforeCutoff(operation.RecordMeta, cutoff) {
			items = append(items, retentionCandidate{id: id, at: operation.UpdatedAt})
		}
	}
	return candidateIDs(items, limit)
}

func (r *RetentionRepository) messageIDs(cutoff time.Time, limit int) []uuid.UUID {
	items := make([]retentionCandidate, 0)
	for id, message := range r.messages.base.records {
		if !terminalMessage(message.Status) || !beforeCutoff(message.RecordMeta, cutoff) || r.messageReferenced(id) {
			continue
		}
		items = append(items, retentionCandidate{id: id, at: message.UpdatedAt})
	}
	return candidateIDs(items, limit)
}

func (r *RetentionRepository) messageReferenced(id uuid.UUID) bool {
	for _, attempt := range r.attempts.base.records {
		if attempt.MessageID == id {
			return true
		}
	}
	for _, item := range r.inbox.base.records {
		if item.MessageID == id {
			return true
		}
	}
	return false
}

func (r *RetentionRepository) eventIDs(cutoff time.Time, limit int) []uuid.UUID {
	items := make([]retentionCandidate, 0)
	for id, event := range r.events.base.records {
		if event.Status != domain.EventStatusProcessed || !beforeCutoff(event.RecordMeta, cutoff) || r.eventReferenced(id) {
			continue
		}
		items = append(items, retentionCandidate{id: id, at: event.UpdatedAt})
	}
	return candidateIDs(items, limit)
}

func (r *RetentionRepository) eventReferenced(id uuid.UUID) bool {
	for _, message := range r.messages.base.records {
		if message.EventID == id {
			return true
		}
	}
	for _, operation := range r.retries.base.records {
		if operation.EventID == id {
			return true
		}
	}
	return false
}

func (r *RetentionRepository) publicationIDs(cutoff time.Time, limit int) []uuid.UUID {
	items := make([]retentionCandidate, 0)
	for id, publication := range r.publications.base.records {
		if publication.Status != domain.PublicationStatusCompleted && publication.Status != domain.PublicationStatusFailed {
			continue
		}
		if !beforeCutoff(publication.RecordMeta, cutoff) || r.publicationReferenced(id) {
			continue
		}
		items = append(items, retentionCandidate{id: id, at: publication.UpdatedAt})
	}
	return candidateIDs(items, limit)
}

func (r *RetentionRepository) publicationReferenced(id uuid.UUID) bool {
	for _, event := range r.events.base.records {
		if event.PublicationID == id {
			return true
		}
	}
	return false
}
