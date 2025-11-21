package main

import (
	"context"
	"database/sql"

	i18n "github.com/goliatone/go-i18n"
	"github.com/goliatone/go-notifications/examples/web/config"
	"github.com/goliatone/go-notifications/internal/commands"
	"github.com/goliatone/go-notifications/pkg/adapters"
	"github.com/goliatone/go-notifications/pkg/adapters/console"
	notifierconfig "github.com/goliatone/go-notifications/pkg/config"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/goliatone/go-notifications/pkg/notifier"
	"github.com/goliatone/go-notifications/pkg/storage"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

type DemoUser struct {
	ID       string
	Name     string
	Email    string
	Locale   string
	TenantID string
	IsAdmin  bool
}

type App struct {
	Config     config.Config
	Module     *notifier.Module
	Catalog    *commands.Catalog
	DB         *bun.DB
	Logger     logger.Logger
	WSHub      *WebSocketHub
	Users      map[string]*DemoUser
	Sessions   map[string]*DemoUser
	Translator i18n.Translator
}

func NewApp(ctx context.Context, cfg config.Config) (*App, error) {
	lgr := newStdLogger()

	sqldb, err := sql.Open(sqliteshim.ShimName, cfg.Persistence.DSN)
	if err != nil {
		return nil, err
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())

	providers := storage.NewMemoryProviders()

	translator := &NoopTranslator{}

	consoleAdapter := console.New(lgr)

	var wsHub *WebSocketHub
	if cfg.Features.EnableWebSocket {
		wsHub = NewWebSocketHub(lgr)
		go wsHub.Run()
	}

	module, err := notifier.NewModule(notifier.ModuleOptions{
		Config:      notifierConfig(),
		Storage:     providers,
		Logger:      lgr,
		Translator:  translator,
		Broadcaster: wsHub,
		Adapters:    []adapters.Messenger{consoleAdapter},
	})
	if err != nil {
		return nil, err
	}

	app := &App{
		Config:     cfg,
		Module:     module,
		Catalog:    module.Commands().Catalog,
		DB:         db,
		Logger:     lgr,
		WSHub:      wsHub,
		Users:      make(map[string]*DemoUser),
		Sessions:   make(map[string]*DemoUser),
		Translator: translator,
	}

	app.initDemoUsers()

	if err := SeedData(ctx, app); err != nil {
		return nil, err
	}

	return app, nil
}

func (a *App) initDemoUsers() {
	users := []DemoUser{
		{
			ID:       uuid.New().String(),
			Name:     "Alice",
			Email:    "alice@example.com",
			Locale:   "en",
			TenantID: "tenant-1",
			IsAdmin:  true,
		},
		{
			ID:       uuid.New().String(),
			Name:     "Bob",
			Email:    "bob@example.com",
			Locale:   "en",
			TenantID: "tenant-1",
			IsAdmin:  false,
		},
		{
			ID:       uuid.New().String(),
			Name:     "Carlos",
			Email:    "carlos@example.com",
			Locale:   "es",
			TenantID: "tenant-1",
			IsAdmin:  false,
		},
	}

	for i := range users {
		a.Users[users[i].Email] = &users[i]
	}
}

func (a *App) GetUserByEmail(email string) *DemoUser {
	return a.Users[email]
}

func (a *App) CreateSession(user *DemoUser) string {
	sessionID := uuid.New().String()
	a.Sessions[sessionID] = user
	return sessionID
}

func (a *App) GetUserBySession(sessionID string) *DemoUser {
	return a.Sessions[sessionID]
}

func (a *App) DeleteSession(sessionID string) {
	delete(a.Sessions, sessionID)
}

func (a *App) Close() error {
	if a.WSHub != nil {
		a.WSHub.Close()
	}
	if a.DB != nil {
		return a.DB.Close()
	}
	return nil
}

func notifierConfig() notifierconfig.Config {
	return notifierconfig.Config{
		Localization: notifierconfig.LocalizationConfig{
			DefaultLocale: "en",
		},
		Dispatcher: notifierconfig.DispatcherConfig{
			MaxRetries: 3,
			MaxWorkers: 4,
		},
	}
}

type NoopTranslator struct{}

func (n *NoopTranslator) Translate(locale, key string, args ...any) (string, error) {
	return key, nil
}
