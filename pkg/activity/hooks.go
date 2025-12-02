package activity

import (
	"context"
	"time"
)

// Event captures the common fields consumers need to record activity/audit events.
type Event struct {
	Verb           string
	ActorID        string
	UserID         string
	TenantID       string
	OrgID          string
	ObjectType     string
	ObjectID       string
	Channel        string
	DefinitionCode string
	Recipients     []string
	Metadata       map[string]any
	OccurredAt     time.Time
}

// Hook observers receive activity events.
type Hook interface {
	Notify(ctx context.Context, evt Event)
}

// Hooks provides a convenient fan-out collection.
type Hooks []Hook

// Notify delivers the event to every hook, skipping nil entries.
func (h Hooks) Notify(ctx context.Context, evt Event) {
	if len(h) == 0 {
		return
	}
	if evt.OccurredAt.IsZero() {
		evt.OccurredAt = time.Now().UTC()
	}
	for _, hook := range h {
		if hook == nil {
			continue
		}
		hook.Notify(ctx, evt)
	}
}

// Nop is a no-op hook useful for defaults.
type Nop struct{}

func (Nop) Notify(_ context.Context, _ Event) {}

// CloneMetadata makes a shallow copy so hooks can mutate without affecting callers.
func CloneMetadata(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
