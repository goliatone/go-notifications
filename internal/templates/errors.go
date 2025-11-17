package templates

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrTranslatorRequired indicates the service cannot operate without a translator.
	ErrTranslatorRequired = errors.New("templates: translator is required")
	// ErrRendererConfig indicates the template renderer was misconfigured.
	ErrRendererConfig = errors.New("templates: renderer configuration is incomplete")
	// ErrTemplateNotFound is returned when a code/channel/locale combination has no variant.
	ErrTemplateNotFound = errors.New("templates: template variant not found")
	// ErrInvalidRenderRequest is returned when mandatory render inputs are missing.
	ErrInvalidRenderRequest = errors.New("templates: invalid render request")
)

// SchemaError surfaces missing or invalid placeholders in the render payload.
type SchemaError struct {
	Missing []string
}

func (e SchemaError) Error() string {
	if len(e.Missing) == 0 {
		return "templates: schema validation failed"
	}
	return fmt.Sprintf("templates: missing placeholders: %s", strings.Join(e.Missing, ", "))
}
