package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/internal/dispatcher"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/queue"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/receipts"
	"github.com/google/uuid"
)

func TestScopedIdempotencyReturnsReplayWithoutRedispatch(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)
	req := IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"user@example.com"},
		IdempotencyScope: "tenant:one", IdempotencyKey: "  Key-1  ",
	}
	first, err := service.EnqueueWithReceipt(ctx, req)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	second, err := service.EnqueueWithReceipt(ctx, req)
	if err != nil {
		t.Fatalf("replay enqueue: %v", err)
	}
	if first.EventID != second.EventID || !second.Replay || len(disp.events) != 1 {
		t.Fatalf("unexpected replay: first=%+v second=%+v dispatches=%d", first, second, len(disp.events))
	}
}

func TestConcurrentScopedIdempotencyCreatesOneEvent(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)
	req := IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
		IdempotencyScope: "system", IdempotencyKey: "same",
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, enqueueErr := service.EnqueueWithReceipt(ctx, req); enqueueErr != nil {
				errCh <- enqueueErr
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for enqueueErr := range errCh {
		t.Errorf("enqueue concurrently: %v", enqueueErr)
	}
	result, err := evtRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected one event, got %d", result.Total)
	}
	if len(disp.events) != 1 {
		t.Fatalf("expected one initial fan-out, got %d", len(disp.events))
	}
}

func TestConcurrentDigestIntakeCreatesOneDurableWindow(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, _ := setupDeps(t)
	publications := memory.NewPublicationRepository()
	q := &lockedQueue{}
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: publications,
		Dispatcher: disp, Queue: q, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for idx := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, enqueueErr := service.EnqueueWithReceipt(ctx, IntakeRequest{
				DefinitionCode: "welcome",
				Recipients:     []string{fmt.Sprintf("subject-%d", idx)},
				Digest:         &DigestOptions{Key: "daily", Delay: time.Hour},
			}); enqueueErr != nil {
				errCh <- enqueueErr
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for enqueueErr := range errCh {
		t.Errorf("enqueue digest member: %v", enqueueErr)
	}
	publicationResult, err := publications.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list publications: %v", err)
	}
	if publicationResult.Total != 1 || q.count() != 1 {
		t.Fatalf("expected one publication/job, got publications=%d jobs=%d", publicationResult.Total, q.count())
	}
	events, err := evtRepo.ListByPublication(ctx, publicationResult.Items[0].ID)
	if err != nil || len(events) != 20 {
		t.Fatalf("expected 20 durable members, got %d (err=%v)", len(events), err)
	}
}

func TestIdempotencyFingerprintConflict(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)
	req := IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
		IdempotencyScope: "system", IdempotencyKey: "same",
	}
	if _, err := service.EnqueueWithReceipt(ctx, req); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	req.Recipients = []string{"subject-2"}
	_, err := service.EnqueueWithReceipt(ctx, req)
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "idempotency_conflict" {
		t.Fatalf("expected safe idempotency conflict, got %v", err)
	}
}

func TestNormalizeIdempotencyContract(t *testing.T) {
	tests := []struct {
		name        string
		scope       string
		tenant      string
		key         string
		wantScope   string
		wantKey     string
		wantErrCode string
	}{
		{name: "empty key remains non-idempotent", scope: "ignored", key: "  "},
		{name: "explicit scope and key trim", scope: " Tenant:One ", key: " Key-A ", wantScope: "Tenant:One", wantKey: "Key-A"},
		{name: "tenant derived", tenant: "tenant-1", key: "Key-A", wantScope: "tenant:tenant-1", wantKey: "Key-A"},
		{name: "system derived", key: "Key-A", wantScope: "system", wantKey: "Key-A"},
		{name: "key is case sensitive", key: "KEY-a", wantScope: "system", wantKey: "KEY-a"},
		{name: "key length", key: strings.Repeat("k", maxIdempotencyKeyLength+1), wantErrCode: "idempotency_key_too_long"},
		{name: "scope length", scope: strings.Repeat("s", maxIdempotencyKeyLength+1), key: "key", wantErrCode: "idempotency_scope_too_long"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scope, key, err := normalizeIdempotency(tc.scope, tc.tenant, tc.key)
			if tc.wantErrCode != "" {
				var safe privacy.SafeError
				if !errors.As(err, &safe) || safe.Code != tc.wantErrCode {
					t.Fatalf("expected %s, got %v", tc.wantErrCode, err)
				}
				return
			}
			if err != nil || scope != tc.wantScope || key != tc.wantKey {
				t.Fatalf("normalize = (%q, %q, %v), want (%q, %q, nil)", scope, key, err, tc.wantScope, tc.wantKey)
			}
		})
	}
}

