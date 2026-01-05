package dispatcher

import (
	"context"
	"errors"
	"sync"
	"testing"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/links"
	"github.com/goliatone/go-notifications/pkg/templates"
	"github.com/google/uuid"
)

const testRecipient = "user@example.com"

type captureLinkBuilder struct {
	mu         sync.Mutex
	calls      []links.LinkRequest
	perChannel map[string]links.LinkRequest
	buildFn    func(req links.LinkRequest) (links.ResolvedLinks, error)
}

func (b *captureLinkBuilder) Build(ctx context.Context, req links.LinkRequest) (links.ResolvedLinks, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.perChannel == nil {
		b.perChannel = make(map[string]links.LinkRequest)
	}
	b.calls = append(b.calls, req)
	b.perChannel[req.Channel] = req
	if b.buildFn == nil {
		return links.ResolvedLinks{}, nil
	}
	return b.buildFn(req)
}

type captureStore struct {
	mu             sync.Mutex
	calls          int
	records        [][]links.LinkRecord
	err            error
	messageRepo    *memory.MessageRepository
	prePersistHits int
}

func (s *captureStore) Save(ctx context.Context, records []links.LinkRecord) error {
	s.mu.Lock()
	s.calls++
	s.records = append(s.records, records)
	s.mu.Unlock()
	if s.messageRepo != nil {
		result, err := s.messageRepo.List(ctx, store.ListOptions{})
		if err == nil && result.Total > 0 {
			s.mu.Lock()
			s.prePersistHits++
			s.mu.Unlock()
		}
	}
	return s.err
}

type captureObserver struct {
	mu        sync.Mutex
	calls     []links.LinkResolution
	storeHits *int
}

func (o *captureObserver) OnLinksResolved(ctx context.Context, info links.LinkResolution) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls = append(o.calls, info)
}

type testAdapter struct {
	name     string
	channels []string
	mu       sync.Mutex
	sends    []adapters.Message
	err      error
}

func (a *testAdapter) Name() string {
	return a.name
}

func (a *testAdapter) Capabilities() adapters.Capability {
	return adapters.Capability{
		Name:     a.name,
		Channels: a.channels,
		Formats:  []string{"text/plain"},
	}
}

func (a *testAdapter) Send(ctx context.Context, msg adapters.Message) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sends = append(a.sends, msg)
	return a.err
}

func (a *testAdapter) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.sends)
}

func TestApplyResolvedLinksToPayloadAndMessage(t *testing.T) {
	payload := domain.JSONMap{
		"keep": "value",
	}
	resolved := links.ResolvedLinks{
		ActionURL:   "https://example.com/action",
		ManifestURL: "https://example.com/manifest",
		URL:         "https://example.com/url",
		Metadata: map[string]any{
			"token": "abc123",
		},
	}

	applyResolvedLinksToPayload(payload, resolved)
	if payload[links.ResolvedURLActionKey] != resolved.ActionURL {
		t.Fatalf("expected action_url %s, got %v", resolved.ActionURL, payload[links.ResolvedURLActionKey])
	}
	if payload[links.ResolvedURLManifestKey] != resolved.ManifestURL {
		t.Fatalf("expected manifest_url %s, got %v", resolved.ManifestURL, payload[links.ResolvedURLManifestKey])
	}
	if payload[links.ResolvedURLKey] != resolved.URL {
		t.Fatalf("expected url %s, got %v", resolved.URL, payload[links.ResolvedURLKey])
	}

	message := &domain.NotificationMessage{
		Metadata: domain.JSONMap{
			"keep": "yes",
		},
	}
	applyResolvedLinksToMessage(message, resolved)
	if message.ActionURL != resolved.ActionURL {
		t.Fatalf("expected message action_url %s, got %s", resolved.ActionURL, message.ActionURL)
	}
	if message.ManifestURL != resolved.ManifestURL {
		t.Fatalf("expected message manifest_url %s, got %s", resolved.ManifestURL, message.ManifestURL)
	}
	if message.URL != resolved.URL {
		t.Fatalf("expected message url %s, got %s", resolved.URL, message.URL)
	}
	if message.Metadata["token"] != "abc123" {
		t.Fatalf("expected metadata token to be copied")
	}
	if _, ok := message.Metadata[links.ResolvedURLActionKey]; ok {
		t.Fatalf("did not expect action_url in message metadata")
	}
	if _, ok := message.Metadata[links.ResolvedURLManifestKey]; ok {
		t.Fatalf("did not expect manifest_url in message metadata")
	}
	if _, ok := message.Metadata[links.ResolvedURLKey]; ok {
		t.Fatalf("did not expect url in message metadata")
	}
}

