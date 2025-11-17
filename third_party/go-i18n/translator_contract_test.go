package i18n

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

type contractFixture struct {
	DefaultLocale string                       `json:"default_locale"`
	Fallbacks     map[string][]string          `json:"fallbacks"`
	Translations  map[string]map[string]string `json:"translations"`
}

type contractGolden struct {
	Lookups []struct {
		Locale string `json:"locale"`
		Key    string `json:"key"`
		Args   []any  `json:"args"`
		Want   string `json:"want"`
	} `json:"lookups"`
	Missing []struct {
		Locale string `json:"locale"`
		Key    string `json:"key"`
	} `json:"missing"`
}

func TestTranslatorContract_FallbackFixture(t *testing.T) {
	fixture := loadContractFixture(t, "testdata/translator_fallback_fixture.json")
	golden := loadContractGolden(t, "testdata/translator_fallback_golden.json")

	translations := make(Translations, len(fixture.Translations))
	for locale, entries := range fixture.Translations {
		translations[locale] = newStringCatalog(locale, entries)
	}

	opts := []Option{
		WithStore(NewStaticStore(translations)),
		WithDefaultLocale(fixture.DefaultLocale),
	}

	for locale, chain := range fixture.Fallbacks {
		opts = append(opts, WithFallback(locale, chain...))
	}

	cfg, err := NewConfig(opts...)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	translator, err := cfg.BuildTranslator()
	if err != nil {
		t.Fatalf("BuildTranslator: %v", err)
	}

	for _, tc := range golden.Lookups {
		got, err := translator.Translate(tc.Locale, tc.Key, tc.Args...)
		if err != nil {
			t.Fatalf("Translate(%q,%q): unexpected err: %v", tc.Locale, tc.Key, err)
		}
		if got != tc.Want {
			t.Fatalf("Translate(%q,%q) = %q want %q", tc.Locale, tc.Key, got, tc.Want)
		}
	}

	for _, tc := range golden.Missing {
		_, err := translator.Translate(tc.Locale, tc.Key)
		if !errors.Is(err, ErrMissingTranslation) {
			t.Fatalf("missing Translate(%q,%q) err = %v want ErrMissingTranslation", tc.Locale, tc.Key, err)
		}
	}
}

func loadContractFixture(t *testing.T, path string) contractFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	var fx contractFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", path, err)
	}
	return fx
}

func loadContractGolden(t *testing.T, path string) contractGolden {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	var g contractGolden
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("unmarshal golden %s: %v", path, err)
	}
	return g
}