func TestSoftDeletedEventReservesIdempotencyIdentity(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewEventRepository()
	original := &domain.NotificationEvent{
		DefinitionCode:     "welcome",
		IdempotencyScope:   "system",
		IdempotencyKey:     "reserved",
		RequestFingerprint: "fingerprint",
	}
	stored, created, err := repo.CreateIdempotent(ctx, original)
	if err != nil || !created {
		t.Fatalf("create original: created=%v err=%v", created, err)
	}
	if deleteErr := repo.SoftDelete(ctx, stored.ID); deleteErr != nil {
		t.Fatalf("soft delete: %v", deleteErr)
	}
	replayed, created, err := repo.CreateIdempotent(ctx, &domain.NotificationEvent{
		DefinitionCode:     "welcome",
		IdempotencyScope:   "system",
		IdempotencyKey:     "reserved",
		RequestFingerprint: "fingerprint",
	})
	if err != nil || created || replayed.ID != stored.ID {
		t.Fatalf("soft-deleted identity was reused: replayed=%+v created=%v err=%v", replayed, created, err)
	}
}

func TestScheduledIntakeRejectsNopQueue(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, _ := setupDeps(t)
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: memory.NewPublicationRepository(),
		Dispatcher: disp, Queue: &queue.Nop{}, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.EnqueueWithReceipt(ctx, IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
		ScheduleAt: time.Now().Add(time.Hour),
	})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "durable_queue_required" {
		t.Fatalf("expected durable queue error, got %v", err)
	}
}

func TestImmediateTransientSchemaAndCollision(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	if err := defRepo.Create(ctx, &domain.NotificationDefinition{
		Code:     "secure",
		Channels: domain.StringList{"email"},
		Policy: domain.JSONMap{
			"transient_required_keys": []any{"credential_url"},
			"transient_allowed_keys":  []any{"credential_url"},
		},
	}); err != nil {
		t.Fatalf("create secure definition: %v", err)
	}
	service := newTestService(t, defRepo, evtRepo, disp, q)

	_, err := service.EnqueueWithReceipt(ctx, IntakeRequest{
		DefinitionCode: "secure",
		Recipients:     []string{"subject-1"},
	})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "transient_key_required" {
		t.Fatalf("expected required transient error, got %v", err)
	}
	eventsBefore, listErr := evtRepo.List(ctx, store.ListOptions{})
	if listErr != nil || eventsBefore.Total != 0 {
		t.Fatalf("rejected persistent intake created an event: %+v err=%v", eventsBefore, listErr)
	}

	_, err = service.DispatchImmediate(ctx, ImmediateRequest{
		DefinitionCode: "secure",
		Recipients:     []string{"subject-1"},
		Transient:      map[string]any{"unknown": "marker"},
	})
	if !errors.As(err, &safe) || safe.Code != "transient_key_required" {
		t.Fatalf("expected missing required key before unknown-key check, got %v", err)
	}

	marker := "https://credential.invalid/one-time"
	receipt, err := service.DispatchImmediate(ctx, ImmediateRequest{
		DefinitionCode: "secure",
		Recipients:     []string{"subject-1"},
		Context:        map[string]any{"credential_url": "stale"},
		Transient:      map[string]any{"credential_url": marker},
	})
	if err != nil || receipt.Status != receipts.StatusProcessed {
		t.Fatalf("dispatch immediate: receipt=%+v err=%v", receipt, err)
	}
	if len(disp.options) != 1 || disp.options[0].Transient["credential_url"] != marker {
		t.Fatalf("transient value did not reach dispatcher: %+v", disp.options)
	}
	event, err := evtRepo.GetByID(ctx, receipt.EventID)
	if err != nil || !event.TransientDependent || event.Context["credential_url"] != "stale" {
		t.Fatalf("unexpected persisted event projection: event=%+v err=%v", event, err)
	}
}

