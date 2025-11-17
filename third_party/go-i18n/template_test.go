package i18n

import (
	"testing"
	"time"
)

func TestTemplateHelpersTranslateInferredLocale(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{
			"home.title": "Welcome",
		}),
	})

	translator, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	helpers := TemplateHelpers(translator, HelperConfig{LocaleKey: "current_locale"})

	translate, ok := helpers["translate"].(func(any, string, ...any) string)
	if !ok {
		t.Fatalf("translate helper signature mismatch: %T", helpers["translate"])
	}

	ctx := map[string]any{"current_locale": "en"}

	if got := translate(ctx, "home.title"); got != "Welcome" {
		t.Fatalf("translate inferred locale = %q", got)
	}

	if got := translate("en", "home.title"); got != "Welcome" {
		t.Fatalf("translate explicit locale = %q", got)
	}
}

func TestTemplateHelpersMissingTranslationHandler(t *testing.T) {
	translator, err := NewSimpleTranslator(NewStaticStore(nil), WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	var called bool
	onMissing := func(locale, key string, args []any, err error) string {
		called = true
		if locale != "en" {
			t.Fatalf("expected locale en, got %q", locale)
		}
		if err != ErrMissingTranslation {
			t.Fatalf("unexpected error: %v", err)
		}
		return "missing"
	}

	helpers := TemplateHelpers(translator, HelperConfig{
		LocaleKey: "locale",
		OnMissing: onMissing,
	})

	translate := helpers["translate"].(func(any, string, ...any) string)

	ctx := map[string]any{"locale": "en"}

	if got := translate(ctx, "unknown"); got != "missing" {
		t.Fatalf("translate missing = %q", got)
	}

	if !called {
		t.Fatal("expected missing handler invocation")
	}
}

func TestTemplateHelpersCurrentLocaleHelper(t *testing.T) {
	helpers := TemplateHelpers(nil, HelperConfig{LocaleKey: "locale"})

	currentLocale := helpers["current_locale"].(func(any) string)

	ctx := map[string]string{"locale": "es"}
	if got := currentLocale(ctx); got != "es" {
		t.Fatalf("current_locale helper = %q", got)
	}

	if got := currentLocale("fr"); got != "fr" {
		t.Fatalf("current_locale fallback string = %q", got)
	}
}

func TestTemplateHelpersFormatterUsesProvider(t *testing.T) {
	registry := NewFormatterRegistry()

	registry.RegisterProvider("es", func(_ string) map[string]any {
		return map[string]any{
			"format_number": func(_ string, value float64, decimals int) string {
				return "es" // distinctive output to prove provider usage
			},
		}
	})

	helpers := TemplateHelpers(nil, HelperConfig{Registry: registry})

	formatNumber, ok := helpers["format_number"].(func(string, float64, int) string)
	if !ok {
		t.Fatalf("format_number helper signature mismatch: %T", helpers["format_number"])
	}

	if got := formatNumber("es", 12.34, 2); got != "es" {
		t.Fatalf("format_number provider output = %q", got)
	}

	if got := formatNumber("en", 12.34, 2); got != FormatNumber("en", 12.34, 2) {
		t.Fatalf("format_number default output = %q", got)
	}
}

func TestTemplateHelpersFormatterCurrencyProvider(t *testing.T) {
	registry := NewFormatterRegistry()

	registry.RegisterProvider("fr", func(_ string) map[string]any {
		return map[string]any{
			"format_currency": func(_ string, amount float64, currency string) string {
				return "fr"
			},
		}
	})

	helpers := TemplateHelpers(nil, HelperConfig{Registry: registry})

	formatCurrency, ok := helpers["format_currency"].(func(string, float64, string) string)
	if !ok {
		t.Fatalf("format_currency helper signature mismatch: %T", helpers["format_currency"])
	}

	if got := formatCurrency("fr", 10, "EUR"); got != "fr" {
		t.Fatalf("format_currency provider output = %q", got)
	}

	if got := formatCurrency("en", 10, "USD"); got != "$ 10.00" {
		t.Fatalf("format_currency default output = %q", got)
	}
}

func TestTemplateHelpersCustomTranslateKey(t *testing.T) {
	helpers := TemplateHelpers(nil, HelperConfig{TemplateHelperKey: "t"})

	helper, ok := helpers["t"].(func(any, string, ...any) string)
	if !ok {
		t.Fatalf("custom translate helper missing: %T", helpers["t"])
	}

	if got := helper("", "foo"); got != "foo" {
		t.Fatalf("custom translate fallback = %q", got)
	}
}

// Regression test: es-MX should inherit Spanish formatting from es locale
func TestTemplateHelpers_RegionalLocaleFallback_esMX(t *testing.T) {
	// Use standalone TemplateHelpers without a pre-configured registry
	helpers := TemplateHelpers(nil, HelperConfig{})

	formatDate, ok := helpers["format_date"].(func(string, time.Time) string)
	if !ok {
		t.Fatalf("format_date helper missing: %T", helpers["format_date"])
	}

	testDate := time.Date(2025, 10, 7, 0, 0, 0, 0, time.UTC)

	// es-MX should inherit Spanish formatting (not English or ISO)
	got := formatDate("es-MX", testDate)
	want := "7 de octubre de 2025"

	if got != want {
		t.Errorf("format_date(es-MX) = %q; want %q (Spanish format, not English)", got, want)
	}
}

// Regression test: el (Greek) should fallback to English
func TestTemplateHelpers_FallbackOnlyLocale_el(t *testing.T) {
	// Use standalone TemplateHelpers without a pre-configured registry
	helpers := TemplateHelpers(nil, HelperConfig{})

	formatDate, ok := helpers["format_date"].(func(string, time.Time) string)
	if !ok {
		t.Fatalf("format_date helper missing: %T", helpers["format_date"])
	}

	testDate := time.Date(2025, 10, 7, 0, 0, 0, 0, time.UTC)

	// el should fallback to English formatting
	got := formatDate("el", testDate)
	want := "October 7, 2025"

	if got != want {
		t.Errorf("format_date(el) = %q; want %q (English fallback)", got, want)
	}
}

// Test that es base locale still works correctly
func TestTemplateHelpers_BaseLocale_es(t *testing.T) {
	helpers := TemplateHelpers(nil, HelperConfig{})

	formatDate, ok := helpers["format_date"].(func(string, time.Time) string)
	if !ok {
		t.Fatalf("format_date helper missing: %T", helpers["format_date"])
	}

	testDate := time.Date(2025, 10, 7, 0, 0, 0, 0, time.UTC)

	got := formatDate("es", testDate)
	want := "7 de octubre de 2025"

	if got != want {
		t.Errorf("format_date(es) = %q; want %q", got, want)
	}
}

// Test multi-part locale tags (e.g., zh-Hant-HK → zh-Hant → zh → en)
func TestTemplateHelpers_MultiPartLocaleFallback(t *testing.T) {
	helpers := TemplateHelpers(nil, HelperConfig{})

	formatDate, ok := helpers["format_date"].(func(string, time.Time) string)
	if !ok {
		t.Fatalf("format_date helper missing: %T", helpers["format_date"])
	}

	testDate := time.Date(2025, 10, 7, 0, 0, 0, 0, time.UTC)

	// zh-Hant-HK should fall through zh-Hant → zh → en
	// Since we don't have Chinese providers, it should ultimately use English
	got := formatDate("zh-Hant-HK", testDate)
	want := "October 7, 2025" // English fallback

	if got != want {
		t.Errorf("format_date(zh-Hant-HK) = %q; want %q (should fallback through full chain to English)", got, want)
	}

	// Verify the same for pt-BR (should fall through pt → en)
	got2 := formatDate("pt-BR", testDate)
	want2 := "October 7, 2025" // English fallback

	if got2 != want2 {
		t.Errorf("format_date(pt-BR) = %q; want %q (should fallback through pt to English)", got2, want2)
	}
}
