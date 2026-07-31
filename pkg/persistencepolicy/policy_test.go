package persistencepolicy

import (
	"context"
	"testing"

	"github.com/goliatone/go-notifications/pkg/domain"
)

func TestDefinitionPolicyResolvesExplicitModes(t *testing.T) {
	decision, err := (DefinitionPolicy{}).Resolve(context.Background(), &domain.NotificationDefinition{
		Policy: domain.JSONMap{
			"persistence_mode":       "metadata_only",
			"inbox_persistence_mode": "state_only",
			"persist_link_urls":      false,
			"persist_link_records":   false,
			"allowed_metadata":       []string{"campaign_id"},
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if decision.MessageMode != MetadataOnly || decision.InboxMode != StateOnly ||
		decision.PersistLinkURLs || decision.PersistLinkRecords ||
		len(decision.AllowedMetadata) != 1 {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestTransientOverlayAlwaysDisablesContentAndLinks(t *testing.T) {
	decision := WithTransientOverlay(Decision{
		MessageMode: Full, InboxMode: Full, PersistLinkURLs: true, PersistLinkRecords: true,
		AllowedMetadata: []string{"rendered_secret"},
	}, true)
	if decision.MessageMode != MetadataOnly || decision.InboxMode != StateOnly ||
		decision.PersistLinkURLs || decision.PersistLinkRecords || len(decision.AllowedMetadata) != 0 {
		t.Fatalf("transient overlay did not fail closed: %+v", decision)
	}
}
