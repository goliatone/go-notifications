package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goliatone/go-notifications/examples/web/config"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/notifier"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/goliatone/go-notifications/pkg/storage"
)

func TestPhase7EndToEndDemo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := config.Defaults()
	cfg.Features.EnableWebSocket = false
	tmpDB := filepath.Join(os.TempDir(), "phase7-e2e-demo.db")
	_ = os.Remove(tmpDB)
	cfg.Persistence.DSN = "file:phase7-e2e?mode=memory&cache=shared&_busy_timeout=5000&_fk=1"

	lgr := &captureLogger{}
	app, fakes := buildTestApp(ctx, t, cfg, lgr)
	t.Cleanup(func() {
		_ = app.Close()
		_ = os.Remove(tmpDB)
	})

	bob := app.Users["bob@example.com"]
	carlos := app.Users["carlos@example.com"]
	if bob == nil || carlos == nil {
		t.Fatalf("seeded demo users missing")
	}

	preflightSecret := func(user *DemoUser, provider string) {
		ref := secrets.Reference{Scope: secrets.ScopeUser, SubjectID: user.ID, Channel: "chat", Provider: provider, Key: "default"}
		resolved, err := app.Directory.ResolveSecrets(ref)
		if err != nil {
			t.Fatalf("preflight secret for %s/%s: %v", user.Name, provider, err)
		}
		if val, ok := resolved[ref]; !ok || len(val.Data) == 0 {
			t.Fatalf("preflight secret for %s/%s missing", user.Name, provider)
		}
		tokenRef := secrets.Reference{Scope: secrets.ScopeUser, SubjectID: user.ID, Channel: "chat", Provider: provider, Key: "token"}
		if _, err := app.Directory.provider.Get(tokenRef); err != nil {
			t.Fatalf("preflight token missing for %s/%s: %v", user.Name, provider, err)
		}
	}

	preflightSecret(bob, "slack")
	preflightSecret(carlos, "telegram")

	fireEvent := func(user *DemoUser) {
		err := app.Catalog.EnqueueEvent.Execute(ctx, events.IntakeRequest{
			DefinitionCode: "test_notification",
			Recipients:     []string{user.ID},
			Context: map[string]any{
				"name":    user.Name,
				"message": "This is a test notification",
			},
		})
		if err != nil {
			t.Fatalf("enqueue event for %s: %v\nlogs: %v", user.Name, err, lgr.entries)
		}
	}

	fireEvent(bob)
	fireEvent(carlos)

	// Provider selection via preferences should route Bob->slack and Carlos->telegram.
	if len(fakes["slack"].sent) != 1 {
		t.Fatalf("expected slack to send once, got %d", len(fakes["slack"].sent))
	}
	if len(fakes["telegram"].sent) != 1 {
		t.Fatalf("expected telegram to send once, got %d", len(fakes["telegram"].sent))
	}

	slackMsg := fakes["slack"].sent[0]
	if slackMsg.Provider != "slack" || slackMsg.Channel != "chat" {
		t.Fatalf("unexpected slack message provider/channel: %+v", slackMsg)
	}
	if got := slackMsg.Metadata["token"]; !strings.Contains(fmt.Sprint(got), "xoxb-bob") {
		t.Fatalf("expected bob token injected, got %v (meta=%v logs=%v)", got, slackMsg.Metadata, lgr.entries)
	}

	telegramMsg := fakes["telegram"].sent[0]
	if telegramMsg.Provider != "telegram" || telegramMsg.Channel != "chat" {
		t.Fatalf("unexpected telegram message provider/channel: %+v", telegramMsg)
	}
	if got := telegramMsg.Metadata["token"]; !strings.Contains(fmt.Sprint(got), "telegram-carlos") {
		t.Fatalf("expected carlos telegram token injected, got %v", got)
	}

	// Delivery logs should reflect provider choice for each user.
	logs, err := app.DeliveryLogs.LastForUser(ctx, bob.ID, 5)
	if err != nil {
		t.Fatalf("delivery logs bob: %v", err)
	}
	if len(logs) == 0 || logs[0].Provider != "slack" {
		t.Fatalf("expected slack delivery log for bob, got %+v", logs)
	}

	logs, err = app.DeliveryLogs.LastForUser(ctx, carlos.ID, 5)
	if err != nil {
		t.Fatalf("delivery logs carlos: %v", err)
	}
	if len(logs) == 0 || logs[0].Provider != "telegram" {
		t.Fatalf("expected telegram delivery log for carlos, got %+v", logs)
	}

	// Ensure masked logging removed raw secrets.
	for _, entry := range lgr.entries {
		if strings.Contains(entry, "xoxb-") || strings.Contains(entry, "telegram-") {
			t.Fatalf("found unmasked secret in logs: %s", entry)
		}
	}
	hasMasked := false
	for _, entry := range lgr.entries {
		if strings.Contains(entry, "***") {
			hasMasked = true
			break
		}
	}
	if !hasMasked {
		t.Fatalf("expected masked secrets to be logged")
	}
}

