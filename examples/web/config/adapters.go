package config

import (
	"os"
	"time"
)

// AdapterConfig holds configuration for all optional adapters.
type AdapterConfig struct {
	Slack    SlackConfig
	Telegram TelegramConfig
	Twilio   TwilioConfig
	SendGrid SendGridConfig
	Mailgun  MailgunConfig
	WhatsApp WhatsAppConfig
}

// SlackConfig holds Slack adapter configuration.
type SlackConfig struct {
	Token   string
	Channel string
}

// IsConfigured returns true if required fields are set.
func (c SlackConfig) IsConfigured() bool {
	return c.Token != "" && c.Channel != ""
}

// TelegramConfig holds Telegram adapter configuration.
type TelegramConfig struct {
	BotToken string
	ChatID   string
}

// IsConfigured returns true if required fields are set.
func (c TelegramConfig) IsConfigured() bool {
	return c.BotToken != "" && c.ChatID != ""
}

// TwilioConfig holds Twilio adapter configuration.
type TwilioConfig struct {
	AccountSID string
	AuthToken  string
	FromPhone  string
}

// IsConfigured returns true if required fields are set.
func (c TwilioConfig) IsConfigured() bool {
	return c.AccountSID != "" && c.AuthToken != "" && c.FromPhone != ""
}

// SendGridConfig holds SendGrid adapter configuration.
type SendGridConfig struct {
	APIKey    string
	FromEmail string
	FromName  string
}

// IsConfigured returns true if required fields are set.
func (c SendGridConfig) IsConfigured() bool {
	return c.APIKey != "" && c.FromEmail != ""
}

// MailgunConfig holds Mailgun adapter configuration.
type MailgunConfig struct {
	APIKey    string
	Domain    string
	FromEmail string
	FromName  string
}

// IsConfigured returns true if required fields are set.
func (c MailgunConfig) IsConfigured() bool {
	return c.APIKey != "" && c.Domain != "" && c.FromEmail != ""
}

// WhatsAppConfig holds WhatsApp adapter configuration.
type WhatsAppConfig struct {
	AccountSID string
	AuthToken  string
	FromPhone  string
}

// IsConfigured returns true if required fields are set.
func (c WhatsAppConfig) IsConfigured() bool {
	return c.AccountSID != "" && c.AuthToken != "" && c.FromPhone != ""
}

// LoadAdapterConfig loads adapter configuration from environment variables.
func LoadAdapterConfig() AdapterConfig {
	return AdapterConfig{
		Slack: SlackConfig{
			Token:   os.Getenv("SLACK_TOKEN"),
			Channel: getEnvOrDefault("SLACK_CHANNEL", "#notifications"),
		},
		Telegram: TelegramConfig{
			BotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
			ChatID:   os.Getenv("TELEGRAM_CHAT_ID"),
		},
		Twilio: TwilioConfig{
			AccountSID: os.Getenv("TWILIO_ACCOUNT_SID"),
			AuthToken:  os.Getenv("TWILIO_AUTH_TOKEN"),
			FromPhone:  os.Getenv("TWILIO_FROM_PHONE"),
		},
		SendGrid: SendGridConfig{
			APIKey:    os.Getenv("SENDGRID_API_KEY"),
			FromEmail: os.Getenv("SENDGRID_FROM_EMAIL"),
			FromName:  getEnvOrDefault("SENDGRID_FROM_NAME", "Notification System"),
		},
		Mailgun: MailgunConfig{
			APIKey:    os.Getenv("MAILGUN_API_KEY"),
			Domain:    os.Getenv("MAILGUN_DOMAIN"),
			FromEmail: os.Getenv("MAILGUN_FROM_EMAIL"),
			FromName:  getEnvOrDefault("MAILGUN_FROM_NAME", "Notification System"),
		},
		WhatsApp: WhatsAppConfig{
			AccountSID: os.Getenv("WHATSAPP_ACCOUNT_SID"),
			AuthToken:  os.Getenv("WHATSAPP_AUTH_TOKEN"),
			FromPhone:  os.Getenv("WHATSAPP_FROM_PHONE"),
		},
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// Common timeout for all HTTP-based adapters.
const DefaultAdapterTimeout = 30 * time.Second
