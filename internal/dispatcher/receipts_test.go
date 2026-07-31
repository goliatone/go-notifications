package dispatcher

import (
	"context"
	"testing"

	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/privacy"
)

func TestReceiptOutcomesPreserveRecipientChannelAndRegistryOrder(t *testing.T) {
	ctx := context.Background()
	definitions := memory.NewDefinitionRepository()
	messages := memory.NewMessageRepository()
	attempts := memory.NewDeliveryRepository()
	if err := definitions.Create(ctx, &domain.NotificationDefinition{
		Code: "ordered", Channels: domain.StringList{"email"},
	}); err != nil {
		t.Fatalf("create definition: %v", err)
	}
	event := &domain.NotificationEvent{
		DefinitionCode: "ordered", Recipients: domain.StringList{"subject-b", "subject-a"},
	}
	for _, recipient := range event.Recipients {
		message := &domain.NotificationMessage{
			EventID: event.ID, Receiver: recipient, Channel: "email",
			Status: domain.MessageStatusDelivered,
		}
		if err := messages.Create(ctx, message); err != nil {
			t.Fatalf("create message: %v", err)
		}
		for _, provider := range []string{"provider-a", "provider-b"} {
			if err := attempts.Create(ctx, &domain.DeliveryAttempt{
				MessageID: message.ID, Adapter: provider, Status: domain.AttemptStatusSucceeded,
			}); err != nil {
				t.Fatalf("create attempt: %v", err)
			}
		}
	}
	registry := adapters.NewRegistry(
		receiptMessenger{name: "provider-b"},
		receiptMessenger{name: "provider-a"},
	)
	service := &Service{
		definitions: definitions, messages: messages, attempts: attempts,
		registry: registry, privacy: privacy.DefaultPolicy{},
	}
	outcomes := service.reconstructOutcomes(ctx, event, DispatchOptions{}, nil)
	if len(outcomes) != 4 {
		t.Fatalf("expected four outcomes, got %+v", outcomes)
	}
	expectedSubjects := []string{"subject-b", "subject-b", "subject-a", "subject-a"}
	expectedProviders := []string{"provider-b", "provider-a", "provider-b", "provider-a"}
	for idx := range outcomes {
		if outcomes[idx].SubjectID != expectedSubjects[idx] || outcomes[idx].Provider != expectedProviders[idx] {
			t.Fatalf("outcome %d order mismatch: %+v", idx, outcomes)
		}
	}
}

type receiptMessenger struct{ name string }

func (m receiptMessenger) Name() string { return m.name }
func (m receiptMessenger) Capabilities() adapters.Capability {
	return adapters.Capability{Name: m.name, Channels: []string{"email"}}
}
func (m receiptMessenger) Send(context.Context, adapters.Message) error { return nil }
