package main

import (
	"fmt"
	"slices"

	"github.com/goliatone/go-notifications/examples/web/config"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/adapters/console"
	"github.com/goliatone/go-notifications/pkg/adapters/mailgun"
	"github.com/goliatone/go-notifications/pkg/adapters/sendgrid"
	"github.com/goliatone/go-notifications/pkg/adapters/slack"
	"github.com/goliatone/go-notifications/pkg/adapters/telegram"
	"github.com/goliatone/go-notifications/pkg/adapters/twilio"
	"github.com/goliatone/go-notifications/pkg/adapters/whatsapp"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/secrets"
)

// AdapterRegistry holds information about configured adapters.
type AdapterRegistry struct {
	Adapters        []adapters.Messenger
	EnabledAdapters []string
	EnabledChannels []string
	AdapterConfig   config.AdapterConfig
}

// BuildAdapters detects and builds all configured adapters.
func BuildAdapters(lgr logger.Logger, cfg config.AdapterConfig, dir *Directory, resolver secrets.Resolver, logs *DeliveryLogStore) *AdapterRegistry {
	registry := &AdapterRegistry{
		Adapters:        make([]adapters.Messenger, 0),
		EnabledAdapters: make([]string, 0),
		EnabledChannels: make([]string, 0),
		AdapterConfig:   cfg,
	}

	// Console adapter is always enabled
	consoleAdapter := wrapMessenger(console.New(lgr), dir, resolver, logs, lgr)
	registry.Adapters = append(registry.Adapters, consoleAdapter)
	registry.EnabledAdapters = append(registry.EnabledAdapters, "console")
	registry.addChannels("console", "email", "email:console")

	registry.addChatAdapters(lgr, dir, resolver, logs)
	registry.addTwilio(lgr, dir, resolver, logs)
	registry.addEmailAdapters(lgr, dir, resolver, logs)
	registry.addWhatsApp(lgr, dir, resolver, logs)
	return registry
}

func (r *AdapterRegistry) addChatAdapters(
	lgr logger.Logger,
	dir *Directory,
	resolver secrets.Resolver,
	logs *DeliveryLogStore,
) {
	cfg := r.AdapterConfig
	if cfg.Slack.IsConfigured() {
		slackAdapter := wrapMessenger(slack.New(lgr, slack.WithConfig(slack.Config{
			Token:   cfg.Slack.Token,
			Channel: cfg.Slack.Channel,
			BaseURL: "https://slack.com/api",
			Timeout: config.DefaultAdapterTimeout,
		})), dir, resolver, logs, lgr)
		r.Adapters = append(r.Adapters, slackAdapter)
		r.EnabledAdapters = append(r.EnabledAdapters, "slack")
		r.addChannels("chat", "slack", "chat:slack")
	}

	if cfg.Telegram.IsConfigured() {
		telegramAdapter := wrapMessenger(telegram.New(lgr, telegram.WithConfig(telegram.Config{
			Token:   cfg.Telegram.BotToken,
			ChatID:  cfg.Telegram.ChatID,
			BaseURL: "https://api.telegram.org",
			Timeout: config.DefaultAdapterTimeout,
		})), dir, resolver, logs, lgr)
		r.Adapters = append(r.Adapters, telegramAdapter)
		r.EnabledAdapters = append(r.EnabledAdapters, "telegram")
		r.addChannels("chat", "chat:telegram", "telegram")
	}
}

func (r *AdapterRegistry) addTwilio(
	lgr logger.Logger,
	dir *Directory,
	resolver secrets.Resolver,
	logs *DeliveryLogStore,
) {
	cfg := r.AdapterConfig
	if cfg.Twilio.IsConfigured() {
		twilioAdapter := wrapMessenger(twilio.New(lgr, twilio.WithConfig(twilio.Config{
			AccountSID: cfg.Twilio.AccountSID,
			AuthToken:  cfg.Twilio.AuthToken,
			From:       cfg.Twilio.FromPhone,
			Timeout:    config.DefaultAdapterTimeout,
		})), dir, resolver, logs, lgr)
		r.Adapters = append(r.Adapters, twilioAdapter)
		r.EnabledAdapters = append(r.EnabledAdapters, "twilio")
		r.addChannels("sms", "sms:twilio")
	}
}

