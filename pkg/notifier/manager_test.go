package notifier

import (
	"context"
	"errors"
	"testing"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/internal/inbox"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/adapters/console"
	"github.com/goliatone/go-notifications/pkg/adapters/twilio"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	prefsvc "github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/templates"
)

func TestManagerSendMultiChannelSuccess(t *testing.T) {
	ctx := context.Background()
	defRepo := memory.NewDefinitionRepository()
	eventRepo := memory.NewEventRepository()
	msgRepo := memory.NewMessageRepository()
	attemptRepo := memory.NewDeliveryRepository()
	tplRepo := memory.NewTemplateRepository()
	prefRepo := memory.NewPreferenceRepository()
	inboxRepo := memory.NewInboxRepository()

	translator := newTestTranslator(t)
	tplSvc, err := templates.New(templates.Dependencies{
		Repository: tplRepo,
		Cache:      &cache.Nop{},
		Logger:     &logger.Nop{},
		Translator: translator,
	})
	if err != nil {
		t.Fatalf("template service: %v", err)
	}

	createTemplate(t, tplSvc, templates.TemplateInput{
		Code:    "welcome-email",
		Channel: "email",
		Locale:  "en",
		Subject: "Welcome {{ Name }}",
		Body:    "Email body for {{ Name }}",
		Format:  "text/plain",
		Schema:  domain.TemplateSchema{Required: []string{"Name"}},
	})
	createTemplate(t, tplSvc, templates.TemplateInput{
		Code:    "welcome-sms",
		Channel: "sms",
		Locale:  "en",
		Subject: "SMS {{ Name }}",
		Body:    "SMS body for {{ Name }}",
		Format:  "text/plain",
		Schema:  domain.TemplateSchema{Required: []string{"Name"}},
	})

	definition := &domain.NotificationDefinition{
		Code:         "welcome",
		Channels:     domain.StringList{"email:console", "sms:twilio"},
		TemplateKeys: domain.StringList{"email:welcome-email", "sms:welcome-sms"},
	}
	if err := defRepo.Create(ctx, definition); err != nil {
		t.Fatalf("create definition: %v", err)
	}

	registry := adapters.NewRegistry(console.New(&logger.Nop{}), twilio.New(&logger.Nop{}))

	prefs := newPreferenceService(t, prefRepo)
	inboxSvc := newInboxService(t, inboxRepo)

	manager, err := New(Dependencies{
		Definitions: defRepo,
		Events:      eventRepo,
		Messages:    msgRepo,
		Attempts:    attemptRepo,
		Templates:   tplSvc,
		Adapters:    registry,
		Logger:      &logger.Nop{},
		Config: config.DispatcherConfig{
			Enabled:              true,
			MaxRetries:           2,
			MaxWorkers:           4,
			EnvFallbackAllowlist: []string{"user@example.com"},
		},
		Preferences: prefs,
		Inbox:       inboxSvc,
	})
	if err != nil {
		t.Fatalf("manager: %v", err)
	}

	err = manager.Send(ctx, Event{
		DefinitionCode: "welcome",
		Recipients:     []string{"user@example.com"},
		Context: map[string]any{
			"Name": "Rosa",
		},
		Locale: "en",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	eventList, err := eventRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(eventList.Items) != 1 {
		t.Fatalf("expected 1 event, got %d", len(eventList.Items))
	}
	if eventList.Items[0].Status != domain.EventStatusProcessed {
		t.Fatalf("expected event processed, got %s", eventList.Items[0].Status)
	}

	msgList, err := msgRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if msgList.Total != 2 {
		t.Fatalf("expected 2 messages, got %d", msgList.Total)
	}
	for _, msg := range msgList.Items {
		if msg.Status != domain.MessageStatusDelivered {
			t.Fatalf("expected delivered message, got %s", msg.Status)
		}
	}

	attemptList, err := attemptRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if attemptList.Total != 2 {
		t.Fatalf("expected 2 attempts, got %d", attemptList.Total)
	}
}

func TestManagerSendRecordsFailures(t *testing.T) {
	ctx := context.Background()
	defRepo := memory.NewDefinitionRepository()
	eventRepo := memory.NewEventRepository()
	msgRepo := memory.NewMessageRepository()
	attemptRepo := memory.NewDeliveryRepository()
	tplRepo := memory.NewTemplateRepository()
	prefRepo := memory.NewPreferenceRepository()
	inboxRepo := memory.NewInboxRepository()

	translator := newTestTranslator(t)
	tplSvc, err := templates.New(templates.Dependencies{
		Repository: tplRepo,
		Cache:      &cache.Nop{},
		Logger:     &logger.Nop{},
		Translator: translator,
	})
	if err != nil {
		t.Fatalf("template service: %v", err)
	}

	createTemplate(t, tplSvc, templates.TemplateInput{
		Code:    "alert-email",
		Channel: "email",
		Locale:  "en",
		Subject: "Alert {{ Name }}",
		Body:    "Body {{ Name }}",
		Format:  "text/plain",
		Schema:  domain.TemplateSchema{Required: []string{"Name"}},
	})

	def := &domain.NotificationDefinition{
		Code:         "alert",
		Channels:     domain.StringList{"email:failing"},
		TemplateKeys: domain.StringList{"email:alert-email"},
	}
	if err := defRepo.Create(ctx, def); err != nil {
		t.Fatalf("create definition: %v", err)
	}

	failAdapter := &failingAdapter{
		name:       "failing",
		capability: adapters.Capability{Name: "failing", Channels: []string{"email"}, Formats: []string{"text/plain"}},
		failures:   3,
	}

	registry := adapters.NewRegistry(failAdapter)

	prefs := newPreferenceService(t, prefRepo)
	inboxSvc := newInboxService(t, inboxRepo)

	manager, err := New(Dependencies{
		Definitions: defRepo,
		Events:      eventRepo,
		Messages:    msgRepo,
		Attempts:    attemptRepo,
		Templates:   tplSvc,
		Adapters:    registry,
		Logger:      &logger.Nop{},
		Config: config.DispatcherConfig{
			Enabled:              true,
			MaxRetries:           2,
			MaxWorkers:           1,
			EnvFallbackAllowlist: []string{"ops@example.com"},
		},
		Preferences: prefs,
		Inbox:       inboxSvc,
	})
	if err != nil {
		t.Fatalf("manager: %v", err)
	}

	err = manager.Send(ctx, Event{
		DefinitionCode: "alert",
		Recipients:     []string{"ops@example.com"},
		Context:        map[string]any{"Name": "Ops"},
	})
	if err == nil {
		t.Fatalf("expected send failure")
	}

	attemptList, err := attemptRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	if attemptList.Total != 2 {
		t.Fatalf("expected 2 attempts due to retries, got %d", attemptList.Total)
	}
	for _, attempt := range attemptList.Items {
		if attempt.Status != domain.AttemptStatusFailed {
			t.Fatalf("attempt should fail, got %s", attempt.Status)
		}
	}
}

func TestManagerSkipsBlockedPreferences(t *testing.T) {
	ctx := context.Background()
	defRepo := memory.NewDefinitionRepository()
	eventRepo := memory.NewEventRepository()
	msgRepo := memory.NewMessageRepository()
	attemptRepo := memory.NewDeliveryRepository()
	tplRepo := memory.NewTemplateRepository()
	prefRepo := memory.NewPreferenceRepository()
	inboxRepo := memory.NewInboxRepository()

	translator := newTestTranslator(t)
	tplSvc, err := templates.New(templates.Dependencies{
		Repository: tplRepo,
		Cache:      &cache.Nop{},
		Logger:     &logger.Nop{},
		Translator: translator,
	})
	if err != nil {
		t.Fatalf("template service: %v", err)
	}

	createTemplate(t, tplSvc, templates.TemplateInput{
		Code:    "pref-email",
		Channel: "email",
		Locale:  "en",
		Subject: "Hello {{ Name }}",
		Body:    "Body {{ Name }}",
		Format:  "text/plain",
		Schema:  domain.TemplateSchema{Required: []string{"Name"}},
	})

	definition := &domain.NotificationDefinition{
		Code:         "pref-block",
		Channels:     domain.StringList{"email:console"},
		TemplateKeys: domain.StringList{"email:pref-email"},
	}
	if err := defRepo.Create(ctx, definition); err != nil {
		t.Fatalf("create definition: %v", err)
	}

	prefs := newPreferenceService(t, prefRepo)
	inboxSvc := newInboxService(t, inboxRepo)
	enabled := boolPtr(false)
	if _, err := prefs.Upsert(ctx, prefsvc.PreferenceInput{
		SubjectType:    "user",
		SubjectID:      "blocked@example.com",
		DefinitionCode: "pref-block",
		Channel:        "email",
		Enabled:        enabled,
	}); err != nil {
		t.Fatalf("seed preference: %v", err)
	}

	registry := adapters.NewRegistry(console.New(&logger.Nop{}))

	manager, err := New(Dependencies{
		Definitions: defRepo,
		Events:      eventRepo,
		Messages:    msgRepo,
		Attempts:    attemptRepo,
		Templates:   tplSvc,
		Adapters:    registry,
		Logger:      &logger.Nop{},
		Config: config.DispatcherConfig{
			Enabled:    true,
			MaxRetries: 1,
			MaxWorkers: 1,
		},
		Preferences: prefs,
		Inbox:       inboxSvc,
	})
	if err != nil {
		t.Fatalf("manager: %v", err)
	}

	if err := manager.Send(ctx, Event{
		DefinitionCode: "pref-block",
		Recipients:     []string{"blocked@example.com"},
		Context:        map[string]any{"Name": "Blocked"},
	}); err != nil {
		t.Fatalf("send: %v", err)
	}

	msgs, err := msgRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if msgs.Total != 0 {
		t.Fatalf("expected no messages when preferences block delivery, got %d", msgs.Total)
	}
}

func newPreferenceService(t *testing.T, repo *memory.PreferenceRepository) *prefsvc.Service {
	t.Helper()
	svc, err := prefsvc.New(prefsvc.Dependencies{
		Repository: repo,
		Logger:     &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("preferences service: %v", err)
	}
	return svc
}

func boolPtr(v bool) *bool { return &v }

func newInboxService(t *testing.T, repo *memory.InboxRepository) *inbox.Service {
	t.Helper()
	svc, err := inbox.NewService(inbox.Dependencies{
		Repository:  repo,
		Broadcaster: &broadcaster.Nop{},
		Logger:      &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("inbox service: %v", err)
	}
	return svc
}

// Helpers --------------------------------------------------------------------

func createTemplate(t *testing.T, svc *templates.Service, input templates.TemplateInput) {
	t.Helper()
	if _, err := svc.Create(context.Background(), input); err != nil {
		t.Fatalf("create template %s: %v", input.Code, err)
	}
}

type failingAdapter struct {
	name       string
	capability adapters.Capability
	failures   int
}

func (f *failingAdapter) Name() string { return f.name }

func (f *failingAdapter) Capabilities() adapters.Capability { return f.capability }

func (f *failingAdapter) Send(ctx context.Context, msg adapters.Message) error {
	if f.failures > 0 {
		f.failures--
		return errors.New("injected failure")
	}
	return nil
}

func newTestTranslator(t *testing.T) i18n.Translator {
	t.Helper()
	translations := i18n.Translations{
		"en": newCatalog("en", map[string]string{
			"welcome.subject": "Welcome %s",
			"welcome.body":    "Hello %s",
			"alert.subject":   "Alert %s",
			"alert.body":      "Body %s",
		}),
	}
	store := i18n.NewStaticStore(translations)
	translator, err := i18n.NewSimpleTranslator(store, i18n.WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("translator: %v", err)
	}
	return translator
}

func newCatalog(locale string, entries map[string]string) *i18n.TranslationCatalog {
	catalog := &i18n.TranslationCatalog{
		Locale:   i18n.Locale{Code: locale},
		Messages: make(map[string]i18n.Message),
	}
	for key, template := range entries {
		msg := i18n.Message{}
		msg.SetContent(template)
		catalog.Messages[key] = msg
	}
	return catalog
}
