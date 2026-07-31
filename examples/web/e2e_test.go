package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/goliatone/go-notifications/examples/web/config"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/events"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/notifier"
	"github.com/goliatone/go-notifications/pkg/privacy"
	"github.com/goliatone/go-notifications/pkg/secrets"
	"github.com/goliatone/go-notifications/pkg/storage"
)

func TestPhase7EndToEndDemo(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := config.Defaults()
	cfg.Features.EnableWebSocket = false
	tmpDB := filepath.Join(os.TempDir(), "phase7-e2e-demo.db")
	if err := os.Remove(tmpDB); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove stale test database: %v", err)
	}
	cfg.Persistence.DSN = "file:phase7-e2e?mode=memory&cache=shared&_busy_timeout=5000&_fk=1"

	lgr := &captureLogger{}
	app, fakes := buildTestApp(ctx, t, cfg, lgr)
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Errorf("close app: %v", err)
		}
		if err := os.Remove(tmpDB); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove test database: %v", err)
		}
	})

	bob := app.Users["bob@example.com"]
	carlos := app.Users["carlos@example.com"]
	if bob == nil || carlos == nil {
		t.Fatalf("seeded demo users missing")
	}

	assertPreflightSecret(t, app, bob, "slack")
	assertPreflightSecret(t, app, carlos, "telegram")
	fireTestEvent(ctx, t, app, bob, lgr)
	fireTestEvent(ctx, t, app, carlos, lgr)
	assertProviderMessages(t, fakes, lgr)
	assertDeliveryProvider(ctx, t, app, bob, "slack")
	assertDeliveryProvider(ctx, t, app, carlos, "telegram")
	assertMaskedLogs(t, lgr.entries)
}

func assertPreflightSecret(t *testing.T, app *App, user *DemoUser, provider string) {
	t.Helper()
	ref := secrets.Reference{Scope: secrets.ScopeUser, SubjectID: user.ID, Channel: "chat", Provider: provider, Key: "default"}
	resolved, err := app.Directory.ResolveSecrets(ref)
	if err != nil {
		t.Fatalf("preflight secret for %s/%s: %v", user.Name, provider, err)
	}
	if value, ok := resolved[ref]; !ok || len(value.Data) == 0 {
		t.Fatalf("preflight secret for %s/%s missing", user.Name, provider)
	}
	tokenRef := secrets.Reference{Scope: secrets.ScopeUser, SubjectID: user.ID, Channel: "chat", Provider: provider, Key: "token"}
	if _, err := app.Directory.provider.Get(tokenRef); err != nil {
		t.Fatalf("preflight token missing for %s/%s: %v", user.Name, provider, err)
	}
}

func fireTestEvent(ctx context.Context, t *testing.T, app *App, user *DemoUser, lgr *captureLogger) {
	t.Helper()
	err := app.Catalog.EnqueueEvent.Execute(ctx, events.IntakeRequest{
		DefinitionCode: "test_notification", Recipients: []string{user.ID},
		Context: map[string]any{"name": user.Name, "message": "This is a test notification"},
	})
	if err != nil {
		t.Fatalf("enqueue event for %s: %v\nlogs: %v", user.Name, err, lgr.entries)
	}
}

func assertProviderMessages(t *testing.T, fakes map[string]*capturingMessenger, lgr *captureLogger) {
	t.Helper()
	assertProviderMessage(t, fakes["slack"], "slack", "xoxb-bob", lgr)
	assertProviderMessage(t, fakes["telegram"], "telegram", "telegram-carlos", lgr)
}

func assertProviderMessage(
	t *testing.T,
	messenger *capturingMessenger,
	provider, tokenFragment string,
	lgr *captureLogger,
) {
	t.Helper()
	if len(messenger.sent) != 1 {
		t.Fatalf("expected %s to send once, got %d", provider, len(messenger.sent))
	}
	message := messenger.sent[0]
	if message.Provider != provider || message.Channel != "chat" {
		t.Fatalf("unexpected %s message provider/channel: %+v", provider, message)
	}
	if got := message.Metadata["token"]; !strings.Contains(fmt.Sprint(got), tokenFragment) {
		t.Fatalf("expected %s token injected, got %v (meta=%v logs=%v)", provider, got, message.Metadata, lgr.entries)
	}
}

func assertDeliveryProvider(ctx context.Context, t *testing.T, app *App, user *DemoUser, provider string) {
	t.Helper()
	logs, err := app.DeliveryLogs.LastForUser(ctx, user.ID, 5)
	if err != nil {
		t.Fatalf("delivery logs %s: %v", user.Name, err)
	}
	if len(logs) == 0 || logs[0].Provider != provider {
		t.Fatalf("expected %s delivery log for %s, got %+v", provider, user.Name, logs)
	}
}

func assertMaskedLogs(t *testing.T, entries []string) {
	t.Helper()
	for _, entry := range entries {
		if strings.Contains(entry, "xoxb-") || strings.Contains(entry, "telegram-") {
			t.Fatalf("found unmasked secret in logs: %s", entry)
		}
	}
	if !slices.ContainsFunc(entries, func(entry string) bool { return strings.Contains(entry, "***") }) {
		t.Fatal("expected masked secrets to be logged")
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
		Diagnostic:  diagnosticLogger{t: t},
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

type diagnosticLogger struct{ t *testing.T }

func (d diagnosticLogger) Report(_ context.Context, event privacy.DiagnosticEvent) {
	d.t.Logf("diagnostic %s: %v", event.Operation, event.Cause)
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
	userID, ok := msg.Metadata["recipient_id"].(string)
	if !ok {
		m.logger.Warn("recipient identifier is not a string")
		return
	}
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
	m.logger.Info("masked secrets", "secrets", masked)
}

type captureLogger struct {
	entries []string
}

func (l *captureLogger) Trace(msg string, args ...any) { l.record("TRACE", msg, args) }
func (l *captureLogger) Debug(msg string, args ...any) { l.record("DEBUG", msg, args) }
func (l *captureLogger) Info(msg string, args ...any)  { l.record("INFO", msg, args) }
func (l *captureLogger) Warn(msg string, args ...any)  { l.record("WARN", msg, args) }
func (l *captureLogger) Error(msg string, args ...any) { l.record("ERROR", msg, args) }
func (l *captureLogger) Fatal(msg string, args ...any) { l.record("FATAL", msg, args) }
func (l *captureLogger) WithContext(ctx context.Context) logger.Logger {
	return l
}

func (l *captureLogger) record(level, msg string, args []any) {
	var parts []string
	for i := 0; i < len(args); {
		if key, ok := args[i].(string); ok && i+1 < len(args) {
			parts = append(parts, fmt.Sprintf("%s=%v", key, args[i+1]))
			i += 2
			continue
		}
		parts = append(parts, fmt.Sprint(args[i]))
		i++
	}
	entry := fmt.Sprintf("%s %s %s", level, msg, strings.Join(parts, " "))
	l.entries = append(l.entries, strings.TrimSpace(entry))
}
