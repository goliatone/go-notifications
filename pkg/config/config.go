package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Config captures module-level configuration knobs. Feature packages (dispatcher,
// inbox, templates, etc.) pull from these nested structs.
type Config struct {
	Localization LocalizationConfig `mapstructure:"localization" json:"localization"`
	Dispatcher   DispatcherConfig   `mapstructure:"dispatcher" json:"dispatcher"`
	Inbox        InboxConfig        `mapstructure:"inbox" json:"inbox"`
	Templates    TemplateConfig     `mapstructure:"templates" json:"templates"`
	Realtime     RealtimeConfig     `mapstructure:"realtime" json:"realtime"`
	Options      OptionsConfig      `mapstructure:"options" json:"options"`
}

// LocalizationConfig controls default locale + fallback chains.
type LocalizationConfig struct {
	DefaultLocale string `mapstructure:"default_locale" json:"default_locale,omitempty"`
}

// DispatcherConfig toggles the worker pool used for deliveries.
type DispatcherConfig struct {
	Enabled     bool `mapstructure:"enabled" json:"enabled,omitempty"`
	MaxAttempts int  `mapstructure:"max_attempts" json:"max_attempts,omitempty"`
	MaxWorkers  int  `mapstructure:"max_workers" json:"max_workers,omitempty"`
	// EnvFallbackAllowlist gates using global config/env credentials for specific subjects (e.g., admin/test users).
	EnvFallbackAllowlist []string `mapstructure:"env_fallback_allowlist" json:"env_fallback_allowlist,omitempty"`
}

// InboxConfig enables the in-app notification center.
type InboxConfig struct {
	Enabled bool `mapstructure:"enabled" json:"enabled,omitempty"`
}

// TemplateConfig scopes cache + rendering behaviors.
type TemplateConfig struct {
	CacheTTL time.Duration `mapstructure:"cache_ttl" json:"cache_ttl,omitempty"`
}

// RealtimeConfig controls optional broadcaster integration.
type RealtimeConfig struct {
	Enabled bool `mapstructure:"enabled" json:"enabled,omitempty"`
}

// OptionsConfig governs go-options specific behaviors.
type OptionsConfig struct {
	EnableScopeSchema bool `mapstructure:"enable_scope_schema" json:"enable_scope_schema,omitempty"`
}

// Defaults returns the baseline configuration.
func Defaults() Config {
	return Config{
		Localization: LocalizationConfig{DefaultLocale: "en"},
		Dispatcher: DispatcherConfig{
			Enabled:              true,
			MaxAttempts:          3,
			MaxWorkers:           4,
			EnvFallbackAllowlist: []string{},
		},
		Inbox: InboxConfig{
			Enabled: true,
		},
		Templates: TemplateConfig{
			CacheTTL: time.Minute,
		},
		Realtime: RealtimeConfig{
			Enabled: true,
		},
		Options: OptionsConfig{
			EnableScopeSchema: false,
		},
	}
}

// Validate ensures required fields are present and sane.
func (c *Config) Validate() error {
	if c.Localization.DefaultLocale == "" {
		return errors.New("localization.default_locale is required")
	}
	if c.Dispatcher.MaxAttempts <= 0 {
		return fmt.Errorf("dispatcher.max_attempts must be > 0")
	}
	if c.Dispatcher.MaxWorkers <= 0 {
		return fmt.Errorf("dispatcher.max_workers must be > 0")
	}
	if c.Templates.CacheTTL < 0 {
		return fmt.Errorf("templates.cache_ttl must be >= 0")
	}
	return nil
}

// Load decodes input onto initialized defaults. Omitted fields preserve defaults.
func Load(input any) (Config, error) {
	cfg := Defaults()
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return Config{}, fmt.Errorf("config: encode input: %w", err)
		}
		if string(payload) != "null" {
			decoder := json.NewDecoder(bytes.NewReader(payload))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&cfg); err != nil {
				return Config{}, fmt.Errorf("config: decode input: %w", err)
			}
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
