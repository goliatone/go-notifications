package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCultureService_GetCurrencyCode(t *testing.T) {
	loader := NewCultureDataLoader(filepath.Join("testdata", "culture", "example_culture_data.json"))
	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load culture data: %v", err)
	}

	resolver := NewStaticFallbackResolver()
	resolver.Set("es-MX", "es", "en")
	resolver.Set("es", "en")

	service := NewCultureService(data, resolver)

	tests := []struct {
		name           string
		locale         string
		expectedCode   string
		expectedSymbol string
		wantErr        bool
	}{
		{
			name:           "en_locale",
			locale:         "en",
			expectedCode:   "USD",
			expectedSymbol: "$",
			wantErr:        false,
		},
		{
			name:           "es_locale",
			locale:         "es",
			expectedCode:   "EUR",
			expectedSymbol: "€",
			wantErr:        false,
		},
		{
			name:           "es-MX_locale",
			locale:         "es-MX",
			expectedCode:   "MXN",
			expectedSymbol: "$",
			wantErr:        false,
		},
		{
			name:           "el_locale",
			locale:         "el",
			expectedCode:   "EUR",
			expectedSymbol: "€",
			wantErr:        false,
		},
		{
			name:           "unknown_locale_defaults",
			locale:         "fr",
			expectedCode:   "USD",
			expectedSymbol: "$",
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.GetCurrencyCode(tt.locale)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCurrencyCode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if _, err := service.GetCurrency(tt.locale); err == nil {
					t.Errorf("GetCurrency(%q) expected error", tt.locale)
				}
				return
			}

			if got != tt.expectedCode {
				t.Errorf("GetCurrencyCode(%q) = %q; want %q", tt.locale, got, tt.expectedCode)
			}

			info, err := service.GetCurrency(tt.locale)
			if err != nil {
				t.Fatalf("GetCurrency(%q) unexpected error: %v", tt.locale, err)
			}
			if info.Code != tt.expectedCode || info.Symbol != tt.expectedSymbol {
				t.Errorf("GetCurrency(%q) = %+v; want code=%q symbol=%q", tt.locale, info, tt.expectedCode, tt.expectedSymbol)
			}
		})
	}
}

func TestCultureService_GetSupportNumber(t *testing.T) {
	loader := NewCultureDataLoader(filepath.Join("testdata", "culture", "example_culture_data.json"))
	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load culture data: %v", err)
	}

	service := NewCultureService(data, nil)

	tests := []struct {
		name     string
		locale   string
		expected string
		wantErr  bool
	}{
		{
			name:     "en_support_number",
			locale:   "en",
			expected: "+1 555 010 4242",
			wantErr:  false,
		},
		{
			name:     "es_support_number",
			locale:   "es",
			expected: "+34 900 123 456",
			wantErr:  false,
		},
		{
			name:     "es-MX_support_number",
			locale:   "es-MX",
			expected: "+52 55 1234 5678",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.GetSupportNumber(tt.locale)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSupportNumber() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("GetSupportNumber(%q) = %q; want %q", tt.locale, got, tt.expected)
			}
		})
	}
}

func TestCultureService_GetList(t *testing.T) {
	loader := NewCultureDataLoader(filepath.Join("testdata", "culture", "example_culture_data.json"))
	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load culture data: %v", err)
	}

	resolver := NewStaticFallbackResolver()
	resolver.Set("es-MX", "es", "en")

	service := NewCultureService(data, resolver)

	tests := []struct {
		name     string
		locale   string
		listName string
		expected []string
		wantErr  bool
	}{
		{
			name:     "en_trending_products",
			locale:   "en",
			listName: "trending_products",
			expected: []string{"coffee", "tea", "cake"},
			wantErr:  false,
		},
		{
			name:     "es_trending_products",
			locale:   "es",
			listName: "trending_products",
			expected: []string{"café", "té", "pastel"},
			wantErr:  false,
		},
		{
			name:     "es-MX_trending_products",
			locale:   "es-MX",
			listName: "trending_products",
			expected: []string{"café", "pan dulce", "chocolate"},
			wantErr:  false,
		},
		{
			name:     "unknown_list",
			locale:   "en",
			listName: "unknown",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.GetList(tt.locale, tt.listName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.expected) {
					t.Errorf("GetList(%q, %q) = %v; want %v", tt.locale, tt.listName, got, tt.expected)
					return
				}
				for i, v := range got {
					if v != tt.expected[i] {
						t.Errorf("GetList(%q, %q)[%d] = %q; want %q", tt.locale, tt.listName, i, v, tt.expected[i])
					}
				}
			}
		})
	}
}

func TestCultureService_GetMeasurementPreference(t *testing.T) {
	loader := NewCultureDataLoader(filepath.Join("testdata", "culture", "example_culture_data.json"))
	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load culture data: %v", err)
	}

	service := NewCultureService(data, nil)

	tests := []struct {
		name            string
		locale          string
		measurementType string
		expectedUnit    string
		wantErr         bool
	}{
		{
			name:            "en_weight_preference",
			locale:          "en",
			measurementType: "weight",
			expectedUnit:    "lb",
			wantErr:         false,
		},
		{
			name:            "es_weight_preference_falls_back_to_default",
			locale:          "es",
			measurementType: "weight",
			expectedUnit:    "kg",
			wantErr:         false,
		},
		{
			name:            "en_distance_preference",
			locale:          "en",
			measurementType: "distance",
			expectedUnit:    "mi",
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := service.GetMeasurementPreference(tt.locale, tt.measurementType)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMeasurementPreference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.Unit != tt.expectedUnit {
				t.Errorf("GetMeasurementPreference(%q, %q).Unit = %q; want %q", tt.locale, tt.measurementType, got.Unit, tt.expectedUnit)
			}
		})
	}
}

