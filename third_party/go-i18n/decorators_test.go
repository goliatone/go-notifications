package i18n

import "testing"

type recordingHook struct {
	beforeCalls int
	afterCalls  int
	lastErr     error
	lastResult  string
	lastLocale  string
	lastKey     string
}

func (h *recordingHook) BeforeTranslate(ctx *TranslatorHookContext) {
	h.beforeCalls++
	h.lastLocale = ctx.Locale
	h.lastKey = ctx.Key
}

func (h *recordingHook) AfterTranslate(ctx *TranslatorHookContext) {
	h.afterCalls++
	h.lastErr = ctx.Error
	h.lastResult = ctx.Result
}

type mutatingHook struct{}

func (mutatingHook) BeforeTranslate(ctx *TranslatorHookContext) {
	ctx.Locale = "en"
	ctx.Key = "home.title"
	ctx.Args = append([]any(nil), ctx.Args...)
}

func (mutatingHook) AfterTranslate(ctx *TranslatorHookContext) {
	ctx.Result = ctx.Result + "!"
	ctx.Error = nil
}

func TestWrapTranslatorWithHooks(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
	})

	base, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	recorder := &recordingHook{}
	translator := WrapTranslatorWithHooks(base, recorder)

	got, err := translator.Translate("en", "home.title")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if got != "Welcome" {
		t.Fatalf("Translate() = %q want Welcome", got)
	}

	if recorder.beforeCalls != 1 || recorder.afterCalls != 1 {
		t.Fatalf("unexpected hook counts before=%d after=%d", recorder.beforeCalls, recorder.afterCalls)
	}

	if recorder.lastLocale != "en" || recorder.lastKey != "home.title" {
		t.Fatalf("hook saw locale/key %s/%s", recorder.lastLocale, recorder.lastKey)
	}

	if recorder.lastErr != nil {
		t.Fatalf("expected nil error in hook, got %v", recorder.lastErr)
	}

	if recorder.lastResult != "Welcome" {
		t.Fatalf("expected hook result Welcome, got %q", recorder.lastResult)
	}
}

func TestWrapTranslatorWithHooksError(t *testing.T) {
	store := NewStaticStore(nil)
	base, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	recorder := &recordingHook{}
	translator := WrapTranslatorWithHooks(base, recorder)

	if _, err := translator.Translate("en", "missing"); err != ErrMissingTranslation {
		t.Fatalf("expected ErrMissingTranslation, got %v", err)
	}

	if recorder.lastErr != ErrMissingTranslation {
		t.Fatalf("hook saw err %v, want %v", recorder.lastErr, ErrMissingTranslation)
	}
}

func TestHookedTranslatorUsesMutatedContext(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
	})

	base, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	translator := WrapTranslatorWithHooks(base, mutatingHook{})

	got, err := translator.Translate("es", "ignored")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if got != "Welcome!" {
		t.Fatalf("expected mutated result, got %q", got)
	}
}

func TestTranslationHookFuncsMutation(t *testing.T) {
	store := NewStaticStore(Translations{
		"en": newStringCatalog("en", map[string]string{"home.title": "Welcome"}),
	})

	base, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	hook := TranslationHookFuncs{
		Before: func(ctx *TranslatorHookContext) {
			ctx.Locale = "en"
			ctx.Key = "home.title"
		},
		After: func(ctx *TranslatorHookContext) {
			ctx.Result = ctx.Result + "!"
		},
	}

	translator := WrapTranslatorWithHooks(base, hook)

	got, err := translator.Translate("es", "ignored")
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if got != "Welcome!" {
		t.Fatalf("expected hook funcs mutation, got %q", got)
	}
}

func TestHookedTranslatorCapturesPluralMetadata(t *testing.T) {
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

	base, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	var plural PluralHookMetadata
	hook := TranslationHookFuncs{
		After: func(ctx *TranslatorHookContext) {
			plural, _ = ctx.PluralMetadata()
		},
	}

	translator := WrapTranslatorWithHooks(base, hook)

	got, err := translator.Translate("en", "cart.items", WithCount(3))
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if got != "You have 3 items" {
		t.Fatalf("unexpected result: %q", got)
	}

	if plural.Category != PluralOther {
		t.Fatalf("expected plural.category metadata %v, got %v", PluralOther, plural.Category)
	}

	if plural.Count != 3 {
		t.Fatalf("expected plural.count metadata 3, got %v", plural.Count)
	}

	if plural.Message != "You have {count} items" {
		t.Fatalf("expected plural message, got %q", plural.Message)
	}

	if plural.Missing != nil {
		t.Fatalf("did not expect missing plural event, got %#v", plural.Missing)
	}
}

func TestHookedTranslatorEmitsMissingPluralEvent(t *testing.T) {
	rules := &PluralRuleSet{
		Locale: "en",
		Rules: []PluralRule{
			{
				Category: PluralOne,
				Groups:   [][]PluralCondition{{{Operand: "i", Operator: OperatorEquals, Values: []float64{1}}}},
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
					PluralOther: {Template: "You have {count} items"},
				},
			},
		},
		CardinalRules: rules,
	}

	store := NewStaticStore(Translations{"en": catalog})
	base, err := NewSimpleTranslator(store, WithTranslatorDefaultLocale("en"))
	if err != nil {
		t.Fatalf("NewSimpleTranslator: %v", err)
	}

	var (
		plural     PluralHookMetadata
		rawMissing any
		hasMissing bool
	)
	hook := TranslationHookFuncs{
		After: func(ctx *TranslatorHookContext) {
			plural, _ = ctx.PluralMetadata()
			rawMissing, hasMissing = ctx.MetadataValue(metadataPluralMissing)
		},
	}

	translator := WrapTranslatorWithHooks(base, hook)

	got, err := translator.Translate("en", "cart.items", WithCount(1))
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if got != "You have 1 items" {
		t.Fatalf("unexpected result: %q", got)
	}

	if plural.Missing == nil {
		t.Fatalf("expected missing plural metadata (raw=%#v present=%v)", rawMissing, hasMissing)
	}

	if plural.Missing.Requested != PluralOne {
		t.Fatalf("expected requested plural one, got %v", plural.Missing.Requested)
	}

	if plural.Missing.Fallback != PluralOther {
		t.Fatalf("expected fallback plural other, got %v", plural.Missing.Fallback)
	}
}
