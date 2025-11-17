package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type formatterContractFixture struct {
	Timestamp        string                    `json:"timestamp"`
	NumberValue      float64                   `json:"number_value"`
	NumberDecimals   int                       `json:"number_decimals"`
	CurrencyAmount   float64                   `json:"currency_amount"`
	PercentValue     float64                   `json:"percent_value"`
	PercentDecimals  int                       `json:"percent_decimals"`
	ListItems        []string                  `json:"list_items"`
	OrdinalValue     int                       `json:"ordinal_value"`
	MeasurementValue float64                   `json:"measurement_value"`
	MeasurementUnit  string                    `json:"measurement_unit"`
	Locales          []formatterContractLocale `json:"locales"`
}

type formatterContractLocale struct {
	Locale            string                       `json:"locale"`
	CurrencyCode      string                       `json:"currency_code"`
	PhoneInput        string                       `json:"phone_input"`
	UseCustomProvider bool                         `json:"use_custom_provider"`
	Expected          formatterContractExpectation `json:"expected"`
}

type formatterContractExpectation struct {
	Date        string `json:"date"`
	DateTime    string `json:"datetime"`
	Time        string `json:"time"`
	Number      string `json:"number"`
	Currency    string `json:"currency"`
	Percent     string `json:"percent"`
	List        string `json:"list"`
	Ordinal     string `json:"ordinal"`
	Measurement string `json:"measurement"`
	Phone       string `json:"phone"`
}

func TestFormatterContractFixture(t *testing.T) {
	fixture := loadFormatterContractFixture(t, "testdata/formatters_contract.json")

	moment, err := time.Parse(time.RFC3339, fixture.Timestamp)
	if err != nil {
		t.Fatalf("parse timestamp: %v", err)
	}

	registry := NewFormatterRegistry()

	for _, locale := range fixture.Locales {
		if locale.UseCustomProvider {
			registry.RegisterTypedProvider(locale.Locale, newFixtureProvider(locale.Expected, fixture))
		}
	}

	for _, locale := range fixture.Locales {
		locale := locale
		t.Run(locale.Locale, func(t *testing.T) {
			verifyFormatterContract(t, registry, fixture, locale, moment)
		})
	}
}

func verifyFormatterContract(t *testing.T, registry *FormatterRegistry, fx formatterContractFixture, locale formatterContractLocale, moment time.Time) {
	funcs := registry.FuncMap(locale.Locale)

	dateFn := mustFormatterFunc[func(string, time.Time) string](t, funcs, "format_date")
	if got := dateFn(locale.Locale, moment); got != locale.Expected.Date {
		t.Fatalf("format_date mismatch: got %q want %q", got, locale.Expected.Date)
	}

	datetimeFn := mustFormatterFunc[func(string, time.Time) string](t, funcs, "format_datetime")
	if got := datetimeFn(locale.Locale, moment); got != locale.Expected.DateTime {
		t.Fatalf("format_datetime mismatch: got %q want %q", got, locale.Expected.DateTime)
	}

	timeFn := mustFormatterFunc[func(string, time.Time) string](t, funcs, "format_time")
	if got := timeFn(locale.Locale, moment); got != locale.Expected.Time {
		t.Fatalf("format_time mismatch: got %q want %q", got, locale.Expected.Time)
	}

	numberFn := mustFormatterFunc[func(string, float64, int) string](t, funcs, "format_number")
	if got := numberFn(locale.Locale, fx.NumberValue, fx.NumberDecimals); got != locale.Expected.Number {
		t.Fatalf("format_number mismatch: got %q want %q", got, locale.Expected.Number)
	}

	currencyFn := mustFormatterFunc[func(string, float64, string) string](t, funcs, "format_currency")
	if got := currencyFn(locale.Locale, fx.CurrencyAmount, locale.CurrencyCode); got != locale.Expected.Currency {
		t.Fatalf("format_currency mismatch: got %q want %q", got, locale.Expected.Currency)
	}

	percentFn := mustFormatterFunc[func(string, float64, int) string](t, funcs, "format_percent")
	if got := percentFn(locale.Locale, fx.PercentValue, fx.PercentDecimals); got != locale.Expected.Percent {
		t.Fatalf("format_percent mismatch: got %q want %q", got, locale.Expected.Percent)
	}

	listFn := mustFormatterFunc[func(string, []string) string](t, funcs, "format_list")
	if got := listFn(locale.Locale, append([]string(nil), fx.ListItems...)); got != locale.Expected.List {
		t.Fatalf("format_list mismatch: got %q want %q", got, locale.Expected.List)
	}

	ordinalFn := mustFormatterFunc[func(string, int) string](t, funcs, "format_ordinal")
	if got := ordinalFn(locale.Locale, fx.OrdinalValue); got != locale.Expected.Ordinal {
		t.Fatalf("format_ordinal mismatch: got %q want %q", got, locale.Expected.Ordinal)
	}

	measurementFn := mustFormatterFunc[func(string, float64, string) string](t, funcs, "format_measurement")
	if got := measurementFn(locale.Locale, fx.MeasurementValue, fx.MeasurementUnit); got != locale.Expected.Measurement {
		t.Fatalf("format_measurement mismatch: got %q want %q", got, locale.Expected.Measurement)
	}

	phoneFn := mustFormatterFunc[func(string, string) string](t, funcs, "format_phone")
	if got := phoneFn(locale.Locale, locale.PhoneInput); got != locale.Expected.Phone {
		t.Fatalf("format_phone mismatch: got %q want %q", got, locale.Expected.Phone)
	}
}

