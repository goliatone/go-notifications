package config

import "testing"

func TestLoadFromMap(t *testing.T) {
	input := map[string]any{
		"localization": map[string]any{
			"default_locale": "es",
		},
		"dispatcher": map[string]any{
			"max_attempts": 5,
			"max_workers":  2,
		},
	}

	cfg, err := Load(input)
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}
	if cfg.Localization.DefaultLocale != "es" {
		t.Fatalf("expected locale es, got %s", cfg.Localization.DefaultLocale)
	}
	if cfg.Dispatcher.MaxAttempts != 5 {
		t.Fatalf("expected attempts 5, got %d", cfg.Dispatcher.MaxAttempts)
	}
	if cfg.Dispatcher.MaxWorkers != 2 {
		t.Fatalf("expected workers 2, got %d", cfg.Dispatcher.MaxWorkers)
	}
}

func TestLoadFromStruct(t *testing.T) {
	input := Config{
		Localization: LocalizationConfig{DefaultLocale: "fr"},
		Dispatcher:   DispatcherConfig{Enabled: true, MaxAttempts: 1, MaxWorkers: 10},
	}

	cfg, err := Load(input)
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}
	if cfg.Localization.DefaultLocale != "fr" {
		t.Fatalf("expected locale fr, got %s", cfg.Localization.DefaultLocale)
	}
	if cfg.Dispatcher.MaxAttempts != 1 {
		t.Fatalf("expected attempts 1, got %d", cfg.Dispatcher.MaxAttempts)
	}
	if cfg.Dispatcher.MaxWorkers != 10 {
		t.Fatalf("expected workers 10, got %d", cfg.Dispatcher.MaxWorkers)
	}
	if !cfg.Inbox.Enabled {
		t.Fatalf("expected inbox enabled by default")
	}
}

func TestLoadPreservesExplicitDisabledFlags(t *testing.T) {
	input := map[string]any{
		"inbox": map[string]any{
			"enabled": false,
		},
		"realtime": map[string]any{
			"enabled": false,
		},
	}

	cfg, err := Load(input)
	if err != nil {
		t.Fatalf("load returned error: %v", err)
	}
	if cfg.Inbox.Enabled {
		t.Fatalf("expected inbox disabled to be preserved")
	}
	if cfg.Realtime.Enabled {
		t.Fatalf("expected realtime disabled to be preserved")
	}
}
