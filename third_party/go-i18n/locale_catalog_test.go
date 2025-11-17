package i18n

import "testing"

func TestNewLocaleCatalog_SanitizesFallbacks(t *testing.T) {
	catalog, err := newLocaleCatalog("en-US", map[string]LocaleDefinition{
		"en-US": {
			Fallbacks: []string{"en-US", " en ", "es", "es"},
		},
		"en": {},
		"es": {},
	})
	if err != nil {
		t.Fatalf("newLocaleCatalog: %v", err)
	}
	if catalog == nil {
		t.Fatal("expected catalog instance, got nil")
	}

	fallbacks := catalog.Fallbacks("en-US")
	expected := []string{"en", "es"}
	if len(fallbacks) != len(expected) {
		t.Fatalf("fallback length = %d; want %d", len(fallbacks), len(expected))
	}
	for i, locale := range expected {
		if fallbacks[i] != locale {
			t.Fatalf("fallback[%d] = %q; want %q", i, fallbacks[i], locale)
		}
	}

	if !catalog.IsActive("en-US") {
		t.Fatal("expected locale to default to active")
	}
}

func TestNewLocaleCatalog_Validation(t *testing.T) {
	if _, err := newLocaleCatalog("en", map[string]LocaleDefinition{
		"en": {
			Fallbacks: []string{"fr"},
		},
	}); err == nil {
		t.Fatal("expected error for undefined fallback, got nil")
	}

	if _, err := newLocaleCatalog("fr", map[string]LocaleDefinition{
		"en": {},
	}); err == nil {
		t.Fatal("expected error for missing default locale, got nil")
	}

	activeFalse := false
	catalog, err := newLocaleCatalog("en", map[string]LocaleDefinition{
		"en": {
			Active: &activeFalse,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error when building catalog: %v", err)
	}
	if catalog.IsActive("en") {
		t.Fatal("expected inactive locale when active=false")
	}
}
