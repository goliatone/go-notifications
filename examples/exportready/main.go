package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/adapters/console"
	"github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/exportready"
	"github.com/goliatone/go-notifications/pkg/inbox"
	"github.com/goliatone/go-notifications/pkg/interfaces/broadcaster"
	"github.com/goliatone/go-notifications/pkg/interfaces/cache"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/notifier"
	"github.com/goliatone/go-notifications/pkg/templates"
)

func main() {
	ctx := context.Background()

	// Translators
	store := i18n.NewStaticStore(exportready.Translations())
	translator, err := i18n.NewSimpleTranslator(store, i18n.WithTranslatorDefaultLocale("en"))
	if err != nil {
		log.Fatal(err)
	}

	// Repositories
	defRepo := memory.NewDefinitionRepository()
	tplRepo := memory.NewTemplateRepository()
	evtRepo := memory.NewEventRepository()
	msgRepo := memory.NewMessageRepository()
	attRepo := memory.NewDeliveryRepository()
	inboxRepo := memory.NewInboxRepository()

	// Services
	tplSvc, err := templates.New(templates.Dependencies{
		Repository:    tplRepo,
		Cache:         &cache.Nop{},
		Logger:        &logger.Nop{},
		Translator:    translator,
		Fallbacks:     i18n.NewStaticFallbackResolver(),
		DefaultLocale: "en",
	})
	if err != nil {
		log.Fatal(err)
	}

	inboxSvc, err := inbox.New(inbox.Dependencies{
		Repository:  inboxRepo,
		Broadcaster: &broadcaster.Nop{},
		Logger:      &logger.Nop{},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Register assets (idempotent)
	regResult, err := exportready.Register(ctx, exportready.Dependencies{
		Definitions: defRepo,
		Templates:   tplSvc,
	}, exportready.Options{})
	if err != nil {
		log.Fatal(err)
	}

	// Adapters
	stdlog := &stdoutLogger{}
	registry := adapters.NewRegistry(console.New(stdlog))

	// Manager
	mgr, err := notifier.New(notifier.Dependencies{
		Definitions: defRepo,
		Events:      evtRepo,
		Messages:    msgRepo,
		Attempts:    attRepo,
		Templates:   tplSvc,
		Adapters:    registry,
		Logger:      stdlog,
		// Allow env fallback so the console adapter works without a secrets resolver.
		Config: config.DispatcherConfig{
			EnvFallbackAllowlist: []string{"user-1"},
		},
		Inbox:       inboxSvc,
	})
	if err != nil {
		log.Fatal(err)
	}

	exp, err := exportready.NewNotifier(mgr, regResult.DefinitionCode)
	if err != nil {
		log.Fatal(err)
	}

	// Send sample payload
	err = exp.Send(ctx, exportready.ExportReadyEvent{
		Recipients:  []string{"user-1"},
		Locale:      "en",
		FileName:    "orders.csv",
		Format:      "csv",
		URL:         "https://example.com/orders.csv",
		ExpiresAt:   time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
		Rows:        1200,
		Parts:       3,
		ManifestURL: "https://example.com/manifest.json",
		Message:     "Filtered by segment",
		ChannelOverrides: map[string]map[string]any{
			"email": {
				"cta_label":  "Download now",
				"action_url": "https://cdn.example.com/orders.csv",
			},
		},
		Channels: []string{"email", "in-app"},
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Println("export-ready notification sent (check console output)")
}

// stdoutLogger is a minimal logger that prints to stdout for the example.
type stdoutLogger struct{}

func (l *stdoutLogger) With(fields ...logger.Field) logger.Logger { return l }
func (l *stdoutLogger) Debug(msg string, fields ...logger.Field)  { l.log("debug", msg, fields...) }
func (l *stdoutLogger) Info(msg string, fields ...logger.Field)   { l.log("info", msg, fields...) }
func (l *stdoutLogger) Warn(msg string, fields ...logger.Field)   { l.log("warn", msg, fields...) }
func (l *stdoutLogger) Error(msg string, fields ...logger.Field)  { l.log("error", msg, fields...) }

func (l *stdoutLogger) log(level, msg string, fields ...logger.Field) {
	parts := []string{msg}
	for _, f := range fields {
		parts = append(parts, fmt.Sprintf("%s=%v", f.Key, f.Value))
	}
	log.Printf("[%s] %s", level, strings.Join(parts, " "))
}