func buildTestApp(ctx context.Context, t *testing.T, cfg config.Config, lgr logger.Logger) (*App, map[string]*capturingMessenger) {
	t.Helper()

	db, err := openDatabase(ctx, cfg.Persistence, lgr)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	providers := storage.NewBunProviders(db)

	secretProvider, secretResolver, err := buildSecretsProvider(db, lgr)
	if err != nil {
		t.Fatalf("secrets: %v", err)
	}

	directory := NewDirectory(db, secretProvider, secretResolver, lgr)
	deliveryLogs := NewDeliveryLogStore(db, lgr)

	fakes := map[string]*capturingMessenger{
		"slack":    newCapturingMessenger("slack", []string{"chat", "chat:slack", "slack"}, lgr),
		"telegram": newCapturingMessenger("telegram", []string{"chat", "chat:telegram", "telegram"}, lgr),
	}
	adaptersList := []adapters.Messenger{
		ResolvingMessenger{inner: fakes["slack"], directory: directory, secrets: secretResolver, logs: deliveryLogs, logger: lgr},
		ResolvingMessenger{inner: fakes["telegram"], directory: directory, secrets: secretResolver, logs: deliveryLogs, logger: lgr},
	}
	registry := &AdapterRegistry{
		Adapters:        adaptersList,
		EnabledAdapters: []string{"slack", "telegram"},
		EnabledChannels: []string{"chat", "slack", "telegram", "chat:slack", "chat:telegram"},
	}

	module, err := notifier.NewModule(notifier.ModuleOptions{
		Config:      notifierConfig(),
		Storage:     providers,
		Logger:      lgr,
		Translator:  &NoopTranslator{},
		Broadcaster: nil,
		Adapters:    adaptersList,
		Secrets:     secretResolver,
	})
	if err != nil {
		t.Fatalf("module: %v", err)
	}

	app := &App{
		Config:          cfg,
		Module:          module,
		Catalog:         module.Commands().Catalog,
		DB:              db,
		Logger:          lgr,
		Directory:       directory,
		DeliveryLogs:    deliveryLogs,
		Users:           make(map[string]*DemoUser),
		Sessions:        make(map[string]*DemoUser),
		Translator:      &NoopTranslator{},
		AdapterRegistry: registry,
	}

	app.initDemoUsers()
	if err := SeedData(ctx, app); err != nil {
		t.Fatalf("seed data: %v", err)
	}

	return app, fakes
}

type capturingMessenger struct {
	name     string
	channels []string
	sent     []adapters.Message
	logger   logger.Logger
}

func newCapturingMessenger(name string, channels []string, lgr logger.Logger) *capturingMessenger {
	return &capturingMessenger{name: name, channels: channels, logger: lgr}
}

func (m *capturingMessenger) Name() string { return m.name }

func (m *capturingMessenger) Capabilities() adapters.Capability {
	return adapters.Capability{
		Name:     m.name,
		Channels: m.channels,
		Formats:  []string{"text/plain"},
	}
}

func (m *capturingMessenger) Send(_ context.Context, msg adapters.Message) error {
	m.sent = append(m.sent, msg)
	m.logMaskedSecrets(msg)
	return nil
}

func (m *capturingMessenger) logMaskedSecrets(msg adapters.Message) {
	if m.logger == nil {
		return
	}
	raw, ok := msg.Metadata["secrets"]
	if !ok {
		return
	}
	userID, _ := msg.Metadata["recipient_id"].(string)
	values := make(map[secrets.Reference]secrets.SecretValue)

	switch secretsMap := raw.(type) {
	case map[string][]byte:
		for key, data := range secretsMap {
			values[secrets.Reference{Scope: secrets.ScopeUser, SubjectID: userID, Channel: msg.Channel, Provider: msg.Provider, Key: key}] = secrets.SecretValue{Data: data}
		}
	case map[string]any:
		for key, val := range secretsMap {
			switch data := val.(type) {
			case []byte:
				values[secrets.Reference{Scope: secrets.ScopeUser, SubjectID: userID, Channel: msg.Channel, Provider: msg.Provider, Key: key}] = secrets.SecretValue{Data: data}
			case string:
				values[secrets.Reference{Scope: secrets.ScopeUser, SubjectID: userID, Channel: msg.Channel, Provider: msg.Provider, Key: key}] = secrets.SecretValue{Data: []byte(data)}
			}
		}
	}

	masked := secrets.MaskValues(values)
	m.logger.Info("masked secrets", logger.Field{Key: "secrets", Value: masked})
}

type captureLogger struct {
	entries []string
}

func (l *captureLogger) With(fields ...logger.Field) logger.Logger { return l }
func (l *captureLogger) Debug(msg string, fields ...logger.Field)  { l.record("DEBUG", msg, fields) }
func (l *captureLogger) Info(msg string, fields ...logger.Field)   { l.record("INFO", msg, fields) }
func (l *captureLogger) Warn(msg string, fields ...logger.Field)   { l.record("WARN", msg, fields) }
func (l *captureLogger) Error(msg string, fields ...logger.Field)  { l.record("ERROR", msg, fields) }

func (l *captureLogger) record(level, msg string, fields []logger.Field) {
	var parts []string
	for _, f := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", f.Key, f.Value))
	}
	entry := fmt.Sprintf("%s %s %s", level, msg, strings.Join(parts, " "))
	l.entries = append(l.entries, strings.TrimSpace(entry))
}
