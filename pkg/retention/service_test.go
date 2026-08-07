package retention

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/privacy"
)

func TestRequestValidationRejectsMissingFutureAndUnboundedInputs(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	valid := Request{
		EventsBefore: past, MessagesBefore: past, AttemptsBefore: past,
		InboxBefore: past, PublicationsBefore: past, RetryOperationsBefore: past,
		BatchSize: 1,
	}
	tests := []Request{
		{},
		func() Request { v := valid; v.BatchSize = MaxBatchSize + 1; return v }(),
		func() Request { v := valid; v.EventsBefore = time.Now().UTC().Add(time.Hour); return v }(),
	}
	for _, req := range tests {
		if err := req.Validate(); err == nil {
			t.Fatalf("invalid request passed validation: %+v", req)
		}
	}
}

func TestServiceReturnsSafeAggregateResult(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	repo := &retentionRepoStub{counts: store.RetentionCounts{Events: 2, Messages: 3}, hasMore: true}
	service, err := New(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	req := Request{
		EventsBefore: past, MessagesBefore: past, AttemptsBefore: past,
		InboxBefore: past, PublicationsBefore: past, RetryOperationsBefore: past,
		BatchSize: 10,
	}
	result, err := service.Purge(context.Background(), req)
	if err != nil || result.EventsDeleted != 2 || result.MessagesDeleted != 3 || !result.HasMore || result.BatchSize != 10 {
		t.Fatalf("purge result=%+v err=%v", result, err)
	}
	repo.err = errors.New("database included raw destination user@example.test")
	_, err = service.Purge(context.Background(), req)
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "retention_purge_failed" || errors.Unwrap(err) != nil {
		t.Fatalf("expected sanitized purge error, got %#v", err)
	}
}

func TestUnavailableServiceFailsSafely(t *testing.T) {
	service, err := New(nil)
	if err != nil {
		t.Fatalf("new optional service: %v", err)
	}
	_, err = service.Purge(context.Background(), Request{})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "retention_unavailable" {
		t.Fatalf("unavailable purge error = %#v", err)
	}
}

type retentionRepoStub struct {
	counts  store.RetentionCounts
	hasMore bool
	err     error
}

func (s *retentionRepoStub) PurgeTerminal(context.Context, store.RetentionCutoffs, int) (store.RetentionCounts, bool, error) {
	return s.counts, s.hasMore, s.err
}
