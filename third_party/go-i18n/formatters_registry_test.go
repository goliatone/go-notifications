package i18n

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestFormatterRegistryProvider(t *testing.T) {
	registry := NewFormatterRegistry()

	registry.RegisterProvider("fr", func(locale string) map[string]any {
		return map[string]any{
			"format_number": func(_ string, value float64, decimals int) string {
				return "fr-" + FormatNumber("", value, decimals)
			},
		}
	})

	fnAny, ok := registry.Formatter("format_number", "fr")
	if !ok {
		t.Fatalf("expected provider formatter")
	}

	fn := fnAny.(func(string, float64, int) string)
	if got := fn("fr", 12.3, 1); got != "fr-12.3" {
		t.Fatalf("provider formatter = %q", got)
	}

	funcs := registry.FuncMap("fr")
	if funcs["format_number"].(func(string, float64, int) string)("fr", 1.2, 1) != "fr-1.2" {
		t.Fatalf("func map should reflect provider override")
	}
}

func TestFormatterRegistryProviderOverrideOrder(t *testing.T) {
	registry := NewFormatterRegistry()

	registry.RegisterProvider("fr", func(locale string) map[string]any {
		return map[string]any{
			"format_number": func(_ string, value float64, decimals int) string {
				return "provider"
			},
		}
	})

	registry.RegisterLocale("fr", "format_number", func(_ string, value float64, decimals int) string {
		return "override"
	})

	fnAny, ok := registry.Formatter("format_number", "fr")
	if !ok {
		t.Fatal("expected formatter")
	}

	fn := fnAny.(func(string, float64, int) string)
	if got := fn("fr", 1, 0); got != "override" {
		t.Fatalf("override should win, got %q", got)
	}
}

func TestFormatterRegistryCompositeTypedProvider(t *testing.T) {
	registry := NewFormatterRegistry()

	p1 := newStubTypedProvider(map[string]any{
		"format_percent": func(_ string, value float64, decimals int) string {
			return "provider-one"
		},
	}, FormatterCapabilities{Number: true})

	p2 := newStubTypedProvider(map[string]any{
		"format_measurement": func(_ string, value float64, unit string) string {
			return "provider-two"
		},
	}, FormatterCapabilities{Measurement: true})

	registry.RegisterTypedProvider("zz", p1)
	registry.RegisterTypedProvider("zz", p2)

	percentAny, ok := registry.Formatter("format_percent", "zz")
	if !ok {
		t.Fatal("expected composite percent formatter")
	}
	if got := percentAny.(func(string, float64, int) string)("zz", 1.0, 0); got != "provider-one" {
		t.Fatalf("composite percent formatter = %q", got)
	}

	measurementAny, ok := registry.Formatter("format_measurement", "zz")
	if !ok {
		t.Fatal("expected composite measurement formatter")
	}
	if got := measurementAny.(func(string, float64, string) string)("zz", 1.0, "kg"); got != "provider-two" {
		t.Fatalf("composite measurement formatter = %q", got)
	}

	funcs := registry.FuncMap("zz")
	if funcs["format_percent"].(func(string, float64, int) string)("zz", 1.0, 0) != "provider-one" {
		t.Fatal("func map should expose first provider formatter")
	}
	if funcs["format_measurement"].(func(string, float64, string) string)("zz", 1.0, "kg") != "provider-two" {
		t.Fatal("func map should expose second provider formatter")
	}
}

func TestFormatterRegistryMissingProviderPanics(t *testing.T) {
	// With the new implementation, all configured locales get XText providers
	// registered automatically (with fallback to English formatting rules).
	// This test now verifies that the registry doesn't panic and provides
	// a working formatter even for unconfigured locales.

	registry := NewFormatterRegistry(
		WithFormatterRegistryResolver(NewStaticFallbackResolver()),
		WithFormatterRegistryLocales("fr"),
	)

	// Verify that "fr" now has a provider (falls back to English rules)
	funcs := registry.FuncMap("fr")
	if _, ok := funcs["format_currency"]; !ok {
		t.Fatal("expected format_currency to be available for 'fr' locale")
	}
}

