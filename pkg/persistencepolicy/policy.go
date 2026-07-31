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
	return DefaultDecision(), nil
}

// DefaultDecision returns the unrestricted persistence policy used when a
// definition does not configure narrower storage rules.
func DefaultDecision() Decision {
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
	decision := DefaultDecision()
	if def == nil || len(def.Policy) == 0 {
		return decision, nil
	}
	if err := applyContentModes(def.Policy, &decision); err != nil {
		return Decision{}, err
	}
	applyPersistenceOptions(def.Policy, &decision)
	return decision, nil
}

func applyContentModes(policy domain.JSONMap, decision *Decision) error {
	if raw, ok := policy["persistence_mode"].(string); ok && raw != "" {
		if mode := ContentMode(raw); validMode(mode) {
			decision.MessageMode = mode
		} else {
			return fmt.Errorf("persistence policy: invalid message mode")
		}
	}
	if raw, ok := policy["inbox_persistence_mode"].(string); ok && raw != "" {
		if mode := ContentMode(raw); validMode(mode) {
			decision.InboxMode = mode
		} else {
			return fmt.Errorf("persistence policy: invalid inbox mode")
		}
	}
	return nil
}

func applyPersistenceOptions(policy domain.JSONMap, decision *Decision) {
	if raw, ok := policy["persist_link_urls"].(bool); ok {
		decision.PersistLinkURLs = raw
	}
	if raw, ok := policy["persist_link_records"].(bool); ok {
		decision.PersistLinkRecords = raw
	}
	if raw, ok := policy["allowed_metadata"].([]string); ok {
		decision.AllowedMetadata = append([]string(nil), raw...)
	} else if raw, ok := policy["allowed_metadata"].([]any); ok {
		for _, value := range raw {
			if key, ok := value.(string); ok && key != "" {
				decision.AllowedMetadata = append(decision.AllowedMetadata, key)
			}
		}
	}
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
	decision.AllowedMetadata = nil
	return decision
}

func validMode(mode ContentMode) bool {
	return mode == Full || mode == MetadataOnly || mode == StateOnly
}
