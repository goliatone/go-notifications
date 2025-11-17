package preferences

import (
	"context"
	"testing"
	"time"

	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/domain"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	pkgoptions "github.com/goliatone/go-notifications/pkg/options"
	opts "github.com/goliatone/go-options"
)

func TestServiceUpsertCreatesAndUpdates(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPreferenceRepository()
	service := newTestService(t, repo)
	enabled := boolPtr(false)
	locale := strPtr("es")

	created, err := service.Upsert(ctx, PreferenceInput{
		SubjectType:    "user",
		SubjectID:      "u1",
		DefinitionCode: "billing.alert",
		Channel:        "email",
		Enabled:        enabled,
		Locale:         locale,
		QuietHours: &QuietHoursWindow{
			Start: "21:00",
			End:   "06:00",
		},
	})
	if err != nil {
		t.Fatalf("upsert create: %v", err)
	}
	if created.Enabled != false {
		t.Fatalf("expected enabled=false, got %v", created.Enabled)
	}
	if created.Locale != "es" {
		t.Fatalf("locale not stored: %s", created.Locale)
	}

	// Update quiet hours and locale.
	newLocale := strPtr("en")
	updated, err := service.Upsert(ctx, PreferenceInput{
		SubjectType:    "user",
		SubjectID:      "u1",
		DefinitionCode: "billing.alert",
		Channel:        "email",
		Locale:         newLocale,
		QuietHours: &QuietHoursWindow{
			Start: "20:00",
			End:   "05:00",
		},
	})
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if updated.Locale != "en" {
		t.Fatalf("expected locale update, got %s", updated.Locale)
	}
	if updated.QuietHours["start"] != "20:00" {
		t.Fatalf("quiet hours not updated: %+v", updated.QuietHours)
	}
}

func TestServiceEvaluateOptOut(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPreferenceRepository()
	service := newTestService(t, repo)

	system := &domain.NotificationPreference{
		SubjectType:    "system",
		SubjectID:      "global",
		DefinitionCode: "billing.alert",
		Channel:        "email",
		Enabled:        true,
	}
	user := &domain.NotificationPreference{
		SubjectType:    "user",
		SubjectID:      "user-42",
		DefinitionCode: "billing.alert",
		Channel:        "email",
		Enabled:        false,
	}
	if err := repo.Create(ctx, system); err != nil {
		t.Fatalf("seed system: %v", err)
	}
	if err := repo.Create(ctx, user); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	res, err := service.Evaluate(ctx, EvaluationRequest{
		DefinitionCode: "billing.alert",
		Channel:        "email",
		Scopes: []pkgoptions.PreferenceScopeRef{
			{
				Scope:       opts.NewScope("user", opts.ScopePriorityUser),
				SubjectType: "user",
				SubjectID:   "user-42",
			},
			{
				Scope:       opts.NewScope("system", opts.ScopePrioritySystem),
				SubjectType: "system",
				SubjectID:   "global",
			},
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Allowed {
		t.Fatalf("expected opt-out to block delivery")
	}
	if res.Reason != ReasonOptOut {
		t.Fatalf("expected reason %s, got %s", ReasonOptOut, res.Reason)
	}
	if res.Trace.Path != "enabled" || len(res.Trace.Layers) == 0 {
		t.Fatalf("trace not recorded: %+v", res.Trace)
	}
}

func TestServiceEvaluateQuietHours(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPreferenceRepository()
	service := newTestService(t, repo)

	record := &domain.NotificationPreference{
		SubjectType:    "user",
		SubjectID:      "quiet",
		DefinitionCode: "status.update",
		Channel:        "sms",
		Enabled:        true,
		QuietHours: domain.JSONMap{
			"start":    "09:00",
			"end":      "17:00",
			"timezone": "UTC",
		},
	}
	if err := repo.Create(ctx, record); err != nil {
		t.Fatalf("seed preference: %v", err)
	}

	now := time.Date(2024, 10, 10, 10, 30, 0, 0, time.UTC)
	res, err := service.Evaluate(ctx, EvaluationRequest{
		DefinitionCode: "status.update",
		Channel:        "sms",
		Timestamp:      now,
		Scopes: []pkgoptions.PreferenceScopeRef{
			{
				Scope:       opts.NewScope("user", opts.ScopePriorityUser),
				SubjectType: "user",
				SubjectID:   "quiet",
			},
		},
	})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.Allowed {
		t.Fatalf("expected quiet hours to block delivery")
	}
	if !res.QuietHoursActive {
		t.Fatalf("expected quiet hours flag")
	}
	if res.Reason != ReasonQuietHours {
		t.Fatalf("expected quiet hours reason, got %s", res.Reason)
	}
}

func TestServiceEvaluateSubscriptionFilter(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPreferenceRepository()
	service := newTestService(t, repo)

	record := &domain.NotificationPreference{
		SubjectType:    "user",
		SubjectID:      "s1",
		DefinitionCode: "digest",
		Channel:        "email",
		Enabled:        true,
		AdditionalRules: domain.JSONMap{
			"subscriptions": []string{"ops"},
		},
	}
	if err := repo.Create(ctx, record); err != nil {
		t.Fatalf("seed preference: %v", err)
	}

	res, err := service.Evaluate(ctx, EvaluationRequest{
		DefinitionCode: "digest",
		Channel:        "email",
		Scopes: []pkgoptions.PreferenceScopeRef{
			{
				Scope:       opts.NewScope("user", opts.ScopePriorityUser),
				SubjectType: "user",
				SubjectID:   "s1",
			},
		},
		Subscriptions: []string{"sales"},
	})
	if err != nil {
		t.Fatalf("evaluate missing subscription: %v", err)
	}
	if res.Allowed {
		t.Fatalf("expected subscription filter to block")
	}
	if res.Reason != ReasonSubscriptionFilter {
		t.Fatalf("expected subscription reason, got %s", res.Reason)
	}

	res, err = service.Evaluate(ctx, EvaluationRequest{
		DefinitionCode: "digest",
		Channel:        "email",
		Scopes: []pkgoptions.PreferenceScopeRef{
			{
				Scope:       opts.NewScope("user", opts.ScopePriorityUser),
				SubjectType: "user",
				SubjectID:   "s1",
			},
		},
		Subscriptions: []string{"ops"},
	})
	if err != nil {
		t.Fatalf("evaluate with subscription: %v", err)
	}
	if !res.Allowed {
		t.Fatalf("expected allowed when subscription matches, got reason %s", res.Reason)
	}
}

func newTestService(t *testing.T, repo *memory.PreferenceRepository) *Service {
	t.Helper()
	svc, err := NewService(Dependencies{
		Repository: repo,
		Logger:     &logger.Nop{},
		Clock: func() time.Time {
			return time.Date(2024, 10, 10, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func boolPtr(v bool) *bool { return &v }

func strPtr(v string) *string { return &v }