func TestFormatterRegistryCLDRBundles(t *testing.T) {
	registry := NewFormatterRegistry()

	fmEs := registry.FuncMap("es")
	listFn, ok := fmEs["format_list"].(func(string, []string) string)
	if !ok {
		t.Fatalf("format_list signature mismatch: %T", fmEs["format_list"])
	}

	if got := listFn("es", []string{"uno", "dos", "tres"}); got != "uno, dos y tres" {
		t.Fatalf("format_list es = %q", got)
	}

	ordinalFn, ok := fmEs["format_ordinal"].(func(string, int) string)
	if !ok {
		t.Fatalf("format_ordinal signature mismatch: %T", fmEs["format_ordinal"])
	}

	if got := ordinalFn("es", 1); got != "1º" {
		t.Fatalf("format_ordinal es = %q", got)
	}

	measurementFn, ok := fmEs["format_measurement"].(func(string, float64, string) string)
	if !ok {
		t.Fatalf("format_measurement signature mismatch: %T", fmEs["format_measurement"])
	}

	if got := measurementFn("es", 12.34, "km"); got != "12,34 kilómetros" {
		t.Fatalf("format_measurement es = %q", got)
	}

	phoneFn, ok := fmEs["format_phone"].(func(string, string) string)
	if !ok {
		t.Fatalf("format_phone signature mismatch: %T", fmEs["format_phone"])
	}

	if got := phoneFn("es", "+34123456789"); got != "+34 123 456 789" {
		t.Fatalf("format_phone es = %q", got)
	}

	fmEn := registry.FuncMap("en")
	ordinalEn := fmEn["format_ordinal"].(func(string, int) string)
	if got := ordinalEn("en", 21); got != "21st" {
		t.Fatalf("format_ordinal en = %q", got)
	}

	listEn := fmEn["format_list"].(func(string, []string) string)
	if got := listEn("en", []string{"a", "b", "c"}); got != "a, b, and c" {
		t.Fatalf("format_list en = %q", got)
	}

	measurementEn := fmEn["format_measurement"].(func(string, float64, string) string)
	if got := measurementEn("en", 12.34, "km"); got != "12.34 kilometers" {
		t.Fatalf("format_measurement en = %q", got)
	}

	phoneEn := fmEn["format_phone"].(func(string, string) string)
	if got := phoneEn("en", "1234567890"); got != "+1 123 456 7890" {
		t.Fatalf("format_phone en = %q", got)
	}
}

func TestFormatterRegistryLocaleFallbackResolution(t *testing.T) {
	resolver := NewStaticFallbackResolver()
	registry := NewFormatterRegistry(
		WithFormatterRegistryResolver(resolver),
		WithFormatterRegistryLocales("en", "en-GB"),
	)

	registry.RegisterLocale("en", "format_list", func(_ string, items []string) string {
		return "fallback-" + strings.Join(items, "|")
	})

	fnAny, ok := registry.Formatter("format_list", "en-GB")
	if !ok {
		t.Fatal("expected formatter via fallback chain")
	}
	got := fnAny.(func(string, []string) string)("en-GB", []string{"a", "b"})
	if got != "fallback-a|b" {
		t.Fatalf("fallback formatting mismatch: %q", got)
	}
}

func TestFormatterRegistryFallsBackToDefaults(t *testing.T) {
	registry := NewFormatterRegistry()

	fnAny, ok := registry.Formatter("format_phone", "fr")
	if !ok {
		t.Fatal("expected default formatter for format_phone")
	}

	fn := fnAny.(func(string, string) string)
	if got := fn("fr", "123"); got != "123" {
		t.Fatalf("default formatter mismatch: %q", got)
	}
}

type stubTypedProvider struct {
	funcs map[string]any
	caps  FormatterCapabilities
}

func newStubTypedProvider(funcs map[string]any, caps FormatterCapabilities) *stubTypedProvider {
	return &stubTypedProvider{
		funcs: funcs,
		caps:  caps,
	}
}

func (p *stubTypedProvider) Formatter(name string) (any, bool) {
	if p == nil || p.funcs == nil {
		return nil, false
	}
	fn, ok := p.funcs[name]
	return fn, ok
}

func (p *stubTypedProvider) FuncMap() map[string]any {
	result := make(map[string]any, len(p.funcs))
	for key, value := range p.funcs {
		result[key] = value
	}
	return result
}

func (p *stubTypedProvider) Capabilities() FormatterCapabilities {
	if p == nil {
		return FormatterCapabilities{}
	}
	return p.caps
}

// Fixture helpers for regression tests

type FormatterTestCase struct {
	Locale   string  `json:"locale"`
	Date     string  `json:"date"` // RFC3339 format
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
	Value    float64 `json:"value"`
	Unit     string  `json:"unit"`
}

type FormatterExpected struct {
	Date        string `json:"date"`
	Currency    string `json:"currency"`
	Measurement string `json:"measurement"`
}

func loadFormatterFixture(t *testing.T, path string) FormatterTestCase {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("load fixture %s: %v", path, err)
	}
	var tc FormatterTestCase
	if err := json.Unmarshal(data, &tc); err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	return tc
}

