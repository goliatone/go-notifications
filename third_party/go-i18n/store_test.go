package i18n

import "testing"

func TestStaticStoreGet(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
		"es": newStringCatalog("es", map[string]string{"home.title": "Bienvenido"}),
	})

	tests := []struct {
		locale string
		key    string
		want   string
		ok     bool
	}{
		{locale: "en", key: "home.title", want: "Welcome", ok: true},
		{locale: "es", key: "home.title", want: "Bienvenido", ok: true},
		{locale: "en", key: "missing", want: "", ok: false},
		{locale: "fr", key: "home.title", want: "", ok: false},
	}

	for _, tc := range tests {
		got, ok := store.Get(tc.locale, tc.key)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("Get(%q,%q) = %q,%v want %q,%v", tc.locale, tc.key, got, ok, tc.want, tc.ok)
		}

		msg, mok := store.Message(tc.locale, tc.key)
		if tc.ok {
			if !mok || msg.Content() != tc.want {
				t.Fatalf("Message(%q,%q) = %+v,%v", tc.locale, tc.key, msg, mok)
			}
		} else if mok {
			t.Fatalf("unexpected message for %q/%q", tc.locale, tc.key)
		}
	}

	locales := store.Locales()
	if len(locales) != 2 || locales[0] != "en" || locales[1] != "es" {
		t.Fatalf("Locales() = %v", locales)
	}

	if _, ok := store.Rules("en"); ok {
		t.Fatal("expected no plural rules for en")
	}
}

func TestNewStaticStoreCopiesInput(t *testing.T) {
	src := Translations{
		"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
	}
	src["en"].CardinalRules = &PluralRuleSet{
		Locale: "en",
		Rules: []PluralRule{
			{Category: PluralOne},
			{Category: PluralOther},
		},
	}

	store := NewStaticStore(src)

	src["en"].Messages["home.title"] = Message{
		MessageMetadata: MessageMetadata{ID: "home.title", Locale: "en"},
		Variants:        map[PluralCategory]MessageVariant{PluralOther: {Template: "Changed"}},
	}
	src["en"].Messages["new"] = Message{
		MessageMetadata: MessageMetadata{ID: "new", Locale: "en"},
		Variants:        map[PluralCategory]MessageVariant{PluralOther: {Template: "new"}},
	}

	got, ok := store.Get("en", "home.title")
	if !ok || got != "Welcome" {
		t.Fatalf("expected snapshot to remain unchanged, got %q, ok=%v", got, ok)
	}

	msg, mok := store.Message("en", "home.title")
	if !mok || msg.Content() != "Welcome" {
		t.Fatalf("Message snapshot mismatch: %+v,%v", msg, mok)
	}

	msg.SetContent("Mutated")
	if got, _ := store.Get("en", "home.title"); got != "Welcome" {
		t.Fatalf("mutating returned message should not affect store, got %q", got)
	}

	if _, ok := store.Get("en", "new"); ok {
		t.Fatal("unexpected key copied from mutated input")
	}

	rules, rok := store.Rules("en")
	if !rok {
		t.Fatal("expected rules snapshot")
	}

	src["en"].CardinalRules.Rules[0].Category = PluralFew

	if got := rules.Rules[0].Category; got != PluralOne {
		t.Fatalf("rules snapshot mutated: %v", got)
	}
}

func TestNewStaticStoreFromLoader(t *testing.T) {
	called := false
	loader := LoaderFunc(func() (Translations, error) {
		called = true
		return Translations{
			"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
		}, nil
	})

	store, err := NewStaticStoreFromLoader(loader)
	if err != nil {
		t.Fatalf("NewStaticStoreFromLoader: %v", err)
	}

	if !called {
		t.Fatal("loader not invoked")
	}

	if msg, ok := store.Get("en", "home.title"); !ok || msg != "Welcome" {
		t.Fatalf("Get returned %q,%v", msg, ok)
	}

	if message, ok := store.Message("en", "home.title"); !ok || message.Content() != "Welcome" {
		t.Fatalf("Message returned %+v,%v", message, ok)
	}

	if _, ok := store.Rules("en"); ok {
		t.Fatal("expected no rules from loader catalog")
	}
}

func TestNewStaticStoreFromLoaderNil(t *testing.T) {
	store, err := NewStaticStoreFromLoader(nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if store == nil {
		t.Fatal("expected non-nil store")
	}

	if locales := store.Locales(); len(locales) != 0 {
		t.Fatalf("expected no locales, got %v", locales)
	}
}
