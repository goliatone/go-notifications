package usersink

import (
	"context"
	"time"

	"github.com/goliatone/go-notifications/pkg/activity"
	"github.com/goliatone/go-users/pkg/types"
	"github.com/google/uuid"
)

// Hook adapts activity events into go-users ActivitySink records.
type Hook struct {
	Sink types.ActivitySink
}

// Notify maps the activity event into a types.ActivityRecord and forwards it.
func (h Hook) Notify(ctx context.Context, evt activity.Event) {
	if h.Sink == nil {
		return
	}
	record := types.ActivityRecord{
		ID:         uuid.New(),
		UserID:     parseUUID(evt.UserID),
		ActorID:    parseUUID(evt.ActorID),
		Verb:       evt.Verb,
		ObjectType: evt.ObjectType,
		ObjectID:   evt.ObjectID,
		Channel:    evt.Channel,
		TenantID:   parseUUID(evt.TenantID),
		OrgID:      parseUUID(evt.OrgID),
		Data:       buildData(evt),
		OccurredAt: evt.OccurredAt,
	}
	if record.OccurredAt.IsZero() {
		record.OccurredAt = time.Now().UTC()
	}
	_ = h.Sink.Log(ctx, record)
}

func buildData(evt activity.Event) map[string]any {
	data := activity.CloneMetadata(evt.Metadata)
	if data == nil {
		data = make(map[string]any)
	}
	if evt.DefinitionCode != "" {
		data["definition_code"] = evt.DefinitionCode
	}
	if len(evt.Recipients) > 0 {
		data["recipients"] = append([]string(nil), evt.Recipients...)
	}
	return data
}

func parseUUID(raw string) uuid.UUID {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil
	}
	return id
}
