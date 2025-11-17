package gocms

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/templates"
)

func TestTemplatesFromBlockSnapshot(t *testing.T) {
	snapshot := BlockVersionSnapshot{
		Configuration: map[string]any{"layout": "hero"},
		Metadata:      map[string]any{"definition": "notification.hero"},
		Translations: []BlockTranslationSnapshot{
			{
				Locale: "locale-en",
				Content: map[string]any{
					"subject": "Welcome {{ Name }}",
					"body":    "<p>Hello {{ Name }}</p>",
					"blocks": []any{
						map[string]any{"type": "richtext", "body": "<p>{{ Name }}</p>"},
					},
				},
			},
			{
				Locale: "es",
				Content: map[string]any{
					"subject": "Bienvenida {{ Name }}",
					"blocks": []any{
						map[string]any{"type": "richtext", "body": "<p>{{ Name }}</p>"},
					},
				},
				AttributeOverrides: map[string]any{
					"body": "<p>Hola {{ Name }}</p>",
				},
			},
		},
	}

	spec := TemplateSpec{
		Code:        "welcome",
		Channel:     "email",
		Description: "go-cms block imports",
		Schema:      domain.TemplateSchema{Required: []string{"Name"}},
		Metadata:    domain.JSONMap{"source": "cms"},
		ResolveLocale: func(raw string) (string, error) {
			if raw == "locale-en" {
				return "en", nil
			}
			return raw, nil
		},
	}

	inputs, err := TemplatesFromBlockSnapshot(spec, snapshot)
	if err != nil {
		t.Fatalf("TemplatesFromBlockSnapshot: %v", err)
	}
	if len(inputs) != 2 {
		t.Fatalf("expected 2 templates, got %d", len(inputs))
	}

	byLocale := make(map[string]templates.TemplateInput, len(inputs))
	for _, tpl := range inputs {
		byLocale[tpl.Locale] = tpl
	}

	en, ok := byLocale["en"]
	if !ok {
		t.Fatalf("expected en template")
	}

	if en.Source.Type != TemplateSourceType {
		t.Fatalf("unexpected source type: %s", en.Source.Type)
	}

	subject, _ := en.Source.Payload["subject"].(string)
	if subject == "" {
		t.Fatalf("expected subject in payload: %#v", en.Source.Payload)
	}

	body, _ := en.Source.Payload["body"].(string)
	if body == "" {
		t.Fatalf("expected body in payload")
	}

	blocks, ok := en.Source.Payload["blocks"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("expected blocks array in payload: %#v", en.Source.Payload["blocks"])
	}

	config, ok := en.Source.Payload["configuration"].(map[string]any)
	if !ok || config["layout"] != "hero" {
		t.Fatalf("expected configuration clone in payload: %#v", en.Source.Payload["configuration"])
	}

	metadata, ok := en.Source.Payload["metadata"].(map[string]any)
	if !ok || metadata["definition"] != "notification.hero" {
		t.Fatalf("expected metadata clone in payload: %#v", en.Source.Payload["metadata"])
	}

	if en.Metadata["source"] != "cms" {
		t.Fatalf("expected metadata set on template input: %#v", en.Metadata)
	}
	en.Metadata["source"] = "mutated"
	if spec.Metadata["source"] != "cms" {
		t.Fatalf("expected spec metadata to remain unchanged")
	}

	es, ok := byLocale["es"]
	if !ok {
		t.Fatalf("expected es template")
	}
	esBody, _ := es.Source.Payload["body"].(string)
	if esBody != "<p>Hola {{ Name }}</p>" {
		t.Fatalf("expected override body, got %q", esBody)
	}
	if es.Source.Payload["attribute_overrides"] == nil {
		t.Fatalf("expected attribute overrides in payload")
	}
}

func TestTemplatesFromWidgetDocument(t *testing.T) {
	doc := WidgetDocument{
		Configuration: map[string]any{"widget": "notification"},
		Metadata:      map[string]any{"scope": "dashboard"},
		Translations: []WidgetTranslation{
			{
				Locale: "en",
				Content: map[string]any{
					"subject": "Widget Subject",
					"sections": []map[string]any{
						{"body": "<p>{{ Name }}</p>"},
					},
				},
			},
		},
	}
	spec := TemplateSpec{
		Code:    "widget.welcome",
		Channel: "email",
		Fields: FieldMapping{
			Blocks: "sections",
		},
	}
	inputs, err := TemplatesFromWidgetDocument(spec, doc)
	if err != nil {
		t.Fatalf("TemplatesFromWidgetDocument: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("expected 1 template, got %d", len(inputs))
	}
	payload := inputs[0].Source.Payload
	if payload == nil {
		t.Fatalf("expected payload")
	}
	subject, _ := payload["subject"].(string)
	if subject == "" {
		t.Fatalf("expected subject in payload: %#v", payload)
	}
	blocks, _ := payload["blocks"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("expected blocks slice: %#v", payload["blocks"])
	}
	content, _ := payload["content"].(map[string]any)
	if _, ok := content["sections"]; !ok {
		buf, _ := json.Marshal(payload)
		t.Fatalf("expected content clone (payload=%s)", buf)
	}
}

func TestTemplatesFromBlockSnapshotErrors(t *testing.T) {
	_, err := TemplatesFromBlockSnapshot(TemplateSpec{}, BlockVersionSnapshot{})
	if !errors.Is(err, ErrCodeRequired) {
		t.Fatalf("expected ErrCodeRequired, got %v", err)
	}

	_, err = TemplatesFromBlockSnapshot(TemplateSpec{Code: "code"}, BlockVersionSnapshot{})
	if !errors.Is(err, ErrChannelRequired) {
		t.Fatalf("expected ErrChannelRequired, got %v", err)
	}

	_, err = TemplatesFromBlockSnapshot(TemplateSpec{Code: "code", Channel: "email"}, BlockVersionSnapshot{})
	if !errors.Is(err, ErrNoTranslations) {
		t.Fatalf("expected ErrNoTranslations, got %v", err)
	}

	_, err = TemplatesFromBlockSnapshot(
		TemplateSpec{Code: "code", Channel: "email"},
		BlockVersionSnapshot{
			Translations: []BlockTranslationSnapshot{
				{Locale: "", Content: map[string]any{"subject": "x"}},
			},
		},
	)
	if !errors.Is(err, ErrLocaleIdentifierMissing) {
		t.Fatalf("expected ErrLocaleIdentifierMissing, got %v", err)
	}
}
