package templates

import (
	"fmt"
	"strings"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/links"
)

func defaultHelperFuncs() map[string]any {
	return map[string]any{
		"secure_link": secureLink,
	}
}

func secureLink(args ...any) string {
	var data map[string]any
	var key string
	for _, arg := range args {
		switch v := arg.(type) {
		case map[string]any:
			data = v
		case domain.JSONMap:
			data = map[string]any(v)
		case string:
			if key == "" {
				key = v
			}
		}
	}
	if key == "" {
		key = links.ResolvedURLActionKey
	}
	if data == nil {
		return ""
	}
	if value := stringFromTemplateValue(data[key]); value != "" {
		return value
	}
	if key == links.ResolvedURLActionKey {
		if value := stringFromTemplateValue(data[links.ResolvedURLKey]); value != "" {
			return value
		}
	}
	return ""
}

func stringFromTemplateValue(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}