func TestDispatcherPerChannelLinkBuilderUsesOverrides(t *testing.T) {
	ctx := context.Background()
	builder := &captureLinkBuilder{
		buildFn: func(req links.LinkRequest) (links.ResolvedLinks, error) {
			return links.ResolvedLinks{
				ActionURL: "builder-" + req.Channel,
			}, nil
		},
	}
	adapter := &testAdapter{name: "test", channels: []string{"email", "sms"}}
	svc, msgRepo, tplSvc := newTestDispatcher(t, builder, nil, nil, links.FailurePolicy{}, adapter)

	seedTemplate(t, tplSvc, "welcome-email", "email")
	seedTemplate(t, tplSvc, "welcome-sms", "sms")

	def := &domain.NotificationDefinition{
		Code:         "welcome",
		Channels:     domain.StringList{"email", "sms"},
		TemplateKeys: domain.StringList{"email:welcome-email", "sms:welcome-sms"},
	}
	event := &domain.NotificationEvent{
		RecordMeta:     domain.RecordMeta{ID: uuid.New()},
		DefinitionCode: def.Code,
		Recipients:     domain.StringList{testRecipient},
		Context: domain.JSONMap{
			"action_url": "original",
			"channel_overrides": map[string]any{
				"email": map[string]any{
					"action_url": "override-email",
				},
				"sms": map[string]any{
					"action_url": "override-sms",
				},
			},
		},
	}

	emailJob := deliveryJob{
		event:        event,
		channel:      "email",
		templateCode: "welcome-email",
		recipient:    testRecipient,
		locale:       "en",
	}
	if err := svc.processDelivery(ctx, event, def, emailJob); err != nil {
		t.Fatalf("process email delivery: %v", err)
	}

	smsJob := deliveryJob{
		event:        event,
		channel:      "sms",
		templateCode: "welcome-sms",
		recipient:    testRecipient,
		locale:       "en",
	}
	if err := svc.processDelivery(ctx, event, def, smsJob); err != nil {
		t.Fatalf("process sms delivery: %v", err)
	}

	builder.mu.Lock()
	emailReq := builder.perChannel["email"]
	smsReq := builder.perChannel["sms"]
	callCount := len(builder.calls)
	builder.mu.Unlock()

	if callCount != 2 {
		t.Fatalf("expected 2 builder calls, got %d", callCount)
	}
	if got := emailReq.Payload[links.ResolvedURLActionKey]; got != "override-email" {
		t.Fatalf("expected email payload action_url override, got %v", got)
	}
	if got := smsReq.Payload[links.ResolvedURLActionKey]; got != "override-sms" {
		t.Fatalf("expected sms payload action_url override, got %v", got)
	}

	list, err := msgRepo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("expected 2 messages, got %d", list.Total)
	}
	for _, msg := range list.Items {
		expected := "builder-" + msg.Channel
		if msg.ActionURL != expected {
			t.Fatalf("expected %s action_url, got %s", expected, msg.ActionURL)
		}
	}
}