func TestPublicationRecoveryUsesStableQueueKey(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, _ := setupDeps(t)
	publications := memory.NewPublicationRepository()
	failing := &failingQueue{err: errors.New("queue unavailable")}
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: publications,
		Dispatcher: disp, Queue: failing, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.EnqueueWithReceipt(ctx, IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
		ScheduleAt: time.Now().Add(time.Hour),
	})
	if err == nil {
		t.Fatalf("expected enqueue failure")
	}
	success := &stubQueue{}
	service.queue = success
	if err := service.RecoverPending(ctx, 10); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if len(failing.jobs) != 1 || len(success.jobs) != 1 || failing.jobs[0].Key != success.jobs[0].Key {
		t.Fatalf("expected stable recovery key: failed=%v recovered=%v", failing.jobs, success.jobs)
	}
}

func TestFailedPublicationReplayDoesNotRedispatch(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	publications := memory.NewPublicationRepository()
	publication := &domain.NotificationPublication{
		Kind: "scheduled", QueueKey: "notification-publication:" + uuid.NewString(),
		Status: domain.PublicationStatusFailed, ErrorCode: "notification_delivery_failed",
	}
	if err := publications.Create(ctx, publication); err != nil {
		t.Fatalf("create publication: %v", err)
	}
	event := &domain.NotificationEvent{
		DefinitionCode: "welcome", Recipients: domain.StringList{"subject-1"},
		PublicationID: publication.ID, Status: domain.EventStatusFailed,
	}
	if err := evtRepo.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: publications,
		Dispatcher: disp, Queue: q, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	receipt, err := service.ProcessPublication(ctx, PublicationJobPayload{PublicationID: publication.ID})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || receipt.Status != receipts.StatusFailed || !receipt.Replay {
		t.Fatalf("expected failed replay, receipt=%+v err=%v", receipt, err)
	}
	if len(disp.events) != 0 {
		t.Fatalf("failed publication redispatched %d events", len(disp.events))
	}
}

func TestExpiredPublicationClaimRecoversOnce(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	publications := memory.NewPublicationRepository()
	publication := &domain.NotificationPublication{
		Kind: "scheduled", QueueKey: "notification-publication:" + uuid.NewString(),
		Status: domain.PublicationStatusProcessing, ClaimUntil: time.Now().Add(-time.Minute),
	}
	if err := publications.Create(ctx, publication); err != nil {
		t.Fatalf("create publication: %v", err)
	}
	event := &domain.NotificationEvent{
		DefinitionCode: "welcome", Recipients: domain.StringList{"subject-1"},
		PublicationID: publication.ID, Status: domain.EventStatusScheduled,
	}
	if err := evtRepo.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: publications,
		Dispatcher: disp, Queue: q, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	first, err := service.ProcessPublication(ctx, PublicationJobPayload{PublicationID: publication.ID})
	if err != nil || first.Status != receipts.StatusProcessed {
		t.Fatalf("recover expired claim: receipt=%+v err=%v", first, err)
	}
	second, err := service.ProcessPublication(ctx, PublicationJobPayload{PublicationID: publication.ID})
	if err != nil || !second.Replay || len(disp.events) != 1 {
		t.Fatalf("duplicate publication was not idempotent: receipt=%+v dispatches=%d err=%v", second, len(disp.events), err)
	}
}