func loadFormatterExpected(t *testing.T, path string) FormatterExpected {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("load expected %s: %v", path, err)
	}
	var expected FormatterExpected
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatalf("parse expected %s: %v", path, err)
	}
	return expected
}

// Regression tests for Spanish locale formatting bug

func TestFormatterRegistry_SpanishDateFormatting(t *testing.T) {
	tests := []struct {
		name         string
		locale       string
		fixtureInput string
		fixtureWant  string
	}{
		{
			name:         "es_locale_date",
			locale:       "es",
			fixtureInput: "testdata/formatter_contracts/es_date_input.json",
			fixtureWant:  "testdata/formatter_contracts/es_date_expected.json",
		},
		{
			name:         "es_MX_locale_date",
			locale:       "es-MX",
			fixtureInput: "testdata/formatter_contracts/es_date_input.json",
			fixtureWant:  "testdata/formatter_contracts/es_MX_date_expected.json",
		},
		{
			name:         "el_locale_date_fallback",
			locale:       "el",
			fixtureInput: "testdata/formatter_contracts/es_date_input.json",
			fixtureWant:  "testdata/formatter_contracts/el_date_expected.json",
		},
		{
			name:         "ar_locale_date_fallback",
			locale:       "ar",
			fixtureInput: "testdata/formatter_contracts/es_date_input.json",
			fixtureWant:  "testdata/formatter_contracts/ar_date_expected.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup registry with resolver
			resolver := NewStaticFallbackResolver()
			resolver.Set("es", "en")
			resolver.Set("es-MX", "es", "en")
			resolver.Set("el", "en")
			resolver.Set("ar", "en")

			registry := NewFormatterRegistry(
				WithFormatterRegistryResolver(resolver),
				WithFormatterRegistryLocales("en", "es", "es-MX", "el", "ar"),
			)

			// Load fixtures
			input := loadFormatterFixture(t, tt.fixtureInput)
			want := loadFormatterExpected(t, tt.fixtureWant)

			// Parse date
			dateTime, err := time.Parse(time.RFC3339, input.Date)
			if err != nil {
				t.Fatalf("parse date: %v", err)
			}

			// Get formatter from registry
			funcMap := registry.FuncMap(tt.locale)
			formatDate, ok := funcMap["format_date"].(func(string, time.Time) string)
			if !ok {
				t.Fatalf("format_date not found or wrong type")
			}

			// Execute
			got := formatDate(tt.locale, dateTime)

			// Compare
			if got != want.Date {
				t.Errorf("format_date(%q, %v) = %q; want %q", tt.locale, dateTime, got, want.Date)
			}
		})
	}
}

func TestFormatterRegistry_SpanishCurrencyFormatting(t *testing.T) {
	tests := []struct {
		name         string
		locale       string
		fixtureInput string
		fixtureWant  string
	}{
		{
			name:         "es_locale_currency",
			locale:       "es",
			fixtureInput: "testdata/formatter_contracts/es_currency_input.json",
			fixtureWant:  "testdata/formatter_contracts/es_currency_expected.json",
		},
		{
			name:         "es_MX_locale_currency",
			locale:       "es-MX",
			fixtureInput: "testdata/formatter_contracts/es_currency_input.json",
			fixtureWant:  "testdata/formatter_contracts/es_MX_currency_expected.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup registry with resolver
			resolver := NewStaticFallbackResolver()
			resolver.Set("es", "en")
			resolver.Set("es-MX", "es", "en")

			registry := NewFormatterRegistry(
				WithFormatterRegistryResolver(resolver),
				WithFormatterRegistryLocales("en", "es", "es-MX"),
			)

			// Load fixtures
			input := loadFormatterFixture(t, tt.fixtureInput)
			want := loadFormatterExpected(t, tt.fixtureWant)

			// Get formatter from registry
			funcMap := registry.FuncMap(tt.locale)
			formatCurrency, ok := funcMap["format_currency"].(func(string, float64, string) string)
			if !ok {
				t.Fatalf("format_currency not found or wrong type")
			}

			// Execute
			got := formatCurrency(tt.locale, input.Amount, input.Currency)

			// Compare
			if got != want.Currency {
				t.Errorf("format_currency(%q, %.2f, %q) = %q; want %q", tt.locale, input.Amount, input.Currency, got, want.Currency)
			}
		})
	}
}

