package persistencepolicy

import (
	"context"
	"fmt"

	"github.com/goliatone/go-notifications/pkg/domain"
)

type ContentMode string

const (
	Full         ContentMode = "full"
	MetadataOnly ContentMode = "metadata_only"
	StateOnly    ContentMode = "state_only"
)

type Decision struct {
	MessageMode        ContentMode
	InboxMode          ContentMode
	PersistLinkURLs    bool
	PersistLinkRecords bool
	AllowedMetadata    []string
}

type Policy interface {
	Resolve(context.Context, *domain.NotificationDefinition) (Decision, error)
}

type FullPolicy struct{}

func (FullPolicy) Resolve(context.Context, *domain.NotificationDefinition) (Decision, error) {
	return fullDecision(), nil
}

func fullDecision() Decision {
	return Decision{
		MessageMode:        Full,
		InboxMode:          Full,
		PersistLinkURLs:    true,
		PersistLinkRecords: true,
	}
}

// DefinitionPolicy reads optional persistence settings from definition.Policy.
// Invalid configured modes fail closed.
type DefinitionPolicy struct{}

func (DefinitionPolicy) Resolve(_ context.Context, def *domain.NotificationDefinition) (Decision, error) {
	decision := fullDecision()
	if def == nil || len(def.Policy) == 0 {
		return decision, nil
	}
	if raw, ok := def.Policy["persistence_mode"].(string); ok && raw != "" {
		mode := ContentMode(raw)
		if !validMode(mode) {
			return Decision{}, fmt.Errorf("persistence policy: invalid message mode")
		}
		decision.MessageMode = mode
	}
	if raw, ok := def.Policy["inbox_persistence_mode"].(string); ok && raw != "" {
		mode := ContentMode(raw)
		if !validMode(mode) {
			return Decision{}, fmt.Errorf("persistence policy: invalid inbox mode")
		}
		decision.InboxMode = mode
	}
	if raw, ok := def.Policy["persist_link_urls"].(bool); ok {
		decision.PersistLinkURLs = raw
	}
	if raw, ok := def.Policy["persist_link_records"].(bool); ok {
		decision.PersistLinkRecords = raw
	}
	if raw, ok := def.Policy["allowed_metadata"].([]string); ok {
		decision.AllowedMetadata = append([]string(nil), raw...)
	}
	return decision, nil
}

// WithTransientOverlay applies the mandatory non-persistence boundary for
// transient rendering.
func WithTransientOverlay(decision Decision, transient bool) Decision {
	if !transient {
		return decision
	}
	decision.MessageMode = MetadataOnly
	decision.InboxMode = StateOnly
	decision.PersistLinkURLs = false
	decision.PersistLinkRecords = false
	return decision
}

func validMode(mode ContentMode) bool {
	return mode == Full || mode == MetadataOnly || mode == StateOnly
}
