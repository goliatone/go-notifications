package bunrepo

import (
	"context"
	"database/sql"
	"testing"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupSQLiteDB(t *testing.T) *bun.DB {
	t.Helper()

	sqldb, err := sql.Open(sqliteshim.DriverName(), "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())

	ctx := context.Background()
	models := []any{
		(*domain.NotificationDefinition)(nil),
	}
	for _, model := range models {
		_, err := db.NewCreateTable().Model(model).IfNotExists().Exec(ctx)
		if err != nil {
			t.Fatalf("create table: %v", err)
		}
	}
	return db
}

func TestDefinitionRepositoryBun(t *testing.T) {
	db := setupSQLiteDB(t)
	repo := NewDefinitionRepository(db)
	ctx := context.Background()

	def := &domain.NotificationDefinition{
		Code:     "billing.alert",
		Name:     "Billing Alert",
		Channels: domain.StringList{"email", "sms"},
	}
	if err := repo.Create(ctx, def); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByCode(ctx, "billing.alert")
	if err != nil {
		t.Fatalf("get by code: %v", err)
	}
	if got.Code != "billing.alert" {
		t.Fatalf("unexpected code %s", got.Code)
	}

	list, err := repo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list.Total != 1 {
		t.Fatalf("expected total 1, got %d", list.Total)
	}
}
