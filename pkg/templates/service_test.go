package templates

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	i18n "github.com/goliatone/go-i18n"
	memstore "github.com/goliatone/go-notifications/internal/storage/memory"
	internaltemplates "github.com/goliatone/go-notifications/internal/templates"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
)

func TestServiceRenderUsesFallbackChain(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	cache := newMapCache()
	resolver := i18n.NewStaticFallbackResolver()
	resolver.Set("es-mx", "es", "en")
	svc := newTestService(t, repo, cache, resolver)

	seedTemplate(t, repo, domain.NotificationTemplate{
		Code:    "welcome",
		Channel: "email",
		Locale:  "en",
		Subject: `{{ t(locale, "welcome.subject", Name) }}`,
		Body:    `{{ t(locale, "welcome.body", Name) }}`,
		Format:  "text/html",
		Schema:  domain.TemplateSchema{Required: []string{"Name"}},
	})
	seedTemplate(t, repo, domain.NotificationTemplate{
		Code:    "welcome",
		Channel: "email",
		Locale:  "es",
		Subject: `{{ t(locale, "welcome.subject", Name) }}`,
		Body:    `{{ t(locale, "welcome.body", Name) }}`,
		Format:  "text/html",
		Schema:  domain.TemplateSchema{Required: []string{"Name"}},
	})

	result, err := svc.Render(ctx, RenderRequest{
		Code:    "welcome",
		Channel: "email",
		Locale:  "es-mx",
		Data: map[string]any{
			"Name": "Rosa",
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result.Locale != "es" {
		t.Fatalf("expected locale es, got %s", result.Locale)
	}
	if !result.UsedFallback {
		t.Fatalf("expected fallback to be used")
	}
	if _, ok := cache.values[cacheKey("welcome", "email", "es")]; !ok {
		t.Fatalf("expected cache to store es variant")
	}
}

func TestServiceSchemaValidation(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	svc := newTestService(t, repo, &cache.Nop{}, i18n.NewStaticFallbackResolver())

	seedTemplate(t, repo, domain.NotificationTemplate{
		Code:    "account.alert",
		Channel: "email",
		Locale:  "en",
		Subject: "Alert",
		Body:    "Hello {{ user.name }}",
		Format:  "text/plain",
		Schema:  domain.TemplateSchema{Required: []string{"user.name"}},
	})

	_, err := svc.Render(ctx, RenderRequest{
		Code:    "account.alert",
		Channel: "email",
		Locale:  "en",
		Data:    map[string]any{},
	})
	var schemaErr internaltemplates.SchemaError
	if err == nil || !errors.As(err, &schemaErr) {
		t.Fatalf("expected schema error, got %v", err)
	}

	_, err = svc.Render(ctx, RenderRequest{
		Code:    "account.alert",
		Channel: "email",
		Locale:  "en",
		Data: map[string]any{
			"user": map[string]any{"name": "Pat"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected render error: %v", err)
	}
}

func TestServiceSecureLinkHelper(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	svc := newTestService(t, repo, &cache.Nop{}, i18n.NewStaticFallbackResolver())

	seedTemplate(t, repo, domain.NotificationTemplate{
		Code:    "links.helper",
		Channel: "email",
		Locale:  "en",
		Subject: `{{ secure_link . }}`,
		Body:    `{{ secure_link . "manifest_url" }}`,
		Format:  "text/plain",
	})

	result, err := svc.Render(ctx, RenderRequest{
		Code:    "links.helper",
		Channel: "email",
		Locale:  "en",
		Data: map[string]any{
			"action_url":   "https://example.com/action",
			"manifest_url": "https://example.com/manifest",
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result.Subject != "https://example.com/action" {
		t.Fatalf("expected action_url subject, got %s", result.Subject)
	}
	if result.Body != "https://example.com/manifest" {
		t.Fatalf("expected manifest_url body, got %s", result.Body)
	}

	result, err = svc.Render(ctx, RenderRequest{
		Code:    "links.helper",
		Channel: "email",
		Locale:  "en",
		Data: map[string]any{
			"url": "https://example.com/fallback",
		},
	})
	if err != nil {
		t.Fatalf("render fallback: %v", err)
	}
	if result.Subject != "https://example.com/fallback" {
		t.Fatalf("expected url fallback, got %s", result.Subject)
	}
}

func TestServiceSupportsGoCMSPayloads(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	svc := newTestService(t, repo, &cache.Nop{}, i18n.NewStaticFallbackResolver())

	source := domain.TemplateSource{
		Type: "gocms-block",
		Payload: domain.JSONMap{
			"subject": "CMS Subject for {{ Title }}",
			"blocks": []any{
				map[string]any{
					"body": `<p>{{ t(locale, "welcome.body", Name) }}</p>`,
				},
			},
		},
	}

	seedTemplate(t, repo, domain.NotificationTemplate{
		Code:    "cms.block",
		Channel: "email",
		Locale:  "en",
		Source:  source,
		Format:  "text/html",
		Schema:  domain.TemplateSchema{Required: []string{"Name"}},
	})

	result, err := svc.Render(ctx, RenderRequest{
		Code:    "cms.block",
		Channel: "email",
		Locale:  "en",
		Data: map[string]any{
			"Name":  "Chris",
			"Title": "Launch",
		},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if result.Subject == "" || result.Body == "" {
		t.Fatalf("expected subject/body from go-cms payload")
	}
	if result.Metadata != nil {
		t.Fatalf("metadata should be nil when not set")
	}
	if result.UsedFallback {
		t.Fatalf("did not expect fallback for en locale")
	}
}

func TestServiceCreateAndUpdateBumpRevision(t *testing.T) {
	ctx := context.Background()
	repo := memstore.NewTemplateRepository()
	cache := newMapCache()
	svc := newTestService(t, repo, cache, i18n.NewStaticFallbackResolver())

	created, err := svc.Create(ctx, TemplateInput{
		Code:    "reminder",
		Channel: "sms",
		Locale:  "en",
		Subject: "ignored for sms",
		Body:    "Initial {{ Message }}",
		Format:  "text/plain",
		Schema:  domain.TemplateSchema{Required: []string{"Message"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Revision != 1 {
		t.Fatalf("expected revision 1, got %d", created.Revision)
	}

	updated, err := svc.Update(ctx, TemplateInput{
		Code:    "reminder",
		Channel: "sms",
		Locale:  "en",
		Body:    "Updated {{ Message }}",
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", updated.Revision)
	}
	if updated.Body != "Updated {{ Message }}" {
		t.Fatalf("body not updated: %s", updated.Body)
	}
	if _, ok := cache.values[cacheKey("reminder", "sms", "en")]; !ok {
		t.Fatalf("expected cache to store sms template")
	}
}

// Helpers

func newTestService(t *testing.T, repo *memstore.TemplateRepository, cache cache.Cache, resolver i18n.FallbackResolver) *Service {
	t.Helper()
	translator := newTestTranslator(t)
	svc, err := New(Dependencies{
		Repository:    repo,
		Cache:         cache,
		Logger:        &logger.Nop{},
		Translator:    translator,
		Fallbacks:     resolver,
		DefaultLocale: "en",
		CacheTTL:      time.Minute,
	})
	if err != nil {
		t.Fatalf("New service: %v", err)
	}
	return svc
}

func newTestTranslator(t *testing.T) i18n.Translator {
	t.Helper()
	translations := i18n.Translations{
		"en": newCatalog("en", map[string]string{
			"welcome.subject": "Welcome %s",
			"welcome.body":    "Hello %s",
		}),
		"es": newCatalog("es", map[string]string{
			"welcome.subject": "Bienvenida %s",
			"welcome.body":    "Hola %s",
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

func seedTemplate(t *testing.T, repo *memstore.TemplateRepository, tpl domain.NotificationTemplate) {
	t.Helper()
	if err := repo.Create(context.Background(), &tpl); err != nil {
		t.Fatalf("seed template: %v", err)
	}
}

type mapCache struct {
	mu     sync.RWMutex
	values map[string]cacheEntry
}

type cacheEntry struct {
	value any
	ttl   time.Duration
}

func newMapCache() *mapCache {
	return &mapCache{
		values: make(map[string]cacheEntry),
	}
}

func (m *mapCache) Get(ctx context.Context, key string) (any, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.values[key]
	return entry.value, ok, nil
}

func (m *mapCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = cacheEntry{value: value, ttl: ttl}
	return nil
}

func (m *mapCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, key)
	return nil
}
