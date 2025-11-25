package exportready

import (
	"context"
	"strings"
	"testing"

	i18n "github.com/goliatone/go-i18n"
	memstore "github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/inbox"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/notifier"
	"github.com/goliatone/go-notifications/pkg/templates"
)

func TestExportNotifierSendsEmail(t *testing.T) {
	ctx := context.Background()
	defRepo := memstore.NewDefinitionRepository()
	tplRepo := memstore.NewTemplateRepository()
	evtRepo := memstore.NewEventRepository()
	msgRepo := memstore.NewMessageRepository()
	attRepo := memstore.NewDeliveryRepository()
	inboxRepo := memstore.NewInboxRepository()

	tplSvc := newTemplateService(t, tplRepo)
	inboxSvc := newInboxService(t, inboxRepo)
	registry := adapters.NewRegistry(&stubMessenger{})

	if _, err := Register(ctx, Dependencies{
		Definitions: defRepo,
		Templates:   tplSvc,
	}, Options{}); err != nil {
		t.Fatalf("register assets: %v", err)
	}

	mgr, err := notifier.New(notifier.Dependencies{
		Definitions: defRepo,
		Events:      evtRepo,
		Messages:    msgRepo,
		Attempts:    attRepo,
		Templates:   tplSvc,
		Adapters:    registry,
		Logger:      &logger.Nop{},
		Config:      config.DispatcherConfig{},
		Inbox:       inboxSvc,
	})
	if err != nil {
		t.Fatalf("build manager: %v", err)
	}

	exportNotifier, err := NewNotifier(mgr, "")
	if err != nil {
		t.Fatalf("build export notifier: %v", err)
	}

	payload := ExportReadyEvent{
		Recipients:  []string{"user-1"},
		Locale:      "en",
		FileName:    "orders.csv",
		Format:      "csv",
		URL:         "https://example.com/orders.csv",
		ExpiresAt:   "2024-05-01T00:00:00Z",
		Rows:        42,
		Parts:       2,
		ManifestURL: "https://example.com/manifest.json",
		Message:     "Filtered by customer segment",
		Channels:    []string{"email"},
	}

	if err := exportNotifier.Send(ctx, payload); err != nil {
		t.Fatalf("send: %v", err)
	}

	sent := registry.List("email")
	if len(sent) == 0 {
		t.Fatalf("expected at least one registered messenger")
	}
	mock, ok := sent[0].(*stubMessenger)
	if !ok {
		t.Fatalf("expected stub messenger")
	}
	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email send, got %d", len(mock.sent))
	}
	msg := mock.sent[0]
	if msg.Channel != "email" {
		t.Fatalf("expected email channel, got %s", msg.Channel)
	}
	if !strings.Contains(msg.Subject, "orders.csv") {
		t.Fatalf("expected subject to include filename, got %s", msg.Subject)
	}
	if !strings.Contains(msg.Body, payload.ManifestURL) {
		t.Fatalf("expected body to include manifest url, got %s", msg.Body)
	}
}

func TestExportNotifierSendsInApp(t *testing.T) {
	ctx := context.Background()
	defRepo := memstore.NewDefinitionRepository()
	tplRepo := memstore.NewTemplateRepository()
	evtRepo := memstore.NewEventRepository()
	msgRepo := memstore.NewMessageRepository()
	attRepo := memstore.NewDeliveryRepository()
	inboxRepo := memstore.NewInboxRepository()

	tplSvc := newTemplateService(t, tplRepo)
	inboxSvc := newInboxService(t, inboxRepo)
	registry := adapters.NewRegistry(&stubMessenger{}) // email only, in-app goes to inbox

	if _, err := Register(ctx, Dependencies{
		Definitions: defRepo,
		Templates:   tplSvc,
	}, Options{}); err != nil {
		t.Fatalf("register assets: %v", err)
	}

	mgr, err := notifier.New(notifier.Dependencies{
		Definitions: defRepo,
		Events:      evtRepo,
		Messages:    msgRepo,
		Attempts:    attRepo,
		Templates:   tplSvc,
		Adapters:    registry,
		Logger:      &logger.Nop{},
		Config:      config.DispatcherConfig{},
		Inbox:       inboxSvc,
	})
	if err != nil {
		t.Fatalf("build manager: %v", err)
	}

	exportNotifier, err := NewNotifier(mgr, "")
	if err != nil {
		t.Fatalf("build export notifier: %v", err)
	}

	payload := ExportReadyEvent{
		Recipients: []string{"user-2"},
		Locale:     "en",
		FileName:   "accounts.xlsx",
		Format:     "xlsx",
		URL:        "https://example.com/accounts.xlsx",
		ExpiresAt:  "2024-05-02T00:00:00Z",
		Channels:   []string{"in-app"},
	}

	if err := exportNotifier.Send(ctx, payload); err != nil {
		t.Fatalf("send: %v", err)
	}

	result, err := inboxRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected 1 inbox item, got %d", result.Total)
	}
	if !strings.Contains(result.Items[0].Title, "accounts.xlsx") {
		t.Fatalf("expected inbox title to include filename, got %s", result.Items[0].Title)
	}
}

// Helpers

type stubMessenger struct {
	sent []adapters.Message
}

func (s *stubMessenger) Name() string { return "stub-email" }

func (s *stubMessenger) Capabilities() adapters.Capability {
	return adapters.Capability{
		Name:     "stub-email",
		Channels: []string{"email"},
		Formats:  []string{"text/html"},
	}
}

func (s *stubMessenger) Send(ctx context.Context, msg adapters.Message) error {
	s.sent = append(s.sent, msg)
	return nil
}

func newTemplateService(t *testing.T, repo *memstore.TemplateRepository) *templates.Service {
	t.Helper()
	translator := newTranslator(t)
	svc, err := templates.New(templates.Dependencies{
		Repository:    repo,
		Cache:         &cache.Nop{},
		Logger:        &logger.Nop{},
		Translator:    translator,
		Fallbacks:     i18n.NewStaticFallbackResolver(),
		DefaultLocale: "en",
	})
	if err != nil {
		t.Fatalf("template service: %v", err)
	}
	return svc
}

func newInboxService(t *testing.T, repo *memstore.InboxRepository) *inbox.Service {
	t.Helper()
	svc, err := inbox.New(inbox.Dependencies{
		Repository:  repo,
		Broadcaster: &broadcaster.Nop{},
		Logger:      &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("inbox service: %v", err)
	}
	return svc
}

func newTranslator(t *testing.T) i18n.Translator {
	t.Helper()
	store := i18n.NewStaticStore(Translations())
	translator, err := i18n.NewSimpleTranslator(store, i18n.WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("translator: %v", err)
	}
	return translator
}
