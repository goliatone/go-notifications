package options

import (
	"testing"

	opts "github.com/goliatone/go-options"
)

func TestNewResolverMergesSnapshots(t *testing.T) {
	t.Helper()
	system := opts.NewScope("system", opts.ScopePrioritySystem, opts.WithScopeLabel("System"))
	user := opts.NewScope("user", opts.ScopePriorityUser, opts.WithScopeLabel("User"))

	resolver, err := NewResolver(
		Snapshot{
			Scope: system,
			Data: map[string]any{
				"enabled":       true,
				"subscriptions": []any{"alpha"},
			},
		},
		Snapshot{
			Scope: user,
			Data: map[string]any{
				"enabled":       false,
				"subscriptions": []string{"beta"},
				"locale":        "es",
			},
		},
	)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}

	enabled, trace, err := resolver.ResolveBool("enabled")
	if err != nil {
		t.Fatalf("resolve bool: %v", err)
	}
	if enabled {
		t.Fatalf("expected user override to disable notifications")
	}
	if trace.Path != "enabled" || len(trace.Layers) != 2 {
		t.Fatalf("unexpected trace contents: %+v", trace)
	}

	locales, _, err := resolver.ResolveString("locale")
	if err != nil {
		t.Fatalf("resolve string: %v", err)
	}
	if locales != "es" {
		t.Fatalf("expected locale es, got %s", locales)
	}

	subs, _, err := resolver.ResolveStringSlice("subscriptions")
	if err != nil {
		t.Fatalf("resolve list: %v", err)
	}
	if len(subs) != 1 || subs[0] != "beta" {
		t.Fatalf("subscriptions merge incorrect: %+v", subs)
	}

	if _, err := resolver.Schema(); err != nil {
		t.Fatalf("schema: %v", err)
	}
}

func TestNewResolverValidation(t *testing.T) {
	_, err := NewResolver()
	if err != ErrNoSnapshots {
		t.Fatalf("expected ErrNoSnapshots, got %v", err)
	}

	_, err = NewResolver(Snapshot{
		Scope: opts.Scope{},
		Data:  map[string]any{},
	})
	if err == nil {
		t.Fatalf("expected error for missing scope name")
	}
}