func TestRetryIsDurableAndIdempotent(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	event := &domain.NotificationEvent{
		DefinitionCode: "welcome", Recipients: domain.StringList{"subject-1"},
		Status: domain.EventStatusFailed,
	}
	if err := evtRepo.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: memory.NewPublicationRepository(),
		RetryOperations: memory.NewRetryOperationRepository(), Dispatcher: disp,
		Queue: q, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	req := RetryRequest{EventID: event.ID, IdempotencyKey: "retry-1"}
	first, err := service.RetryWithReceipt(ctx, req)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	second, err := service.RetryWithReceipt(ctx, req)
	if err != nil {
		t.Fatalf("retry replay: %v", err)
	}
	if first.RetryOperationID != second.RetryOperationID || !second.Replay || len(disp.events) != 1 {
		t.Fatalf("unexpected retry receipts: first=%+v second=%+v dispatches=%d", first, second, len(disp.events))
	}
}

func TestRetryRecoversExpiredOperationAndEventLeases(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	event := &domain.NotificationEvent{
		DefinitionCode: "welcome", Recipients: domain.StringList{"subject-1"},
		Status: domain.EventStatusRetrying, RetryClaimUntil: time.Now().Add(-time.Minute),
	}
	if err := evtRepo.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	operations := memory.NewRetryOperationRepository()
	seed := &domain.NotificationRetryOperation{
		EventID: event.ID, RetryScope: "system", IdempotencyKey: "recover",
		CorrelationID: "retry-correlation", RequestID: "retry-request",
		Status: domain.RetryStatusProcessing, ClaimUntil: time.Now().Add(-time.Minute),
	}
	if _, _, err := operations.CreateIdempotent(ctx, seed); err != nil {
		t.Fatalf("seed retry operation: %v", err)
	}
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: memory.NewPublicationRepository(),
		RetryOperations: operations, Dispatcher: disp, Queue: q, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	receipt, err := service.RetryWithReceipt(ctx, RetryRequest{
		EventID: event.ID, IdempotencyKey: "recover",
		CorrelationID: "ignored-on-replay", RequestID: "ignored-on-replay",
	})
	if err != nil {
		t.Fatalf("recover retry: %v", err)
	}
	if !receipt.Replay || receipt.RetryOperationID != seed.ID || len(disp.events) != 1 {
		t.Fatalf("expired retry did not recover once: receipt=%+v dispatches=%d", receipt, len(disp.events))
	}
	if receipt.CorrelationID != seed.CorrelationID || receipt.RequestID != seed.RequestID {
		t.Fatalf("retry audit metadata was not reconstructed: %+v", receipt)
	}
	stored, err := operations.GetByID(ctx, seed.ID)
	if err != nil || stored.Status != domain.RetryStatusCompleted || !stored.ClaimUntil.IsZero() {
		t.Fatalf("retry operation not completed: operation=%+v err=%v", stored, err)
	}
}

