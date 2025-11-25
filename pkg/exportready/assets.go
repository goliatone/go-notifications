package exportready

import (
	"github.com/goliatone/go-notifications/pkg/domain"
)

const (
	// DefinitionCode is the default notification definition code for export-ready events.
	DefinitionCode = "export.ready"
	// EmailTemplateCode is the default template code for the email channel.
	EmailTemplateCode = "export.ready.email"
	// InAppTemplateCode is the default template code for the in-app channel.
	InAppTemplateCode = "export.ready.inapp"
)

var templateSchema = domain.TemplateSchema{
	Required: []string{"FileName", "Format", "URL", "ExpiresAt"},
	Optional: []string{"Rows", "Parts", "ManifestURL", "Message"},
}

// Definition returns the default export-ready notification definition with channel mappings.
func Definition() domain.NotificationDefinition {
	return domain.NotificationDefinition{
		Code:        DefinitionCode,
		Name:        "Export Ready",
		Description: "Export-ready notification with email and in-app variants",
		Severity:    "info",
		Category:    "export",
		Channels:    domain.StringList{"email", "in-app"},
		TemplateKeys: domain.StringList{
			"email:" + EmailTemplateCode,
			"in-app:" + InAppTemplateCode,
		},
		Metadata: domain.JSONMap{
			"placeholders_required": templateSchema.Required,
			"placeholders_optional": templateSchema.Optional,
			"default_locale":        "en",
		},
	}
}

// Templates returns the default export-ready templates for the email and in-app channels.
func Templates() []domain.NotificationTemplate {
	return []domain.NotificationTemplate{
		{
			Code:        EmailTemplateCode,
			Channel:     "email",
			Locale:      "en",
			Subject:     `{{ t(locale, "export.ready.subject", FileName) }}`,
			Body:        emailBody,
			Description: "Email template for export-ready notifications",
			Format:      "text/html",
			Schema:      templateSchema,
			Metadata: domain.JSONMap{
				"category": "export",
				"cta":      "download",
			},
		},
		{
			Code:        InAppTemplateCode,
			Channel:     "in-app",
			Locale:      "en",
			Subject:     `{{ t(locale, "export.ready.title", FileName) }}`,
			Body:        inAppBody,
			Description: "In-app template for export-ready notifications",
			Format:      "text/markdown",
			Schema:      templateSchema,
			Metadata: domain.JSONMap{
				"category": "export",
				"cta":      "open",
			},
		},
	}
}

// Schema returns the template schema describing required/optional placeholders.
func Schema() domain.TemplateSchema {
	return templateSchema
}

const emailBody = `
{{ t(locale, "export.ready.body.intro", FileName, Format) }}
{{ t(locale, "export.ready.body.link_label") }}: {{ URL }}
{{ t(locale, "export.ready.body.expires", ExpiresAt) }}
{{ if Rows }}{{ t(locale, "export.ready.body.rows", Rows) }}{{ end }}
{{ if Parts }}{{ t(locale, "export.ready.body.parts", Parts) }}{{ end }}
{{ if ManifestURL }}{{ t(locale, "export.ready.body.manifest", ManifestURL) }}{{ end }}
{{ if Message }}{{ t(locale, "export.ready.body.message", Message) }}{{ end }}
`

const inAppBody = `
{{ t(locale, "export.ready.body.intro", FileName, Format) }}
{{ t(locale, "export.ready.body.expires", ExpiresAt) }}
{{ if Message }}{{ t(locale, "export.ready.body.message", Message) }}{{ end }}
{{ t(locale, "export.ready.body.link_label") }}: {{ URL }}
{{ if ManifestURL }}{{ t(locale, "export.ready.body.manifest", ManifestURL) }}{{ end }}
{{ if Rows }}{{ t(locale, "export.ready.body.rows", Rows) }}{{ end }}
{{ if Parts }}{{ t(locale, "export.ready.body.parts", Parts) }}{{ end }}
`
