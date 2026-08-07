// Package retention provides bounded deletion of terminal notification data.
package retention

import (
	"context"
	"errors"
	"time"

	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/privacy"
)

const MaxBatchSize = 1000

// Request defines explicit host-owned cutoffs for every retained record type.
type Request struct {
	EventsBefore          time.Time `json:"events_before"`
	MessagesBefore        time.Time `json:"messages_before"`
	AttemptsBefore        time.Time `json:"attempts_before"`
	InboxBefore           time.Time `json:"inbox_before"`
	PublicationsBefore    time.Time `json:"publications_before"`
	RetryOperationsBefore time.Time `json:"retry_operations_before"`
	BatchSize             int       `json:"batch_size"`
}

func (Request) Type() string { return "notifications.retention.purge" }

func (r Request) Validate() error {
	if r.BatchSize <= 0 || r.BatchSize > MaxBatchSize {
		return errors.New("retention: batch size must be between 1 and 1000")
	}
	now := time.Now().UTC()
	for _, cutoff := range []time.Time{
		r.EventsBefore, r.MessagesBefore, r.AttemptsBefore, r.InboxBefore,
		r.PublicationsBefore, r.RetryOperationsBefore,
	} {
		if cutoff.IsZero() {
			return errors.New("retention: every cutoff is required")
		}
		if cutoff.After(now) {
			return errors.New("retention: cutoffs cannot be in the future")
		}
	}
	return nil
}

// Result contains only safe aggregate purge metadata.
type Result struct {
	EventsDeleted          int       `json:"events_deleted"`
	MessagesDeleted        int       `json:"messages_deleted"`
	AttemptsDeleted        int       `json:"attempts_deleted"`
	InboxDeleted           int       `json:"inbox_deleted"`
	PublicationsDeleted    int       `json:"publications_deleted"`
	RetryOperationsDeleted int       `json:"retry_operations_deleted"`
	EventsBefore           time.Time `json:"events_before"`
	MessagesBefore         time.Time `json:"messages_before"`
	AttemptsBefore         time.Time `json:"attempts_before"`
	InboxBefore            time.Time `json:"inbox_before"`
	PublicationsBefore     time.Time `json:"publications_before"`
	RetryOperationsBefore  time.Time `json:"retry_operations_before"`
	BatchSize              int       `json:"batch_size"`
	HasMore                bool      `json:"has_more"`
}

type Service struct{ repository store.RetentionRepository }

func New(repository store.RetentionRepository) (*Service, error) {
	return &Service{repository: repository}, nil
}

// Purge applies one bounded, transactional deletion pass.
func (s *Service) Purge(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.repository == nil {
		return Result{}, privacy.SafeError{Category: "configuration", Code: "retention_unavailable", Message: "notification retention is unavailable"}
	}
	if err := req.Validate(); err != nil {
		return Result{}, privacy.SafeError{Category: "validation", Code: "retention_request_invalid", Message: err.Error()}
	}
	counts, hasMore, err := s.repository.PurgeTerminal(ctx, store.RetentionCutoffs{
		EventsBefore: req.EventsBefore, MessagesBefore: req.MessagesBefore,
		AttemptsBefore: req.AttemptsBefore, InboxBefore: req.InboxBefore,
		PublicationsBefore: req.PublicationsBefore, RetryOperationsBefore: req.RetryOperationsBefore,
	}, req.BatchSize)
	if err != nil {
		return Result{}, privacy.SafeError{Category: "notification", Code: "retention_purge_failed", Message: "notification retention purge failed"}
	}
	return Result{
		EventsDeleted: counts.Events, MessagesDeleted: counts.Messages,
		AttemptsDeleted: counts.Attempts, InboxDeleted: counts.Inbox,
		PublicationsDeleted: counts.Publications, RetryOperationsDeleted: counts.RetryOperations,
		EventsBefore: req.EventsBefore, MessagesBefore: req.MessagesBefore,
		AttemptsBefore: req.AttemptsBefore, InboxBefore: req.InboxBefore,
		PublicationsBefore: req.PublicationsBefore, RetryOperationsBefore: req.RetryOperationsBefore,
		BatchSize: req.BatchSize, HasMore: hasMore,
	}, nil
}