func TestConcurrentRetryKeysSerializeOneDelivery(t *testing.T) {
	ctx := context.Background()
	defRepo := memory.NewDefinitionRepository()
	if err := defRepo.Create(ctx, &domain.NotificationDefinition{Code: "welcome"}); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	evtRepo := memory.NewEventRepository()
	event := &domain.NotificationEvent{
		DefinitionCode: "welcome", Recipients: domain.StringList{"subject-1"},
		Status: domain.EventStatusFailed,
	}
	if err := evtRepo.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	disp := newBlockingDispatcher()
	service, err := NewService(Dependencies{
		Definitions: defRepo, Events: evtRepo, Publications: memory.NewPublicationRepository(),
		RetryOperations: memory.NewRetryOperationRepository(), Dispatcher: disp,
		Queue: &stubQueue{}, Logger: &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	firstDone := make(chan error, 1)
	go func() {
		_, retryErr := service.RetryWithReceipt(ctx, RetryRequest{EventID: event.ID, IdempotencyKey: "retry-a"})
		firstDone <- retryErr
	}()
	<-disp.started

	second, secondErr := service.RetryWithReceipt(ctx, RetryRequest{EventID: event.ID, IdempotencyKey: "retry-b"})
	if secondErr != nil {
		t.Fatalf("concurrent retry should return current state: %v", secondErr)
	}
	if second.Status != receipts.StatusRetrying || !second.Replay {
		t.Fatalf("expected current retrying receipt, got %+v", second)
	}
	close(disp.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first retry: %v", err)
	}
	if disp.count() != 1 {
		t.Fatalf("different retry keys dispatched %d times", disp.count())
	}
}

func TestRetryRejectsSuccessfulAndTransientRequiredEvents(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name       string
		status     string
		policy     domain.JSONMap
		wantCode   string
		wantEvents int
	}{
		{name: "successful", status: domain.EventStatusProcessed, wantCode: "retry_not_eligible"},
		{
			name: "definition requires transient", status: domain.EventStatusFailed,
			policy: domain.JSONMap{"transient_required_keys": []any{"credential_url"}},
			wantCode: "transient_retry_forbidden",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			defRepo := memory.NewDefinitionRepository()
			if err := defRepo.Create(ctx, &domain.NotificationDefinition{Code: "welcome", Policy: test.policy}); err != nil {
				t.Fatalf("seed definition: %v", err)
			}
			evtRepo := memory.NewEventRepository()
			event := &domain.NotificationEvent{
				DefinitionCode: "welcome", Recipients: domain.StringList{"subject-1"}, Status: test.status,
			}
			if err := evtRepo.Create(ctx, event); err != nil {
				t.Fatalf("create event: %v", err)
			}
			disp := &stubDispatcher{}
			service, err := NewService(Dependencies{
				Definitions: defRepo, Events: evtRepo, Publications: memory.NewPublicationRepository(),
				RetryOperations: memory.NewRetryOperationRepository(), Dispatcher: disp,
				Queue: &stubQueue{}, Logger: &logger.Nop{},
			})
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			_, err = service.RetryWithReceipt(ctx, RetryRequest{EventID: event.ID, IdempotencyKey: "retry"})
			var safe privacy.SafeError
			if !errors.As(err, &safe) || safe.Code != test.wantCode {
				t.Fatalf("expected safe code %q, got %#v", test.wantCode, err)
			}
			if len(disp.events) != test.wantEvents {
				t.Fatalf("rejected retry dispatched %d events", len(disp.events))
			}
		})
	}
}

func TestRetryScopeValidation(t *testing.T) {
	_, err := validateRetryScope(strings.Repeat("x", maxIdempotencyKeyLength+1), "")
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "retry_scope_too_long" {
		t.Fatalf("expected retry_scope_too_long, got %#v", err)
	}
}

func TestEnqueueImmediateDispatch(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)

	request := IntakeRequest{
		DefinitionCode: "welcome",
		Recipients:     []string{"user@example.com"},
		Context:        map[string]any{"name": "Rosa"},
	}
	if err := service.Enqueue(ctx, request); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(disp.events) != 1 {
		t.Fatalf("expected dispatcher call, got %d", len(disp.events))
	}
	events, err := evtRepo.List(ctx, store.ListOptions{})
	if err != nil || len(events.Items) != 1 || events.Items[0].Status != domain.EventStatusProcessed {
		t.Fatalf("expected durable processed state, events=%+v err=%v", events, err)
	}
}

func TestEnqueueSchedulesFutureJob(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)

	schedule := time.Now().Add(10 * time.Minute)
	err := service.Enqueue(ctx, IntakeRequest{
		DefinitionCode: "welcome",
		Recipients:     []string{"user@example.com"},
		ScheduleAt:     schedule,
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("expected job scheduled, got %d", len(q.jobs))
	}
}

