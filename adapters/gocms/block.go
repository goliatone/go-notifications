package gocms

import (
	"fmt"

	"github.com/goliatone/go-notifications/pkg/templates"
)

// BlockVersionSnapshot mirrors the JSON snapshot emitted by go-cms block versions.
type BlockVersionSnapshot struct {
	Configuration map[string]any             `json:"configuration"`
	Translations  []BlockTranslationSnapshot `json:"translations"`
	Metadata      map[string]any             `json:"metadata"`
}

// BlockTranslationSnapshot mirrors the translation payload captured inside a block snapshot.
type BlockTranslationSnapshot struct {
	Locale             string         `json:"locale"`
	Content            map[string]any `json:"content"`
	AttributeOverrides map[string]any `json:"attribute_overrides"`
}

// TemplatesFromBlockSnapshot converts the provided go-cms block snapshot into template inputs.
func TemplatesFromBlockSnapshot(spec TemplateSpec, snapshot BlockVersionSnapshot) ([]templates.TemplateInput, error) {
	spec, err := spec.normalized()
	if err != nil {
		return nil, err
	}
	if len(snapshot.Translations) == 0 {
		return nil, ErrNoTranslations
	}
	inputs := make([]templates.TemplateInput, 0, len(snapshot.Translations))
	for _, tr := range snapshot.Translations {
		input, err := buildTemplateInput(spec, translationPayload{
			locale:        tr.Locale,
			content:       tr.Content,
			overrides:     tr.AttributeOverrides,
			configuration: snapshot.Configuration,
			metadata:      snapshot.Metadata,
		})
		if err != nil {
			return nil, fmt.Errorf("gocms: build template for locale %q: %w", tr.Locale, err)
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}