func TestCultureService_GetMeasurementPreference_DefaultPrecedesFallback(t *testing.T) {
	loader := NewCultureDataLoader(filepath.Join("testdata", "culture", "example_culture_data.json"))
	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load culture data: %v", err)
	}

	resolver := NewStaticFallbackResolver()
	resolver.Set("es", "en")

	service := NewCultureService(data, resolver)

	pref, err := service.GetMeasurementPreference("es", "weight")
	if err != nil {
		t.Fatalf("GetMeasurementPreference: %v", err)
	}
	if pref.Unit != "kg" {
		t.Errorf("GetMeasurementPreference(es, weight) unit = %q; want kg", pref.Unit)
	}
	if pref.Symbol != "kg" {
		t.Errorf("GetMeasurementPreference(es, weight) symbol = %q; want kg", pref.Symbol)
	}
}

func TestCultureService_ConvertMeasurement(t *testing.T) {
	loader := NewCultureDataLoader(filepath.Join("testdata", "culture", "example_culture_data.json"))
	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load culture data: %v", err)
	}

	service := NewCultureService(data, nil)

	tests := []struct {
		name            string
		locale          string
		value           float64
		fromUnit        string
		measurementType string
		expectedValue   float64
		expectedUnit    string
		expectedSymbol  string
		wantErr         bool
	}{
		{
			name:            "en_convert_kg_to_lb",
			locale:          "en",
			value:           2.75,
			fromUnit:        "kg",
			measurementType: "weight",
			expectedValue:   6.062705,
			expectedUnit:    "lb",
			expectedSymbol:  "lb",
			wantErr:         false,
		},
		{
			name:            "es_no_conversion_needed",
			locale:          "es",
			value:           2.75,
			fromUnit:        "kg",
			measurementType: "weight",
			expectedValue:   2.75,
			expectedUnit:    "kg",
			expectedSymbol:  "kg",
			wantErr:         false,
		},
		{
			name:            "en_convert_km_to_mi",
			locale:          "en",
			value:           10.0,
			fromUnit:        "km",
			measurementType: "distance",
			expectedValue:   6.21371,
			expectedUnit:    "mi",
			expectedSymbol:  "mi",
			wantErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotValue, gotUnit, gotSymbol, err := service.ConvertMeasurement(tt.locale, tt.value, tt.fromUnit, tt.measurementType)
			if (err != nil) != tt.wantErr {
				t.Errorf("ConvertMeasurement() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotUnit != tt.expectedUnit {
					t.Errorf("ConvertMeasurement() unit = %q; want %q", gotUnit, tt.expectedUnit)
				}
				if gotSymbol != tt.expectedSymbol {
					t.Errorf("ConvertMeasurement() symbol = %q; want %q", gotSymbol, tt.expectedSymbol)
				}
				// Check value with tolerance for floating point
				if diff := gotValue - tt.expectedValue; diff > 0.0001 || diff < -0.0001 {
					t.Errorf("ConvertMeasurement() value = %f; want %f", gotValue, tt.expectedValue)
				}
			}
		})
	}
}

func TestCultureDataLoader_Override(t *testing.T) {
	// Create a temporary override file
	tmpDir := t.TempDir()
	overridePath := filepath.Join(tmpDir, "override.json")

	overrideData := `{
	"currencies": {
		"en": { "code": "GBP", "symbol": "£" }
	}
}`

	if err := os.WriteFile(overridePath, []byte(overrideData), 0644); err != nil {
		t.Fatalf("Write override file: %v", err)
	}

	loader := NewCultureDataLoader(filepath.Join("testdata", "culture", "example_culture_data.json"))
	loader.AddOverride("en", overridePath)

	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load culture data: %v", err)
	}

	// Verify the override was applied
	if info := data.Currencies["en"]; info.Code != "GBP" || info.Symbol != "£" {
		t.Errorf("Override not applied: Currencies[en] = %+v; want GBP/£", info)
	}

	// Verify other data is still intact
	if info := data.Currencies["es"]; info.Code != "EUR" || info.Symbol != "€" {
		t.Errorf("Original data lost: Currencies[es] = %+v; want EUR/€", info)
	}
}

func TestCultureService_FallsBackToParentLocaleWithoutResolver(t *testing.T) {
	loader := NewCultureDataLoader(filepath.Join("testdata", "culture", "example_culture_data.json"))
	data, err := loader.Load()
	if err != nil {
		t.Fatalf("Load culture data: %v", err)
	}

	service := NewCultureService(data, nil)

	code, err := service.GetCurrencyCode("en-GB")
	if err != nil {
		t.Fatalf("GetCurrencyCode: %v", err)
	}
	if code != "USD" {
		t.Fatalf("GetCurrencyCode(en-GB) = %q; want %q", code, "USD")
	}
}
