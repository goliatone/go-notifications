package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goliatone/go-notifications/examples/web/config"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// secretRecord mirrors the Bun store schema used by the encrypted secrets provider.
type secretRecord struct {
	bun.BaseModel `bun:"table:secrets"`

	ID        int64          `bun:",pk,autoincrement"`
	Scope     string         `bun:",notnull,unique:secret_identity"`
	SubjectID string         `bun:",notnull,unique:secret_identity"`
	Channel   string         `bun:",notnull,unique:secret_identity"`
	Provider  string         `bun:",notnull,unique:secret_identity"`
	Key       string         `bun:",notnull,unique:secret_identity"`
	Version   string         `bun:",notnull,unique:secret_identity"`
	Cipher    []byte         `bun:",notnull"`
	Nonce     []byte         `bun:",notnull"`
	Metadata  map[string]any `bun:",type:jsonb"`
	CreatedAt time.Time      `bun:",nullzero,notnull,default:current_timestamp"`
	UpdatedAt time.Time      `bun:",nullzero,notnull,default:current_timestamp"`
	DeletedAt bun.NullTime   `bun:",soft_delete,nullzero"`
}

// demoContact stores per-user contact details for the web example.
type demoContact struct {
	bun.BaseModel `bun:"table:demo_contacts"`

	ID              int64     `bun:",pk,autoincrement"`
	UserID          string    `bun:",unique,notnull"`
	DisplayName     string    `bun:",notnull"`
	Email           string    `bun:",notnull"`
	Phone           string    `bun:",notnull"`
	SlackID         string    `bun:",notnull"`
	TelegramChatID  string    `bun:",notnull"`
	PreferredLocale string    `bun:",notnull"`
	TenantID        string    `bun:",notnull"`
	CreatedAt       time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	UpdatedAt       time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

// deliveryLogRecord records the last delivery attempt for UI display.
type deliveryLogRecord struct {
	bun.BaseModel `bun:"table:demo_delivery_logs"`

	ID             int64          `bun:",pk,autoincrement"`
	UserID         string         `bun:",notnull"`
	Channel        string         `bun:",notnull"`
	Provider       string         `bun:",notnull"`
	Address        string         `bun:",notnull"`
	DefinitionCode string         `bun:",notnull"`
	Status         string         `bun:",notnull"`
	Message        string         `bun:",notnull"`
	Metadata       domain.JSONMap `bun:",type:jsonb"`
	CreatedAt      time.Time      `bun:",nullzero,notnull,default:current_timestamp"`
}

func openDatabase(ctx context.Context, cfg config.PersistenceConfig, lgr logger.Logger) (*bun.DB, error) {
	if cfg.Driver != "" && cfg.Driver != "sqlite" {
		return nil, fmt.Errorf("persistence: unsupported driver %s", cfg.Driver)
	}
	dsn := strings.TrimSpace(cfg.DSN)
	if dsn == "" {
		dsn = config.Defaults().Persistence.DSN
	}
	if err := ensureSQLiteDir(dsn); err != nil {
		return nil, err
	}

	sqldb, err := sql.Open(sqliteshim.DriverName(), dsn)
	if err != nil {
		return nil, fmt.Errorf("persistence: open sqlite: %w", err)
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())

	if _, err := sqldb.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil && lgr != nil {
		lgr.Warn("persistence: enable sqlite foreign keys", logger.Field{Key: "error", Value: err})
	}

	if err := ensureSchema(ctx, db); err != nil {
		return nil, err
	}
	return db, nil
}

func ensureSQLiteDir(dsn string) error {
	if !strings.HasPrefix(dsn, "file:") {
		return nil
	}
	path := strings.TrimPrefix(dsn, "file:")
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if path == "" || path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func ensureSchema(ctx context.Context, db *bun.DB) error {
	models := []any{
		(*domain.NotificationDefinition)(nil),
		(*domain.NotificationTemplate)(nil),
		(*domain.NotificationEvent)(nil),
		(*domain.NotificationMessage)(nil),
		(*domain.DeliveryAttempt)(nil),
		(*domain.NotificationPreference)(nil),
		(*domain.SubscriptionGroup)(nil),
		(*domain.InboxItem)(nil),
		(*secretRecord)(nil),
		(*demoContact)(nil),
		(*deliveryLogRecord)(nil),
	}
	for _, model := range models {
		if _, err := db.NewCreateTable().Model(model).IfNotExists().Exec(ctx); err != nil {
			return fmt.Errorf("persistence: create table for %T: %w", model, err)
		}
	}
	return nil
}
