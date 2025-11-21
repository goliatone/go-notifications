package main

import (
	"fmt"

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

	wrap := func(m adapters.Messenger) adapters.Messenger {
		if dir == nil {
			return m
		}
		return ResolvingMessenger{
			inner:     m,
			directory: dir,
			secrets:   resolver,
			logs:      logs,
			logger:    lgr,
		}
	}

	// Console adapter is always enabled
	consoleAdapter := wrap(console.New(lgr))
	registry.Adapters = append(registry.Adapters, consoleAdapter)
	registry.EnabledAdapters = append(registry.EnabledAdapters, "console")
	registry.addChannels("console", "email", "email:console")

	// Slack
	if cfg.Slack.IsConfigured() {
		slackAdapter := wrap(slack.New(lgr, slack.WithConfig(slack.Config{
			Token:   cfg.Slack.Token,
			Channel: cfg.Slack.Channel,
			BaseURL: "https://slack.com/api",
			Timeout: config.DefaultAdapterTimeout,
		})))
		registry.Adapters = append(registry.Adapters, slackAdapter)
		registry.EnabledAdapters = append(registry.EnabledAdapters, "slack")
		registry.addChannels("chat", "slack", "chat:slack")
	}

	// Telegram
	if cfg.Telegram.IsConfigured() {
		telegramAdapter := wrap(telegram.New(lgr, telegram.WithConfig(telegram.Config{
			Token:   cfg.Telegram.BotToken,
			ChatID:  cfg.Telegram.ChatID,
			BaseURL: "https://api.telegram.org",
			Timeout: config.DefaultAdapterTimeout,
		})))
		registry.Adapters = append(registry.Adapters, telegramAdapter)
		registry.EnabledAdapters = append(registry.EnabledAdapters, "telegram")
		registry.addChannels("chat", "chat:telegram", "telegram")
	}

	// Twilio (SMS)
	if cfg.Twilio.IsConfigured() {
		twilioAdapter := wrap(twilio.New(lgr, twilio.WithConfig(twilio.Config{
			AccountSID: cfg.Twilio.AccountSID,
			AuthToken:  cfg.Twilio.AuthToken,
			From:       cfg.Twilio.FromPhone,
			Timeout:    config.DefaultAdapterTimeout,
		})))
		registry.Adapters = append(registry.Adapters, twilioAdapter)
		registry.EnabledAdapters = append(registry.EnabledAdapters, "twilio")
		registry.addChannels("sms", "sms:twilio")
	}

	// SendGrid (Email)
	if cfg.SendGrid.IsConfigured() {
		fromEmail := cfg.SendGrid.FromEmail
		if cfg.SendGrid.FromName != "" {
			fromEmail = cfg.SendGrid.FromName + " <" + cfg.SendGrid.FromEmail + ">"
		}
		sendgridAdapter := wrap(sendgrid.New(lgr,
			sendgrid.WithAPIKey(cfg.SendGrid.APIKey),
			sendgrid.WithFrom(fromEmail),
			sendgrid.WithTimeout(30),
		))
		registry.Adapters = append(registry.Adapters, sendgridAdapter)
		registry.EnabledAdapters = append(registry.EnabledAdapters, "sendgrid")
		registry.addChannels("email", "email:sendgrid")
	}

	// Mailgun (Email)
	if cfg.Mailgun.IsConfigured() {
		fromEmail := cfg.Mailgun.FromEmail
		if cfg.Mailgun.FromName != "" {
			fromEmail = cfg.Mailgun.FromName + " <" + cfg.Mailgun.FromEmail + ">"
		}
		mailgunAdapter := wrap(mailgun.New(lgr, mailgun.WithConfig(mailgun.Config{
			APIKey:     cfg.Mailgun.APIKey,
			Domain:     cfg.Mailgun.Domain,
			From:       fromEmail,
			TimeoutSec: 30,
		})))
		registry.Adapters = append(registry.Adapters, mailgunAdapter)
		registry.EnabledAdapters = append(registry.EnabledAdapters, "mailgun")
		registry.addChannels("email", "email:mailgun")
	}

	// WhatsApp
	if cfg.WhatsApp.IsConfigured() {
		whatsappAdapter := wrap(whatsapp.New(lgr, whatsapp.WithConfig(whatsapp.Config{
			Token:         cfg.WhatsApp.AuthToken,
			PhoneNumberID: cfg.WhatsApp.FromPhone,
			Timeout:       config.DefaultAdapterTimeout,
		})))
		registry.Adapters = append(registry.Adapters, whatsappAdapter)
		registry.EnabledAdapters = append(registry.EnabledAdapters, "whatsapp")
		registry.addChannels("whatsapp", "whatsapp:whatsapp")
	}

	return registry
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
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
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
