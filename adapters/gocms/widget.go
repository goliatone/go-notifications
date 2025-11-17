package gocms

import (
	"fmt"

	"github.com/goliatone/go-notifications/pkg/templates"
)

// WidgetDocument captures the structure go-cms emits when exporting widget instances.
type WidgetDocument struct {
	Configuration map[string]any      `json:"configuration"`
	Translations  []WidgetTranslation `json:"translations"`
	Metadata      map[string]any      `json:"metadata"`
}

// WidgetTranslation stores the locale + payload data for a widget.
type WidgetTranslation struct {
	Locale  string         `json:"locale"`
	Content map[string]any `json:"content"`
}

// TemplatesFromWidgetDocument converts widget translations into template inputs.
func TemplatesFromWidgetDocument(spec TemplateSpec, document WidgetDocument) ([]templates.TemplateInput, error) {
	spec, err := spec.normalized()
	if err != nil {
		return nil, err
	}
	if len(document.Translations) == 0 {
		return nil, ErrNoTranslations
	}
	inputs := make([]templates.TemplateInput, 0, len(document.Translations))
	for _, translation := range document.Translations {
		input, err := buildTemplateInput(spec, translationPayload{
			locale:        translation.Locale,
			content:       translation.Content,
			configuration: document.Configuration,
			metadata:      document.Metadata,
		})
		if err != nil {
			return nil, fmt.Errorf("gocms: build template for widget locale %q: %w", translation.Locale, err)
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}
