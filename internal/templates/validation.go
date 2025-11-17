package templates

import (
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
)

func sanitizeSchema(schema domain.TemplateSchema) domain.TemplateSchema {
	if schema.IsZero() {
		return schema
	}
	return domain.TemplateSchema{
		Required: uniqueStrings(schema.Required),
		Optional: uniqueStrings(schema.Optional),
	}
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, val := range values {
		key := strings.TrimSpace(val)
		if key == "" {
			continue
		}
		key = strings.ToLower(key)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, val)
	}
	return result
}

func validateSchemaData(schema domain.TemplateSchema, data map[string]any) error {
	if schema.IsZero() {
		return nil
	}
	if len(data) == 0 {
		return SchemaError{Missing: schema.Required}
	}
	missing := make([]string, 0)
	for _, field := range schema.Required {
		if !hasField(data, field) {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return SchemaError{Missing: missing}
	}
	return nil
}

func hasField(data map[string]any, path string) bool {
	if len(data) == 0 || path == "" {
		return false
	}
	current := any(data)
	for _, part := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case map[string]any:
			val, ok := typed[part]
			if !ok {
				return false
			}
			current = val
		default:
			return false
		}
	}
	return current != nil
}

func cloneJSONMap(input domain.JSONMap) domain.JSONMap {
	if len(input) == 0 {
		return nil
	}
	out := make(domain.JSONMap, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func cloneData(input map[string]any) map[string]any {
	if len(input) == 0 {
		return make(map[string]any)
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
