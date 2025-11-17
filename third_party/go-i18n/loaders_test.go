package i18n

import (
	"path/filepath"
	"testing"
)

func TestFileLoaderJSONAndYAML(t *testing.T) {
	loader := NewFileLoader(
		filepath.Join("testdata", "loader_en.json"),
		filepath.Join("testdata", "loader_es.yaml"),
	).WithPluralRuleFiles(filepath.Join("testdata", "cldr_cardinal.json"))

	translations, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(translations) != 2 {
		t.Fatalf("expected 2 locales, got %d", len(translations))
	}

	es := translations["es"]
	if es == nil {
		t.Fatalf("missing es catalog")
	}
	if es.CardinalRules == nil {
		t.Fatalf("expected cardinal rules for es")
	}
	if es.Locale.Name != "Spanish" {
		t.Fatalf("expected locale name Spanish, got %q", es.Locale.Name)
	}
	if es.Messages["home.title"].Content() != "Bienvenido" {
		t.Fatalf("unexpected translation for es: %v", es.Messages["home.title"].Content())
	}

	en := translations["en"]
	if en == nil {
		t.Fatalf("missing en catalog")
	}
	msg := en.Messages["cart.items"]
	if msg.Content() != "You have {count} items" {
		t.Fatalf("expected other variant, got %q", msg.Content())
	}
	oneVariant, ok := msg.Variant(PluralOne)
	if !ok || oneVariant.Template != "You have {count} item" {
		t.Fatalf("missing plural variant: %+v", oneVariant)
	}
	if !oneVariant.UsesCount {
		t.Fatalf("expected UsesCount=true for plural variant")
	}
}

func TestFileLoaderUnsupportedExtension(t *testing.T) {
	loader := NewFileLoader(filepath.Join("testdata", "loader_en.json"), "unsupported.txt")

	if _, err := loader.Load(); err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}

func TestFileLoaderIntegration(t *testing.T) {
	loader := NewFileLoader(
		filepath.Join("testdata", "loader_en.json"),
		filepath.Join("testdata", "loader_es.yaml"),
	).WithPluralRuleFiles(filepath.Join("testdata", "cldr_cardinal.json"))

	cfg, err := NewConfig(
		WithLoader(loader),
		WithDefaultLocale("en"),
		WithFallback("es", "en"),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	translator, err := cfg.BuildTranslator()
	if err != nil {
		t.Fatalf("BuildTranslator: %v", err)
	}

	if got, err := translator.Translate("es", "home.title"); err != nil || got != "Bienvenido" {
		t.Fatalf("Translate es/home.title = %q,%v", got, err)
	}

	if got, err := translator.Translate("fr", "home.greeting", "Carlos"); err != nil || got != "Hello Carlos" {
		t.Fatalf("Translate fallback = %q,%v", got, err)
	}
}
