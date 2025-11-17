package i18n

import (
	"fmt"
	"testing"
	"time"
)

// TestXTextProvider_DataDriven_DateFormatting validates that date formatting
// uses data-driven rules instead of hardcoded language checks
func TestXTextProvider_DataDriven_DateFormatting(t *testing.T) {
	tests := []struct {
		locale   string
		date     time.Time
		expected string
	}{
		{
			locale:   "es",
			date:     time.Date(2025, 10, 7, 0, 0, 0, 0, time.UTC),
			expected: "7 de octubre de 2025",
		},
		{
			locale:   "es-MX",
			date:     time.Date(2025, 10, 7, 0, 0, 0, 0, time.UTC),
			expected: "7 de octubre de 2025",
		},
		{
			locale:   "en",
			date:     time.Date(2025, 10, 7, 0, 0, 0, 0, time.UTC),
			expected: "October 7, 2025",
		},
	}

	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			provider := newXTextProvider(tt.locale, nil)
			got := provider.formatDate(tt.locale, tt.date)
			if got != tt.expected {
				t.Errorf("formatDate(%q) = %q; want %q", tt.locale, got, tt.expected)
			}
		})
	}
}

// TestXTextProvider_DataDriven_TimeFormatting validates that time formatting
// uses data-driven rules for 12/24 hour clock preference
func TestXTextProvider_DataDriven_TimeFormatting(t *testing.T) {
	tests := []struct {
		locale   string
		time     time.Time
		expected string
	}{
		{
			locale:   "es",
			time:     time.Date(2025, 10, 7, 14, 30, 0, 0, time.UTC),
			expected: "14:30", // 24-hour format for Spanish
		},
		{
			locale:   "es-MX",
			time:     time.Date(2025, 10, 7, 14, 30, 0, 0, time.UTC),
			expected: "14:30", // 24-hour format for Spanish Mexico
		},
		{
			locale:   "en",
			time:     time.Date(2025, 10, 7, 14, 30, 0, 0, time.UTC),
			expected: "2:30 PM", // 12-hour format for English
		},
	}

	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			provider := newXTextProvider(tt.locale, nil)
			got := provider.formatTime(tt.locale, tt.time)
			if got != tt.expected {
				t.Errorf("formatTime(%q) = %q; want %q", tt.locale, got, tt.expected)
			}
		})
	}
}

// TestXTextProvider_FormattingRulesLoading validates that formatting rules
// are correctly loaded with fallback logic
func TestXTextProvider_FormattingRulesLoading(t *testing.T) {
	tests := []struct {
		locale       string
		expectedLang string // Expected language that rules fall back to
	}{
		{"es", "es"},
		{"es-MX", "es"}, // Falls back to base "es"
		{"es-ES", "es"}, // Falls back to base "es"
		{"en", "en"},
		{"en-US", "en"},   // Falls back to base "en"
		{"fr", "en"},      // Unknown locale falls back to "en"
		{"unknown", "en"}, // Unknown locale falls back to "en"
	}

	for _, tt := range tests {
		t.Run(tt.locale, func(t *testing.T) {
			provider := newXTextProvider(tt.locale, nil)
			if provider.rules == nil {
				t.Fatalf("rules is nil for locale %q", tt.locale)
			}
			if provider.rules.Locale != tt.expectedLang {
				t.Errorf("rules.Locale = %q; want %q", provider.rules.Locale, tt.expectedLang)
			}
		})
	}
}

func TestXTextProvider_FormatNumberAutoPrecision(t *testing.T) {
	provider := newXTextProvider("es", nil)

	tests := []struct {
		value    float64
		decimals int
		want     string
	}{
		{value: 1.2, decimals: -1, want: "1,2"},
		{value: 1234.5, decimals: -1, want: "1.234,5"},
		{value: 1000, decimals: -1, want: "1.000"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("value=%v", tt.value), func(t *testing.T) {
			got := provider.formatNumber("es", tt.value, tt.decimals)
			if got != tt.want {
				t.Errorf("formatNumber(%v, %d) = %q; want %q", tt.value, tt.decimals, got, tt.want)
			}
		})
	}
}

func TestXTextProvider_FormatNumberNegative(t *testing.T) {
	provider := newXTextProvider("es", nil)

	tests := []struct {
		name     string
		value    float64
		decimals int
		want     string
	}{
		{
			name:     "negative with thousand separator",
			value:    -1234567.89,
			decimals: 2,
			want:     "-1.234.567,89",
		},
		{
			name:     "small negative no thousand separator",
			value:    -123.45,
			decimals: 2,
			want:     "-123,45",
		},
		{
			name:     "large negative",
			value:    -9876543.21,
			decimals: 2,
			want:     "-9.876.543,21",
		},
		{
			name:     "negative integer",
			value:    -5000,
			decimals: 0,
			want:     "-5.000",
		},
		{
			name:     "negative auto precision",
			value:    -1234.5,
			decimals: -1,
			want:     "-1.234,5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := provider.formatNumber("es", tt.value, tt.decimals)
			if got != tt.want {
				t.Errorf("formatNumber(%v, %d) = %q; want %q", tt.value, tt.decimals, got, tt.want)
			}
		})
	}
}

func TestXTextProvider_FormatNumberMissingDecimalSeparator(t *testing.T) {
	provider := newXTextProvider("en", nil)
	if provider.rules == nil {
		t.Fatalf("expected rules for locale en")
	}

	// Simulate incomplete configuration where decimal separator is omitted.
	provider.rules.CurrencyRules.DecimalSep = ""
	provider.rules.CurrencyRules.ThousandSep = ","

	const want = "1,234.56"
	if got := provider.formatNumber("en", 1234.56, 2); got != want {
		t.Fatalf("formatNumber() = %q; want %q", got, want)
	}
}
