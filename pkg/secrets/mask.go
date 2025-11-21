package secrets

import (
	"strings"

	masker "github.com/goliatone/go-masker"
)

var defaultSecretFields = []string{
	"token", "access_token", "refresh_token",
	"api_key", "apikey", "apiKey",
	"client_secret", "secret", "signing_key",
	"chat_id", "webhook_url", "from", "from_email",
}

func init() {
	// Register common secret-ish fields so masking uses sane defaults.
	for _, field := range defaultSecretFields {
		// Preserve a few characters where possible; fallback to filled if unknown to masker.
		masker.Default.RegisterMaskField(field, "preserveEnds(2,2)")
	}
}

// MaskValues returns a masked copy of the provided map for safe logging.
// Keys are treated as field names; values are converted to string prior to masking.
func MaskValues(values map[Reference]SecretValue) map[string]any {
	if len(values) == 0 {
		return nil
	}
	masked := make(map[string]any, len(values))
	for ref, val := range values {
		// Use the key name (or provider/key combo) to drive masking rules.
		keyName := ref.Key
		if strings.TrimSpace(keyName) == "" {
			keyName = ref.Provider
		}
		safe := maskString(keyName, string(val.Data))
		masked[keyName] = map[string]any{
			"value":   safe,
			"version": val.Version,
		}
	}
	return masked
}

func maskString(tag, value string) string {
	if value == "" {
		return ""
	}
	// Use a conservative mask type; treating tag as a mask type is unreliable.
	if masked, err := masker.Default.String("preserveEnds(2,2)", value); err == nil {
		return masked
	}
	// Fallback masking if no rule is registered.
	runes := []rune(value)
	if len(runes) <= 4 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:2]) + strings.Repeat("*", len(runes)-4) + string(runes[len(runes)-2:])
}