func TestDigestProcessingMergesEntries(t *testing.T) {
	ctx := context.Background()
	defRepo, evtRepo, disp, q := setupDeps(t)
	service := newTestService(t, defRepo, evtRepo, disp, q)

	req := IntakeRequest{
		DefinitionCode: "welcome",
		Recipients:     []string{"user1"},
		Context:        map[string]any{"id": 1},
		Digest: &DigestOptions{
			Key:   "daily",
			Delay: time.Minute,
		},
	}
	if err := service.Enqueue(ctx, req); err != nil {
		t.Fatalf("enqueue digest: %v", err)
	}
	req2 := req
	req2.Recipients = []string{"user2"}
	req2.Context = map[string]any{"id": 2}
	if err := service.Enqueue(ctx, req2); err != nil {
		t.Fatalf("enqueue digest second: %v", err)
	}
	if len(q.jobs) != 1 {
		t.Fatalf("expected single digest job, got %d", len(q.jobs))
	}
	job := q.jobs[0]
	payload, ok := job.Payload.(PublicationJobPayload)
	if !ok {
		t.Fatalf("expected publication job payload, got %T", job.Payload)
	}
	if _, err := service.ProcessPublication(ctx, payload); err != nil {
		t.Fatalf("process digest: %v", err)
	}
	if len(disp.events) != 1 {
		t.Fatalf("expected single dispatch after digest, got %d", len(disp.events))
	}
	event := disp.events[0]
	if len(event.Recipients) != 2 {
		t.Fatalf("expected merged recipients, got %v", event.Recipients)
	}
}

func setupDeps(t *testing.T) (*memory.DefinitionRepository, *memory.EventRepository, *stubDispatcher, *stubQueue) {
	t.Helper()
	defRepo := memory.NewDefinitionRepository()
	if err := defRepo.Create(context.Background(), &domain.NotificationDefinition{
		Code: "welcome",
	}); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	evtRepo := memory.NewEventRepository()
	disp := &stubDispatcher{}
	q := &stubQueue{}
	return defRepo, evtRepo, disp, q
}

func newTestService(t *testing.T, defRepo *memory.DefinitionRepository, evtRepo *memory.EventRepository, disp *stubDispatcher, q *stubQueue) *Service {
	t.Helper()
	service, err := NewService(Dependencies{
		Definitions:  defRepo,
		Events:       evtRepo,
		Publications: memory.NewPublicationRepository(),
		Dispatcher:   disp,
		Queue:        q,
		Logger:       &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

type stubDispatcher struct {
	events  []*domain.NotificationEvent
	options []dispatcher.DispatchOptions
}

func (s *stubDispatcher) Dispatch(ctx context.Context, event *domain.NotificationEvent, opts dispatcher.DispatchOptions) error {
	s.events = append(s.events, event)
	s.options = append(s.options, opts)
	return nil
}

type blockingDispatcher struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingDispatcher() *blockingDispatcher {
	return &blockingDispatcher{started: make(chan struct{}), release: make(chan struct{})}
}

func (d *blockingDispatcher) Dispatch(context.Context, *domain.NotificationEvent, dispatcher.DispatchOptions) error {
	d.mu.Lock()
	d.calls++
	d.mu.Unlock()
	d.once.Do(func() { close(d.started) })
	<-d.release
	return nil
}

func (d *blockingDispatcher) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

type stubQueue struct {
	jobs []queue.Job
}

func (s *stubQueue) Enqueue(ctx context.Context, job queue.Job) error {
	s.jobs = append(s.jobs, job)
	return nil
}

type failingQueue struct {
	jobs []queue.Job
	err  error
}

func (q *failingQueue) Enqueue(_ context.Context, job queue.Job) error {
	q.jobs = append(q.jobs, job)
	return q.err
}

type lockedQueue struct {
	mu   sync.Mutex
	jobs []queue.Job
}

func (q *lockedQueue) Enqueue(_ context.Context, job queue.Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, job)
	return nil
}

func (q *lockedQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs)
}
