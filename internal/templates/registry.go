package templates

import (
	"strings"
	"sync"

	"github.com/goliatone/go-notifications/pkg/domain"
)

const sourceTypeGoCMSBlock = "gocms-block"

type templateVariant struct {
	template domain.NotificationTemplate
	schema   domain.TemplateSchema
}

func (v *templateVariant) Locale() string {
	if v == nil {
		return ""
	}
	return v.template.Locale
}

func (v *templateVariant) Revision() int {
	if v == nil {
		return 0
	}
	return v.template.Revision
}

func (v *templateVariant) Channel() string {
	if v == nil {
		return ""
	}
	return v.template.Channel
}

func (v *templateVariant) Subject() string {
	if v == nil {
		return ""
	}
	if v.template.Subject != "" {
		return v.template.Subject
	}
	return sourceField(v.template.Source, "subject")
}

func (v *templateVariant) Body() string {
	if v == nil {
		return ""
	}
	if v.template.Body != "" {
		return v.template.Body
	}
	return sourceField(v.template.Source, "body")
}

func (v *templateVariant) Metadata() domain.JSONMap {
	if v == nil {
		return nil
	}
	return cloneJSONMap(v.template.Metadata)
}

func (v *templateVariant) Source() domain.TemplateSource {
	if v == nil {
		return domain.TemplateSource{}
	}
	return v.template.Source
}

func (v *templateVariant) Schema() domain.TemplateSchema {
	if v == nil {
		return domain.TemplateSchema{}
	}
	return v.schema
}

type definitionEntry struct {
	code     string
	schema   domain.TemplateSchema
	variants map[string]map[string]*templateVariant // channel -> locale -> variant
}

type registry struct {
	mu          sync.RWMutex
	definitions map[string]*definitionEntry
}

func newRegistry() *registry {
	return &registry{
		definitions: make(map[string]*definitionEntry),
	}
}

func (r *registry) Upsert(tpl domain.NotificationTemplate) {
	if tpl.Code == "" || tpl.Channel == "" || tpl.Locale == "" {
		return
	}

	codeKey := normalizeKey(tpl.Code)
	channelKey := normalizeKey(tpl.Channel)
	localeKey := normalizeKey(tpl.Locale)

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.definitions[codeKey]
	if !ok {
		entry = &definitionEntry{
			code:     tpl.Code,
			variants: make(map[string]map[string]*templateVariant),
		}
		r.definitions[codeKey] = entry
	}

	schema := sanitizeSchema(tpl.Schema)
	if schema.IsZero() {
		schema = entry.schema
	} else {
		entry.schema = schema
	}

	if entry.variants[channelKey] == nil {
		entry.variants[channelKey] = make(map[string]*templateVariant)
	}

	current := entry.variants[channelKey][localeKey]
	if current != nil && current.Revision() > tpl.Revision {
		return
	}

	entry.variants[channelKey][localeKey] = &templateVariant{
		template: tpl,
		schema:   schema,
	}
}

func (r *registry) Resolve(code, channel string, locales []string) (*templateVariant, string, error) {
	if code == "" || channel == "" {
		return nil, "", ErrTemplateNotFound
	}
	codeKey := normalizeKey(code)
	channelKey := normalizeKey(channel)

	r.mu.RLock()
	entry := r.definitions[codeKey]
	r.mu.RUnlock()
	if entry == nil {
		return nil, "", ErrTemplateNotFound
	}

	channelVariants := entry.variants[channelKey]
	if len(channelVariants) == 0 {
		return nil, "", ErrTemplateNotFound
	}

	seen := make(map[string]struct{}, len(locales))
	for _, candidate := range locales {
		locKey := normalizeLocale(candidate)
		if locKey == "" {
			continue
		}
		if _, ok := seen[locKey]; ok {
			continue
		}
		seen[locKey] = struct{}{}
		if variant := channelVariants[locKey]; variant != nil {
			return variant, candidate, nil
		}
	}
	return nil, "", ErrTemplateNotFound
}

func normalizeKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeLocale(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func sourceField(src domain.TemplateSource, key string) string {
	if !strings.EqualFold(src.Type, sourceTypeGoCMSBlock) || src.Payload == nil {
		return ""
	}
	if v, ok := src.Payload[key]; ok {
		if text, ok := v.(string); ok {
			return text
		}
	}
	// support nested block payloads (e.g., map["block"])
	if blocks, ok := src.Payload["blocks"].([]any); ok {
		for _, block := range blocks {
			if blockMap, ok := block.(map[string]any); ok {
				if val, ok := blockMap[key]; ok {
					if text, ok := val.(string); ok {
						return text
					}
				}
			}
		}
	}
	return ""
}
