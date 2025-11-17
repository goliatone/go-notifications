package memory

import (
	"context"
	"testing"

	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/store"
)

func TestDefinitionRepositoryMemory(t *testing.T) {
	repo := NewDefinitionRepository()
	ctx := context.Background()

	def := &domain.NotificationDefinition{
		Code:     "welcome",
		Name:     "Welcome",
		Channels: domain.StringList{"email"},
	}
	if err := repo.Create(ctx, def); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.GetByCode(ctx, "welcome")
	if err != nil {
		t.Fatalf("get by code: %v", err)
	}
	if got.Code != "welcome" {
		t.Fatalf("expected code welcome, got %s", got.Code)
	}

	result, err := repo.List(ctx, store.ListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("expected total 1, got %d", result.Total)
	}
}