func (r *AdapterRegistry) addEmailAdapters(
	lgr logger.Logger,
	dir *Directory,
	resolver secrets.Resolver,
	logs *DeliveryLogStore,
) {
	cfg := r.AdapterConfig
	if cfg.SendGrid.IsConfigured() {
		fromEmail := cfg.SendGrid.FromEmail
		if cfg.SendGrid.FromName != "" {
			fromEmail = cfg.SendGrid.FromName + " <" + cfg.SendGrid.FromEmail + ">"
		}
		sendgridAdapter := wrapMessenger(sendgrid.New(lgr,
			sendgrid.WithAPIKey(cfg.SendGrid.APIKey),
			sendgrid.WithFrom(fromEmail),
			sendgrid.WithTimeout(30),
		), dir, resolver, logs, lgr)
		r.Adapters = append(r.Adapters, sendgridAdapter)
		r.EnabledAdapters = append(r.EnabledAdapters, "sendgrid")
		r.addChannels("email", "email:sendgrid")
	}

	if cfg.Mailgun.IsConfigured() {
		fromEmail := cfg.Mailgun.FromEmail
		if cfg.Mailgun.FromName != "" {
			fromEmail = cfg.Mailgun.FromName + " <" + cfg.Mailgun.FromEmail + ">"
		}
		mailgunAdapter := wrapMessenger(mailgun.New(lgr, mailgun.WithConfig(mailgun.Config{
			APIKey:     cfg.Mailgun.APIKey,
			Domain:     cfg.Mailgun.Domain,
			From:       fromEmail,
			TimeoutSec: 30,
		})), dir, resolver, logs, lgr)
		r.Adapters = append(r.Adapters, mailgunAdapter)
		r.EnabledAdapters = append(r.EnabledAdapters, "mailgun")
		r.addChannels("email", "email:mailgun")
	}
}

func (r *AdapterRegistry) addWhatsApp(
	lgr logger.Logger,
	dir *Directory,
	resolver secrets.Resolver,
	logs *DeliveryLogStore,
) {
	cfg := r.AdapterConfig
	if cfg.WhatsApp.IsConfigured() {
		whatsappAdapter := wrapMessenger(whatsapp.New(lgr, whatsapp.WithConfig(whatsapp.Config{
			Token:         cfg.WhatsApp.AuthToken,
			PhoneNumberID: cfg.WhatsApp.FromPhone,
			Timeout:       config.DefaultAdapterTimeout,
		})), dir, resolver, logs, lgr)
		r.Adapters = append(r.Adapters, whatsappAdapter)
		r.EnabledAdapters = append(r.EnabledAdapters, "whatsapp")
		r.addChannels("whatsapp", "whatsapp:whatsapp")
	}
}

func wrapMessenger(
	messenger adapters.Messenger,
	directory *Directory,
	resolver secrets.Resolver,
	logs *DeliveryLogStore,
	lgr logger.Logger,
) adapters.Messenger {
	if directory == nil {
		return messenger
	}
	return ResolvingMessenger{
		inner: messenger, directory: directory, secrets: resolver, logs: logs, logger: lgr,
	}
}

// addChannels adds unique channels to the registry.
func (r *AdapterRegistry) addChannels(channels ...string) {
	for _, ch := range channels {
		if !contains(r.EnabledChannels, ch) {
			r.EnabledChannels = append(r.EnabledChannels, ch)
		}
	}
}

// LogEnabledAdapters logs which adapters are configured and enabled.
func (r *AdapterRegistry) LogEnabledAdapters(lgr logger.Logger) {
	if len(r.EnabledAdapters) == 0 {
		lgr.Info("No adapters configured")
		return
	}

	lgr.Info(fmt.Sprintf("Enabled adapters (%d): %v", len(r.EnabledAdapters), r.EnabledAdapters))
	lgr.Info(fmt.Sprintf("Available channels: %v", r.EnabledChannels))
}

// GetAvailableChannels returns the list of channels that can be used.
func (r *AdapterRegistry) GetAvailableChannels() []string {
	// Always include in-app channel
	channels := []string{"in-app"}
	channels = append(channels, r.EnabledChannels...)
	return uniqueStrings(channels)
}

// ProvidersForChannel returns adapter names that can deliver the given channel.
func (r *AdapterRegistry) ProvidersForChannel(channel string) []string {
	if r == nil {
		return nil
	}
	base, _ := adapters.ParseChannel(channel)
	providers := make([]string, 0)
	for _, adapter := range r.Adapters {
		caps := adapter.Capabilities()
		for _, ch := range caps.Channels {
			if candidate, _ := adapters.ParseChannel(ch); candidate == base {
				providers = append(providers, adapter.Name())
				break
			}
		}
	}
	return uniqueStrings(providers)
}

func contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}

func uniqueStrings(slice []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
