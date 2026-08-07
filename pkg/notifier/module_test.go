package notifier

import (
	"context"
	"errors"
	"testing"
	"time"

	command "github.com/goliatone/go-command"
	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/definitions"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/persistencepolicy"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/receipts"
	"github.com/goliatone/go-notifications/pkg/storage"
	"github.com/goliatone/go-notifications/pkg/templates"
)

func TestModuleConstruction(t *testing.T) {
	translator := moduleTranslator(t)
	module, err := NewModule(ModuleOptions{
		Translator: translator,
		Logger:     &logger.Nop{},
		Storage:    storage.NewMemoryProviders(),
	})
	if err != nil {
		t.Fatalf("module: %v", err)
	}
	if module.Manager() == nil {
		t.Fatalf("expected manager")
	}
	if module.Commands() == nil {
		t.Fatalf("expected commands registry")
	}
	if module.Inbox() == nil || module.Events() == nil {
		t.Fatalf("expected inbox and events services")
	}
	commanders := module.Commands().Commanders()
	if len(commanders) != 9 {
		t.Fatalf("expected 9 command handlers, got %d", len(commanders))
	}
	seen := make(map[string]struct{}, len(commanders))
	for _, commander := range commanders {
		registrations, err := command.MessageRegistrationsForCommand(commander)
		if err != nil {
			t.Fatalf("inspect command registration: %v", err)
		}
		if len(registrations) != 1 {
			t.Fatalf("expected one registration for %T, got %d", commander, len(registrations))
		}
		id := registrations[0].ID()
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate command registration %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestModuleConstructionRemainsCompatibleWithoutRetentionProvider(t *testing.T) {
	providers := storage.NewMemoryProviders()
	providers.Retention = nil
	module, err := NewModule(ModuleOptions{
		Translator: moduleTranslator(t), Logger: &logger.Nop{}, Storage: providers,
	})
	if err != nil {
		t.Fatalf("module with legacy provider set: %v", err)
	}
	if module.Retention() == nil || module.Commands().PurgeRetention == nil {
		t.Fatalf("expected unavailable retention facade and command")
	}
}

func TestModuleExposesEffectivePoliciesResolverAndCanonicalServices(t *testing.T) {
	resolver := &moduleResolver{}
	diagnostic := &moduleDiagnostic{}
	module, err := NewModule(ModuleOptions{
		Translator:        moduleTranslator(t),
		Logger:            &logger.Nop{},
		Storage:           storage.NewMemoryProviders(),
		Adapters:          []adapters.Messenger{moduleAdapter{}},
		RecipientResolver: resolver,
		Persistence:       persistencepolicy.FullPolicy{},
		Privacy:           privacy.DefaultPolicy{},
		Diagnostic:        diagnostic,
	})
	if err != nil {
		t.Fatalf("module: %v", err)
	}
	if module.Definitions() == nil || module.Events() == nil || module.Commands().RetryEvent == nil {
		t.Fatalf("canonical definition/event/retry services were not exposed")
	}
	if module.PersistencePolicy() == nil || module.PrivacyPolicy() == nil ||
		module.DiagnosticSink() != diagnostic || module.RecipientResolver() != resolver {
		t.Fatalf("effective policy dependencies were not exposed")
	}
	routed, err := module.AdapterRegistry().Route("email:module")
	if err != nil {
		t.Fatalf("route resolving adapter: %v", err)
	}
	resolving, ok := routed.(adapters.ResolvingMessenger)
	if !ok || resolving.Resolver != resolver || resolving.Diagnostic != diagnostic {
		t.Fatalf("adapter was not decorated with module resolver: %#v", routed)
	}
	if _, err := New(Dependencies{EventService: module.Events()}); err != nil {
		t.Fatalf("event-service-only manager construction should not rebuild delivery dependencies: %v", err)
	}
}

func TestModuleDefaultsAllowImmediateAndRejectAsyncWithNopQueue(t *testing.T) {
	ctx := context.Background()
	cfg := config.Defaults()
	cfg.Dispatcher.EnvFallbackAllowlist = []string{"subject-1", "subject-2"}
	repositories := storage.NewMemoryProviders()
	module, moduleErr := NewModule(ModuleOptions{
		Config:     cfg,
		Translator: moduleTranslator(t),
		Logger:     &logger.Nop{},
		Storage:    repositories,
		Adapters:   []adapters.Messenger{moduleAdapter{}},
	})
	if moduleErr != nil {
		t.Fatalf("module: %v", moduleErr)
	}
	if _, err := module.Definitions().Upsert(ctx, definitions.UpsertInput{
		Code: "welcome", Channels: []string{"email:module"},
		TemplateIDs: []string{"email:welcome-email"},
	}); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	if _, err := module.Templates().Create(ctx, templates.TemplateInput{
		Code: "welcome-email", Channel: "email", Locale: "en", Subject: "Hello", Body: "Body",
	}); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	immediate, err := module.Manager().SendWithReceipt(ctx, Event{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
	})
	if err != nil || immediate.Status != receipts.StatusProcessed {
		t.Fatalf("ordinary immediate default failed: receipt=%+v err=%v", immediate, err)
	}
	scheduled, err := module.Manager().SendWithReceipt(ctx, Event{
		DefinitionCode: "welcome", Recipients: []string{"subject-2"},
		ScheduledAt: time.Now().Add(time.Hour),
	})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || safe.Code != "durable_queue_required" ||
		scheduled.Status != receipts.StatusFailed {
		t.Fatalf("no-op queue accepted asynchronous work: receipt=%+v err=%#v", scheduled, err)
	}
	before, err := repositories.Events.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list events before transient schedule: %v", err)
	}
	_, err = module.Manager().SendWithReceipt(ctx, Event{
		DefinitionCode: "welcome", Recipients: []string{"subject-3"},
		ScheduledAt: time.Now().Add(750 * time.Millisecond),
		Transient:   map[string]any{"credential": "ephemeral"},
	})
	if !errors.As(err, &safe) || safe.Code != "transient_async_forbidden" {
		t.Fatalf("sub-second transient schedule was not rejected: %v", err)
	}
	after, err := repositories.Events.List(ctx, store.ListOptions{})
	if err != nil || after.Total != before.Total {
		t.Fatalf("rejected transient schedule persisted an event: before=%d after=%d err=%v", before.Total, after.Total, err)
	}
}

func TestModuleDelegatesReceiptLookup(t *testing.T) {
	ctx := context.Background()
	cfg := config.Defaults()
	cfg.Dispatcher.EnvFallbackAllowlist = []string{"subject-1"}
	module, err := NewModule(ModuleOptions{
		Config: cfg, Translator: moduleTranslator(t), Logger: &logger.Nop{},
		Storage: storage.NewMemoryProviders(), Adapters: []adapters.Messenger{moduleAdapter{}},
	})
	if err != nil {
		t.Fatalf("module: %v", err)
	}
	if _, err := module.Definitions().Upsert(ctx, definitions.UpsertInput{
		Code: "welcome", Channels: []string{"email:module"}, TemplateIDs: []string{"email:welcome-email"},
	}); err != nil {
		t.Fatalf("seed definition: %v", err)
	}
	if _, err := module.Templates().Create(ctx, templates.TemplateInput{
		Code: "welcome-email", Channel: "email", Locale: "en", Subject: "Hello", Body: "Body",
	}); err != nil {
		t.Fatalf("seed template: %v", err)
	}
	first, err := module.Manager().SendWithReceipt(ctx, Event{
		DefinitionCode: "welcome", Recipients: []string{"subject-1"},
		IdempotencyScope: "system", IdempotencyKey: "lookup",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	got, err := module.LookupReceipt(ctx, events.ReceiptLookup{
		DefinitionCode: "welcome", IdempotencyScope: "system", IdempotencyKey: "lookup",
	})
	if err != nil || got.EventID != first.EventID || !got.Replay {
		t.Fatalf("module lookup: receipt=%+v err=%v", got, err)
	}
}

type moduleAdapter struct{}

func (moduleAdapter) Name() string { return "module" }

func (moduleAdapter) Capabilities() adapters.Capability {
	return adapters.Capability{Name: "module", Channels: []string{"email"}}
}

func (moduleAdapter) Send(context.Context, adapters.Message) error { return nil }

type moduleResolver struct{}

func (*moduleResolver) Resolve(_ context.Context, req adapters.RecipientRequest) (adapters.ResolvedRecipient, error) {
	return adapters.ResolvedRecipient{Destination: req.SubjectID + "@example.test"}, nil
}

type moduleDiagnostic struct{}

func (*moduleDiagnostic) Report(context.Context, privacy.DiagnosticEvent) {}

func moduleTranslator(t *testing.T) i18n.Translator {
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
