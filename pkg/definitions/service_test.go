package definitions

import (
	"context"
	"testing"

	"github.com/goliatone/go-notifications/internal/storage/memory"
)

func TestServiceUpsertCreatesAndUpdatesDefinition(t *testing.T) {
	ctx := context.Background()
	repository := memory.NewDefinitionRepository()
	service, err := New(repository)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := service.Upsert(ctx, UpsertInput{Code: " welcome ", Name: "Welcome"})
	if err != nil || created.Code != "welcome" {
		t.Fatalf("create definition: definition=%+v err=%v", created, err)
	}
	updated, err := service.Upsert(ctx, UpsertInput{
		Code: "welcome", Name: "Updated", AllowUpdate: true,
	})
	if err != nil || updated.ID != created.ID || updated.Name != "Updated" {
		t.Fatalf("update definition: definition=%+v err=%v", updated, err)
	}
}
