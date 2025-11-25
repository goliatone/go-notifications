package onready

import (
	i18n "github.com/goliatone/go-i18n"
)

// Translations returns the default translation catalog for export-ready templates.
func Translations() i18n.Translations {
	return i18n.Translations{
		"en": newCatalog("en", map[string]string{
			"export.ready.subject":         `Your export "%s" is ready`,
			"export.ready.title":           `Export ready: %s`,
			"export.ready.body.intro":      `Your export "%s" (%s) is ready to download.`,
			"export.ready.body.link_label": "Download",
			"export.ready.body.expires":    "Link expires at %s",
			"export.ready.body.rows":       "Rows: %v",
			"export.ready.body.parts":      "Parts: %v",
			"export.ready.body.manifest":   "Manifest: %s",
			"export.ready.body.message":    "Note: %s",
		}),
	}
}

func newCatalog(locale string, entries map[string]string) *i18n.TranslationCatalog {
	catalog := &i18n.TranslationCatalog{
		Locale:   i18n.Locale{Code: locale},
		Messages: make(map[string]i18n.Message),
	}
	for key, template := range entries {
		msg := i18n.Message{}
		msg.SetContent(template)
		catalog.Messages[key] = msg
	}
	return catalog
}
