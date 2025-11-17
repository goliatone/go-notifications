package i18n

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type pluralContractFixture struct {
	DefaultLocale string                 `json:"default_locale"`
	LoaderPaths   []string               `json:"loader_paths"`
	RulePaths     []string               `json:"rule_paths"`
	Locales       []string               `json:"locales"`
	Lookups       []pluralContractLookup `json:"lookups"`
}

type pluralContractLookup struct {
	Name     string   `json:"name"`
	Locale   string   `json:"locale"`
	Key      string   `json:"key"`
	Count    *float64 `json:"count"`
	Want     string   `json:"want"`
	Category string   `json:"category"`
	Missing  bool     `json:"missing"`
}

func TestTranslatorContract_PluralFlows(t *testing.T) {
	fixture := loadPluralContractFixture(t, "testdata/translator_plural_contract.json")

	if len(fixture.LoaderPaths) == 0 {
		t.Fatalf("plural fixture missing loader paths")
	}

	loaderPaths := make([]string, len(fixture.LoaderPaths))
	for i, path := range fixture.LoaderPaths {
		loaderPaths[i] = filepath.Join("testdata", path)
	}

	loader := NewFileLoader(loaderPaths...)

	rulePaths := make([]string, len(fixture.RulePaths))
	for i, path := range fixture.RulePaths {
		rulePaths[i] = filepath.Join("testdata", path)
	}

	opts := []Option{
		WithLoader(loader),
		WithDefaultLocale(fixture.DefaultLocale),
		EnablePluralization(rulePaths...),
	}

	if len(fixture.Locales) > 0 {
		opts = append(opts, WithLocales(fixture.Locales...))
	}

	cfg, err := NewConfig(opts...)
	if err != nil {
		t.Fatalf("NewConfig: %v", err)
	}

	translator, err := cfg.BuildTranslator()
	if err != nil {
		t.Fatalf("BuildTranslator: %v", err)
	}

	metaTranslator, ok := translator.(metadataTranslator)
	if !ok {
		t.Fatalf("translator does not expose metadata")
	}

	for _, tc := range fixture.Lookups {
		name := tc.Name
		if name == "" {
			name = tc.Locale + "/" + tc.Key
		}

		t.Run(name, func(t *testing.T) {
			args := make([]any, 0, 1)
			var originalCount any
			if tc.Count != nil {
				originalCount = countArgument(*tc.Count)
				args = append(args, WithCount(originalCount))
			}

			text, metadata, err := metaTranslator.TranslateWithMetadata(tc.Locale, tc.Key, args...)
			if err != nil {
				t.Fatalf("TranslateWithMetadata(%q,%q): %v", tc.Locale, tc.Key, err)
			}

			if text != tc.Want {
				t.Fatalf("TranslateWithMetadata(%q,%q) = %q want %q", tc.Locale, tc.Key, text, tc.Want)
			}

			if tc.Count == nil {
				if _, ok := metadata[metadataPluralCategory]; ok {
					t.Fatalf("unexpected plural category metadata for %q", tc.Name)
				}
				return
			}

			gotCategory, ok := metadata[metadataPluralCategory]
			if !ok {
				t.Fatalf("missing plural category metadata for %q", tc.Name)
			}

			if category, ok := categoryString(gotCategory); !ok || category != tc.Category {
				t.Fatalf("plural category for %q = %v want %q", tc.Name, gotCategory, tc.Category)
			}

			if value, ok := metadata[metadataPluralCount]; !ok || !reflect.DeepEqual(value, originalCount) {
				t.Fatalf("plural count for %q = %v want %v", tc.Name, metadata[metadataPluralCount], originalCount)
			}

			missingMeta, hasMissing := metadata[metadataPluralMissing]
			if tc.Missing {
				payload, ok := missingMeta.(map[string]any)
				if !hasMissing || !ok {
					t.Fatalf("expected plural missing metadata for %q", tc.Name)
				}

				requested, ok := categoryString(payload["requested"])
				if !ok || requested != tc.Category {
					t.Fatalf("requested plural category for %q = %v want %q", tc.Name, payload["requested"], tc.Category)
				}

				fallback, ok := categoryString(payload["fallback"])
				if !ok || fallback != string(PluralOther) {
					t.Fatalf("fallback plural category for %q = %v want %q", tc.Name, payload["fallback"], PluralOther)
				}
			} else if hasMissing {
				t.Fatalf("unexpected plural missing metadata for %q", tc.Name)
			}
		})
	}
}

func loadPluralContractFixture(t *testing.T, path string) pluralContractFixture {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plural fixture %s: %v", path, err)
	}

	var fixture pluralContractFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("unmarshal plural fixture %s: %v", path, err)
	}

	return fixture
}

func countArgument(value float64) any {
	rounded := math.Round(value)
	if math.Abs(value-rounded) < 1e-9 {
		return int(rounded)
	}
	return value
}

func categoryString(value any) (string, bool) {
	switch v := value.(type) {
	case PluralCategory:
		return string(v), true
	case string:
		return v, true
	default:
		return "", false
	}
}
