package commands

import (
	"context"
	"testing"
	"time"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/inbox"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/preferences"
	"github.com/goliatone/go-notifications/pkg/templates"
)

func TestCatalogCommands(t *testing.T) {
	ctx := context.Background()
	defRepo := memory.NewDefinitionRepository()
	tplRepo := memory.NewTemplateRepository()
	translator := newTestTranslator(t)
	tplSvc, err := templates.New(templates.Dependencies{
		Repository: tplRepo,
		Cache:      &cache.Nop{},
		Logger:     &logger.Nop{},
		Translator: translator,
	})
	if err != nil {
		t.Fatalf("templates service: %v", err)
	}
	prefRepo := memory.NewPreferenceRepository()
	prefSvc, err := preferences.New(preferences.Dependencies{
		Repository: prefRepo,
	})
	if err != nil {
		t.Fatalf("preferences service: %v", err)
	}
	inboxRepo := memory.NewInboxRepository()
	inboxSvc, err := inbox.New(inbox.Dependencies{
		Repository:  inboxRepo,
		Broadcaster: &broadcaster.Nop{},
		Logger:      &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("inbox service: %v", err)
	}
	eventStub := &stubEvents{}

	cat, err := NewCatalog(Dependencies{
		Definitions: defRepo,
		Templates:   tplSvc,
		Preferences: prefSvc,
		Inbox:       inboxSvc,
		Events:      eventStub,
	})
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}

	if err := cat.CreateDefinition.Execute(ctx, CreateDefinition{Code: "welcome", Name: "Welcome", AllowUpdate: true}); err != nil {
		t.Fatalf("create definition: %v", err)
	}
	if err := cat.SaveTemplate.Execute(ctx, TemplateUpsert{TemplateInput: templates.TemplateInput{Code: "welcome", Channel: "email", Locale: "en", Subject: "Hi", Body: "Body"}, AllowUpdate: true}); err != nil {
		t.Fatalf("save template: %v", err)
	}
	if err := cat.UpsertPreference.Execute(ctx, preferences.PreferenceInput{SubjectType: "user", SubjectID: "u1", DefinitionCode: "welcome", Channel: "email"}); err != nil {
		t.Fatalf("upsert preference: %v", err)
	}

	item, err := inboxSvc.Create(ctx, inbox.CreateInput{UserID: "u1", Title: "Hello", Body: "World"})
	if err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	if err := cat.InboxMarkRead.Execute(ctx, InboxMarkRead{UserID: "u1", IDs: []string{item.ID.String()}, Read: true}); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	if err := cat.InboxSnooze.Execute(ctx, InboxSnooze{UserID: "u1", ID: item.ID.String(), Until: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("snooze: %v", err)
	}
	if err := cat.InboxDismiss.Execute(ctx, InboxDismiss{UserID: "u1", ID: item.ID.String()}); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	if err := cat.EnqueueEvent.Execute(ctx, events.IntakeRequest{DefinitionCode: "welcome", Recipients: []string{"u1"}}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if len(eventStub.requests) != 1 {
		t.Fatalf("expected enqueue call")
	}
}

func newTestTranslator(t *testing.T) i18n.Translator {
	t.Helper()
	translations := i18n.Translations{
		"en": &i18n.TranslationCatalog{Locale: i18n.Locale{Code: "en"}, Messages: map[string]i18n.Message{}},
	}
	store := i18n.NewStaticStore(translations)
	translator, err := i18n.NewSimpleTranslator(store, i18n.WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("translator: %v", err)
	}
	return translator
}

type stubEvents struct {
	requests []events.IntakeRequest
}

func (s *stubEvents) Enqueue(ctx context.Context, req events.IntakeRequest) error {
	s.requests = append(s.requests, req)
	return nil
}
