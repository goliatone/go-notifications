package options

import (
	"context"
	"testing"

	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/domain"
	opts "github.com/goliatone/go-options"
	"github.com/google/uuid"
)

func TestPreferenceSnapshotStoreLoad(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPreferenceRepository()
	store := PreferenceSnapshotStore{Repository: repo}

	record := &domain.NotificationPreference{
		SubjectType:    "user",
		SubjectID:      "u1",
		DefinitionCode: "billing.alert",
		Channel:        "email",
		Enabled:        false,
		Locale:         "es",
		QuietHours: domain.JSONMap{
			"start": "21:00",
			"end":   "06:00",
		},
		AdditionalRules: domain.JSONMap{
			"subscriptions": []string{"billing"},
		},
	}
	if err := repo.Create(ctx, record); err != nil {
		t.Fatalf("seed preference: %v", err)
	}

	scope := opts.NewScope("user", opts.ScopePriorityUser)
	snapshots, err := store.Load(ctx, []PreferenceScopeRef{{
		Scope:          scope,
		SubjectType:    "user",
		SubjectID:      "u1",
		DefinitionCode: "billing.alert",
		Channel:        "email",
	}})
	if err != nil {
		t.Fatalf("load snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snapshots))
	}
	if snapshots[0].Scope.Name != "user" {
		t.Fatalf("unexpected scope name %s", snapshots[0].Scope.Name)
	}
	if snapshots[0].Data["enabled"] != false {
		t.Fatalf("expected enabled override false")
	}
}

func TestPreferenceSnapshotStoreSaveCreatesAndUpdates(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPreferenceRepository()
	store := PreferenceSnapshotStore{Repository: repo}

	scope := PreferenceScopeRef{
		Scope:          opts.NewScope("user", opts.ScopePriorityUser),
		SubjectType:    "user",
		SubjectID:      "u2",
		DefinitionCode: "digest.summary",
		Channel:        "sms",
	}

	enabled := boolPtr(false)
	locale := stringPtr("en")
	created, err := store.Save(ctx, PreferenceSnapshotInput{
		Scope:      scope,
		Enabled:    enabled,
		Locale:     locale,
		QuietHours: domain.JSONMap{"start": "22:00", "end": "07:00"},
	})
	if err != nil {
		t.Fatalf("save create: %v", err)
	}
	if created == nil || created.ID == uuid.Nil {
		t.Fatalf("expected created record with ID")
	}
	if created.QuietHours["start"] != "22:00" || created.QuietHours["end"] != "07:00" {
		t.Fatalf("quiet hours not stored: %+v", created.QuietHours)
	}

	updateQuiet := domain.JSONMap{"start": "20:00", "end": "05:00"}
	updated, err := store.Save(ctx, PreferenceSnapshotInput{
		Scope:      scope,
		QuietHours: updateQuiet,
		Rules: domain.JSONMap{
			"subscriptions": []string{"ops"},
		},
	})
	if err != nil {
		t.Fatalf("save update: %v", err)
	}
	if updated.ID != created.ID {
		t.Fatalf("expected update to mutate existing record")
	}
	if updated.QuietHours["start"] != "20:00" {
		t.Fatalf("quiet hours not updated: %+v", updated.QuietHours)
	}
	if updated.AdditionalRules["subscriptions"] == nil {
		t.Fatalf("subscriptions not stored: %+v", updated.AdditionalRules)
	}
}

func boolPtr(v bool) *bool { return &v }

func stringPtr(v string) *string { return &v }
