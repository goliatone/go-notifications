package commands

import (
	"context"
	"testing"
	"time"

	command "github.com/goliatone/go-command"
	"github.com/goliatone/go-notifications/pkg/definitions"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/receipts"
	"github.com/goliatone/go-notifications/pkg/retention"
	"github.com/goliatone/go-notifications/pkg/templates"
	"github.com/google/uuid"
)

func TestCommandTypesAreStableUniqueAndValidated(t *testing.T) {
	id := uuid.New()
	messages := []struct {
		valid   any
		invalid any
		wantID  string
	}{
		{
			valid: definitions.UpsertInput{Code: "welcome"}, invalid: definitions.UpsertInput{},
			wantID: "notifications.definition.upsert",
		},
		{
			valid:   TemplateUpsert{TemplateInput: templates.TemplateInput{Code: "welcome", Channel: "email", Locale: "en"}},
			invalid: TemplateUpsert{}, wantID: "notifications.template.upsert",
		},
		{
			valid: preferences.PreferenceInput{
				SubjectType: "user", SubjectID: "u1", DefinitionCode: "welcome", Channel: "email",
			},
			invalid: preferences.PreferenceInput{}, wantID: "notifications.preference.upsert",
		},
		{
			valid:   InboxMarkRead{UserID: "u1", IDs: []string{id.String()}},
			invalid: InboxMarkRead{}, wantID: "notifications.inbox.mark_read",
		},
		{
			valid:   InboxDismiss{UserID: "u1", ID: id.String()},
			invalid: InboxDismiss{}, wantID: "notifications.inbox.dismiss",
		},
		{
			valid:   InboxSnooze{UserID: "u1", ID: id.String(), Until: time.Now().Add(time.Hour)},
			invalid: InboxSnooze{}, wantID: "notifications.inbox.snooze",
		},
		{
			valid:   events.IntakeRequest{DefinitionCode: "welcome", Recipients: []string{"u1"}},
			invalid: events.IntakeRequest{}, wantID: "notifications.event.enqueue",
		},
		{
			valid:   events.RetryRequest{EventID: id, IdempotencyKey: "retry-1"},
			invalid: events.RetryRequest{}, wantID: "notifications.event.retry",
		},
		{
			valid: validRetentionRequest(), invalid: retention.Request{},
			wantID: "notifications.retention.purge",
		},
	}
	seen := make(map[string]struct{}, len(messages))
	for _, test := range messages {
		message, ok := test.valid.(command.Message)
		if !ok {
			t.Fatalf("%T does not implement command.Message", test.valid)
		}
		if got := message.Type(); got != test.wantID {
			t.Fatalf("%T type = %q, want %q", test.valid, got, test.wantID)
		}
		if _, exists := seen[test.wantID]; exists {
			t.Fatalf("duplicate command type %q", test.wantID)
		}
		seen[test.wantID] = struct{}{}
		if err := command.ValidateMessage(test.valid); err != nil {
			t.Fatalf("valid %T rejected: %v", test.valid, err)
		}
		if err := command.ValidateMessage(test.invalid); err == nil {
			t.Fatalf("invalid %T passed go-command validation", test.invalid)
		}
	}
}

