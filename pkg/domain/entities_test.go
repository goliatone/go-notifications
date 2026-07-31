package domain

import (
	"database/sql/driver"
	"testing"
)

func TestJSONValueTypesEmitPortableText(t *testing.T) {
	values := []interface {
		Value() (driver.Value, error)
	}{
		JSONMap{"key": "value"},
		StringList{"one", "two"},
		TemplateSource{Type: "inline", Payload: JSONMap{"key": "value"}},
		TemplateSchema{Required: []string{"key"}},
	}
	for _, value := range values {
		got, err := value.Value()
		if err != nil {
			t.Fatalf("%T value: %v", value, err)
		}
		if _, ok := got.(string); !ok {
			t.Fatalf("%T returned %T; PostgreSQL JSONB requires JSON text", value, got)
		}
	}
}