func TestFormatterRegistry_SpanishMeasurementFormatting(t *testing.T) {
	tests := []struct {
		name         string
		locale       string
		fixtureInput string
		fixtureWant  string
	}{
		{
			name:         "es_locale_measurement",
			locale:       "es",
			fixtureInput: "testdata/formatter_contracts/es_measurement_input.json",
			fixtureWant:  "testdata/formatter_contracts/es_measurement_expected.json",
		},
		{
			name:         "es_MX_locale_measurement",
			locale:       "es-MX",
			fixtureInput: "testdata/formatter_contracts/es_measurement_input.json",
			fixtureWant:  "testdata/formatter_contracts/es_MX_measurement_expected.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup registry with resolver
			resolver := NewStaticFallbackResolver()
			resolver.Set("es", "en")
			resolver.Set("es-MX", "es", "en")

			registry := NewFormatterRegistry(
				WithFormatterRegistryResolver(resolver),
				WithFormatterRegistryLocales("en", "es", "es-MX"),
			)

			// Load fixtures
			input := loadFormatterFixture(t, tt.fixtureInput)
			want := loadFormatterExpected(t, tt.fixtureWant)

			// Get formatter from registry
			funcMap := registry.FuncMap(tt.locale)
			formatMeasurement, ok := funcMap["format_measurement"].(func(string, float64, string) string)
			if !ok {
				t.Fatalf("format_measurement not found or wrong type")
			}

			// Execute
			got := formatMeasurement(tt.locale, input.Value, input.Unit)

			// Compare
			if got != want.Measurement {
				t.Errorf("format_measurement(%q, %.2f, %q) = %q; want %q", tt.locale, input.Value, input.Unit, got, want.Measurement)
			}
		})
	}
}

// TestFormatterRegistry_MergeOrder verifies that locale-specific formatters
// are not overwritten by fallback formatters (merge order bug)
//
// EVIDENCE OF BUG (from debug output):
// When requesting formatters for "es" locale:
//  1. candidateLocales("es") returns [es en] (es first, then fallback en)
//  2. funcMapForLocale loops through candidates in order
//  3. First copies from "es" provider → result["format_date"] = Spanish formatter
//  4. Then copies from "en" provider → result["format_date"] = English formatter (OVERWRITES!)
//  5. Final result has English formatter, not Spanish
//
// ROOT CAUSE: maps.Copy overwrites existing keys. Since loop iterates [es, en],
// the en (fallback) provider overwrites the es (specific) provider.
//
// EXPECTED: More specific locales should take precedence over fallbacks.
//
// DEBUG OUTPUT EXCERPT:
//
//	[DEBUG] funcMapForLocale("es") candidates: [es en]
//	[DEBUG] Copying from typed provider for locale: "es"
//	[DEBUG] Copying from typed provider for locale: "en"  ← This overwrites "es"
//	format_date("es", ...) = "October 7, 2025"; want "7 de octubre de 2025"
func TestFormatterRegistry_MergeOrder(t *testing.T) {
	// Create test providers that identify themselves
	createTestProvider := func(locale string, dateFormat string) TypedFormatterProvider {
		return &testTypedProvider{
			locale: locale,
			funcs: map[string]any{
				"format_date": func(_ string, tm time.Time) string {
					return "[" + locale + "]:" + dateFormat
				},
			},
		}
	}

	resolver := NewStaticFallbackResolver()
	resolver.Set("es", "en")

	registry := NewFormatterRegistry(
		WithFormatterRegistryResolver(resolver),
		WithFormatterRegistryLocales("en", "es"),
		WithFormatterRegistryTypedProvider("en", createTestProvider("en", "EN_FORMAT")),
		WithFormatterRegistryTypedProvider("es", createTestProvider("es", "ES_FORMAT")),
	)

	// Test es locale - should get ES_FORMAT, not EN_FORMAT
	funcMap := registry.FuncMap("es")
	formatDate := funcMap["format_date"].(func(string, time.Time) string)
	got := formatDate("es", time.Now())

	if !strings.Contains(got, "ES_FORMAT") {
		t.Errorf("Expected es provider format, got: %q (indicates en provider overwriting es)", got)
	}

	if strings.Contains(got, "EN_FORMAT") {
		t.Errorf("Got en provider format for es locale: %q (confirms merge order bug)", got)
	}
}

type testTypedProvider struct {
	locale string
	funcs  map[string]any
}

func (p *testTypedProvider) Formatter(name string) (any, bool) {
	fn, ok := p.funcs[name]
	return fn, ok
}

func (p *testTypedProvider) FuncMap() map[string]any {
	// Use the existing cloneFuncMap from formatters_registry.go
	return cloneFuncMap(p.funcs)
}

func (p *testTypedProvider) Capabilities() FormatterCapabilities {
	return FormatterCapabilities{}
}
