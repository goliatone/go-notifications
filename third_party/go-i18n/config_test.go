package i18n

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNewConfigDefaults(t *testing.T) {
	cfg, err := NewConfig(
		WithLocales("es", "en", "en"),
		WithDefaultLocale("es"),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	if cfg.DefaultLocale != "es" {
		t.Fatalf("DefaultLocale = %q", cfg.DefaultLocale)
	}

	expected := []string{"en", "es"}
	if len(cfg.Locales) != len(expected) {
		t.Fatalf("Locales length = %d, want %d", len(cfg.Locales), len(expected))
	}
	for i, locale := range expected {
		if cfg.Locales[i] != locale {
			t.Fatalf("Locales[%d] = %q, want %q", i, cfg.Locales[i], locale)
		}
	}

	if cfg.Store == nil {
		t.Fatal("expected default store")
	}

	if cfg.Formatter == nil {
		t.Fatal("expected default formatter")
	}

	if cfg.Resolver == nil {
		t.Fatal("expected fallback resolver")
	}
}

func TestNewConfigWithLoader(t *testing.T) {
	loader := LoaderFunc(func() (Translations, error) {
		return Translations{
			"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
		}, nil
	})

	cfg, err := NewConfig(WithLoader(loader))
	if err != nil {
		t.Fatalf("NewConfig with loader: %v", err)
	}

	msg, ok := cfg.Store.Get("en", "home.title")
	if !ok || msg != "Welcome" {
		t.Fatalf("store lookup returned %q,%v", msg, ok)
	}
}

func TestConfigWithFallbackOption(t *testing.T) {
	cfg, err := NewConfig(
		WithFallback("es", "en", "fr", "en"),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	chain := cfg.Resolver.Resolve("es")

	expected := []string{"en", "fr"}
	if len(chain) != len(expected) {
		t.Fatalf("fallback chain length = %d want %d", len(chain), len(expected))
	}

	for i, locale := range expected {
		if chain[i] != locale {
			t.Fatalf("fallback[%d] = %q want %q", i, chain[i], locale)
		}
	}
}

func TestBuildTranslator(t *testing.T) {
	cfg, err := NewConfig(
		WithLocales("en"),
		WithDefaultLocale("en"),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	t.Setenv("_", "unused")

	translator, err := cfg.BuildTranslator()
	if err != nil {
		t.Fatalf("BuildTranslator: %v", err)
	}

	if translator == nil {
		t.Fatal("expected translator instance")
	}
}

func TestBuildTranslatorUsesFallback(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
	})

	cfg, err := NewConfig(
		WithStore(store),
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

	got, err := translator.Translate("es", "home.title")
	if err != nil {
		t.Fatalf("Translate with fallback: %v", err)
	}

	if got != "Welcome" {
		t.Fatalf("Translate() = %q want Welcome", got)
	}
}

func TestConfigBuildTranslatorNil(t *testing.T) {
	var cfg *Config
	translator, err := cfg.BuildTranslator()
	if err != ErrNotImplemented || translator != nil {
		t.Fatalf("expected ErrNotImplemented, got (%v, %v)", err, translator)
	}
}

func TestBuildTranslatorAppliesHooks(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
	})

	var before, after int
	hook := TranslationHookFuncs{
		Before: func(ctx *TranslatorHookContext) { before++ },
		After: func(ctx *TranslatorHookContext) {
			after++
		},
	}

	cfg, err := NewConfig(
		WithStore(store),
		WithDefaultLocale("en"),
		WithTranslatorHooks(hook),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	translator, err := cfg.BuildTranslator()
	if err != nil {
		t.Fatalf("BuildTranslator: %v", err)
	}

	if _, err := translator.Translate("en", "home.title"); err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if before != 1 || after != 1 {
		t.Fatalf("expected hook counts 1/1, got %d/%d", before, after)
	}
}

func TestEnablePluralizationAddsRuleFiles(t *testing.T) {
	rulePath := filepath.Join("testdata", "cldr_cardinal.json")
	loader := NewFileLoader(filepath.Join("testdata", "loader_en.json"))

	cfg, err := NewConfig(
		WithLoader(loader),
		EnablePluralization(rulePath),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	fileLoader, ok := cfg.Loader.(*FileLoader)
	if !ok {
		t.Fatalf("expected FileLoader, got %[1]T", cfg.Loader)
	}

	if len(fileLoader.rulePaths) != 1 || fileLoader.rulePaths[0] != rulePath {
		t.Fatalf("rulePaths = %#v, want [%q]", fileLoader.rulePaths, rulePath)
	}

	if rules, ok := cfg.Store.Rules("en"); !ok || rules == nil {
		t.Fatal("expected plural rules to be loaded for en")
	}
}

func TestEnablePluralizationDoesNotSeedFallbacksByDefault(t *testing.T) {
	rulePath := filepath.Join("testdata", "cldr_cardinal.json")
	loader := NewFileLoader(filepath.Join("testdata", "loader_en.json"))

	cfg, err := NewConfig(
		WithLoader(loader),
		WithLocales("en-US", "en"),
		WithDefaultLocale("en-US"),
		EnablePluralization(rulePath),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	if _, err := cfg.BuildTranslator(); err != nil {
		t.Fatalf("BuildTranslator: %v", err)
	}

	resolver, ok := cfg.Resolver.(*StaticFallbackResolver)
	if !ok {
		t.Fatalf("expected StaticFallbackResolver, got %[1]T", cfg.Resolver)
	}

	chain := resolver.Resolve("en-US")
	if len(chain) != 0 {
		t.Fatalf("expected no fallback chain, got %#v", chain)
	}
}

func TestEnablePluralizationSeedsParentFallbacksWhenOptedIn(t *testing.T) {
	rulePath := filepath.Join("testdata", "cldr_cardinal.json")
	loader := NewFileLoader(filepath.Join("testdata", "loader_en.json"))

	cfg, err := NewConfig(
		WithLoader(loader),
		WithLocales("en-US", "en"),
		WithDefaultLocale("en-US"),
		EnablePluralization(rulePath),
		EnablePluralFallbackSeeding(),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	if _, err := cfg.BuildTranslator(); err != nil {
		t.Fatalf("BuildTranslator: %v", err)
	}

	resolver, ok := cfg.Resolver.(*StaticFallbackResolver)
	if !ok {
		t.Fatalf("expected StaticFallbackResolver, got %[1]T", cfg.Resolver)
	}

	chain := resolver.Resolve("en-US")
	if len(chain) != 1 || chain[0] != "en" {
		t.Fatalf("expected fallback chain [en], got %#v", chain)
	}
}

func TestConfig_CultureService(t *testing.T) {
	// Create test culture data file
	tmpDir := t.TempDir()
	cultureFile := filepath.Join(tmpDir, "culture.json")

	cultureData := `{
	"currencies": {
		"en": { "code": "USD", "symbol": "$" },
		"es": { "code": "EUR", "symbol": "€" }
	}
}`

	if err := writeTestFile(cultureFile, []byte(cultureData)); err != nil {
		t.Fatalf("write culture file: %v", err)
	}

	cfg, err := NewConfig(
		WithLocales("en", "es"),
		WithCultureData(cultureFile),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	service := cfg.CultureService()
	if service == nil {
		t.Fatal("CultureService() returned nil")
	}

	// Test currency lookup
	currency, err := service.GetCurrencyCode("es")
	if err != nil {
		t.Errorf("GetCurrencyCode(es): %v", err)
	}
	if currency != "EUR" {
		t.Errorf("GetCurrencyCode(es) = %q; want EUR", currency)
	}
}

func TestConfig_CultureServiceWithOverride(t *testing.T) {
	// Create test culture data file
	tmpDir := t.TempDir()
	cultureFile := filepath.Join(tmpDir, "culture.json")
	overrideFile := filepath.Join(tmpDir, "override.json")

	cultureData := `{
	"currencies": {
		"en": { "code": "USD", "symbol": "$" },
		"es": { "code": "EUR", "symbol": "€" }
	}
}`

	overrideData := `{
	"currencies": {
		"en": { "code": "GBP", "symbol": "£" }
	}
}`

	if err := writeTestFile(cultureFile, []byte(cultureData)); err != nil {
		t.Fatalf("write culture file: %v", err)
	}

	if err := writeTestFile(overrideFile, []byte(overrideData)); err != nil {
		t.Fatalf("write override file: %v", err)
	}

	cfg, err := NewConfig(
		WithLocales("en", "es"),
		WithCultureData(cultureFile),
		WithCultureOverride("en", overrideFile),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	service := cfg.CultureService()
	if service == nil {
		t.Fatal("CultureService() returned nil")
	}

	// Test override was applied
	currency, err := service.GetCurrencyCode("en")
	if err != nil {
		t.Errorf("GetCurrencyCode(en): %v", err)
	}
	if currency != "GBP" {
		t.Errorf("GetCurrencyCode(en) = %q; want GBP (override)", currency)
	}

	// Test original data still intact
	currency, err = service.GetCurrencyCode("es")
	if err != nil {
		t.Errorf("GetCurrencyCode(es): %v", err)
	}
	if currency != "EUR" {
		t.Errorf("GetCurrencyCode(es) = %q; want EUR", currency)
	}
}

func TestConfig_TemplateHelpersWithCulture(t *testing.T) {
	// Create test culture data file
	tmpDir := t.TempDir()
	cultureFile := filepath.Join(tmpDir, "culture.json")

	cultureData := `{
		"currencies": {
			"default": { "code": "USD", "symbol": "$" },
			"en": { "code": "USD", "symbol": "$" },
			"es": { "code": "EUR", "symbol": "€" }
		},
		"lists": {
			"trending": {
				"en": ["coffee", "tea"],
				"es": ["café", "té"]
			}
		},
		"measurement_preferences": {
			"default": {
				"weight": { "unit": "kg", "symbol": "kg" }
			},
			"en": {
				"weight": {
					"unit": "lb",
					"symbol": "lb",
					"conversion_from": { "kg": 2.20462 }
				}
			}
		}
	}`

	if err := writeTestFile(cultureFile, []byte(cultureData)); err != nil {
		t.Fatalf("write culture file: %v", err)
	}

	cfg, err := NewConfig(
		WithLocales("en", "es"),
		WithCultureData(cultureFile),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	translator, err := cfg.BuildTranslator()
	if err != nil {
		t.Fatalf("BuildTranslator: %v", err)
	}

	helpers := cfg.TemplateHelpers(translator, HelperConfig{
		LocaleKey: "Locale",
	})

	// Verify culture helpers are present
	if _, ok := helpers["resolve_currency"]; !ok {
		t.Error("resolve_currency helper not found")
	}

	if _, ok := helpers["currency_info"]; !ok {
		t.Error("currency_info helper not found")
	}

	if _, ok := helpers["culture_value"]; !ok {
		t.Error("culture_value helper not found")
	}

	if _, ok := helpers["culture_list"]; !ok {
		t.Error("culture_list helper not found")
	}

	if _, ok := helpers["preferred_measurement"]; !ok {
		t.Error("preferred_measurement helper not found")
	}

	resolve := helpers["resolve_currency"].(func(any) (string, error))
	symbol, err := resolve(map[string]any{"Locale": "es"})
	if err != nil {
		t.Fatalf("resolve_currency: %v", err)
	}
	if symbol != "€" {
		t.Fatalf("resolve_currency returned %q; want €", symbol)
	}

	infoFunc := helpers["currency_info"].(func(any) (CurrencyInfo, error))
	info, err := infoFunc(map[string]any{"Locale": "es"})
	if err != nil {
		t.Fatalf("currency_info: %v", err)
	}
	if info.Code != "EUR" || info.Symbol != "€" {
		t.Fatalf("currency_info returned %+v; want code=EUR symbol=€", info)
	}
}

func TestConfig_LocaleCatalogIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	cultureFile := filepath.Join(tmpDir, "culture.json")

	cultureData := `{
		"default_locale": "es-MX",
		"locales": {
			"en": {
				"display_name": "English",
				"active": true,
				"fallbacks": ["es"]
			},
			"es": {
				"display_name": "Español",
				"fallbacks": ["en"]
			},
			"es-MX": {
				"display_name": "Español (México)",
				"active": true,
				"fallbacks": ["es"],
				"metadata": {
					"currency": "MXN",
					"beta": true
				}
			}
		}
	}`

	if err := writeTestFile(cultureFile, []byte(cultureData)); err != nil {
		t.Fatalf("write culture file: %v", err)
	}

	cfg, err := NewConfig(
		WithCultureData(cultureFile),
		WithFallback("es-MX", "en"),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	expectedLocales := []string{"en", "es", "es-MX"}
	if !reflect.DeepEqual(cfg.Locales, expectedLocales) {
		t.Fatalf("Locales = %#v; want %#v", cfg.Locales, expectedLocales)
	}

	if cfg.DefaultLocale != "es-MX" {
		t.Fatalf("DefaultLocale = %q; want es-MX", cfg.DefaultLocale)
	}

	catalog := cfg.LocaleCatalog()
	if catalog == nil {
		t.Fatal("LocaleCatalog() returned nil")
	}

	if catalog.DefaultLocale() != "es-MX" {
		t.Fatalf("catalog.DefaultLocale() = %q; want es-MX", catalog.DefaultLocale())
	}

	if !reflect.DeepEqual(catalog.ActiveLocaleCodes(), expectedLocales) {
		t.Fatalf("ActiveLocaleCodes = %#v; want %#v", catalog.ActiveLocaleCodes(), expectedLocales)
	}

	if name := catalog.DisplayName("es"); name != "Español" {
		t.Fatalf("DisplayName(es) = %q; want Español", name)
	}

	meta := catalog.Metadata("es-MX")
	if meta == nil {
		t.Fatal("Metadata(es-MX) returned nil")
	}
	if currency, ok := meta["currency"]; !ok || currency != "MXN" {
		t.Fatalf("Metadata(es-MX)[\"currency\"] = %v; want MXN", meta["currency"])
	}
	meta["currency"] = "USD"
	if currency := catalog.Metadata("es-MX")["currency"]; currency != "MXN" {
		t.Fatalf("Metadata copy mutated underlying map, got %v", currency)
	}

	if fallbacks := catalog.Fallbacks("es"); len(fallbacks) != 1 || fallbacks[0] != "en" {
		t.Fatalf("Fallbacks(es) = %#v; want [\"en\"]", fallbacks)
	}

	resolver, ok := cfg.Resolver.(*StaticFallbackResolver)
	if !ok {
		t.Fatalf("expected StaticFallbackResolver, got %[1]T", cfg.Resolver)
	}

	chain := resolver.Resolve("es-MX")
	if len(chain) != 1 || chain[0] != "en" {
		t.Fatalf("Resolver fallback chain for es-MX = %#v; want [\"en\"]", chain)
	}

	chain = resolver.Resolve("es")
	if len(chain) != 1 || chain[0] != "en" {
		t.Fatalf("Resolver fallback chain for es = %#v; want [\"en\"]", chain)
	}
}

func TestConfig_LocaleCatalogValidation(t *testing.T) {
	tmpDir := t.TempDir()
	cultureFile := filepath.Join(tmpDir, "culture.json")

	cultureData := `{
		"default_locale": "en",
		"locales": {
			"en": {
				"display_name": "English",
				"active": true,
				"fallbacks": ["fr"]
			},
			"es": {
				"display_name": "Español",
				"active": true
			}
		}
	}`

	if err := writeTestFile(cultureFile, []byte(cultureData)); err != nil {
		t.Fatalf("write culture file: %v", err)
	}

	loader := NewCultureDataLoader(cultureFile)
	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load culture data: %v", err)
	}
	if meta, ok := data.Locales["en"]; !ok {
		t.Fatal("expected locale metadata for en")
	} else if len(meta.Fallbacks) == 0 {
		t.Fatal("locale metadata missing fallback for en")
	}
	if _, err := newLocaleCatalog(data.DefaultLocale, data.Locales); err == nil {
		t.Fatal("expected newLocaleCatalog to error for undefined fallback")
	}

	if _, err := NewConfig(
		WithCultureData(cultureFile),
	); err == nil {
		t.Fatal("expected error due to undefined fallback locale, got nil")
	}
}

func TestConfig_LocaleCatalogDefaultMustBeActive(t *testing.T) {
	tmpDir := t.TempDir()
	cultureFile := filepath.Join(tmpDir, "culture.json")

	cultureData := `{
		"default_locale": "en",
		"locales": {
			"en": {
				"display_name": "English",
				"active": false
			},
			"es": {
				"display_name": "Español",
				"active": true
			}
		}
	}`

	if err := writeTestFile(cultureFile, []byte(cultureData)); err != nil {
		t.Fatalf("write culture file: %v", err)
	}

	if _, err := NewConfig(
		WithCultureData(cultureFile),
	); err == nil {
		t.Fatal("expected error due to inactive default locale, got nil")
	}
}

func TestConfig_LocaleCatalogWithLocalesOption(t *testing.T) {
	tmpDir := t.TempDir()
	cultureFile := filepath.Join(tmpDir, "culture.json")

	cultureData := `{
		"default_locale": "en",
		"locales": {
			"en": {
				"display_name": "English",
				"active": true
			},
			"es": {
				"display_name": "Español",
				"active": true
			}
		}
	}`

	if err := writeTestFile(cultureFile, []byte(cultureData)); err != nil {
		t.Fatalf("write culture file: %v", err)
	}

	cfg, err := NewConfig(
		WithCultureData(cultureFile),
		WithLocales("es"),
	)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	expected := []string{"es"}
	if !reflect.DeepEqual(cfg.Locales, expected) {
		t.Fatalf("Locales = %#v; want %#v", cfg.Locales, expected)
	}

	if _, err := NewConfig(
		WithCultureData(cultureFile),
		WithLocales("fr"),
	); err == nil {
		t.Fatal("expected error for unknown locale code, got nil")
	}
}

func writeTestFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
