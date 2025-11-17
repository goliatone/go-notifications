package i18n

import "strings"

func newStringCatalog(locale string, entries map[string]string) *TranslationCatalog {
	messages := make(map[string]Message, len(entries))
	for key, template := range entries {
		domain := ""
		if idx := strings.Index(key, "."); idx > 0 {
			domain = key[:idx]
		}
		messages[key] = Message{
			MessageMetadata: MessageMetadata{
				ID:     key,
				Domain: domain,
				Locale: locale,
			},
			Variants: map[PluralCategory]MessageVariant{
				PluralOther: {Template: template},
			},
		}
	}
	return &TranslationCatalog{
		Locale:   Locale{Code: locale},
		Messages: messages,
	}
}
