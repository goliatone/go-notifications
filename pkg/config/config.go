package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/goliatone/go-config/cfgx"
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
	DefaultLocale string `mapstructure:"default_locale" json:"default_locale"`
}

// DispatcherConfig toggles the worker pool used for deliveries.
type DispatcherConfig struct {
	Enabled    bool `mapstructure:"enabled" json:"enabled"`
	MaxRetries int  `mapstructure:"max_retries" json:"max_retries"`
	MaxWorkers int  `mapstructure:"max_workers" json:"max_workers"`
}

// InboxConfig enables the in-app notification center.
type InboxConfig struct {
	Enabled bool `mapstructure:"enabled" json:"enabled"`
}

// TemplateConfig scopes cache + rendering behaviors.
type TemplateConfig struct {
	CacheTTL time.Duration `mapstructure:"cache_ttl" json:"cache_ttl"`
}

// RealtimeConfig controls optional broadcaster integration.
type RealtimeConfig struct {
	Enabled bool `mapstructure:"enabled" json:"enabled"`
}

// OptionsConfig governs go-options specific behaviors.
type OptionsConfig struct {
	EnableScopeSchema bool `mapstructure:"enable_scope_schema" json:"enable_scope_schema"`
}

// Defaults returns the baseline configuration.
func Defaults() Config {
	return Config{
		Localization: LocalizationConfig{DefaultLocale: "en"},
		Dispatcher: DispatcherConfig{
			Enabled:    true,
			MaxRetries: 3,
			MaxWorkers: 4,
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
	if c.Dispatcher.MaxRetries < 0 {
		return fmt.Errorf("dispatcher.max_retries must be >= 0")
	}
	if c.Dispatcher.MaxWorkers <= 0 {
		return fmt.Errorf("dispatcher.max_workers must be > 0")
	}
	if c.Templates.CacheTTL < 0 {
		return fmt.Errorf("templates.cache_ttl must be >= 0")
	}
	return nil
}

// Load decodes arbitrary input (struct, map, cfg struct) using cfgx helpers.
// While cfgx.Build still returns zero values, we fallback to a lightweight
// decoder to keep smoke tests meaningful. Once cfgx is fully implemented we
// can drop the fallback.
func Load(input any, opts ...LoadOption) (Config, error) {
	settings := loadOptions{}
	for _, opt := range opts {
		opt(&settings)
	}

	cfg, err := cfgx.Build(input, settings.buildOpts...)
	if err != nil {
		return Config{}, err
	}

	if isZero(cfg) {
		if err := decodeFallback(input, &cfg); err != nil {
			return Config{}, err
		}
	}

	cfg = cfg.withDefaults()

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// LoadOption lets callers amend cfgx build options.
type LoadOption func(*loadOptions)

type loadOptions struct {
	buildOpts []cfgx.Option[Config]
}

// WithBuildOptions forwards cfgx options (duration hooks, preprocessors, etc.).
func WithBuildOptions(opts ...cfgx.Option[Config]) LoadOption {
	return func(lo *loadOptions) {
		lo.buildOpts = append(lo.buildOpts, opts...)
	}
}

func (c Config) withDefaults() Config {
	defaults := Defaults()

	if c.Localization.DefaultLocale == "" {
		c.Localization.DefaultLocale = defaults.Localization.DefaultLocale
	}
	if c.Dispatcher.MaxWorkers == 0 {
		c.Dispatcher.MaxWorkers = defaults.Dispatcher.MaxWorkers
	}
	if c.Dispatcher.MaxRetries == 0 {
		c.Dispatcher.MaxRetries = defaults.Dispatcher.MaxRetries
	}
	if c.Templates.CacheTTL == 0 {
		c.Templates.CacheTTL = defaults.Templates.CacheTTL
	}
	if !c.Inbox.Enabled {
		c.Inbox.Enabled = defaults.Inbox.Enabled
	}
	if !c.Realtime.Enabled {
		c.Realtime.Enabled = defaults.Realtime.Enabled
	}
	return c
}

func isZero(cfg Config) bool {
	return reflect.DeepEqual(cfg, Config{})
}

func decodeFallback(input any, cfg *Config) error {
	switch v := input.(type) {
	case nil:
		return nil
	case Config:
		*cfg = v
		return nil
	case *Config:
		if v != nil {
			*cfg = *v
		}
		return nil
	case map[string]any:
		return decodeMap(v, cfg)
	default:
		return fmt.Errorf("unsupported config input type: %T", input)
	}
}

func decodeMap(input map[string]any, cfg *Config) error {
	if input == nil {
		return nil
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, cfg)
}
