package dispatcher

import (
	"testing"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/persistencepolicy"
)

func TestProjectMessagePersistenceModes(t *testing.T) {
	message := &domain.NotificationMessage{
		Subject: "private subject", Body: "private body",
		ActionURL: "https://private/action", ManifestURL: "https://private/manifest",
		URL: "https://private", Receiver: "person@example.com",
		Metadata: domain.JSONMap{"campaign_id": "campaign-1", "secret": "private"},
	}

	metadataOnly := projectMessage(message, persistencepolicy.Decision{
		MessageMode: persistencepolicy.MetadataOnly, AllowedMetadata: []string{"campaign_id"},
	})
	if metadataOnly.Subject != "" || metadataOnly.Body != "" || metadataOnly.ActionURL != "" ||
		metadataOnly.Metadata["campaign_id"] != "campaign-1" || metadataOnly.Metadata["secret"] != nil {
		t.Fatalf("metadata-only projection leaked content: %+v", metadataOnly)
	}

	stateOnly := projectMessage(message, persistencepolicy.Decision{MessageMode: persistencepolicy.StateOnly})
	if stateOnly.Subject != "" || stateOnly.Body != "" || len(stateOnly.Metadata) != 0 {
		t.Fatalf("state-only projection leaked content: %+v", stateOnly)
	}

	fullWithoutLinks := projectMessage(message, persistencepolicy.Decision{
		MessageMode: persistencepolicy.Full, PersistLinkURLs: false,
	})
	if fullWithoutLinks.Subject == "" || fullWithoutLinks.Body == "" ||
		fullWithoutLinks.ActionURL != "" || fullWithoutLinks.ManifestURL != "" || fullWithoutLinks.URL != "" {
		t.Fatalf("full projection did not honor link policy: %+v", fullWithoutLinks)
	}
}