func loadFormatterContractFixture(t *testing.T, path string) formatterContractFixture {
	t.Helper()

	full := filepath.Join(".tmp", path)
	data, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read fixture %q: %v", path, err)
	}

	var fx formatterContractFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("unmarshal fixture %q: %v", path, err)
	}
	return fx
}

func mustFormatterFunc[T any](t *testing.T, funcs map[string]any, name string) T {
	t.Helper()

	fn, ok := funcs[name]
	if !ok {
		t.Fatalf("missing formatter %q", name)
	}

	typed, ok := fn.(T)
	if !ok {
		t.Fatalf("unexpected signature for %q: %T", name, fn)
	}
	return typed
}

type fixtureProvider struct {
	funcs map[string]any
	caps  FormatterCapabilities
}

func newFixtureProvider(exp formatterContractExpectation, fx formatterContractFixture) *fixtureProvider {
	formatDecimal := func(value float64, decimals int) string {
		return strings.ReplaceAll(fmt.Sprintf("%.*f", decimals, value), ".", ",")
	}

	formatThousands := func(value float64, decimals int) string {
		number := fmt.Sprintf("%.*f", decimals, value)
		parts := strings.Split(number, ".")
		intPart := parts[0]
		var builder strings.Builder
		for i, r := range intPart {
			if i > 0 && (len(intPart)-i)%3 == 0 {
				builder.WriteRune('.')
			}
			builder.WriteRune(r)
		}
		if len(parts) > 1 && parts[1] != "" {
			builder.WriteRune(',')
			builder.WriteString(parts[1])
		}
		return builder.String()
	}

	funcs := map[string]any{
		"format_date": func(_ string, t time.Time) string {
			return t.Format("02/01/2006")
		},
		"format_datetime": func(_ string, t time.Time) string {
			return t.Format("02/01/2006 15:04")
		},
		"format_time": func(_ string, t time.Time) string {
			return t.Format("15:04")
		},
		"format_number": func(_ string, value float64, decimals int) string {
			return formatThousands(value, decimals)
		},
		"format_currency": func(_ string, value float64, code string) string {
			return formatThousands(value, fx.NumberDecimals) + " €"
		},
		"format_percent": func(_ string, value float64, decimals int) string {
			return formatDecimal(value*100, decimals) + "%"
		},
		"format_list": func(_ string, items []string) string {
			if len(items) == 0 {
				return ""
			}
			if len(items) == 1 {
				return items[0]
			}

			if len(items) == 2 {
				return items[0] + " και " + items[1]
			}

			head := strings.Join(items[:len(items)-1], ", ")
			return head + " και " + items[len(items)-1]
		},
		"format_ordinal": func(_ string, value int) string {
			return formatGreekOrdinal(value)
		},
		"format_measurement": func(_ string, value float64, unit string) string {
			return formatDecimal(value, fx.PercentDecimals) + " κιλά"
		},
		"format_phone": func(_ string, raw string) string {
			return "+30 210 123 4567"
		},
	}

	return &fixtureProvider{
		funcs: funcs,
		caps: FormatterCapabilities{
			Number:      true,
			Currency:    true,
			Date:        true,
			DateTime:    true,
			Time:        true,
			List:        true,
			Ordinal:     true,
			Measurement: true,
			Phone:       true,
		},
	}
}

func (p *fixtureProvider) Formatter(name string) (any, bool) {
	fn, ok := p.funcs[name]
	return fn, ok
}

func (p *fixtureProvider) FuncMap() map[string]any {
	cloned := make(map[string]any, len(p.funcs))
	for key, value := range p.funcs {
		cloned[key] = value
	}
	return cloned
}

func (p *fixtureProvider) Capabilities() FormatterCapabilities {
	return p.caps
}

func formatGreekOrdinal(value int) string {
	return strconv.Itoa(value) + "ος"
}
