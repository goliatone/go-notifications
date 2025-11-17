package i18n

import "testing"

func TestSimpleTranslatorTranslate(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{
			"home.title":    "Welcome",
			"home.greeting": "Hello %s",
		}),
		"es": newStringCatalog("es", map[string]string{
			"home.title": "Bienvenido",
		}),
	})

	translator, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	tests := []struct {
		name    string
		locale  string
		key     string
		args    []any
		want    string
		wantErr error
	}{
		{
			name:   "explicit locale",
			locale: "es",
			key:    "home.title",
			want:   "Bienvenido",
		},
		{
			name: "default locale",
			key:  "home.title",
			want: "Welcome",
		},
		{
			name:   "format args",
			locale: "en",
			key:    "home.greeting",
			args:   []any{"Alice"},
			want:   "Hello Alice",
		},
		{
			name:    "missing key",
			locale:  "en",
			key:     "missing",
			wantErr: ErrMissingTranslation,
		},
		{
			name:    "missing locale",
			locale:  "",
			key:     "spanish",
			wantErr: ErrMissingTranslation,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := translator.Translate(tc.locale, tc.key, tc.args...)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("expected err %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}

			if got != tc.want {
				t.Fatalf("Translate() = %q want %q", got, tc.want)
			}
		})
	}
}

func TestSimpleTranslatorPluralSelection(t *testing.T) {
	rules := &PluralRuleSet{
		Locale: "en",
		Rules: []PluralRule{
			{
				Category: PluralOne,
				Groups: [][]PluralCondition{
					{
						{Operand: "i", Operator: OperatorEquals, Values: []float64{1}},
						{Operand: "v", Operator: OperatorEquals, Values: []float64{0}},
					},
				},
			},
			{Category: PluralOther},
		},
	}

	catalog := &TranslationCatalog{
		Locale: Locale{Code: "en"},
		Messages: map[string]Message{
			"cart.items": {
				MessageMetadata: MessageMetadata{ID: "cart.items", Locale: "en"},
				Variants: map[PluralCategory]MessageVariant{
					PluralOne:   {Template: "You have {count} item"},
					PluralOther: {Template: "You have {count} items"},
				},
			},
		},
		CardinalRules: rules,
	}

	store := NewStaticStore(Translations{"en": catalog})

	translator, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	tests := []struct {
		name  string
		count any
		want  string
	}{
		{name: "singular", count: 1, want: "You have 1 item"},
		{name: "plural", count: 5, want: "You have 5 items"},
		{name: "no count", count: nil, want: "You have {count} items"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := []any{}
			if tc.count != nil {
				args = append(args, WithCount(tc.count))
			}
			got, err := translator.Translate("en", "cart.items", args...)
			if err != nil {
				t.Fatalf("Translate: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Translate() = %q want %q", got, tc.want)
			}
		})
	}
}

func TestSimpleTranslatorPluralFallbackToOther(t *testing.T) {
	rules := &PluralRuleSet{
		Locale: "en",
		Rules: []PluralRule{
			{
				Category: PluralOne,
				Groups: [][]PluralCondition{
					{
						{Operand: "i", Operator: OperatorEquals, Values: []float64{1}},
					},
				},
			},
			{Category: PluralOther},
		},
	}

	catalog := &TranslationCatalog{
		Locale: Locale{Code: "en"},
		Messages: map[string]Message{
			"invite.count": {
				MessageMetadata: MessageMetadata{ID: "invite.count", Locale: "en"},
				Variants: map[PluralCategory]MessageVariant{
					PluralOther: {Template: "Invites: {count}"},
				},
			},
		},
		CardinalRules: rules,
	}

	store := NewStaticStore(Translations{"en": catalog})

	translator, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	got, err := translator.Translate("en", "invite.count", WithCount(3))
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if got != "Invites: 3" {
		t.Fatalf("expected fallback to other variant, got %q", got)
	}
}

func TestSimpleTranslatorCustomFormatter(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{"home.greeting": "Hello %s"}),
	})

	rack := false
	formatter := FormatterFunc(func(template string, args ...any) (string, error) {
		rack = true
		return "custom", nil
	})

	translator, err := NewSimpleTranslator(store,
		WithTranslatorDefaultLocale("en"),
		WithTranslatorFormatter(formatter),
	)
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	got, err := translator.Translate("", "home.greeting", "bob")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if got != "custom" {
		t.Fatalf("Translate() = %q want custom", got)
	}

	if !rack {
		t.Fatal("expected formatter to be invoked")
	}
}

func TestSimpleTranslatorFallbackChain(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
	})

	resolver := NewStaticFallbackResolver()
	resolver.Set("es", "en")

	translator, err := NewSimpleTranslator(store,
		WithTranslatorDefaultLocale("en"),
		WithTranslatorFallbackResolver(resolver),
	)
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	got, err := translator.Translate("es", "home.title")
	if err != nil {
		t.Fatalf("Translate with fallback: %v", err)
	}

	if got != "Welcome" {
		t.Fatalf("Translate fallback = %q want Welcome", got)
	}
}

func TestSimpleTranslatorFallbackMiss(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
	})

	resolver := NewStaticFallbackResolver()
	resolver.Set("es", "fr")

	translator, err := NewSimpleTranslator(store,
		WithTranslatorDefaultLocale("en"),
		WithTranslatorFallbackResolver(resolver),
	)
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	_, err = translator.Translate("es", "unknown")
	if err != ErrMissingTranslation {
		t.Fatalf("expected ErrMissingTranslation, got %v", err)
	}
}
