package preferences

import (
	"context"
	"testing"

	"github.com/goliatone/go-notifications/internal/storage/memory"
	"github.com/goliatone/go-notifications/pkg/interfaces/logger"
	pkgoptions "github.com/goliatone/go-notifications/pkg/options"
	opts "github.com/goliatone/go-options"
)

func TestServiceResolveWithTrace(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewPreferenceRepository()
	service := newPublicService(t, repo)

	enabled := boolPtr(false)
	if _, err := service.Upsert(ctx, PreferenceInput{
		SubjectType:    "user",
		SubjectID:      "user-99",
		DefinitionCode: "digest",
		Channel:        "email",
		Enabled:        enabled,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	req := EvaluationRequest{
		DefinitionCode: "digest",
		Channel:        "email",
		Scopes: []pkgoptions.PreferenceScopeRef{
			{
				Scope:       opts.NewScope("user", opts.ScopePriorityUser),
				SubjectType: "user",
				SubjectID:   "user-99",
			},
		},
	}
	value, trace, err := service.ResolveWithTrace(ctx, req, "enabled")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	boolean, ok := value.(bool)
	if !ok || boolean {
		t.Fatalf("expected resolved bool false, got %v", value)
	}
	if trace.Path != "enabled" || len(trace.Layers) == 0 {
		t.Fatalf("trace missing: %+v", trace)
	}

	doc, err := service.Schema(ctx, req)
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	if doc.Format == "" {
		t.Fatalf("expected schema format, got empty")
	}
}

func newPublicService(t *testing.T, repo *memory.PreferenceRepository) *Service {
	t.Helper()
	svc, err := New(Dependencies{
		Repository: repo,
		Logger:     &logger.Nop{},
	})
	if err != nil {
		t.Fatalf("preferences.New: %v", err)
	}
	return svc
}

func boolPtr(v bool) *bool { return &v }
