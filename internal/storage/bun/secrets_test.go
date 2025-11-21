package bunrepo

import (
	"context"
	"database/sql"
	"testing"
	"time"

	iface "github.com/goliatone/go-notifications/pkg/interfaces/secrets"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

func setupSecretDB(t *testing.T) *bun.DB {
	t.Helper()
	sqldb, err := sql.Open(sqliteshim.DriverName(), "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("sql open: %v", err)
	}
	db := bun.NewDB(sqldb, sqlitedialect.New())
	ctx := context.Background()
	_, err = db.NewCreateTable().Model((*secretRecord)(nil)).IfNotExists().Exec(ctx)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func TestSecretStoreRoundTrip(t *testing.T) {
	db := setupSecretDB(t)
	store := NewSecretStore(db)
	ctx := context.Background()

	rec := iface.Record{
		Scope:     "user",
		SubjectID: "u1",
		Channel:   "chat",
		Provider:  "slack",
		Key:       "token",
		Version:   time.Now().UTC().Format(time.RFC3339Nano),
		Cipher:    []byte("cipher"),
		Nonce:     []byte("nonce"),
		Metadata:  map[string]any{"k": "v"},
	}
	if err := store.Put(ctx, rec); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := store.GetLatest(ctx, rec.Scope, rec.SubjectID, rec.Channel, rec.Provider, rec.Key)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if string(got.Cipher) != "cipher" {
		t.Fatalf("unexpected cipher %s", got.Cipher)
	}

	list, err := store.List(ctx, rec.Scope, rec.SubjectID, rec.Channel, rec.Provider, rec.Key)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 record, got %d", len(list))
	}

	if err := store.Delete(ctx, rec.Scope, rec.SubjectID, rec.Channel, rec.Provider, rec.Key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := store.GetLatest(ctx, rec.Scope, rec.SubjectID, rec.Channel, rec.Provider, rec.Key); err == nil {
		t.Fatalf("expected not found after delete")
	}
}
