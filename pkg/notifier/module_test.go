package notifier

import (
	"testing"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/storage"
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
}

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
