package gocms

import (
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
)

func firstString(source map[string]any, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || len(source) == 0 {
		return ""
	}
	value := findValue(source, path)
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case []byte:
		return string(v)
	default:
		return ""
	}
}

func firstBlocks(source map[string]any, path string) []any {
	path = strings.TrimSpace(path)
	if path == "" || len(source) == 0 {
		return nil
	}
	value := findValue(source, path)
	return normalizeBlocks(value)
}

func findValue(source map[string]any, path string) any {
	if source == nil || path == "" {
		return nil
	}
	segments := strings.Split(path, ".")
	var current any = source
	for _, segment := range segments {
		key := strings.TrimSpace(segment)
		if key == "" {
			continue
		}
		switch typed := current.(type) {
		case map[string]any:
			current = typed[key]
		case domain.JSONMap:
			current = typed[key]
		default:
			return nil
		}
	}
	return current
}

func normalizeBlocks(value any) []any {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneValue(item)
		}
		return out
	case []map[string]any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneMap(item)
		}
		return out
	case []domain.JSONMap:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneJSONMap(item)
		}
		return out
	case map[string]any:
		return []any{cloneMap(v)}
	case domain.JSONMap:
		return []any{cloneJSONMap(v)}
	default:
		return nil
	}
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for key, value := range m {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneJSONMap(m domain.JSONMap) domain.JSONMap {
	if m == nil {
		return nil
	}
	out := make(domain.JSONMap, len(m))
	for key, value := range m {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneSlice(values []any) []any {
	if values == nil {
		return nil
	}
	out := make([]any, len(values))
	for i, value := range values {
		out[i] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneMap(v)
	case domain.JSONMap:
		return cloneJSONMap(v)
	case []any:
		return cloneSlice(v)
	case []map[string]any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneMap(item)
		}
		return out
	case []domain.JSONMap:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = cloneJSONMap(item)
		}
		return out
	default:
		return v
	}
}
