package dispatcher

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/goliatone/go-notifications/pkg/links"
	"github.com/goliatone/go-notifications/pkg/persistencepolicy"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/receipts"
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
	mu    sync.Mutex
	calls []links.LinkResolution
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

type failingAttemptAdapter struct {
	name  string
	calls int
}

type captureInbox struct {
	messages []*domain.NotificationMessage
}

func (i *captureInbox) DeliverFromMessage(_ context.Context, message *domain.NotificationMessage) error {
	i.messages = append(i.messages, message)
	return nil
}

func (a *failingAttemptAdapter) Name() string { return a.name }

func (a *failingAttemptAdapter) Capabilities() adapters.Capability {
	return adapters.Capability{Name: a.name, Channels: []string{"email"}, Formats: []string{"text/plain"}}
}

func (a *failingAttemptAdapter) Send(context.Context, adapters.Message) error {
	a.calls++
	return errors.New("injected failure")
}

type zeroBackoff struct{}

func (zeroBackoff) Next(int) time.Duration { return 0 }

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

func TestNewRejectsInvalidDispatcherConfig(t *testing.T) {
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
		t.Fatalf("template service: %v", err)
	}
	registry := adapters.NewRegistry(&testAdapter{name: "test", channels: []string{"email"}})

	_, err = New(Dependencies{
		Definitions: defRepo,
		Templates:   tplSvc,
		Registry:    registry,
		Config: config.DispatcherConfig{
			MaxAttempts: 0,
			MaxWorkers:  1,
		},
	})
	if err == nil {
		t.Fatalf("expected invalid config error")
	}
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestNewDoesNotMutateProvidedConfig(t *testing.T) {
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
		t.Fatalf("template service: %v", err)
	}
	registry := adapters.NewRegistry(&testAdapter{name: "test", channels: []string{"email"}})
	inputCfg := config.DispatcherConfig{
		Enabled:     true,
		MaxAttempts: 2,
		MaxWorkers:  3,
	}

	svc, err := New(Dependencies{
		Definitions: defRepo,
		Templates:   tplSvc,
		Registry:    registry,
		Config:      inputCfg,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if inputCfg.MaxAttempts != 2 || inputCfg.MaxWorkers != 3 {
		t.Fatalf("expected input config to remain unchanged: %+v", inputCfg)
	}
	if svc.cfg.MaxAttempts != 2 || svc.cfg.MaxWorkers != 3 {
		t.Fatalf("expected service config to preserve provided values: %+v", svc.cfg)
	}
}

func TestDeliverWithRetriesHonorsMaxAttempts(t *testing.T) {
	messenger := &failingAttemptAdapter{name: "failing"}
	svc := &Service{
		cfg: config.DispatcherConfig{
			MaxAttempts: 2,
			MaxWorkers:  1,
		},
		backoff: zeroBackoff{},
		logger:  &logger.Nop{},
	}
	msg := &domain.NotificationMessage{}

	err := svc.deliverWithRetries(context.Background(), messenger, msg, adapters.Message{})
	if err == nil {
		t.Fatalf("expected delivery error")
	}
	if messenger.calls != 2 {
		t.Fatalf("expected 2 attempts, got %d", messenger.calls)
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

func TestTransientDeliveryReachesAdapterButNotPersistence(t *testing.T) {
	ctx := context.Background()
	marker := "one-time-credential-marker"
	builder := &captureLinkBuilder{
		buildFn: func(req links.LinkRequest) (links.ResolvedLinks, error) {
			return links.ResolvedLinks{
				ActionURL: "https://private.invalid/" + marker,
				Metadata:  map[string]any{"token": marker},
				Records: []links.LinkRecord{{
					ID: "link-1", URL: "https://private.invalid/" + marker,
					Metadata: map[string]any{"token": marker},
				}},
			}, nil
		},
	}
	storeSpy := &captureStore{}
	observer := &captureObserver{}
	adapter := &testAdapter{name: "test", channels: []string{"email"}}
	svc, msgRepo, tplSvc := newTestDispatcher(t, builder, storeSpy, observer, links.FailurePolicy{}, adapter)
	svc.cfg.EnvFallbackAllowlist = append(svc.cfg.EnvFallbackAllowlist, "subject-1")
	seedTemplate(t, tplSvc, "secure-email", "email")
	definition := &domain.NotificationDefinition{
		Code:         "secure",
		Channels:     domain.StringList{"email"},
		TemplateKeys: domain.StringList{"email:secure-email"},
	}
	event := &domain.NotificationEvent{
		RecordMeta:         domain.RecordMeta{ID: uuid.New()},
		DefinitionCode:     definition.Code,
		Recipients:         domain.StringList{"subject-1"},
		Context:            domain.JSONMap{"safe": "persisted"},
		TransientDependent: true,
	}
	decision := persistencepolicy.WithTransientOverlay(persistencepolicy.Decision{
		MessageMode:        persistencepolicy.Full,
		InboxMode:          persistencepolicy.Full,
		PersistLinkURLs:    true,
		PersistLinkRecords: true,
		AllowedMetadata:    []string{"html_body"},
	}, true)
	job := deliveryJob{
		event: event, channel: "email", templateCode: "secure-email",
		recipient: "subject-1", locale: "en",
		transient: map[string]any{
			"channel_overrides": map[string]any{
				"email": map[string]any{"body": marker, "html_body": marker},
			},
		},
		persistence: decision,
	}
	if err := svc.processDelivery(ctx, event, definition, job); err != nil {
		t.Fatalf("transient delivery: %v", err)
	}
	assertTransientDelivery(t, ctx, marker, svc, msgRepo, builder, storeSpy, observer, adapter)
}

func assertTransientDelivery(
	t *testing.T,
	ctx context.Context,
	marker string,
	svc *Service,
	msgRepo *memory.MessageRepository,
	builder *captureLinkBuilder,
	storeSpy *captureStore,
	observer *captureObserver,
	adapter *testAdapter,
) {
	t.Helper()
	if adapter.Count() != 1 || adapter.sends[0].Body != marker {
		t.Fatalf("adapter did not receive transient render: %+v", adapter.sends)
	}
	builder.mu.Lock()
	builderPayload := builder.calls[0].Payload
	builder.mu.Unlock()
	overrides, ok := builderPayload["channel_overrides"].(map[string]any)
	if !ok || len(overrides) == 0 {
		t.Fatalf("link builder did not receive transient payload: %+v", builderPayload)
	}
	messages, err := msgRepo.List(ctx, store.ListOptions{})
	if err != nil || messages.Total != 1 {
		t.Fatalf("list messages: %+v err=%v", messages, err)
	}
	assertRedactedMessage(t, messages.Items[0])
	if storeSpy.calls != 0 {
		t.Fatalf("transient delivery persisted %d link records", storeSpy.calls)
	}
	observer.mu.Lock()
	observerCalls := len(observer.calls)
	observer.mu.Unlock()
	if observerCalls != 0 {
		t.Fatalf("transient delivery emitted %d link observations", observerCalls)
	}
	attempts, err := svc.attempts.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list attempts: %v", err)
	}
	encoded, err := json.Marshal(attempts)
	if err != nil || strings.Contains(string(encoded), marker) {
		t.Fatalf("transient marker leaked into attempts: %s err=%v", encoded, err)
	}
}

func assertRedactedMessage(t *testing.T, persisted domain.NotificationMessage) {
	t.Helper()
	if persisted.Subject != "" || persisted.Body != "" || persisted.ActionURL != "" ||
		len(persisted.Metadata) != 0 || persisted.Receiver != "subject-1" {
		t.Fatalf("transient content leaked into message projection: %+v", persisted)
	}
}

func TestTransientInboxDeliveryFailsClosedBeforeInboxPersistence(t *testing.T) {
	ctx := context.Background()
	adapter := &testAdapter{name: "test", channels: []string{"email"}}
	svc, msgRepo, tplSvc := newTestDispatcher(t, nil, nil, nil, links.FailurePolicy{}, adapter)
	inboxSink := &captureInbox{}
	svc.inbox = inboxSink
	seedTemplate(t, tplSvc, "secure-inbox", "inbox")
	definition := &domain.NotificationDefinition{
		Code:         "secure-inbox",
		Channels:     domain.StringList{"inbox"},
		TemplateKeys: domain.StringList{"inbox:secure-inbox"},
	}
	event := &domain.NotificationEvent{
		RecordMeta:         domain.RecordMeta{ID: uuid.New()},
		DefinitionCode:     definition.Code,
		Recipients:         domain.StringList{"subject-1"},
		TransientDependent: true,
	}
	job := deliveryJob{
		event: event, channel: "inbox", templateCode: "secure-inbox",
		recipient: "subject-1", locale: "en",
		transient: map[string]any{"private": "one-time-marker"},
		persistence: persistencepolicy.WithTransientOverlay(persistencepolicy.Decision{
			MessageMode: persistencepolicy.Full,
			InboxMode:   persistencepolicy.Full,
		}, true),
	}
	if err := svc.processDelivery(ctx, event, definition, job); err != nil {
		t.Fatalf("transient inbox delivery: %v", err)
	}
	if len(inboxSink.messages) != 0 {
		t.Fatalf("transient content reached inbox persistence: %+v", inboxSink.messages)
	}
	messages, err := msgRepo.List(ctx, store.ListOptions{})
	if err != nil || messages.Total != 1 ||
		messages.Items[0].Status != domain.MessageStatusSkipped ||
		messages.Items[0].Body != "" {
		t.Fatalf("unexpected durable inbox projection: %+v err=%v", messages, err)
	}
}

type persistenceModeTestCase struct {
	name            string
	decision        persistencepolicy.Decision
	wantBody        string
	wantCampaign    bool
	wantAnyMetadata bool
}

func TestPersistenceModesKeepAdapterContentAndUpdatesRedacted(t *testing.T) {
	tests := []persistenceModeTestCase{
		{
			name: "full",
			decision: persistencepolicy.Decision{
				MessageMode:     persistencepolicy.Full,
				InboxMode:       persistencepolicy.Full,
				PersistLinkURLs: true, PersistLinkRecords: true,
			},
			wantBody: "Private body", wantCampaign: true, wantAnyMetadata: true,
		},
		{
			name: "metadata only",
			decision: persistencepolicy.Decision{
				MessageMode:     persistencepolicy.MetadataOnly,
				InboxMode:       persistencepolicy.MetadataOnly,
				AllowedMetadata: []string{"campaign_id", "secret", "html_body"},
			},
			wantCampaign: true, wantAnyMetadata: true,
		},
		{
			name: "state only",
			decision: persistencepolicy.Decision{
				MessageMode: persistencepolicy.StateOnly,
				InboxMode:   persistencepolicy.StateOnly,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { runPersistenceModeCase(t, tc) })
	}
}

func runPersistenceModeCase(t *testing.T, tc persistenceModeTestCase) {
	ctx := context.Background()
	adapter := &testAdapter{name: "test", channels: []string{"email"}}
	svc, msgRepo, tplSvc := newTestDispatcher(t, nil, nil, nil, links.FailurePolicy{}, adapter)
	svc.cfg.EnvFallbackAllowlist = append(svc.cfg.EnvFallbackAllowlist, "subject-1")
	_, err := tplSvc.Create(ctx, templates.TemplateInput{
		Code: "policy-email", Channel: "email", Locale: "en",
		Subject: "Private subject", Body: "Private body", Format: "text/plain",
		Metadata: domain.JSONMap{
			"campaign_id": "campaign-1",
			"secret":      "private",
			"html_body":   "private html",
		},
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	definition := &domain.NotificationDefinition{
		Code: "policy", Channels: domain.StringList{"email"},
		TemplateKeys: domain.StringList{"email:policy-email"},
	}
	event := &domain.NotificationEvent{
		RecordMeta:     domain.RecordMeta{ID: uuid.New()},
		DefinitionCode: definition.Code, Recipients: domain.StringList{"subject-1"},
	}
	job := deliveryJob{
		event: event, channel: "email", templateCode: "policy-email",
		recipient: "subject-1", locale: "en", persistence: tc.decision,
	}
	if deliveryErr := svc.processDelivery(ctx, event, definition, job); deliveryErr != nil {
		t.Fatalf("process delivery: %v", deliveryErr)
	}
	if adapter.Count() != 1 || adapter.sends[0].Body != "Private body" {
		t.Fatalf("adapter did not receive full content: %+v", adapter.sends)
	}
	messages, err := msgRepo.List(ctx, store.ListOptions{})
	if err != nil || messages.Total != 1 {
		t.Fatalf("list messages: %+v err=%v", messages, err)
	}
	message := messages.Items[0]
	if message.Body != tc.wantBody {
		t.Fatalf("persisted body = %q, want %q", message.Body, tc.wantBody)
	}
	if got := message.Metadata["campaign_id"] == "campaign-1"; got != tc.wantCampaign {
		t.Fatalf("campaign metadata presence = %v, want %v: %+v", got, tc.wantCampaign, message.Metadata)
	}
	if (len(message.Metadata) > 0) != tc.wantAnyMetadata {
		t.Fatalf("metadata presence mismatch: %+v", message.Metadata)
	}
	if tc.decision.MessageMode != persistencepolicy.Full &&
		(message.Subject != "" || message.ActionURL != "" ||
			message.Metadata["secret"] != nil || message.Metadata["html_body"] != nil) {
		t.Fatalf("redacted update restored private content: %+v", message)
	}
}

func TestStrictDefinitionPolicyFailurePreventsAdapterDelivery(t *testing.T) {
	ctx := context.Background()
	adapter := &testAdapter{name: "test", channels: []string{"email"}}
	svc, _, tplSvc := newTestDispatcher(t, nil, nil, nil, links.FailurePolicy{}, adapter)
	seedTemplate(t, tplSvc, "strict-email", "email")
	definition := &domain.NotificationDefinition{
		Code: "strict", Channels: domain.StringList{"email"},
		TemplateKeys: domain.StringList{"email:strict-email"},
		Policy:       domain.JSONMap{"persistence_mode": "invalid"},
	}
	if err := svc.definitions.Create(ctx, definition); err != nil {
		t.Fatalf("create definition: %v", err)
	}
	event := &domain.NotificationEvent{
		DefinitionCode: "strict", Recipients: domain.StringList{testRecipient},
	}
	if err := svc.events.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}
	svc.persistence = persistencepolicy.DefinitionPolicy{}
	receipt, err := svc.DispatchWithReceipt(ctx, event, DispatchOptions{})
	var safe privacy.SafeError
	if !errors.As(err, &safe) || receipt.Status != receipts.StatusFailed {
		t.Fatalf("expected safe strict-policy failure, receipt=%+v err=%v", receipt, err)
	}
	if adapter.Count() != 0 {
		t.Fatalf("strict policy failure delivered %d messages", adapter.Count())
	}
}

func TestLinkObserverReceivesSanitizedProjection(t *testing.T) {
	ctx := context.Background()
	storeSpy := &captureStore{}
	observer := &captureObserver{}
	svc := &Service{
		linkStore: storeSpy, linkObserver: observer,
		linkPolicy: links.FailurePolicy{}, privacy: privacy.DefaultPolicy{},
	}
	request := links.LinkRequest{
		Recipient: "person@example.com",
		Payload: map[string]any{
			"action_url": "https://private.invalid/action",
			"nested":     domain.JSONMap{"token": "private", "result": "ok"},
		},
		ResolvedURLs: map[string]string{"action_url": "https://private.invalid/action"},
	}
	resolved := links.ResolvedLinks{
		ActionURL: "https://private.invalid/action",
		Metadata:  map[string]any{"token": "private", "result": "ok"},
		Records: []links.LinkRecord{{
			URL: "https://private.invalid/action", Recipient: "person@example.com",
			Metadata: map[string]any{"token": "private", "result": "ok"},
		}},
	}
	if err := svc.invokeLinkHooks(ctx, request, resolved, true, true); err != nil {
		t.Fatalf("invoke link hooks: %v", err)
	}
	if storeSpy.calls != 1 || storeSpy.records[0][0].URL == "" {
		t.Fatalf("full persistence store did not receive durable URL projection: %+v", storeSpy.records)
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if len(observer.calls) != 1 {
		t.Fatalf("expected one observer call, got %d", len(observer.calls))
	}
	got := observer.calls[0]
	if got.Request.ResolvedURLs != nil || got.Resolved.ActionURL != "" ||
		got.Resolved.Metadata["token"] != nil || got.Resolved.Records[0].URL != "" ||
		got.Resolved.Records[0].Metadata["token"] != nil {
		t.Fatalf("observer received unsafe link projection: %+v", got)
	}
}

func TestDispatcherLinkHooksErrorHandling(t *testing.T) {
	t.Run("lenient store error continues", testLenientLinkStoreError)
	t.Run("lenient builder error triggers observer only", testLenientLinkBuilderError)
	t.Run("strict store error stops delivery", testStrictLinkStoreError)
}

func testLenientLinkStoreError(t *testing.T) {
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
}

func testLenientLinkBuilderError(t *testing.T) {
	ctx := context.Background()
	builder := &captureLinkBuilder{
		buildFn: func(req links.LinkRequest) (links.ResolvedLinks, error) {
			return links.ResolvedLinks{}, errors.New("builder failed")
		},
	}
	adapter := &testAdapter{name: "test", channels: []string{"email"}}
	storeSpy := &captureStore{}
	observer := &captureObserver{}
	svc, msgRepo, tplSvc := newTestDispatcher(t, builder, storeSpy, observer, links.FailurePolicy{
		Builder: links.FailureLenient,
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
		Context: domain.JSONMap{
			"action_url": "fallback-url",
		},
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
	if storeSpy.calls != 0 {
		t.Fatalf("expected no store calls, got %d", storeSpy.calls)
	}
	observer.mu.Lock()
	observerCalls := len(observer.calls)
	observer.mu.Unlock()
	if observerCalls != 1 {
		t.Fatalf("expected observer call, got %d", observerCalls)
	}
}

func testStrictLinkStoreError(t *testing.T) {
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
			MaxAttempts:          1,
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
