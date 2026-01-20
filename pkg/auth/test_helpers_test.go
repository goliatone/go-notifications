package auth

import (
	"testing"

	i18n "github.com/goliatone/go-i18n"
	memstore "github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/templates"
)

func newTranslator(t *testing.T) i18n.Translator {
	t.Helper()
	translator, err := i18n.NewSimpleTranslator(
		i18n.NewStaticStore(nil),
		i18n.WithTranslatorDefaultLocale("en"),
	)
	if err != nil {
		t.Fatalf("translator: %v", err)
	}
	return translator
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
