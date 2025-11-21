package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/secrets"
)

// ResolvingMessenger injects per-user addresses and credentials before delegating to an adapter.
type ResolvingMessenger struct {
	inner     adapters.Messenger
	directory *Directory
	secrets   secrets.Resolver
	logs      *DeliveryLogStore
	logger    logger.Logger
}

func (m ResolvingMessenger) Name() string {
	return m.inner.Name()
}

func (m ResolvingMessenger) Capabilities() adapters.Capability {
	return m.inner.Capabilities()
}

func (m ResolvingMessenger) Send(ctx context.Context, msg adapters.Message) error {
	if m.directory == nil {
		return errors.New("resolving messenger: directory not configured")
	}
	if msg.Metadata == nil {
		msg.Metadata = map[string]any{}
	}

	recipientID := strings.TrimSpace(msg.To)
	if recipientID == "" {
		return errors.New("resolving messenger: recipient id required")
	}

	var address string
	contact, err := m.directory.GetContact(ctx, recipientID)
	if err == nil {
		address = chooseAddress(contact, msg.Channel, msg.Provider)
		msg.Metadata["recipient_locale"] = contact.PreferredLocale
		msg.Metadata["recipient_email"] = contact.Email
		msg.Metadata["recipient_phone"] = contact.Phone
	} else if errors.Is(err, errContactNotFound) {
		return fmt.Errorf("resolving messenger: contact not found for %s", recipientID)
	} else {
		return err
	}
	if address != "" {
		msg.To = address
	}
	msg.Metadata["recipient_id"] = recipientID

	secretStrings := m.resolveSecretStrings(recipientID, msg.Channel, msg.Provider)
	for key, val := range secretStrings {
		msg.Metadata[key] = val
	}

	err = m.inner.Send(ctx, msg)
	status := "failed"
	if err == nil {
		status = "succeeded"
	}
	if m.logs != nil {
		entry := deliveryLogRecord{
			UserID:         recipientID,
			Channel:        msg.Channel,
			Provider:       msg.Provider,
			Address:        msg.To,
			Status:         status,
			Message:        firstNonEmpty(msg.Subject, msg.Body),
			DefinitionCode: fmt.Sprint(msg.Metadata["definition_code"]),
			CreatedAt:      time.Now().UTC(),
		}
		_ = m.logs.Record(ctx, entry)
	}
	return err
}

func (m ResolvingMessenger) resolveSecretStrings(userID, channel, provider string) map[string]string {
	out := make(map[string]string)
	if m.secrets == nil {
		return out
	}
	keys := secretKeysForProvider(channel, provider)
	if len(keys) == 0 {
		return out
	}
	refs := make([]secrets.Reference, 0, len(keys)*2)
	for _, key := range keys {
		refs = append(refs,
			secrets.Reference{Scope: secrets.ScopeUser, SubjectID: userID, Channel: channel, Provider: provider, Key: key},
			secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: channel, Provider: provider, Key: key},
		)
	}
	resolved, err := m.secrets.Resolve(refs...)
	if err != nil && !errors.Is(err, secrets.ErrNotFound) {
		m.logger.Warn("resolve secrets failed", logger.Field{Key: "error", Value: err}, logger.Field{Key: "provider", Value: provider})
		return out
	}
	for _, key := range keys {
		// Prefer user scope, then system.
		userRef := secrets.Reference{Scope: secrets.ScopeUser, SubjectID: userID, Channel: channel, Provider: provider, Key: key}
		systemRef := secrets.Reference{Scope: secrets.ScopeSystem, SubjectID: "default", Channel: channel, Provider: provider, Key: key}
		if val, ok := resolved[userRef]; ok {
			out[key] = string(val.Data)
			continue
		}
		if val, ok := resolved[systemRef]; ok {
			out[key] = string(val.Data)
		}
	}
	if fallback, ok := out["default"]; ok {
		for _, key := range keys {
			if out[key] == "" {
				out[key] = fallback
			}
		}
	}
	return out
}

func chooseAddress(contact *demoContact, channel, provider string) string {
	if contact == nil {
		return ""
	}
	channel = strings.ToLower(strings.TrimSpace(channel))
	provider = strings.ToLower(strings.TrimSpace(provider))

	switch channel {
	case "email":
		return contact.Email
	case "sms":
		return contact.Phone
	case "chat":
		if provider == "telegram" {
			return contact.TelegramChatID
		}
		if provider == "slack" || provider == "" {
			return contact.SlackID
		}
	case "slack":
		return contact.SlackID
	case "telegram":
		return contact.TelegramChatID
	}
	return ""
}

func secretKeysForProvider(channel, provider string) []string {
	provider = strings.ToLower(provider)
	channel = strings.ToLower(channel)
	keys := []string{}
	switch {
	case provider == "slack" || (provider == "" && channel == "slack"):
		keys = append(keys, "token")
	case provider == "telegram" || (provider == "" && channel == "telegram"):
		keys = append(keys, "token")
	case provider == "sendgrid" || (provider == "" && channel == "email"):
		keys = append(keys, "api_key", "from")
	case provider == "twilio" || (provider == "" && channel == "sms"):
		keys = append(keys, "auth_token", "from")
	}
	keys = append(keys, "default")
	return keys
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
