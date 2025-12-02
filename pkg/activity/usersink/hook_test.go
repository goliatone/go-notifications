package usersink

import (
	"context"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
)

type recordingSink struct {
	records []types.ActivityRecord
}

func (s *recordingSink) Log(_ context.Context, rec types.ActivityRecord) error {
	s.records = append(s.records, rec)
	return nil
}

func TestHookNotifyMapsFields(t *testing.T) {
	sink := &recordingSink{}
	hook := Hook{Sink: sink}
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	evt := activity.Event{
		Verb:           "notification.created",
		ActorID:        uuid.New().String(),
		UserID:         uuid.New().String(),
		TenantID:       uuid.New().String(),
		OrgID:          uuid.New().String(),
		ObjectType:     "notification_event",
		ObjectID:       uuid.New().String(),
		Channel:        "email",
		DefinitionCode: "welcome",
		Recipients:     []string{"user@example.com"},
		Metadata: map[string]any{
			"custom": "value",
		},
		OccurredAt: now,
	}

	hook.Notify(context.Background(), evt)

	if len(sink.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(sink.records))
	}
	rec := sink.records[0]

	if rec.Verb != evt.Verb {
		t.Fatalf("verb mismatch: %s", rec.Verb)
	}
	if rec.ObjectType != evt.ObjectType || rec.ObjectID != evt.ObjectID {
		t.Fatalf("object fields not mapped")
	}
	if rec.Channel != evt.Channel {
		t.Fatalf("channel mismatch")
	}
	if rec.Data["definition_code"] != "welcome" {
		t.Fatalf("definition_code not propagated")
	}
	if rec.Data["custom"] != "value" {
		t.Fatalf("metadata not propagated")
	}
	if rec.OccurredAt != now {
		t.Fatalf("occurred_at mismatch: %v", rec.OccurredAt)
	}
}
