package i18n

import (
	"reflect"
	"testing"
	"time"
)

func TestFormatDateHelpers(t *testing.T) {
	ts := time.Date(2023, 7, 9, 15, 4, 5, 0, time.UTC)

	if got := FormatDate("en", ts); got != "2023-07-09" {
		t.Fatalf("FormatDate = %q", got)
	}

	if got := FormatDateTime("en", ts); got != "2023-07-09T15:04:05Z" {
		t.Fatalf("FormatDateTime = %q", got)
	}

	if got := FormatTime("en", ts); got != "15:04" {
		t.Fatalf("FormatTime = %q", got)
	}
}

func TestFormatNumericHelpers(t *testing.T) {
	if got := FormatNumber("en", 1234.5, 2); got != "1234.50" {
		t.Fatalf("FormatNumber fixed decimals = %q", got)
	}

	if got := FormatNumber("en", 12.3400, -1); got != "12.34" {
		t.Fatalf("FormatNumber trimmed = %q", got)
	}

	if got := FormatCurrency("en", 12, "USD"); got != "USD 12.00" {
		t.Fatalf("FormatCurrency = %q", got)
	}

	if got := FormatCurrency("en", 12, ""); got != "12.00" {
		t.Fatalf("FormatCurrency without code = %q", got)
	}

	if got := FormatPercent("en", 0.1234, 1); got != "12.3%" {
		t.Fatalf("FormatPercent = %q", got)
	}

	if got := FormatMeasurement("en", 12.345, "kg"); got != "12.345 kilograms" {
		t.Fatalf("FormatMeasurement = %q", got)
	}
}

func TestFormatOrdinal(t *testing.T) {
	cases := map[int]string{
		1:   "1st",
		2:   "2nd",
		3:   "3rd",
		4:   "4th",
		11:  "11th",
		12:  "12th",
		13:  "13th",
		21:  "21st",
		22:  "22nd",
		23:  "23rd",
		-1:  "-1st",
		-11: "-11th",
	}

	for input, want := range cases {
		if got := FormatOrdinal("en", input); got != want {
			t.Fatalf("FormatOrdinal(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestFormatList(t *testing.T) {
	if got := FormatList("en", nil); got != "" {
		t.Fatalf("FormatList nil = %q", got)
	}

	if got := FormatList("en", []string{"one"}); got != "one" {
		t.Fatalf("FormatList single = %q", got)
	}

	if got := FormatList("en", []string{"one", "two"}); got != "one and two" {
		t.Fatalf("FormatList two = %q", got)
	}

	if got := FormatList("en", []string{"one", "two", "three"}); got != "one, two, and three" {
		t.Fatalf("FormatList three = %q", got)
	}
}

func TestFormatPhone(t *testing.T) {
	t.Helper()

	cases := []struct {
		name   string
		locale string
		input  string
		want   string
	}{
		{
			name:   "US with leading country code",
			locale: "en",
			input:  "+14155552678",
			want:   "+1 415 555 2678",
		},
		{
			name:   "US without country code",
			locale: "en",
			input:  "4155552678",
			want:   "+1 415 555 2678",
		},
		{
			name:   "Spain national number",
			locale: "es",
			input:  "912345678",
			want:   "+34 912 345 678",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatPhone(tc.locale, tc.input); got != tc.want {
				t.Fatalf("FormatPhone(%s, %s) = %q, want %q", tc.locale, tc.input, got, tc.want)
			}
		})
	}
}

func TestRegisterPhoneDialPlan(t *testing.T) {
	RegisterPhoneDialPlan("fr", PhoneDialPlan{
		CountryCode:    "33",
		NationalPrefix: "0",
		Groups:         []int{1, 2, 2, 2, 2},
	})

	if got := FormatPhone("fr", "0123456789"); got != "+33 1 23 45 67 89" {
		t.Fatalf("FormatPhone with custom dial plan = %q", got)
	}
}

func TestRegisterPhoneFormatter(t *testing.T) {
	RegisterPhoneFormatter("xx", func(locale, raw string) string {
		return locale + ":" + raw
	})

	if got := FormatPhone("xx", "123"); got != "xx:123" {
		t.Fatalf("FormatPhone with custom formatter = %q", got)
	}
}

func TestFormatterRegistry(t *testing.T) {
	registry := NewFormatterRegistry()

	fnAny, ok := registry.Formatter("format_date", "en")
	if !ok {
		t.Fatal("expected default formatter")
	}

	fn, ok := fnAny.(func(string, time.Time) string)
	if !ok {
		t.Fatal("unexpected type assertion failure")
	}

	ts := time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)
	if got := fn("en", ts); got != "January 2, 2023" {
		t.Fatalf("formatter invocation = %q", got)
	}

	custom := func(locale string, t time.Time) string { return "custom" }
	registry.RegisterLocale("fr", "format_date", custom)

	gotAny, ok := registry.Formatter("format_date", "fr")
	if !ok {
		t.Fatal("expected locale override")
	}

	if reflect.ValueOf(gotAny).Pointer() != reflect.ValueOf(custom).Pointer() {
		t.Fatal("locale override not returned")
	}

	fm := registry.FuncMap("fr")
	if len(fm) == 0 {
		t.Fatal("expected non-empty func map")
	}

	if reflect.ValueOf(fm["format_date"]).Pointer() != reflect.ValueOf(custom).Pointer() {
		t.Fatal("func map should include override")
	}

	// Global override should update defaults.
	registry.Register("format_phone", func(locale, raw string) string { return "call" })
	fnAny, ok = registry.Formatter("format_phone", "en")
	if !ok {
		t.Fatal("expected overridden default")
	}

	phoneFn, ok := fnAny.(func(string, string) string)
	if !ok {
		t.Fatal("unexpected phone formatter signature")
	}

	if got := phoneFn("en", "123"); got != "call" {
		t.Fatalf("global override not applied: %q", got)
	}
}
