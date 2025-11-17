package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/goliatone/go-notifications/adapters/gocms"
	"github.com/goliatone/go-notifications/pkg/domain"
)

func main() {
	snapshot := gocms.BlockVersionSnapshot{
		Configuration: map[string]any{"layout": "hero"},
		Metadata:      map[string]any{"definition": "demo.notification"},
		Translations: []gocms.BlockTranslationSnapshot{
			{
				Locale: "locale-en",
				Content: map[string]any{
					"subject": "Welcome {{ Name }}",
					"body":    "<p>Hello {{ Name }}</p>",
					"blocks": []any{
						map[string]any{
							"type": "richtext",
							"body": "<p>{{ Name }}</p>",
						},
					},
				},
			},
			{
				Locale: "es",
				Content: map[string]any{
					"subject": "Bienvenida {{ Name }}",
					"blocks": []any{
						map[string]any{
							"type": "richtext",
							"body": "<p>{{ Name }}</p>",
						},
					},
				},
				AttributeOverrides: map[string]any{
					"body": "<p>Hola {{ Name }}</p>",
				},
			},
		},
	}

	spec := gocms.TemplateSpec{
		Code:        "welcome",
		Channel:     "email",
		Description: "Sample import via go-cms snapshot",
		Schema:      domain.TemplateSchema{Required: []string{"Name"}},
		Metadata:    domain.JSONMap{"source": "cms"},
		ResolveLocale: func(raw string) (string, error) {
			if raw == "locale-en" {
				return "en", nil
			}
			return raw, nil
		},
	}

	templates, err := gocms.TemplatesFromBlockSnapshot(spec, snapshot)
	if err != nil {
		log.Fatalf("translate snapshot: %v", err)
	}
	for _, tpl := range templates {
		encoded, _ := json.MarshalIndent(tpl.Source.Payload, "", "  ")
		fmt.Printf("Template %s/%s\n%s\n\n", tpl.Channel, tpl.Locale, encoded)
	}
}