func TestHandlersDelegateExactlyOnceAndResultHandlersReturnReceipts(t *testing.T) {
	ctx := context.Background()
	definitionsSpy := &definitionServiceSpy{}
	templatesSpy := &templateServiceSpy{}
	preferencesSpy := &preferenceServiceSpy{}
	inboxSpy := &inboxServiceSpy{}
	eventsSpy := &eventServiceSpy{
		enqueueReceipt: events.DispatchReceipt{EventID: uuid.New(), Status: receipts.StatusAccepted},
		retryReceipt: events.DispatchReceipt{
			EventID: uuid.New(), RetryOperationID: uuid.New(), Status: receipts.StatusProcessed,
		},
	}
	retentionSpy := &retentionServiceSpy{result: retention.Result{EventsDeleted: 2, HasMore: true}}
	catalog, catalogErr := NewCatalog(Dependencies{
		Definitions: definitionsSpy,
		Templates:   templatesSpy,
		Preferences: preferencesSpy,
		Inbox:       inboxSpy,
		Events:      eventsSpy,
		Retention:   retentionSpy,
	})
	if catalogErr != nil {
		t.Fatalf("new catalog: %v", catalogErr)
	}
	itemID := uuid.New().String()
	assertCommandSucceeds(t, "definition", catalog.CreateDefinition.Execute(
		ctx,
		CreateDefinition{Code: "welcome"},
	))
	assertCommandSucceeds(t, "template", catalog.SaveTemplate.Execute(ctx, TemplateUpsert{
		TemplateInput: templates.TemplateInput{Code: "welcome", Channel: "email", Locale: "en"},
	}))
	assertCommandSucceeds(t, "preference", catalog.UpsertPreference.Execute(ctx, preferences.PreferenceInput{
		SubjectType: "user", SubjectID: "u1", DefinitionCode: "welcome", Channel: "email",
	}))
	assertCommandSucceeds(t, "mark read", catalog.InboxMarkRead.Execute(ctx, InboxMarkRead{
		UserID: "u1", IDs: []string{itemID}, Read: true,
	}))
	assertCommandSucceeds(t, "dismiss", catalog.InboxDismiss.Execute(
		ctx,
		InboxDismiss{UserID: "u1", ID: itemID},
	))
	assertCommandSucceeds(t, "snooze", catalog.InboxSnooze.Execute(ctx, InboxSnooze{
		UserID: "u1", ID: itemID, Until: time.Now().Add(time.Hour),
	}))
	enqueueReceipt, err := catalog.EnqueueEvent.Run(ctx, events.IntakeRequest{
		DefinitionCode: "welcome", Recipients: []string{"u1"},
	})
	if err != nil || enqueueReceipt.EventID != eventsSpy.enqueueReceipt.EventID ||
		enqueueReceipt.Status != eventsSpy.enqueueReceipt.Status {
		t.Fatalf("enqueue result: receipt=%+v err=%v", enqueueReceipt, err)
	}
	retryReceipt, err := catalog.RetryEvent.Run(ctx, events.RetryRequest{
		EventID: eventsSpy.retryReceipt.EventID, IdempotencyKey: "retry-1",
	})
	if err != nil || retryReceipt.EventID != eventsSpy.retryReceipt.EventID ||
		retryReceipt.RetryOperationID != eventsSpy.retryReceipt.RetryOperationID ||
		retryReceipt.Status != eventsSpy.retryReceipt.Status {
		t.Fatalf("retry result: receipt=%+v err=%v", retryReceipt, err)
	}
	purgeResult, err := catalog.PurgeRetention.Run(ctx, validRetentionRequest())
	if err != nil || purgeResult.EventsDeleted != 2 || !purgeResult.HasMore {
		t.Fatalf("retention result: result=%+v err=%v", purgeResult, err)
	}
	for operation, calls := range map[string]int{
		"definition": definitionsSpy.calls,
		"template":   templatesSpy.calls,
		"preference": preferencesSpy.calls,
		"mark read":  inboxSpy.markReadCalls,
		"dismiss":    inboxSpy.dismissCalls,
		"snooze":     inboxSpy.snoozeCalls,
		"enqueue":    eventsSpy.enqueueCalls,
		"retry":      eventsSpy.retryCalls,
		"retention":  retentionSpy.calls,
	} {
		if calls != 1 {
			t.Fatalf("%s delegated %d times", operation, calls)
		}
	}
}

func validRetentionRequest() retention.Request {
	cutoff := time.Now().UTC().Add(-time.Hour)
	return retention.Request{
		EventsBefore: cutoff, MessagesBefore: cutoff, AttemptsBefore: cutoff,
		InboxBefore: cutoff, PublicationsBefore: cutoff, RetryOperationsBefore: cutoff,
		BatchSize: 10,
	}
}

func assertCommandSucceeds(t *testing.T, operation string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s command: %v", operation, err)
	}
}

type definitionServiceSpy struct{ calls int }

func (s *definitionServiceSpy) Upsert(context.Context, definitions.UpsertInput) (*domain.NotificationDefinition, error) {
	s.calls++
	return &domain.NotificationDefinition{}, nil
}

type templateServiceSpy struct{ calls int }

func (s *templateServiceSpy) Upsert(context.Context, templates.TemplateInput, bool) (*domain.NotificationTemplate, error) {
	s.calls++
	return &domain.NotificationTemplate{}, nil
}

type preferenceServiceSpy struct{ calls int }

func (s *preferenceServiceSpy) Upsert(context.Context, preferences.PreferenceInput) (*domain.NotificationPreference, error) {
	s.calls++
	return &domain.NotificationPreference{}, nil
}

type inboxServiceSpy struct {
	markReadCalls int
	dismissCalls  int
	snoozeCalls   int
}

func (s *inboxServiceSpy) MarkRead(context.Context, string, []string, bool) error {
	s.markReadCalls++
	return nil
}

func (s *inboxServiceSpy) Dismiss(context.Context, string, string) error {
	s.dismissCalls++
	return nil
}

func (s *inboxServiceSpy) Snooze(context.Context, string, string, int64) error {
	s.snoozeCalls++
	return nil
}

type eventServiceSpy struct {
	enqueueCalls   int
	retryCalls     int
	enqueueReceipt events.DispatchReceipt
	retryReceipt   events.DispatchReceipt
}

func (s *eventServiceSpy) EnqueueWithReceipt(context.Context, events.IntakeRequest) (events.DispatchReceipt, error) {
	s.enqueueCalls++
	return s.enqueueReceipt, nil
}

func (s *eventServiceSpy) RetryWithReceipt(context.Context, events.RetryRequest) (events.DispatchReceipt, error) {
	s.retryCalls++
	return s.retryReceipt, nil
}

type retentionServiceSpy struct {
	calls  int
	result retention.Result
}

func (s *retentionServiceSpy) Purge(context.Context, retention.Request) (retention.Result, error) {
	s.calls++
	return s.result, nil
}