func TestDispatcherLinkHooksErrorHandling(t *testing.T) {
	t.Run("lenient store error continues", func(t *testing.T) {
		ctx := context.Background()
		builder := &captureLinkBuilder{
			buildFn: func(req links.LinkRequest) (links.ResolvedLinks, error) {
				return links.ResolvedLinks{ActionURL: "builder-url"}, nil
			},
		}
		adapter := &testAdapter{name: "test", channels: []string{"email"}}
		storeSpy := &captureStore{
			err: errors.New("store failed"),
		}
		observer := &captureObserver{}
		svc, msgRepo, tplSvc := newTestDispatcher(t, builder, storeSpy, observer, links.FailurePolicy{
			Store: links.FailureLenient,
		}, adapter)
		storeSpy.messageRepo = msgRepo

		seedTemplate(t, tplSvc, "welcome-email", "email")

		def := &domain.NotificationDefinition{
			Code:         "welcome",
			Channels:     domain.StringList{"email"},
			TemplateKeys: domain.StringList{"email:welcome-email"},
		}
		event := &domain.NotificationEvent{
			RecordMeta:     domain.RecordMeta{ID: uuid.New()},
			DefinitionCode: def.Code,
			Recipients:     domain.StringList{testRecipient},
			Context:        domain.JSONMap{},
		}

		job := deliveryJob{
			event:        event,
			channel:      "email",
			templateCode: "welcome-email",
			recipient:    testRecipient,
			locale:       "en",
		}
		if err := svc.processDelivery(ctx, event, def, job); err != nil {
			t.Fatalf("expected delivery to continue, got %v", err)
		}

		list, err := msgRepo.List(ctx, store.ListOptions{})
		if err != nil {
			t.Fatalf("list messages: %v", err)
		}
		if list.Total != 1 {
			t.Fatalf("expected 1 message, got %d", list.Total)
		}
		if adapter.Count() != 1 {
			t.Fatalf("expected adapter send, got %d", adapter.Count())
		}
		if storeSpy.calls != 1 {
			t.Fatalf("expected store call, got %d", storeSpy.calls)
		}
		if storeSpy.prePersistHits != 0 {
			t.Fatalf("expected store before persistence, got %d hits", storeSpy.prePersistHits)
		}
		observer.mu.Lock()
		observerCalls := len(observer.calls)
		observer.mu.Unlock()
		if observerCalls != 1 {
			t.Fatalf("expected observer call, got %d", observerCalls)
		}
	})

	t.Run("strict store error stops delivery", func(t *testing.T) {
		ctx := context.Background()
		builder := &captureLinkBuilder{
			buildFn: func(req links.LinkRequest) (links.ResolvedLinks, error) {
				return links.ResolvedLinks{ActionURL: "builder-url"}, nil
			},
		}
		adapter := &testAdapter{name: "test", channels: []string{"email"}}
		storeSpy := &captureStore{
			err: errors.New("store failed"),
		}
		observer := &captureObserver{}
		svc, msgRepo, tplSvc := newTestDispatcher(t, builder, storeSpy, observer, links.FailurePolicy{
			Store: links.FailureStrict,
		}, adapter)
		storeSpy.messageRepo = msgRepo

		seedTemplate(t, tplSvc, "welcome-email", "email")

		def := &domain.NotificationDefinition{
			Code:         "welcome",
			Channels:     domain.StringList{"email"},
			TemplateKeys: domain.StringList{"email:welcome-email"},
		}
		event := &domain.NotificationEvent{
			RecordMeta:     domain.RecordMeta{ID: uuid.New()},
			DefinitionCode: def.Code,
			Recipients:     domain.StringList{testRecipient},
			Context:        domain.JSONMap{},
		}

		job := deliveryJob{
			event:        event,
			channel:      "email",
			templateCode: "welcome-email",
			recipient:    testRecipient,
			locale:       "en",
		}
		if err := svc.processDelivery(ctx, event, def, job); err == nil {
			t.Fatalf("expected error on strict store failure")
		}

		list, err := msgRepo.List(ctx, store.ListOptions{})
		if err != nil {
			t.Fatalf("list messages: %v", err)
		}
		if list.Total != 0 {
			t.Fatalf("expected no message persisted, got %d", list.Total)
		}
		if adapter.Count() != 0 {
			t.Fatalf("expected no adapter send, got %d", adapter.Count())
		}
		if storeSpy.calls != 1 {
			t.Fatalf("expected store call, got %d", storeSpy.calls)
		}
		if storeSpy.prePersistHits != 0 {
			t.Fatalf("expected store before persistence, got %d hits", storeSpy.prePersistHits)
		}
		observer.mu.Lock()
		observerCalls := len(observer.calls)
		observer.mu.Unlock()
		if observerCalls != 0 {
			t.Fatalf("expected no observer call, got %d", observerCalls)
		}
	})
}

func newTestDispatcher(t *testing.T, builder links.LinkBuilder, store links.LinkStore, observer links.LinkObserver, policy links.FailurePolicy, adapter adapters.Messenger) (*Service, *memory.MessageRepository, *templates.Service) {
	t.Helper()
	defRepo := memory.NewDefinitionRepository()
	eventRepo := memory.NewEventRepository()
	msgRepo := memory.NewMessageRepository()
	attemptRepo := memory.NewDeliveryRepository()
	tplRepo := memory.NewTemplateRepository()

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

	registry := adapters.NewRegistry(adapter)
	svc, err := New(Dependencies{
		Definitions:  defRepo,
		Events:       eventRepo,
		Messages:     msgRepo,
		Attempts:     attemptRepo,
		Templates:    tplSvc,
		Registry:     registry,
		LinkBuilder:  builder,
		LinkStore:    store,
		LinkObserver: observer,
		LinkPolicy:   policy,
		Logger:       &logger.Nop{},
		Config: config.DispatcherConfig{
			Enabled:              true,
			MaxRetries:           1,
			MaxWorkers:           1,
			EnvFallbackAllowlist: []string{testRecipient},
		},
	})
	if err != nil {
		t.Fatalf("dispatcher service: %v", err)
	}
	return svc, msgRepo, tplSvc
}

func seedTemplate(t *testing.T, svc *templates.Service, code, channel string) {
	t.Helper()
	_, err := svc.Create(context.Background(), templates.TemplateInput{
		Code:    code,
		Channel: channel,
		Locale:  "en",
		Subject: "Subject",
		Body:    "Body",
		Format:  "text/plain",
	})
	if err != nil {
		t.Fatalf("seed template: %v", err)
	}
}

func newTestTranslator(t *testing.T) i18n.Translator {
	t.Helper()
	translations := i18n.Translations{
		"en": newCatalog("en", map[string]string{}),
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
